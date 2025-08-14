package olla

// 											Olla Proxy Implementation
//
// The Olla proxy implementation is a high-performance, resilient reverse proxy purpose-built for AI inference traffic
// (eg. LLMs, embedding APIs). It improves on Sherpa's implementation with additional safeguards, tuning and zero-GC
// optimisations. Most of the code is inspired by Sherpa, but Olla introduces several enhancements.
//
// Compared to Sherpa, Olla introduces:
// - **Per-endpoint connection pools**: Enables isolated TCP connection reuse, avoiding cross-endpoint interference.
// - **Circuit breakers**: Automatically trips on failure patterns to prevent cascading errors and allow graceful recovery.
// - **Aggressive object pooling**: Reuses request contexts, buffers and error objects to minimise heap allocations and GC pauses.
// - **Atomic stats correction**: Tracks min/max/total latencies lock-free under high concurrency.
// - **TCP optimisations**: Fine-grained tuning (eg. `SetNoDelay`, long keep-alive) designed for streaming workloads.
// - **Backpressure safe streaming**: Handles partial reads, client disconnects and stalled upstreams with resilient fallbacks.
//
// Suitable for workloads with:
// - Long-lived, token-streaming HTTP responses
// - Intermittently unreliable clients (eg. mobile devices, mini-PCs)
// - Multiple backend replicas (with health-state divergence)
//
// Olla is designed for edge/gateway use cases requiring robustness, high availability and minimal jitter under load.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"runtime"
	"runtime/debug"
	"sync/atomic"
	"time"

	"github.com/thushan/olla/internal/core/constants"

	"github.com/puzpuzpuz/xsync/v4"

	"github.com/thushan/olla/internal/adapter/health"
	"github.com/thushan/olla/internal/adapter/proxy/common"
	"github.com/thushan/olla/internal/adapter/proxy/core"
	"github.com/thushan/olla/internal/app/middleware"
	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/core/ports"
	"github.com/thushan/olla/internal/logger"
	"github.com/thushan/olla/internal/util"
	"github.com/thushan/olla/pkg/pool"
)

const (
	// TCP connection tuning for AI streaming workloads
	DefaultMaxIdleConns        = 100
	DefaultMaxConnsPerHost     = 50
	DefaultIdleConnTimeout     = 90 * time.Second
	DefaultTLSHandshakeTimeout = 10 * time.Second
	DefaultTimeout             = 30 * time.Second
	DefaultKeepAlive           = 30 * time.Second
	DefaultReadTimeout         = 30 * time.Second
	DefaultStreamBufferSize    = 64 * 1024
	DefaultSetNoDelay          = true

	ClientDisconnectionBytesThreshold = 1024
	ClientDisconnectionTimeThreshold  = 5 * time.Second

	// Circuit breaker threshold higher than health checker for tolerance
	circuitBreakerThreshold = 5 // vs health.DefaultCircuitBreakerThreshold (3)
)

// Service implements the Olla proxy - optimised for high performance and resilience
type Service struct {
	*core.BaseProxyComponents

	// Object pools for zero-allocation operations
	bufferPool   *pool.Pool[*[]byte]
	requestPool  *pool.Pool[*requestContext]
	responsePool *pool.Pool[[]byte]
	errorPool    *pool.Pool[*errorContext]

	transport     *http.Transport
	configuration *Configuration
	retryHandler  *core.RetryHandler

	// Cleanup management
	cleanupTicker *time.Ticker
	cleanupStop   chan struct{}

	// Per-endpoint connection pools and circuit breakers
	endpointPools   xsync.Map[string, *connectionPool]
	circuitBreakers xsync.Map[string, *circuitBreaker]
}

// connectionPool isolates HTTP transport instances per endpoint
type connectionPool struct {
	transport *http.Transport
	lastUsed  int64 // atomic
	healthy   int64 // atomic: 0=unhealthy, 1=healthy
}

// circuitBreaker prevents overwhelming failing endpoints
type circuitBreaker struct {
	failures    int64 // atomic
	lastFailure int64 // atomic
	state       int64 // atomic: 0=closed, 1=open, 2=half-open
	threshold   int64
}

// requestContext contains per-request data from our object pool
type requestContext struct {
	requestID string
	startTime time.Time
	endpoint  string
	targetURL string
}

func (r *requestContext) Reset() {
	r.requestID = ""
	r.startTime = time.Time{}
	r.endpoint = ""
	r.targetURL = ""
}

// errorContext provides rich error information without allocations
type errorContext struct {
	err       error
	context   string
	duration  time.Duration
	code      int
	allocated bool
}

func (e *errorContext) Reset() {
	e.err = nil
	e.context = ""
	e.duration = 0
	e.code = 0
	e.allocated = false
}

// NewService creates a new Olla proxy service
func NewService(
	discoveryService ports.DiscoveryService,
	selector domain.EndpointSelector,
	configuration *Configuration,
	statsCollector ports.StatsCollector,
	logger logger.StyledLogger,
) (*Service, error) {

	if configuration.StreamBufferSize == 0 {
		configuration.StreamBufferSize = DefaultStreamBufferSize
	}
	if configuration.MaxIdleConns == 0 {
		configuration.MaxIdleConns = DefaultMaxIdleConns
	}
	if configuration.MaxConnsPerHost == 0 {
		configuration.MaxConnsPerHost = DefaultMaxConnsPerHost
	}
	if configuration.IdleConnTimeout == 0 {
		configuration.IdleConnTimeout = DefaultIdleConnTimeout
	}
	if configuration.ReadTimeout == 0 {
		configuration.ReadTimeout = DefaultReadTimeout
	}

	base := core.NewBaseProxyComponents(discoveryService, selector, statsCollector, logger)

	bufferPool, err := pool.NewLitePool(func() *[]byte {
		buf := make([]byte, configuration.StreamBufferSize)
		return &buf
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create buffer pool: %w", err)
	}

	requestPool, err := pool.NewLitePool(func() *requestContext {
		return &requestContext{}
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create request pool: %w", err)
	}

	responsePool, err := pool.NewLitePool(func() []byte {
		return make([]byte, 32*1024) // 32KB for response bodies
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create response pool: %w", err)
	}

	errorPool, err := pool.NewLitePool(func() *errorContext {
		return &errorContext{}
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create error pool: %w", err)
	}

	transport := createOptimisedTransport(configuration)

	service := &Service{
		BaseProxyComponents: base,
		bufferPool:          bufferPool,
		requestPool:         requestPool,
		responsePool:        responsePool,
		errorPool:           errorPool,
		transport:           transport,
		configuration:       configuration,
		retryHandler:        core.NewRetryHandler(discoveryService, logger),
		circuitBreakers:     *xsync.NewMap[string, *circuitBreaker](),
		endpointPools:       *xsync.NewMap[string, *connectionPool](),
		cleanupTicker:       time.NewTicker(5 * time.Minute),
		cleanupStop:         make(chan struct{}),
	}

	// Start cleanup goroutine
	go service.cleanupLoop()

	return service, nil
}

// createOptimisedTransport creates an HTTP transport optimised for AI workloads
func createOptimisedTransport(config *Configuration) *http.Transport {
	return &http.Transport{
		MaxIdleConns:        config.MaxIdleConns,
		MaxIdleConnsPerHost: config.MaxConnsPerHost,
		IdleConnTimeout:     config.IdleConnTimeout,
		TLSHandshakeTimeout: DefaultTLSHandshakeTimeout,
		DisableCompression:  true,
		ForceAttemptHTTP2:   true,
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			dialer := &net.Dialer{
				Timeout:   config.GetConnectionTimeout(),
				KeepAlive: config.GetConnectionKeepAlive(),
			}
			conn, err := dialer.DialContext(ctx, network, addr)
			if err != nil {
				return nil, err
			}
			if tcpConn, ok := conn.(*net.TCPConn); ok {
				// We ignore errors for these settings on purpose
				_ = tcpConn.SetNoDelay(DefaultSetNoDelay)
				_ = tcpConn.SetKeepAlive(true)
				_ = tcpConn.SetKeepAlivePeriod(config.GetConnectionKeepAlive())
			}
			return conn, nil
		},
		MaxResponseHeaderBytes: 32 << 10, // 32KB
		WriteBufferSize:        64 << 10, // 64KB
		ReadBufferSize:         64 << 10, // 64KB
	}
}

// getOrCreateEndpointPool returns a connection pool for the endpoint
func (s *Service) getOrCreateEndpointPool(endpoint string) *connectionPool {
	if pool, ok := s.endpointPools.Load(endpoint); ok {
		atomic.StoreInt64(&pool.lastUsed, time.Now().UnixNano())
		return pool
	}

	newPool := &connectionPool{
		transport: createOptimisedTransport(s.configuration),
		lastUsed:  time.Now().UnixNano(),
		healthy:   1,
	}

	actual, _ := s.endpointPools.LoadOrStore(endpoint, newPool)
	return actual
}

// GetCircuitBreaker returns the circuit breaker for an endpoint (exported for testing)
func (s *Service) GetCircuitBreaker(endpoint string) *circuitBreaker {
	if cb, ok := s.circuitBreakers.Load(endpoint); ok {
		return cb
	}

	newCB := &circuitBreaker{
		threshold: circuitBreakerThreshold,
		state:     0, // closed
	}

	actual, _ := s.circuitBreakers.LoadOrStore(endpoint, newCB)
	return actual
}

// Circuit breaker methods
func (cb *circuitBreaker) IsOpen() bool {
	state := atomic.LoadInt64(&cb.state)
	if state != 1 {
		return false
	}

	// Check if timeout has passed
	lastFailure := atomic.LoadInt64(&cb.lastFailure)
	if time.Since(time.Unix(0, lastFailure)) > health.DefaultCircuitBreakerTimeout {
		// Try half-open state
		if atomic.CompareAndSwapInt64(&cb.state, 1, 2) {
			// State transition: Open -> Half-open
			return false
		}
	}

	return true
}

func (cb *circuitBreaker) RecordSuccess() {
	atomic.StoreInt64(&cb.failures, 0)
	atomic.StoreInt64(&cb.state, 0) // closed
}

func (cb *circuitBreaker) RecordFailure() {
	failures := atomic.AddInt64(&cb.failures, 1)
	atomic.StoreInt64(&cb.lastFailure, time.Now().UnixNano())

	if failures >= cb.threshold {
		atomic.StoreInt64(&cb.state, 1) // open
	}
}

// ProxyRequest handles incoming HTTP requests
func (s *Service) ProxyRequest(ctx context.Context, w http.ResponseWriter, r *http.Request, stats *ports.RequestStats, rlog logger.StyledLogger) error {
	endpoints, err := s.DiscoveryService.GetHealthyEndpoints(ctx)
	if err != nil {
		return err
	}

	return s.ProxyRequestToEndpoints(ctx, w, r, endpoints, stats, rlog)
}

// ProxyRequestToEndpoints delegates to retry-aware implementation
func (s *Service) ProxyRequestToEndpoints(ctx context.Context, w http.ResponseWriter, r *http.Request, endpoints []*domain.Endpoint, stats *ports.RequestStats, rlog logger.StyledLogger) error {
	return s.ProxyRequestToEndpointsWithRetry(ctx, w, r, endpoints, stats, rlog)
}

// proxyToSingleEndpointLegacy retained for reference during migration
// TODO: Remove after retry logic stability confirmed
func (s *Service) proxyToSingleEndpointLegacy(ctx context.Context, w http.ResponseWriter, r *http.Request, endpoints []*domain.Endpoint, stats *ports.RequestStats, rlog logger.StyledLogger) (err error) {
	// Get request context from pool
	reqCtx := s.requestPool.Get()
	defer s.requestPool.Put(reqCtx)

	reqCtx.requestID = stats.RequestID
	reqCtx.startTime = stats.StartTime

	// Panic recovery
	defer func() {
		if rec := recover(); rec != nil {
			s.handlePanic(ctx, w, r, stats, rlog, rec, &err)
		}
	}()

	s.IncrementRequests()

	// Use context logger if available, fallback to provided logger
	ctxLogger := middleware.GetLogger(ctx)
	if ctxLogger != nil {
		ctxLogger.Debug("Olla proxy request started",
			"method", r.Method,
			"url", r.URL.String(),
			"endpoint_count", len(endpoints))
	} else {
		rlog.Debug("proxy request started", "method", r.Method, "url", r.URL.String())
	}

	if len(endpoints) == 0 {
		if ctxLogger != nil {
			ctxLogger.Error("No healthy endpoints available for request")
		} else {
			rlog.Error("no healthy endpoints available")
		}
		s.RecordFailure(ctx, nil, time.Since(stats.StartTime), common.ErrNoHealthyEndpoints)
		return common.ErrNoHealthyEndpoints
	}

	if ctxLogger != nil {
		ctxLogger.Debug("Using provided endpoints", "count", len(endpoints))
	} else {
		rlog.Debug("using provided endpoints", "count", len(endpoints))
	}

	// Select endpoint with circuit breaker check
	endpoint, cb := s.selectEndpointWithCircuitBreaker(endpoints, rlog)
	if endpoint == nil {
		s.RecordFailure(ctx, nil, time.Since(stats.StartTime), fmt.Errorf("all endpoints circuit breakers open"))
		return fmt.Errorf("all endpoints unavailable due to circuit breakers")
	}

	stats.EndpointName = endpoint.Name
	reqCtx.endpoint = endpoint.Name

	// Track connections
	s.Selector.IncrementConnections(endpoint)
	defer s.Selector.DecrementConnections(endpoint)

	// Build target URL
	targetURL := s.buildTargetURL(r, endpoint)
	stats.TargetUrl = targetURL.String()
	reqCtx.targetURL = targetURL.String()

	if ctxLogger != nil {
		ctxLogger.Info("Request dispatching",
			"endpoint", endpoint.Name,
			"target", stats.TargetUrl,
			"model", stats.Model)
	} else {
		rlog.Info("Request dispatching", "endpoint", endpoint.Name, "target", stats.TargetUrl, "model", stats.Model)
	}

	// Create and prepare proxy request
	proxyReq, err := s.prepareProxyRequest(ctx, r, targetURL, stats)
	if err != nil {
		cb.RecordFailure()
		s.RecordFailure(ctx, endpoint, time.Since(stats.StartTime), err)
		return fmt.Errorf("failed to create proxy request: %w", err)
	}

	rlog.Debug("created proxy request")

	// Execute backend request
	resp, err := s.executeBackendRequest(ctx, endpoint, proxyReq, cb, stats, rlog)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Handle successful response
	return s.handleSuccessfulResponse(ctx, w, r, resp, endpoint, cb, stats, rlog)
}

// handlePanic handles panic recovery in proxy requests
func (s *Service) handlePanic(ctx context.Context, w http.ResponseWriter, r *http.Request, stats *ports.RequestStats, rlog logger.StyledLogger, rec interface{}, err *error) {
	s.RecordFailure(ctx, nil, time.Since(stats.StartTime), fmt.Errorf("panic: %v", rec))

	*err = fmt.Errorf("proxy panic recovered after %.1fs: %v", time.Since(stats.StartTime).Seconds(), rec)
	rlog.Error("proxy request panic recovered",
		"panic", rec,
		"method", r.Method,
		"path", r.URL.Path,
		"stack", string(debug.Stack()))

	if w.Header().Get(constants.HeaderContentType) == "" {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

// selectEndpointWithCircuitBreaker selects an endpoint that has a healthy circuit breaker
func (s *Service) selectEndpointWithCircuitBreaker(endpoints []*domain.Endpoint, rlog logger.StyledLogger) (*domain.Endpoint, *circuitBreaker) {
	for _, ep := range endpoints {
		cb := s.GetCircuitBreaker(ep.Name)
		stateBefore := atomic.LoadInt64(&cb.state)
		if !cb.IsOpen() {
			stateAfter := atomic.LoadInt64(&cb.state)
			// Log state transition if it changed (Open -> Half-open)
			if stateBefore == 1 && stateAfter == 2 {
				rlog.Info("Circuit breaker entering half-open state",
					"endpoint", ep.Name,
					"timeout", health.DefaultCircuitBreakerTimeout)
			}
			return ep, cb
		}
		// Log when skipping endpoint due to open circuit breaker
		rlog.Debug("Skipping endpoint due to open circuit breaker",
			"endpoint", ep.Name,
			"failures", atomic.LoadInt64(&cb.failures))
	}
	return nil, nil
}

// buildTargetURL builds the target URL for the proxy request
func (s *Service) buildTargetURL(r *http.Request, endpoint *domain.Endpoint) *url.URL {
	targetPath := util.StripPrefix(r.URL.Path, s.configuration.GetProxyPrefix())
	targetURL := endpoint.URL.ResolveReference(&url.URL{Path: targetPath})
	if r.URL.RawQuery != "" {
		targetURL.RawQuery = r.URL.RawQuery
	}
	return targetURL
}

// prepareProxyRequest creates and prepares the proxy request with headers
func (s *Service) prepareProxyRequest(ctx context.Context, r *http.Request, targetURL *url.URL, stats *ports.RequestStats) (*http.Request, error) {
	proxyReq, err := http.NewRequestWithContext(ctx, r.Method, targetURL.String(), r.Body)
	if err != nil {
		return nil, err
	}

	// Copy headers
	headerStart := time.Now()
	core.CopyHeaders(proxyReq, r)
	stats.HeaderProcessingMs = time.Since(headerStart).Milliseconds()

	// Add model header
	if model, ok := ctx.Value("model").(string); ok && model != "" {
		proxyReq.Header.Set("X-Model", model)
		stats.Model = model
	}

	// Mark request processing complete
	stats.RequestProcessingMs = time.Since(stats.StartTime).Milliseconds()

	return proxyReq, nil
}

// executeBackendRequest executes the request to the backend
func (s *Service) executeBackendRequest(ctx context.Context, endpoint *domain.Endpoint, proxyReq *http.Request, cb *circuitBreaker, stats *ports.RequestStats, rlog logger.StyledLogger) (*http.Response, error) {
	// Get connection pool for this endpoint
	pool := s.getOrCreateEndpointPool(endpoint.Name)

	// Execute request
	rlog.Debug("making round-trip request", "target", proxyReq.URL.String())
	backendStart := time.Now()
	resp, err := pool.transport.RoundTrip(proxyReq)
	stats.BackendResponseMs = time.Since(backendStart).Milliseconds()

	if err != nil {
		// Record failure and check if circuit breaker opened
		failuresBefore := atomic.LoadInt64(&cb.failures)
		cb.RecordFailure()
		failuresAfter := atomic.LoadInt64(&cb.failures)

		// Log if circuit breaker just opened (critical for monitoring)
		if failuresBefore < cb.threshold && failuresAfter >= cb.threshold {
			rlog.Warn("Circuit breaker opened",
				"endpoint", endpoint.Name,
				"failures", failuresAfter,
				"threshold", cb.threshold,
				"error", err)
		}

		rlog.Error("round-trip failed", "error", err)
		s.RecordFailure(ctx, endpoint, time.Since(stats.StartTime), err)

		// Publish circuit breaker event if opened
		if cb.IsOpen() {
			s.PublishEvent(core.ProxyEvent{
				Type:      core.EventTypeCircuitBreaker,
				RequestID: stats.RequestID,
				Endpoint:  endpoint.Name,
				Error:     err,
				Duration:  time.Since(stats.StartTime),
			})
		}

		duration := time.Since(stats.StartTime)
		return nil, common.MakeUserFriendlyError(err, duration, "backend", s.configuration.GetResponseTimeout())
	}

	return resp, nil
}

// handleSuccessfulResponse handles the successful response from the backend
func (s *Service) handleSuccessfulResponse(ctx context.Context, w http.ResponseWriter, r *http.Request, resp *http.Response, endpoint *domain.Endpoint, cb *circuitBreaker, stats *ports.RequestStats, rlog logger.StyledLogger) error {
	// Get context logger for this function scope
	ctxLogger := middleware.GetLogger(ctx)
	// Circuit breaker success
	stateBefore := atomic.LoadInt64(&cb.state)
	cb.RecordSuccess()
	stateAfter := atomic.LoadInt64(&cb.state)

	// Log state transition if circuit breaker closed
	if stateBefore != 0 && stateAfter == 0 {
		rlog.Info("Circuit breaker closed after successful request",
			"endpoint", endpoint.Name,
			"previous_state", map[int64]string{1: "open", 2: "half-open"}[stateBefore])
	}

	rlog.Debug("round-trip success", "status", resp.StatusCode)

	core.SetResponseHeaders(w, stats, endpoint)

	// Copy response headers
	for key, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}

	w.WriteHeader(resp.StatusCode)

	// start streaming the response body
	rlog.Debug("starting response stream")
	streamStart := time.Now()
	stats.FirstDataMs = time.Since(stats.StartTime).Milliseconds()

	buffer := s.bufferPool.Get()
	defer s.bufferPool.Put(buffer)

	// Separate client and upstream contexts for proper cancellation handling
	// Only create a different context if needed to avoid allocations
	upstreamCtx := ctx
	if resp != nil && resp.Request != nil {
		upstreamCtx = resp.Request.Context()
	}

	// Use r.Context() for client context and upstreamCtx for upstream context
	bytesWritten, streamErr := s.streamResponse(r.Context(), upstreamCtx, w, resp, *buffer, rlog)
	stats.StreamingMs = time.Since(streamStart).Milliseconds()
	stats.TotalBytes = bytesWritten

	if streamErr != nil && !errors.Is(streamErr, context.Canceled) {
		rlog.Debug("streaming error", "error", streamErr, "bytes_written", bytesWritten)
		s.RecordFailure(ctx, endpoint, time.Since(stats.StartTime), streamErr)
		return common.MakeUserFriendlyError(streamErr, time.Since(stats.StartTime), "streaming", s.configuration.GetResponseTimeout())
	}

	// stats update
	duration := time.Since(stats.StartTime)
	s.RecordSuccess(endpoint, duration.Milliseconds(), int64(bytesWritten))

	stats.EndTime = time.Now()
	stats.Latency = duration.Milliseconds()

	// Log detailed completion metrics at Debug level to reduce redundancy
	if ctxLogger != nil {
		ctxLogger.Debug("Olla proxy metrics",
			"endpoint", endpoint.Name,
			"latency_ms", stats.Latency,
			"processing_ms", stats.RequestProcessingMs,
			"backend_ms", stats.BackendResponseMs,
			"first_data_ms", stats.FirstDataMs,
			"streaming_ms", stats.StreamingMs,
			"selection_ms", stats.SelectionMs,
			"header_ms", stats.HeaderProcessingMs,
			"total_bytes", stats.TotalBytes,
			"bytes_formatted", middleware.FormatBytes(int64(stats.TotalBytes)),
			"status", resp.StatusCode,
			"request_id", middleware.GetRequestID(ctx))
	} else {
		rlog.Debug("proxy request completed",
			"endpoint", endpoint.Name,
			"latency_ms", stats.Latency,
			"processing_ms", stats.RequestProcessingMs,
			"backend_ms", stats.BackendResponseMs,
			"first_data_ms", stats.FirstDataMs,
			"streaming_ms", stats.StreamingMs,
			"selection_ms", stats.SelectionMs,
			"header_ms", stats.HeaderProcessingMs,
			"total_bytes", stats.TotalBytes,
			"status", resp.StatusCode)
	}

	return nil
}

// streamResponse performs buffered streaming with backpressure handling
func (s *Service) streamResponse(clientCtx, upstreamCtx context.Context, w http.ResponseWriter, resp *http.Response, buffer []byte, rlog logger.StyledLogger) (int, error) {
	totalBytes := 0
	flusher, canFlush := w.(http.Flusher)
	isStreaming := core.AutoDetectStreamingMode(clientCtx, resp, s.configuration.GetProxyProfile())

	clientDisconnected := false
	bytesAfterDisconnect := 0
	disconnectTime := time.Time{}

	// Pre-allocate timer to avoid allocations in hot path
	readDeadline := time.NewTimer(s.configuration.GetReadTimeout())
	defer readDeadline.Stop()

	for {
		select {
		case <-clientCtx.Done():
			if !clientDisconnected {
				clientDisconnected = true
				disconnectTime = time.Now()
				rlog.Debug("client disconnected during streaming", "bytes_sent", totalBytes)

				// Publish client disconnect event
				s.PublishEvent(core.ProxyEvent{
					Type: core.EventTypeClientDisconnect,
					Metadata: core.ProxyEventMetadata{
						BytesSent: int64(totalBytes),
					},
				})
			}
		case <-upstreamCtx.Done():
			return totalBytes, upstreamCtx.Err()
		case <-readDeadline.C:
			return totalBytes, fmt.Errorf("read timeout after %v", s.configuration.GetReadTimeout())
		default:
		}

		// Reset timer for next read
		readDeadline.Reset(s.configuration.GetReadTimeout())

		n, err := resp.Body.Read(buffer)
		if n > 0 {
			if !clientDisconnected {
				written, writeErr := w.Write(buffer[:n])
				totalBytes += written

				if writeErr != nil {
					rlog.Debug("write error during streaming", "error", writeErr, "bytes_written", totalBytes)
					return totalBytes, writeErr
				}

				// force data out for real-time streaming
				if canFlush && isStreaming {
					flusher.Flush()
				}
			} else {
				bytesAfterDisconnect += n

				if bytesAfterDisconnect > ClientDisconnectionBytesThreshold ||
					time.Since(disconnectTime) > ClientDisconnectionTimeThreshold {
					rlog.Debug("stopping stream after client disconnect",
						"bytes_after_disconnect", bytesAfterDisconnect,
						"time_since_disconnect", time.Since(disconnectTime))
					return totalBytes, context.Canceled
				}
			}
		}

		if err != nil {
			if err == io.EOF {
				return totalBytes, nil
			}
			rlog.Debug("read error during streaming", "error", err, "bytes_read", totalBytes)
			return totalBytes, err
		}
	}
}

// GetStats returns current proxy statistics
func (s *Service) GetStats(ctx context.Context) (ports.ProxyStats, error) {
	return s.GetProxyStats(), nil
}

// UpdateConfig updates the proxy configuration
func (s *Service) UpdateConfig(config ports.ProxyConfiguration) {
	newConfig := &Configuration{
		ProxyPrefix:         config.GetProxyPrefix(),
		ConnectionTimeout:   config.GetConnectionTimeout(),
		ConnectionKeepAlive: config.GetConnectionKeepAlive(),
		ResponseTimeout:     config.GetResponseTimeout(),
		ReadTimeout:         config.GetReadTimeout(),
		StreamBufferSize:    config.GetStreamBufferSize(),
		// Preserve Olla-specific settings
		MaxIdleConns:    s.configuration.MaxIdleConns,
		IdleConnTimeout: s.configuration.IdleConnTimeout,
		MaxConnsPerHost: s.configuration.MaxConnsPerHost,
	}

	// Update configuration atomically
	s.configuration = newConfig
}

// cleanupLoop periodically cleans up unused endpoint pools and circuit breakers
func (s *Service) cleanupLoop() {
	defer func() {
		if r := recover(); r != nil {
			s.Logger.Error("cleanupLoop panic recovered", "panic", r)
		}
	}()

	for {
		select {
		case <-s.cleanupStop:
			return
		case <-s.cleanupTicker.C:
			s.cleanupUnusedResources()
		}
	}
}

// cleanupUnusedResources removes stale endpoint pools and circuit breakers
func (s *Service) cleanupUnusedResources() {
	now := time.Now().UnixNano()
	staleThreshold := int64(5 * time.Minute)

	// Cleanup unused endpoint pools
	var poolsRemoved int
	s.endpointPools.Range(func(endpoint string, pool *connectionPool) bool {
		lastUsed := atomic.LoadInt64(&pool.lastUsed)
		if now-lastUsed > staleThreshold {
			s.endpointPools.Delete(endpoint)
			pool.transport.CloseIdleConnections()
			poolsRemoved++
		}
		return true
	})

	// Cleanup circuit breakers for non-existent endpoints
	var cbRemoved int
	endpointExists := make(map[string]bool)
	s.endpointPools.Range(func(endpoint string, _ *connectionPool) bool {
		endpointExists[endpoint] = true
		return true
	})

	s.circuitBreakers.Range(func(endpoint string, cb *circuitBreaker) bool {
		if !endpointExists[endpoint] {
			// Also check if circuit breaker is closed and hasn't failed recently
			state := atomic.LoadInt64(&cb.state)
			lastFailure := atomic.LoadInt64(&cb.lastFailure)
			if state == 0 && (lastFailure == 0 || now-lastFailure > staleThreshold) {
				s.circuitBreakers.Delete(endpoint)
				cbRemoved++
			}
		}
		return true
	})

	if poolsRemoved > 0 || cbRemoved > 0 {
		s.Logger.Debug("cleaned up unused resources",
			"pools_removed", poolsRemoved,
			"circuit_breakers_removed", cbRemoved)
	}
}

// Cleanup cleans up resources
func (s *Service) Cleanup() {
	// Stop cleanup goroutine
	if s.cleanupStop != nil {
		close(s.cleanupStop)
	}
	if s.cleanupTicker != nil {
		s.cleanupTicker.Stop()
	}

	// Close all endpoint pools
	s.endpointPools.Range(func(key string, pool *connectionPool) bool {
		pool.transport.CloseIdleConnections()
		return true
	})

	s.endpointPools.Clear()
	s.circuitBreakers.Clear()

	s.BaseProxyComponents.Shutdown()

	// force GC to clean up
	runtime.GC()

	s.Logger.Debug("Olla proxy service cleaned up")
}
