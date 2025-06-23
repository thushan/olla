package proxy

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sync/atomic"
	"time"

	"github.com/thushan/olla/internal/core/constants"
	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/core/ports"
	"github.com/thushan/olla/internal/logger"
	"github.com/thushan/olla/internal/util"
	"github.com/thushan/olla/internal/version"
)

var (
	errNoHealthyEndpoints = errors.New("no healthy endpoints")
)

const (
	// connection tuning - these values work well for AI workloads
	OllaDefaultMaxIdleConns        = 100
	OllaDefaultMaxConnsPerHost     = 50
	OllaDefaultIdleConnTimeout     = 90 * time.Second
	OllaDefaultTLSHandshakeTimeout = 10 * time.Second
	OllaDefaultTimeout             = 30 * time.Second
	OllaDefaultKeepAlive           = 30 * time.Second
	OllaDefaultReadTimeout         = 30 * time.Second
	OllaDefaultStreamBufferSize    = 64 * 1024
	OllaDefaultSetNoDelay          = true

	// handle LLM responses where client might reconnect shortly
	OllaClientDisconnectionBytesThreshold = 1024
	OllaClientDisconnectionTimeThreshold  = 5 * time.Second
)

type OllaProxyService struct {
	discoveryService ports.DiscoveryService
	selector         domain.EndpointSelector
	transport        *http.Transport
	configuration    *OllaConfiguration
	statsCollector   ports.StatsCollector
	logger           logger.StyledLogger

	// lock-free stats for hot path performance
	stats struct {
		totalRequests      int64
		successfulRequests int64
		failedRequests     int64
		totalLatency       int64
		minLatency         int64
		maxLatency         int64
	}
}

func NewOllaService(
	discoveryService ports.DiscoveryService,
	selector domain.EndpointSelector,
	configuration *OllaConfiguration,
	statsCollector ports.StatsCollector,
	logger logger.StyledLogger,
) *OllaProxyService {
	// Apply comprehensive defaults
	if configuration.StreamBufferSize == 0 {
		configuration.StreamBufferSize = OllaDefaultStreamBufferSize
	}
	if configuration.MaxIdleConns == 0 {
		configuration.MaxIdleConns = OllaDefaultMaxIdleConns
	}
	if configuration.MaxConnsPerHost == 0 {
		configuration.MaxConnsPerHost = OllaDefaultMaxConnsPerHost
	}
	if configuration.IdleConnTimeout == 0 {
		configuration.IdleConnTimeout = OllaDefaultIdleConnTimeout
	}
	if configuration.ReadTimeout == 0 {
		configuration.ReadTimeout = OllaDefaultReadTimeout
	}

	// Create optimised transport with TCP tuning
	transport := &http.Transport{
		MaxIdleConns:        configuration.MaxIdleConns,
		MaxIdleConnsPerHost: configuration.MaxConnsPerHost,
		IdleConnTimeout:     configuration.IdleConnTimeout,
		TLSHandshakeTimeout: OllaDefaultTLSHandshakeTimeout,
		DisableCompression:  true, // client should handle this, not proxy
		ForceAttemptHTTP2:   true, // http/2 is faster for streaming
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			dialer := &net.Dialer{
				Timeout:   OllaDefaultTimeout,
				KeepAlive: OllaDefaultKeepAlive,
			}
			conn, err := dialer.DialContext(ctx, network, addr)
			if err != nil {
				return nil, err
			}
			// tune tcp for low latency - nagle's algorithm hurts streaming
			if tcpConn, ok := conn.(*net.TCPConn); ok {
				_ = tcpConn.SetNoDelay(OllaDefaultSetNoDelay)
				_ = tcpConn.SetKeepAlive(true)
				_ = tcpConn.SetKeepAlivePeriod(OllaDefaultKeepAlive)
			}
			return conn, nil
		},
	}

	service := &OllaProxyService{
		discoveryService: discoveryService,
		selector:         selector,
		transport:        transport,
		configuration:    configuration,
		statsCollector:   statsCollector,
		logger:           logger,
	}

	// start with max int64 so first real latency becomes min
	atomic.StoreInt64(&service.stats.minLatency, int64(^uint64(0)>>1))

	return service
}

// updateStats updates statistics using atomic operations for lock-free performance
func (s *OllaProxyService) updateStats(success bool, latency time.Duration) {
	atomic.AddInt64(&s.stats.totalRequests, 1)

	if success {
		atomic.AddInt64(&s.stats.successfulRequests, 1)
		latencyMs := latency.Milliseconds()
		atomic.AddInt64(&s.stats.totalLatency, latencyMs)

		// atomic compare-and-swap loop to handle races on min latency
		for {
			current := atomic.LoadInt64(&s.stats.minLatency)
			if latencyMs >= current {
				break
			}
			if atomic.CompareAndSwapInt64(&s.stats.minLatency, current, latencyMs) {
				break
			}
		}

		// same for max latency
		for {
			current := atomic.LoadInt64(&s.stats.maxLatency)
			if latencyMs <= current {
				break
			}
			if atomic.CompareAndSwapInt64(&s.stats.maxLatency, current, latencyMs) {
				break
			}
		}
	} else {
		atomic.AddInt64(&s.stats.failedRequests, 1)
	}
}

// recordFailure records a failure in stats collector
func (s *OllaProxyService) recordFailure(endpoint *domain.Endpoint, duration time.Duration, bytes int64) {
	if endpoint != nil {
		s.statsCollector.RecordRequest(endpoint, "failure", duration, bytes)
	}
}

// ProxyRequest handles incoming HTTP requests and proxies them to healthy endpoints
func (s *OllaProxyService) ProxyRequest(ctx context.Context, w http.ResponseWriter, r *http.Request, stats *ports.RequestStats, rlog logger.StyledLogger) error {
	endpoints, err := s.discoveryService.GetHealthyEndpoints(ctx)
	if err != nil {
		return err
	}

	return s.ProxyRequestToEndpoints(ctx, w, r, endpoints, stats, rlog)
}

// ProxyRequestToEndpoints proxies the request to the provided endpoints
func (s *OllaProxyService) ProxyRequestToEndpoints(ctx context.Context, w http.ResponseWriter, r *http.Request, endpoints []*domain.Endpoint, stats *ports.RequestStats, rlog logger.StyledLogger) (err error) {
	// catch panics to prevent taking down the whole service
	defer func() {
		if rec := recover(); rec != nil {
			s.updateStats(false, time.Since(stats.StartTime))
			s.recordFailure(nil, time.Since(stats.StartTime), 0)
			err = fmt.Errorf("proxy panic recovered: %v", rec)
			rlog.Error("Proxy request panic recovered", "panic", rec)
		}
	}()

	// wrapper to keep error handling consistent
	handleError := func(endpoint *domain.Endpoint, err error, statusCode int, bytes int64) error {
		duration := time.Since(stats.StartTime)
		s.updateStats(false, duration)
		s.recordFailure(endpoint, duration, bytes)

		rlog.Error("Proxy request failed",
			"error", err,
			"duration", duration,
			"status", statusCode,
			"bytes", bytes,
			"endpoint", func() string {
				if endpoint != nil {
					return endpoint.Name
				}
				return "none"
			}())

		return domain.NewProxyError(
			stats.RequestID,
			stats.TargetUrl,
			r.Method,
			r.URL.Path,
			statusCode,
			duration,
			int(bytes),
			s.makeUserFriendlyError(err, duration),
		)
	}

	rlog.Debug("Proxy request started", "method", r.Method, "url", r.URL.String())

	if len(endpoints) == 0 {
		return handleError(nil, errNoHealthyEndpoints, 0, 0)
	}

	// fast path endpoint selection
	selectionStart := time.Now()
	endpoint, err := s.selector.Select(ctx, endpoints)
	if err != nil {
		return handleError(nil, fmt.Errorf("failed to select endpoint: %w", err), 0, 0)
	}
	stats.SelectionMs = time.Since(selectionStart).Milliseconds()

	// strip olla prefix and build target url
	targetPath := s.stripRoutePrefix(r.Context(), r.URL.Path)
	targetURL := endpoint.URL.ResolveReference(&url.URL{Path: targetPath})
	if r.URL.RawQuery != "" {
		targetURL.RawQuery = r.URL.RawQuery
	}

	stats.EndpointName = endpoint.Name
	stats.TargetUrl = targetURL.String()

	rlog.Info("Request dispatching to endpoint", "endpoint", endpoint.Name, "target", stats.TargetUrl)

	// timeout the upstream if it takes too long
	upstreamCtx := ctx
	var cancel context.CancelFunc
	if s.configuration.ResponseTimeout > 0 {
		upstreamCtx, cancel = context.WithTimeout(ctx, s.configuration.ResponseTimeout)
		defer cancel()
	}

	// Create proxy request
	requestStart := time.Now()
	proxyReq, err := http.NewRequestWithContext(upstreamCtx, r.Method, targetURL.String(), r.Body)
	if err != nil {
		return handleError(endpoint, fmt.Errorf("failed to create proxy request: %w", err), 0, 0)
	}

	// Copy headers with timing
	headerStart := time.Now()
	s.copyHeaders(proxyReq, r)
	stats.HeaderProcessingMs = time.Since(headerStart).Milliseconds()
	stats.RequestProcessingMs = time.Since(requestStart).Milliseconds()

	rlog.Debug("making round-trip request", "target", targetURL.String())

	// Make request with optimised transport
	backendStart := time.Now()
	resp, err := s.transport.RoundTrip(proxyReq)
	if err != nil {
		return handleError(endpoint, s.makeUserFriendlyError(err, time.Since(backendStart)), 0, 0)
	}
	defer resp.Body.Close()
	stats.BackendResponseMs = time.Since(backendStart).Milliseconds()

	rlog.Debug("round-trip success", "status", resp.StatusCode)

	// tell selector we're using this endpoint's connection
	s.selector.IncrementConnections(endpoint)
	defer s.selector.DecrementConnections(endpoint)

	// Copy response headers
	for k, vals := range resp.Header {
		for _, v := range vals {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)

	rlog.Debug("starting response stream")

	// stream with client disconnect handling for AI workloads
	streamStart := time.Now()
	bytesWritten, err := s.streamResponseAdvanced(ctx, upstreamCtx, w, resp.Body, rlog)
	if err != nil {
		return handleError(endpoint, fmt.Errorf("streaming failed: %w", err), resp.StatusCode, int64(bytesWritten))
	}
	stats.StreamingMs = time.Since(streamStart).Milliseconds()

	// Finalise stats
	duration := time.Since(stats.StartTime)
	stats.TotalBytes = bytesWritten
	stats.EndTime = time.Now()
	stats.Latency = duration.Milliseconds()
	stats.FirstDataMs = streamStart.Sub(stats.StartTime).Milliseconds()

	s.updateStats(true, duration)
	s.statsCollector.RecordRequest(endpoint, "success", duration, int64(bytesWritten))

	rlog.Debug("Proxy request completed",
		"latency_ms", stats.Latency,
		"processing_ms", stats.RequestProcessingMs,
		"backend_ms", stats.BackendResponseMs,
		"first_data_ms", stats.FirstDataMs,
		"streaming_ms", stats.StreamingMs,
		"selection_ms", stats.SelectionMs,
		"header_ms", stats.HeaderProcessingMs,
		"total_bytes", stats.TotalBytes)

	return nil
}

// streamResponseAdvanced provides optimised streaming with client disconnection handling
func (s *OllaProxyService) streamResponseAdvanced(clientCtx, upstreamCtx context.Context, w http.ResponseWriter, body io.Reader, rlog logger.StyledLogger) (int, error) {
	buffer := make([]byte, s.configuration.StreamBufferSize)
	flusher, canFlush := w.(http.Flusher)
	readTimeout := s.configuration.ReadTimeout

	rlog.Debug("Starting response stream", "read_timeout", readTimeout, "buffer_size", len(buffer))

	totalBytes := 0
	readCount := 0
	lastReadTime := time.Now()

	// watch both client and upstream for cancellation
	combinedCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		select {
		case <-clientCtx.Done():
			cancel()
		case <-upstreamCtx.Done():
			cancel()
		case <-combinedCtx.Done():
			return
		}
	}()

	// main read loop with timeout and cancellation handling
	for {
		select {
		case <-combinedCtx.Done():
			if clientCtx.Err() != nil {
				rlog.Info("client disconnected during streaming",
					"total_bytes", totalBytes,
					"read_count", readCount,
					"error", clientCtx.Err())

				// for LLM responses, try to continue briefly in case they reconnect
				if s.shouldContinueAfterClientDisconnect(totalBytes, time.Since(lastReadTime)) {
					rlog.Debug("continuing stream briefly after client disconnect")
					combinedCtx = context.Background()
					continue
				} else {
					return totalBytes, fmt.Errorf("client disconnected during streaming")
				}
			} else {
				rlog.Error("upstream timeout exceeded",
					"total_bytes", totalBytes,
					"read_count", readCount,
					"error", upstreamCtx.Err())
				return totalBytes, s.makeUserFriendlyError(upstreamCtx.Err(), time.Since(lastReadTime))
			}
		default:
		}

		// non-blocking read using goroutine and channel
		type readResult struct {
			err error
			n   int
		}

		readCh := make(chan readResult, 1)
		go func() {
			n, err := body.Read(buffer)
			readCh <- readResult{n: n, err: err}
		}()

		readTimer := time.NewTimer(readTimeout)
		select {
		case <-combinedCtx.Done():
			readTimer.Stop()
			continue

		case <-readTimer.C:
			rlog.Error("read timeout exceeded between chunks",
				"timeout", readTimeout,
				"total_bytes", totalBytes,
				"read_count", readCount)
			return totalBytes, fmt.Errorf("AI backend stopped responding - no data received for %.1fs", readTimeout.Seconds())

		case result := <-readCh:
			readTimer.Stop()

			if result.n > 0 {
				totalBytes += result.n
				readCount++
				lastReadTime = time.Now()

				rlog.Debug("stream read success",
					"read_num", readCount,
					"bytes", result.n,
					"total_bytes", totalBytes)

				if _, err := w.Write(buffer[:result.n]); err != nil {
					return totalBytes, fmt.Errorf("failed to write response to client: %w", err)
				}
				if canFlush {
					flusher.Flush()
				}
			}

			if result.err != nil {
				if errors.Is(result.err, io.EOF) {
					rlog.Debug("stream ended normally", "total_bytes", totalBytes, "read_count", readCount)
					return totalBytes, nil
				}
				return totalBytes, s.makeUserFriendlyError(result.err, time.Since(lastReadTime))
			}
		}
	}
}

func (s *OllaProxyService) stripRoutePrefix(ctx context.Context, path string) string {
	return util.StripRoutePrefix(ctx, path, s.configuration.ProxyPrefix)
}

func (s *OllaProxyService) copyHeaders(proxyReq, originalReq *http.Request) {
	// copy headers efficiently - single value vs multi-value optimisation
	for k, vals := range originalReq.Header {
		if len(vals) == 1 {
			proxyReq.Header.Set(k, vals[0])
		} else {
			proxyReq.Header[k] = make([]string, len(vals))
			copy(proxyReq.Header[k], vals)
		}
	}

	// add standard proxy identification headers
	proto := constants.ProtocolHTTP
	if originalReq.TLS != nil {
		proto = constants.ProtocolHTTPS
	}
	proxyReq.Header.Set("X-Forwarded-Host", originalReq.Host)
	proxyReq.Header.Set("X-Forwarded-Proto", proto)

	if ip, _, err := net.SplitHostPort(originalReq.RemoteAddr); err == nil {
		proxyReq.Header.Set("X-Forwarded-For", ip)
	}

	proxyReq.Header.Set("X-Proxied-By", fmt.Sprintf("%s/%s", version.Name, version.Version))
	proxyReq.Header.Set("Via", fmt.Sprintf("1.1 %s/%s", version.ShortName, version.Version))
}

func (s *OllaProxyService) shouldContinueAfterClientDisconnect(bytesRead int, timeSinceLastRead time.Duration) bool {
	return bytesRead > OllaClientDisconnectionBytesThreshold && timeSinceLastRead < OllaClientDisconnectionTimeThreshold
}

// makeUserFriendlyError converts technical errors to user-friendly messages
func (s *OllaProxyService) makeUserFriendlyError(err error, duration time.Duration) error {
	if err == nil {
		return nil
	}

	// check for common network timeouts
	var netErr net.Error
	if errors.As(err, &netErr) {
		if netErr.Timeout() {
			return fmt.Errorf("AI backend took too long to respond (timeout after %.1fs)", duration.Seconds())
		}
	}

	// connection refused means backend is down
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		if opErr.Op == "dial" {
			return fmt.Errorf("could not connect to AI backend (connection refused)")
		}
	}

	// context timeouts and cancellations
	if errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("AI backend response timeout exceeded after %.1fs", duration.Seconds())
	}
	if errors.Is(err, context.Canceled) {
		return fmt.Errorf("request was cancelled after %.1fs", duration.Seconds())
	}

	// add timing context to help debug issues
	return fmt.Errorf("%w (after %.1fs)", err, duration.Seconds())
}

func (s *OllaProxyService) GetStats(ctx context.Context) (ports.ProxyStats, error) {
	total := atomic.LoadInt64(&s.stats.totalRequests)
	successful := atomic.LoadInt64(&s.stats.successfulRequests)
	failed := atomic.LoadInt64(&s.stats.failedRequests)
	totalLatency := atomic.LoadInt64(&s.stats.totalLatency)
	minLatency := atomic.LoadInt64(&s.stats.minLatency)
	maxLatency := atomic.LoadInt64(&s.stats.maxLatency)

	var avgLatency int64
	if total > 0 {
		avgLatency = totalLatency / total
	}

	// handle the case where we haven't recorded any requests yet
	if minLatency == int64(^uint64(0)>>1) {
		minLatency = 0
	}

	return ports.ProxyStats{
		TotalRequests:      total,
		SuccessfulRequests: successful,
		FailedRequests:     failed,
		AverageLatency:     avgLatency,
		MinLatency:         minLatency,
		MaxLatency:         maxLatency,
	}, nil
}

func (s *OllaProxyService) UpdateConfig(config ports.ProxyConfiguration) {
	newConfig := &OllaConfiguration{
		ProxyPrefix:         config.GetProxyPrefix(),
		ConnectionTimeout:   config.GetConnectionTimeout(),
		ConnectionKeepAlive: config.GetConnectionKeepAlive(),
		ResponseTimeout:     config.GetResponseTimeout(),
		ReadTimeout:         config.GetReadTimeout(),
		StreamBufferSize:    config.GetStreamBufferSize(),
		MaxIdleConns:        s.configuration.MaxIdleConns,
		IdleConnTimeout:     s.configuration.IdleConnTimeout,
		MaxConnsPerHost:     s.configuration.MaxConnsPerHost,
	}

	// swap config atomically to avoid races
	s.configuration = newConfig

	// only update transport if connection settings actually changed
	s.transport.MaxIdleConns = newConfig.MaxIdleConns
	s.transport.MaxIdleConnsPerHost = newConfig.MaxConnsPerHost
	s.transport.IdleConnTimeout = newConfig.IdleConnTimeout
}
