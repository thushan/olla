package proxy

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sync"
	"sync/atomic"
	"time"

	"github.com/thushan/olla/internal/core/constants"
	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/core/ports"
	"github.com/thushan/olla/internal/logger"
	"github.com/thushan/olla/internal/util"
	"github.com/thushan/olla/internal/version"
)

type SherpaProxyService struct {
	discoveryService ports.DiscoveryService
	selector         domain.EndpointSelector
	transport        *http.Transport
	configuration    *Configuration
	stats            *proxyStats
	statsCollector   ports.StatsCollector
	bufferPool       sync.Pool
	logger           logger.StyledLogger
}

type proxyStats struct {
	totalRequests      int64
	successfulRequests int64
	failedRequests     int64
	totalLatency       int64
}

const (
	// TODO: add these to settings/config
	DefaultReadTimeout      = 60 * time.Second
	DefaultStreamBufferSize = 8 * 1024

	DefaultSetNoDelay         = true
	DefaultDisableCompression = false

	DefaultTimeout   = 60 * time.Second
	DefaultKeepAlive = 60 * time.Second

	DefaultMaxIdleConns        = 20
	DefaultMaxIdleConnsPerHost = 5

	DefaultIdleConnTimeout            = 90 * time.Second
	DefaultTLSHandshakeTimeout        = 10 * time.Second
	ClientDisconnectionBytesThreshold = 1024
	ClientDisconnectionTimeThreshold  = 5 * time.Second
)

func NewSherpaService(
	discoveryService ports.DiscoveryService,
	selector domain.EndpointSelector,
	configuration *Configuration,
	statsCollector ports.StatsCollector,
	logger logger.StyledLogger,
) *SherpaProxyService {
	transport := &http.Transport{
		MaxIdleConns:        DefaultMaxIdleConns,
		IdleConnTimeout:     DefaultIdleConnTimeout,
		DisableCompression:  DefaultDisableCompression,
		TLSHandshakeTimeout: DefaultTLSHandshakeTimeout,
		MaxIdleConnsPerHost: DefaultMaxIdleConnsPerHost,
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			dialer := &net.Dialer{
				Timeout:   DefaultTimeout,
				KeepAlive: DefaultKeepAlive,
			}
			conn, err := dialer.DialContext(ctx, network, addr)
			if err != nil {
				return nil, err
			}
			if tcpConn, ok := conn.(*net.TCPConn); ok {
				if terr := tcpConn.SetNoDelay(DefaultSetNoDelay); terr != nil {
					logger.Warn("Failed to set NoDelay", "err", terr)
				}
			}
			return conn, nil
		},
	}

	return &SherpaProxyService{
		discoveryService: discoveryService,
		selector:         selector,
		transport:        transport,
		configuration:    configuration,
		stats:            &proxyStats{},
		statsCollector:   statsCollector,
		bufferPool: sync.Pool{
			New: func() interface{} {
				return make([]byte, DefaultStreamBufferSize)
			},
		},
		logger: logger,
	}
}

func (s *SherpaProxyService) ProxyRequest(ctx context.Context, w http.ResponseWriter, r *http.Request) (ports.RequestStats, error) {
	endpoints, err := s.discoveryService.GetHealthyEndpoints(ctx)
	if err != nil {
		return ports.RequestStats{}, err
	}

	return s.ProxyRequestToEndpoints(ctx, w, r, endpoints)
}

func (s *SherpaProxyService) ProxyRequestToEndpoints(ctx context.Context, w http.ResponseWriter, r *http.Request, endpoints []*domain.Endpoint) (stats ports.RequestStats, err error) {
	requestID, _ := ctx.Value(constants.RequestIDKey).(string)
	if requestID == "" {
		requestID = util.GenerateRequestID()
		s.logger.Warn("Request context missing request_id, using current time, please report bug.", "new_request_id", requestID)
	}

	rlog := s.logger.With("request_id", requestID)

	startTime, _ := ctx.Value(constants.RequestTimeKey).(time.Time)
	if startTime.IsZero() {
		startTime = time.Now()
		rlog.Warn("Request context missing start_time, using current time, please report bug.")
	}

	stats = ports.RequestStats{
		RequestID: requestID,
		StartTime: startTime,
	}

	// Panic recovery for critical path
	defer func() {
		if rec := recover(); rec != nil {
			atomic.AddInt64(&s.stats.failedRequests, 1)
			s.recordFailure(nil, time.Since(startTime), 0)
			err = fmt.Errorf("proxy panic recovered after %.1fs: %v (this is a bug, please report)", time.Since(startTime).Seconds(), rec)
			rlog.Error("Proxy request panic recovered",
				"panic", rec,
				"method", r.Method,
				"path", r.URL.Path)

			// Try to write error response if headers haven't been sent
			if w.Header().Get("Content-Type") == "" {
				http.Error(w, "Internal server error", http.StatusInternalServerError)
			}
		}
	}()

	atomic.AddInt64(&s.stats.totalRequests, 1)

	rlog.Debug("proxy request started", "method", r.Method, "url", r.URL.String())

	if len(endpoints) == 0 {
		// No healthy endpoints available, log and return error
		atomic.AddInt64(&s.stats.failedRequests, 1)
		s.recordFailure(nil, time.Since(startTime), 0)

		rlog.Error("no healthy endpoints available")

		return stats, domain.NewProxyError(requestID, "", r.Method, r.URL.Path, 0, time.Since(startTime), stats.TotalBytes,
			fmt.Errorf("no healthy AI backends available after %.1fs - all endpoints may be down or still being health checked", time.Since(startTime).Seconds()))
	}

	rlog.Debug("using provided endpoints", "count", len(endpoints))

	selectionStart := time.Now()
	endpoint, err := s.selector.Select(ctx, endpoints)
	selectionEnd := time.Now()
	stats.SelectionMs = selectionEnd.Sub(selectionStart).Milliseconds()

	if err != nil {
		atomic.AddInt64(&s.stats.failedRequests, 1)
		s.recordFailure(endpoint, time.Since(startTime), 0)
		rlog.Error("failed to select endpoint", "error", err)
		return stats, domain.NewProxyError(requestID, "", r.Method, r.URL.Path, 0, time.Since(startTime), stats.TotalBytes,
			makeUserFriendlyError(fmt.Errorf("failed to select endpoint: %w", err), time.Since(startTime), "selection", s.configuration.ResponseTimeout))
	}

	stats.EndpointName = endpoint.Name

	// Strip route prefix from request path for upstream
	targetPath := s.stripRoutePrefix(r.Context(), r.URL.Path)
	targetURL := endpoint.URL.ResolveReference(&url.URL{Path: targetPath}) // no string allocation ma!
	if r.URL.RawQuery != "" {
		targetURL.RawQuery = r.URL.RawQuery
	}

	stats.TargetUrl = targetURL.String()

	rlog.Debug("built target URL", "target", stats.TargetUrl)
	rlog.Info("Request dispatching to endpoint", "endpoint", endpoint.Name, "target", stats.TargetUrl)

	// Create upstream context with response timeout
	upstreamCtx := ctx
	var cancel context.CancelFunc
	if s.configuration.ResponseTimeout > 0 {
		upstreamCtx, cancel = context.WithTimeout(ctx, s.configuration.ResponseTimeout)
		defer cancel()
	}

	proxyReq, err := http.NewRequestWithContext(upstreamCtx, r.Method, targetURL.String(), r.Body)
	if err != nil {
		atomic.AddInt64(&s.stats.failedRequests, 1)
		s.recordFailure(endpoint, time.Since(startTime), 0)
		rlog.Error("failed to create proxy request", "error", err)
		return stats, domain.NewProxyError(requestID, targetURL.String(), r.Method, r.URL.Path, 0, time.Since(startTime), stats.TotalBytes,
			fmt.Errorf("failed to create proxy request after %.1fs: %w", time.Since(startTime).Seconds(), err))
	}

	rlog.Debug("created proxy request")

	headerStart := time.Now()
	s.copyHeaders(proxyReq, r)
	headerEnd := time.Now()
	stats.HeaderProcessingMs = headerEnd.Sub(headerStart).Milliseconds()

	requestProcessingEnd := time.Now()
	stats.RequestProcessingMs = requestProcessingEnd.Sub(startTime).Milliseconds()

	rlog.Debug("making round-trip request", "target", targetURL.String())

	backendStart := time.Now()
	resp, err := s.transport.RoundTrip(proxyReq)
	backendEnd := time.Now()
	stats.BackendResponseMs = backendEnd.Sub(backendStart).Milliseconds()

	if err != nil {
		atomic.AddInt64(&s.stats.failedRequests, 1)
		s.recordFailure(endpoint, time.Since(startTime), 0)
		rlog.Error("round-trip failed", "error", err)
		return stats, domain.NewProxyError(requestID, targetURL.String(), r.Method, r.URL.Path, 0, time.Since(startTime), stats.TotalBytes,
			makeUserFriendlyError(err, time.Since(startTime), "backend", s.configuration.ResponseTimeout))
	}
	defer resp.Body.Close()

	rlog.Debug("round-trip success", "status", resp.StatusCode)

	// Track connection after successful establishment
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

	streamStart := time.Now()
	sumBytes, err := s.streamResponse(ctx, upstreamCtx, w, resp.Body, rlog)
	streamEnd := time.Now()
	stats.StreamingMs = streamEnd.Sub(streamStart).Milliseconds()

	if err != nil {
		atomic.AddInt64(&s.stats.failedRequests, 1)
		s.recordFailure(endpoint, time.Since(startTime), int64(sumBytes))
		rlog.Error("streaming failed", "error", err)
		stats.TotalBytes = sumBytes
		return stats, domain.NewProxyError(requestID, targetURL.String(), r.Method, r.URL.Path, resp.StatusCode, time.Since(startTime), sumBytes,
			makeUserFriendlyError(err, time.Since(startTime), "streaming", s.configuration.ResponseTimeout))
	}

	stats.TotalBytes = sumBytes
	stats.EndTime = time.Now()
	stats.Latency = stats.EndTime.Sub(startTime).Milliseconds()
	stats.FirstDataMs = streamStart.Sub(startTime).Milliseconds()

	atomic.AddInt64(&s.stats.successfulRequests, 1)
	atomic.AddInt64(&s.stats.totalLatency, stats.Latency)

	// Record successful request in stats collector
	s.statsCollector.RecordRequest(
		endpoint,
		"success",
		time.Duration(stats.Latency)*time.Millisecond,
		int64(stats.TotalBytes),
	)

	rlog.Debug("proxy request completed",
		"latency_ms", stats.Latency,
		"processing_ms", stats.RequestProcessingMs,
		"backend_ms", stats.BackendResponseMs,
		"first_data_ms", stats.FirstDataMs,
		"streaming_ms", stats.StreamingMs,
		"selection_ms", stats.SelectionMs,
		"header_ms", stats.HeaderProcessingMs,
		"total_bytes", stats.TotalBytes)

	return stats, nil
}

func (s *SherpaProxyService) recordFailure(endpoint *domain.Endpoint, duration time.Duration, bytes int64) {
	if endpoint == nil {
		// if we have no endpoint, we can't record it
		return
	}
	s.statsCollector.RecordRequest(endpoint, "failure", duration, bytes)
}

func (s *SherpaProxyService) stripRoutePrefix(ctx context.Context, path string) string {
	return util.StripRoutePrefix(ctx, path, s.configuration.ProxyPrefix)
}

func (s *SherpaProxyService) copyHeaders(proxyReq, originalReq *http.Request) {
	// TODO: copy only safe headers (SECURITY_JUNE2025)
	if len(originalReq.Header) > 0 {
		for k, vals := range originalReq.Header {
			if len(vals) == 1 {
				// single value no biggie smalls
				proxyReq.Header.Set(k, vals[0])
			} else {
				// multi-value, allocate new slice
				proxyReq.Header[k] = make([]string, len(vals))
				copy(proxyReq.Header[k], vals)
			}
		}
	}

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

//nolint:gocognit // TODO: Refactor this function to reduce complexity
func (s *SherpaProxyService) streamResponse(clientCtx, upstreamCtx context.Context, w http.ResponseWriter, body io.Reader, rlog logger.StyledLogger) (int, error) {
	bufferSize := s.configuration.StreamBufferSize
	if bufferSize == 0 {
		bufferSize = DefaultStreamBufferSize
	}
	buf := s.getBuffer(bufferSize)
	defer s.bufferPool.Put(buf)

	// Resize if needed at this point to avoid resizing during streaming
	if len(buf) != bufferSize {
		buf = make([]byte, bufferSize)
	}

	flusher, canFlush := w.(http.Flusher)

	readTimeout := s.configuration.ReadTimeout
	if readTimeout == 0 {
		readTimeout = DefaultReadTimeout
	}

	rlog.Debug("starting response stream", "read_timeout", readTimeout, "buffer_size", bufferSize)

	totalBytes := 0
	readCount := 0
	lastReadTime := time.Now()

	// Create a combined context for proper cancellation coordination
	combinedCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Monitor both client and upstream contexts
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

	for {
		// Check if either context is cancelled
		select {
		case <-combinedCtx.Done():
			if clientCtx.Err() != nil {
				rlog.Info("client disconnected during streaming",
					"total_bytes", totalBytes,
					"read_count", readCount,
					"error", clientCtx.Err())
				// Continue briefly for LLM responses in case client reconnects
				if s.shouldContinueAfterClientDisconnect(totalBytes, time.Since(lastReadTime)) {
					rlog.Debug("continuing stream briefly after client disconnect")
					combinedCtx = context.Background()
				} else {
					duration := time.Since(lastReadTime)
					if duration < 2*time.Second {
						return totalBytes, fmt.Errorf("client disconnected immediately during streaming - possible network issue")
					}
					return totalBytes, fmt.Errorf("client disconnected after %.1fs during streaming", duration.Seconds())
				}
			} else {
				rlog.Error("upstream timeout exceeded",
					"total_bytes", totalBytes,
					"read_count", readCount,
					"error", upstreamCtx.Err())
				return totalBytes, makeUserFriendlyError(upstreamCtx.Err(), time.Since(lastReadTime), "streaming", s.configuration.ResponseTimeout)
			}
		default:
		}

		type readResult struct {
			err error
			n   int
		}

		readCh := make(chan readResult, 1)
		readStart := time.Now()

		go func() {
			n, err := body.Read(buf)
			readCh <- readResult{n: n, err: err}
		}()

		readTimer := time.NewTimer(readTimeout)

		select {
		case <-combinedCtx.Done():
			readTimer.Stop()
			if clientCtx.Err() != nil {
				rlog.Debug("client context cancelled during read wait",
					"total_bytes", totalBytes,
					"read_count", readCount)
				// Try to complete current read and send it
				select {
				case result := <-readCh:
					if result.n > 0 {
						if _, err := w.Write(buf[:result.n]); err != nil {
							return totalBytes, fmt.Errorf("failed to write response after client disconnect: %w", err)
						}
						if canFlush {
							flusher.Flush()
						}
					}
				case <-time.After(1 * time.Second):
					// Read taking too long after client disconnect
				}
				return totalBytes, makeUserFriendlyError(clientCtx.Err(), time.Since(readStart), "streaming", s.configuration.ResponseTimeout)
			} else {
				rlog.Error("upstream context cancelled during read wait",
					"total_bytes", totalBytes,
					"read_count", readCount)
				return totalBytes, makeUserFriendlyError(upstreamCtx.Err(), time.Since(readStart), "streaming", s.configuration.ResponseTimeout)
			}

		case <-readTimer.C:
			rlog.Error("read timeout exceeded between chunks",
				"timeout", readTimeout,
				"total_bytes", totalBytes,
				"read_count", readCount,
				"time_since_last_read", time.Since(lastReadTime))
			return totalBytes, fmt.Errorf("AI backend stopped responding - no data received for %.1fs (backend may be overloaded)", readTimeout.Seconds())

		case result := <-readCh:
			readTimer.Stop()
			readDuration := time.Since(readStart)

			if result.n > 0 {
				totalBytes += result.n
				readCount++
				lastReadTime = time.Now()

				rlog.Debug("stream read success",
					"read_num", readCount,
					"bytes", result.n,
					"duration_ms", readDuration.Milliseconds(),
					"total_bytes", totalBytes)

				if _, err := w.Write(buf[:result.n]); err != nil {
					rlog.Error("failed to write response", "error", err)
					return totalBytes, fmt.Errorf("failed to write response to client: %w", err)
				}
				if canFlush {
					flusher.Flush()
				}
			} else if result.n == 0 && result.err == nil {
				rlog.Debug("empty read",
					"read_num", readCount+1,
					"duration_ms", readDuration.Milliseconds())
			}

			if result.err != nil {
				if errors.Is(result.err, io.EOF) {
					rlog.Debug("stream ended normally",
						"total_bytes", totalBytes,
						"read_count", readCount)
					return totalBytes, nil
				}
				rlog.Error("stream read error",
					"error", result.err,
					"total_bytes", totalBytes,
					"read_count", readCount)
				return totalBytes, makeUserFriendlyError(result.err, time.Since(lastReadTime), "streaming", s.configuration.ResponseTimeout)
			}
		}
	}
}

func (s *SherpaProxyService) getBuffer(bufferSize int) []byte {
	value := s.bufferPool.Get()
	if buf, ok := value.([]byte); ok {
		// Resize if smaller than neededs
		if len(buf) < bufferSize {
			return make([]byte, bufferSize)
		}
		return buf[:bufferSize]
	}

	if value != nil {
		s.logger.Warn("bufferPool returned unexpected type", "type", fmt.Sprintf("%T", value))
	}
	return make([]byte, bufferSize)
}

func (s *SherpaProxyService) shouldContinueAfterClientDisconnect(bytesRead int, timeSinceLastRead time.Duration) bool {
	// Continue if we've read significant data and stream is still active
	// Allows for brief network interruptions during long LLM responses
	return bytesRead > ClientDisconnectionBytesThreshold && timeSinceLastRead < ClientDisconnectionTimeThreshold
}

func (s *SherpaProxyService) GetStats(context.Context) (ports.ProxyStats, error) {
	return s.statsCollector.GetProxyStats(), nil
}

func (s *SherpaProxyService) UpdateConfig(config ports.ProxyConfiguration) {
	newConfig := &Configuration{
		ProxyPrefix:         config.GetProxyPrefix(),
		ConnectionTimeout:   config.GetConnectionTimeout(),
		ConnectionKeepAlive: config.GetConnectionKeepAlive(),
		ResponseTimeout:     config.GetResponseTimeout(),
		ReadTimeout:         config.GetReadTimeout(),
		StreamBufferSize:    config.GetStreamBufferSize(),
	}
	s.configuration = newConfig
}
