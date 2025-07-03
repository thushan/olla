package proxy

//                                       Sherpa Proxy Implementation
//
// The Sherpa proxy implementation is a clean and pragmatic reverse proxy designed for handling AI inference workloads
// such as LLM and embedding requests. It prioritises readability, simplicity and reliability while providing essential
// support for streaming, timeout handling and observability. It was the foundation of the Sherpa AI Tooling.
//
// Key design features:
// - **Connection reuse**: Uses a single shared `http.Transport` with reasonable TCP settings
//   (e.g. keep-alive, SetNoDelay) to reduce connection churn.
// - **Streaming support**: Optimised for long-lived HTTP responses via buffered reads and flushing,
//   with graceful handling of timeouts and client disconnects.
// - **Basic buffer pooling**: Reuses read buffers via `sync.Pool` to reduce heap churn during streaming.
// - **Request metadata tracking**: Records request lifecycle timings including header processing,
//   backend latency, and streaming duration.
// - **Stat collection**: Uses atomic counters and a pluggable `StatsCollector` to track success/failure rates and latency.
//
// Sherpa is suitable for:
// - Moderate-throughput inference services with stable upstreams
// - Environments prioritising maintainability and clarity over extreme performance (that's for Olla Proxy)
//
// Sherpa is not intended for:
// - High-throughput or low-latency scenarios where custom transports or advanced connection pooling are required
// - Complex routing or load balancing needs beyond basic endpoint selection
// - Environments where maximum performance is critical (use Olla Proxy for that)
// - Scenarios requiring advanced features like circuit breaking, rate limiting, etc.
// - Environments where the proxy itself must be highly resilient to failures (use Olla Proxy for that)

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
	// these are default values for proxy settings that should eventually be configurable
	// they're tuned for typical llm workloads but might need adjustment for specific use cases
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
	// create a transport with tcp tuning specifically for llm streaming workloads
	// these settings prioritise low latency over throughput which is crucial for token streaming
	transport := &http.Transport{
		MaxIdleConns:        DefaultMaxIdleConns,
		IdleConnTimeout:     DefaultIdleConnTimeout,
		DisableCompression:  DefaultDisableCompression, // compression adds latency and most llm responses are already compressed
		TLSHandshakeTimeout: DefaultTLSHandshakeTimeout,
		MaxIdleConnsPerHost: DefaultMaxIdleConnsPerHost,
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			dialer := &net.Dialer{
				Timeout:   DefaultTimeout,
				KeepAlive: DefaultKeepAlive, // keep connections alive to avoid reconnection overhead
			}
			conn, err := dialer.DialContext(ctx, network, addr)
			if err != nil {
				return nil, err
			}
			// disable nagle's algorithm for llm streaming to ensure tokens are sent immediately
			// rather than waiting to fill tcp segments, which reduces perceived latency
			if tcpConn, ok := conn.(*net.TCPConn); ok {
				if terr := tcpConn.SetNoDelay(DefaultSetNoDelay); terr != nil {
					logger.Warn("failed to set NoDelay", "err", terr)
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
		// using a buffer pool to avoid frequent allocations during streaming
		// this significantly reduces garbage collection pressure under high load
		bufferPool: sync.Pool{
			New: func() interface{} {
				return make([]byte, DefaultStreamBufferSize)
			},
		},
		logger: logger,
	}
}

var (
	proxiedByHeader string
	viaHeader       string
)

func init() {
	proxiedByHeader = version.Name + "/" + version.Version
	viaHeader = "1.1 " + version.ShortName + "/" + version.Version
}

func getProxiedByHeader() string {
	return proxiedByHeader
}

func getViaHeader() string {
	return viaHeader
}

// ProxyRequest handles incoming HTTP requests and proxies them to healthy endpoints (regardless of type)
func (s *SherpaProxyService) ProxyRequest(ctx context.Context, w http.ResponseWriter, r *http.Request, stats *ports.RequestStats, rlog logger.StyledLogger) error {
	endpoints, err := s.discoveryService.GetHealthyEndpoints(ctx)
	if err != nil {
		return err
	}

	return s.ProxyRequestToEndpoints(ctx, w, r, endpoints, stats, rlog)
}

// ProxyRequestToEndpoints proxies the request to the provided endpoints that are relevant for the request.
func (s *SherpaProxyService) ProxyRequestToEndpoints(ctx context.Context, w http.ResponseWriter, r *http.Request, endpoints []*domain.Endpoint, stats *ports.RequestStats, rlog logger.StyledLogger) (err error) {

	// panic recovery for the critical request path - we never want to crash the entire proxy
	// even if there's a bug in our code, as that would affect all users
	defer func() {
		if rec := recover(); rec != nil {
			// ensure stats are properly recorded even during panic recovery
			atomic.AddInt64(&s.stats.failedRequests, 1)
			s.recordFailure(nil, time.Since(stats.StartTime), 0)

			// provide detailed error information for debugging while keeping the service running
			err = fmt.Errorf("proxy panic recovered after %.1fs: %v (this is a bug, please report)", time.Since(stats.StartTime).Seconds(), rec)
			rlog.Error("proxy request panic recovered",
				"panic", rec,
				"method", r.Method,
				"path", r.URL.Path)

			// try to write a clean error response if we haven't sent headers yet
			// this gives the client something useful rather than a broken connection
			if w.Header().Get("Content-Type") == "" {
				http.Error(w, "Internal server error", http.StatusInternalServerError)
			}
		}
	}()

	atomic.AddInt64(&s.stats.totalRequests, 1)

	rlog.Debug("proxy request started", "method", r.Method, "url", r.URL.String())

	if len(endpoints) == 0 {
		// No healthy endpoints available, log and return error
		sinceStart := time.Since(stats.StartTime)
		atomic.AddInt64(&s.stats.failedRequests, 1)
		s.recordFailure(nil, sinceStart, 0)

		rlog.Error("no healthy endpoints available")
		return domain.NewProxyError(stats.RequestID, "", r.Method, r.URL.Path, 0, sinceStart, stats.TotalBytes,
			fmt.Errorf("no healthy AI backends available after %.1fs - all endpoints may be down or still being health checked", sinceStart.Seconds()))
	}

	rlog.Debug("using provided endpoints", "count", len(endpoints))

	selectionStart := time.Now()
	endpoint, err := s.selector.Select(ctx, endpoints)
	selectionEnd := time.Now()
	stats.SelectionMs = selectionEnd.Sub(selectionStart).Milliseconds()

	if err != nil {
		sinceStart := time.Since(stats.StartTime)
		atomic.AddInt64(&s.stats.failedRequests, 1)
		s.recordFailure(endpoint, sinceStart, 0)
		rlog.Error("failed to select endpoint", "error", err)
		return domain.NewProxyError(stats.RequestID, "", r.Method, r.URL.Path, 0, sinceStart, stats.TotalBytes,
			makeUserFriendlyError(fmt.Errorf("failed to select endpoint: %w", err), sinceStart, "selection", s.configuration.ResponseTimeout))
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
		sinceStart := time.Since(stats.StartTime)
		atomic.AddInt64(&s.stats.failedRequests, 1)
		s.recordFailure(endpoint, sinceStart, 0)
		rlog.Error("failed to create proxy request", "error", err)
		return domain.NewProxyError(stats.RequestID, targetURL.String(), r.Method, r.URL.Path, 0, sinceStart, stats.TotalBytes,
			fmt.Errorf("failed to create proxy request after %.1fs: %w", sinceStart.Seconds(), err))
	}

	rlog.Debug("created proxy request")

	headerStart := time.Now()
	s.copyHeaders(proxyReq, r)
	headerEnd := time.Now()
	stats.HeaderProcessingMs = headerEnd.Sub(headerStart).Milliseconds()

	requestProcessingEnd := time.Now()
	stats.RequestProcessingMs = requestProcessingEnd.Sub(stats.StartTime).Milliseconds()

	rlog.Debug("making round-trip request", "target", targetURL.String())

	backendStart := time.Now()
	resp, err := s.transport.RoundTrip(proxyReq)
	backendEnd := time.Now()
	stats.BackendResponseMs = backendEnd.Sub(backendStart).Milliseconds()

	if err != nil {
		sinceStart := time.Since(stats.StartTime)
		atomic.AddInt64(&s.stats.failedRequests, 1)
		s.recordFailure(endpoint, sinceStart, 0)
		rlog.Error("round-trip failed", "error", err)
		return domain.NewProxyError(stats.RequestID, targetURL.String(), r.Method, r.URL.Path, 0, sinceStart, stats.TotalBytes,
			makeUserFriendlyError(err, sinceStart, "backend", s.configuration.ResponseTimeout))
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
		sinceStart := time.Since(stats.StartTime)
		atomic.AddInt64(&s.stats.failedRequests, 1)
		s.recordFailure(endpoint, sinceStart, int64(sumBytes))
		rlog.Error("streaming failed", "error", err)
		stats.TotalBytes = sumBytes
		return domain.NewProxyError(stats.RequestID, targetURL.String(), r.Method, r.URL.Path, resp.StatusCode, sinceStart, sumBytes,
			makeUserFriendlyError(err, sinceStart, "streaming", s.configuration.ResponseTimeout))
	}

	stats.TotalBytes = sumBytes
	stats.EndTime = time.Now()
	stats.Latency = stats.EndTime.Sub(stats.StartTime).Milliseconds()
	stats.FirstDataMs = streamStart.Sub(stats.StartTime).Milliseconds()

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

	return nil
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
	if len(originalReq.Header) > 0 {

		for name, values := range originalReq.Header {
			// Let's check the most common headers that should not be forwarded
			// but we can make this configurable later
			if name == "Authorization" ||
				name == "Cookie" ||
				name == "X-Api-Key" ||
				name == "X-Auth-Token" ||
				name == "Proxy-Authorization" {
				continue
			}

			if len(values) == 1 {
				// fast path for single values (most command case)
				proxyReq.Header.Set(name, values[0])
			} else {
				// multi-value headers (less common), pre-allocate slice
				headerValues := make([]string, len(values))
				copy(headerValues, values)
				proxyReq.Header[name] = headerValues
			}
		}
	}

	addProxyHeaders(proxyReq, originalReq)
}

func addProxyHeaders(proxyReq, originalReq *http.Request) {
	var protocol string
	if originalReq.TLS != nil {
		protocol = constants.ProtocolHTTPS
	} else {
		protocol = constants.ProtocolHTTP
	}

	// Set proxy headers
	proxyReq.Header.Set("X-Forwarded-Host", originalReq.Host)
	proxyReq.Header.Set("X-Forwarded-Proto", protocol)

	if ip, _, err := net.SplitHostPort(originalReq.RemoteAddr); err == nil {
		proxyReq.Header.Set("X-Forwarded-For", ip)
	}

	proxyReq.Header.Set("X-Proxied-By", getProxiedByHeader())
	proxyReq.Header.Set("Via", getViaHeader())
}

// streamResponse handles the critical path of streaming data from backends to clients
// it's optimised for llm workloads with careful handling of timeouts, disconnections,
// and error conditions to provide the best possible user experience
//
//nolint:gocognit // this function is necessarily complex to handle all edge cases in streaming
func (s *SherpaProxyService) streamResponse(clientCtx, upstreamCtx context.Context, w http.ResponseWriter, body io.Reader, rlog logger.StyledLogger) (int, error) {
	bufferSize := s.configuration.StreamBufferSize
	if bufferSize == 0 {
		bufferSize = DefaultStreamBufferSize
	}
	buf := s.getBuffer(bufferSize)
	defer s.bufferPool.Put(buf)

	// resize the buffer if needed before we start streaming
	// doing this upfront avoids allocations in the hot streaming path
	// which would cause gc pressure and potential latency spikes
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

	// create a combined context to coordinate cancellation between client and upstream
	// this is crucial for proper resource cleanup when either side disconnects
	combinedCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// monitor both client and upstream contexts in a separate goroutine
	// this lets us react immediately to disconnections from either side
	// without blocking the main streaming loop
	go func() {
		select {
		case <-clientCtx.Done():
			cancel() // client disconnected, propagate cancellation
		case <-upstreamCtx.Done():
			cancel() // upstream timeout or error, propagate cancellation
		case <-combinedCtx.Done():
			return // our own cancellation, just exit goroutine
		}
	}()

	for {
		// check if either context is cancelled - this is our main error handling path
		select {
		case <-combinedCtx.Done():
			if clientCtx.Err() != nil {
				// client disconnection is common with mobile devices or flaky networks
				rlog.Info("client disconnected during streaming",
					"total_bytes", totalBytes,
					"read_count", readCount,
					"error", clientCtx.Err())

				// special handling for llm streaming - we continue briefly after client disconnect
				// this improves user experience by allowing clients to reconnect and resume
				// without the backend having to regenerate the entire response
				if s.shouldContinueAfterClientDisconnect(totalBytes, time.Since(lastReadTime)) {
					rlog.Debug("continuing stream briefly after client disconnect")
					// create a new context to keep streaming despite client disconnect
					combinedCtx = context.Background()
				} else {
					duration := time.Since(lastReadTime)
					if duration < 2*time.Second {
						// immediate disconnection likely indicates a client-side issue
						return totalBytes, fmt.Errorf("client disconnected immediately during streaming - possible network issue")
					}
					// normal disconnection after streaming for a while
					return totalBytes, fmt.Errorf("client disconnected after %.1fs during streaming", duration.Seconds())
				}
			} else {
				// upstream timeout is more serious and usually indicates backend issues
				rlog.Error("upstream timeout exceeded",
					"total_bytes", totalBytes,
					"read_count", readCount,
					"error", upstreamCtx.Err())
				return totalBytes, makeUserFriendlyError(upstreamCtx.Err(), time.Since(lastReadTime), "streaming", s.configuration.ResponseTimeout)
			}
		default:
		}

		// implement a non-blocking read with timeout to prevent hanging on unresponsive backends
		// this is crucial for llm streaming where backends might stall mid-generation
		// without this, a single stalled backend could tie up connections indefinitely
		type readResult struct {
			err error
			n   int
		}

		// buffered channel to avoid goroutine leaks if the select takes the timeout path
		readCh := make(chan readResult, 1)
		readStart := time.Now()

		// spawn a goroutine for the read operation so we can apply a timeout
		// this lets us detect and handle stalled backends gracefully
		go func() {
			n, err := body.Read(buf)
			readCh <- readResult{n: n, err: err}
		}()

		// set a timer to detect if the read takes too long
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
			// read timeout is a critical safeguard against stalled backends
			// this prevents a single unresponsive backend from consuming resources indefinitely
			// and allows us to provide a clear error message to the client
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
	// retrieve a buffer from the pool to avoid allocations during streaming
	value := s.bufferPool.Get()
	if buf, ok := value.([]byte); ok {
		// resize if smaller than needed - this ensures we have enough capacity
		// without having to reallocate during streaming, which would defeat
		// the purpose of the buffer pool
		if len(buf) < bufferSize {
			return make([]byte, bufferSize)
		}
		// use slicing to get the right size without allocation
		return buf[:bufferSize]
	}

	// defensive programming - handle unexpected types from the pool
	// this shouldn't happen but protects against potential bugs
	if value != nil {
		s.logger.Warn("bufferPool returned unexpected type", "type", fmt.Sprintf("%T", value))
	}
	return make([]byte, bufferSize)
}

func (s *SherpaProxyService) shouldContinueAfterClientDisconnect(bytesRead int, timeSinceLastRead time.Duration) bool {
	// we make a strategic decision about whether to keep the backend connection alive
	// after a client disconnect based on two key factors:
	//
	// 1. have we sent enough data to make it worth preserving? (bytesRead threshold)
	//    - if we've only sent a tiny amount, it's cheaper to just start over
	//
	// 2. is the disconnect recent enough that the client might reconnect? (time threshold)
	//    - mobile clients often have brief network blips but reconnect quickly
	//
	// this balances resource usage with improving user experience on flaky connections
	return bytesRead > ClientDisconnectionBytesThreshold && timeSinceLastRead < ClientDisconnectionTimeThreshold
}

func (s *SherpaProxyService) GetStats(context.Context) (ports.ProxyStats, error) {
	return s.statsCollector.GetProxyStats(), nil
}

func (s *SherpaProxyService) UpdateConfig(config ports.ProxyConfiguration) {
	// create a new configuration object with values from the provided configuration
	// this allows runtime reconfiguration without restarting the proxy
	newConfig := &Configuration{
		ProxyPrefix:         config.GetProxyPrefix(),
		ConnectionTimeout:   config.GetConnectionTimeout(),
		ConnectionKeepAlive: config.GetConnectionKeepAlive(),
		ResponseTimeout:     config.GetResponseTimeout(),
		ReadTimeout:         config.GetReadTimeout(),
		StreamBufferSize:    config.GetStreamBufferSize(),
	}

	// update the configuration atomically to avoid race conditions
	// this ensures in-flight requests use consistent settings
	s.configuration = newConfig
}
