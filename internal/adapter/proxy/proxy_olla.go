package proxy

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
	"sync/atomic"
	"time"

	"github.com/puzpuzpuz/xsync/v4"

	"github.com/thushan/olla/internal/core/constants"
	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/core/ports"
	"github.com/thushan/olla/internal/logger"
	"github.com/thushan/olla/internal/util"
	"github.com/thushan/olla/internal/version"
	"github.com/thushan/olla/pkg/pool"
)

var (
	errNoHealthyEndpoints = errors.New("no healthy endpoints")
)

type OllaProxyService struct {

	// we're using object pools to avoid garbage collection pressure during high-throughput operations.
	// this dramatically reduces latency spikes by preventing allocations in hot paths
	bufferPool   *pool.Pool[*[]byte]
	requestPool  *pool.Pool[*requestContext]
	responsePool *pool.Pool[[]byte]
	errorPool    *pool.Pool[*errorContext]

	discoveryService ports.DiscoveryService
	selector         domain.EndpointSelector
	statsCollector   ports.StatsCollector
	logger           logger.StyledLogger

	transport     *http.Transport
	configuration *OllaConfiguration

	// we maintain separate connection pools per endpoint to maximise tcp connection reuse
	// this avoids the overhead of establishing new connections for each request
	endpointPools xsync.Map[string, *connectionPool]

	// circuit breakers protect the system when backends are struggling
	// they prevent cascading failures by failing fast when an endpoint is unhealthy
	circuitBreakers xsync.Map[string, *circuitBreaker]

	// using atomic operations for stats to avoid mutex contention at high throughput
	// this gives us thread-safe counters without locking overhead
	stats struct {
		totalRequests      int64
		successfulRequests int64
		failedRequests     int64
		totalLatency       int64
		minLatency         int64
		maxLatency         int64
	}
}

// connectionPool isolates http transport instances per endpoint
// this prevents a misbehaving endpoint from affecting others and allows
// for endpoint-specific connection management
type connectionPool struct {
	transport *http.Transport
	lastUsed  int64 // atomic for thread-safe access
	healthy   int64 // atomic flag: 0=unhealthy, 1=healthy
}

// circuitBreaker prevents overwhelming failing endpoints
// it tracks failures and automatically trips after threshold is reached,
// then allows periodic retry attempts in half-open state
type circuitBreaker struct {
	failures    int64 // atomic counter for thread-safe increments
	lastFailure int64 // atomic timestamp for timeout calculations
	state       int64 // atomic state machine: 0=closed, 1=open, 2=half-open
	threshold   int64
}

// requestContext contains per-request data from our object pool
// reusing these structs avoids heap allocations in the hot path
// which is critical for reducing gc pauses during high load
type requestContext struct {
	requestID string
	startTime time.Time
	endpoint  string
	targetURL string
}

// errorContext provides rich error information without allocations
// we pool these to avoid creating garbage during error handling
// which helps maintain performance even when errors occur
type errorContext struct {
	err       error
	context   string
	duration  time.Duration
	code      int
	allocated bool
}

const (
	// tcp connection tuning specifically optimised for ai streaming workloads
	// we need more connections than typical web apps because llm requests are long-lived
	// and we want to avoid connection exhaustion during traffic spikes
	OllaDefaultMaxIdleConns        = 100
	OllaDefaultMaxConnsPerHost     = 50
	OllaDefaultIdleConnTimeout     = 90 * time.Second
	OllaDefaultTLSHandshakeTimeout = 10 * time.Second
	OllaDefaultTimeout             = 30 * time.Second
	OllaDefaultKeepAlive           = 30 * time.Second
	OllaDefaultReadTimeout         = 30 * time.Second
	// larger buffer size improves throughput for token streaming from llms
	// 64kb is a sweet spot between memory usage and reducing syscall frequency
	OllaDefaultStreamBufferSize = 64 * 1024
	// disabling nagle's algorithm is crucial for llm streaming to reduce latency
	// we want tokens sent immediately rather than waiting to fill tcp segments
	OllaDefaultSetNoDelay = true

	// llm clients often disconnect and reconnect during streaming sessions
	// these thresholds let us continue streaming briefly after a disconnect
	// which helps mobile clients with flaky connections get complete responses
	OllaClientDisconnectionBytesThreshold = 1024
	OllaClientDisconnectionTimeThreshold  = 5 * time.Second

	// circuit breaker prevents cascading failures when backends are struggling
	// after 5 consecutive failures, we'll stop sending traffic to that endpoint
	// but we'll try again after 30 seconds to see if it's recovered
	circuitBreakerThreshold = 5
	circuitBreakerTimeout   = 30 * time.Second
)

func NewOllaService(
	discoveryService ports.DiscoveryService,
	selector domain.EndpointSelector,
	configuration *OllaConfiguration,
	statsCollector ports.StatsCollector,
	logger logger.StyledLogger,
) *OllaProxyService {
	// apply comprehensive defaults
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

	// custom transport with tcp tuning for llm streaming workloads
	// we're optimising for consistent token delivery rather than throughput
	transport := &http.Transport{
		MaxIdleConns:        configuration.MaxIdleConns,
		MaxIdleConnsPerHost: configuration.MaxConnsPerHost,
		IdleConnTimeout:     configuration.IdleConnTimeout,
		TLSHandshakeTimeout: OllaDefaultTLSHandshakeTimeout,
		DisableCompression:  true, // compression adds latency and llm responses are often already compressed
		ForceAttemptHTTP2:   true, // http/2 multiplexing is better for streaming responses
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			dialer := &net.Dialer{
				Timeout:   OllaDefaultTimeout,
				KeepAlive: OllaDefaultKeepAlive,
			}
			conn, err := dialer.DialContext(ctx, network, addr)
			if err != nil {
				return nil, err
			}
			// critical tcp tuning for llm streaming - we need to disable nagle's algorithm
			// because it buffers small packets to reduce overhead, but for llms we want
			// each token sent immediately even if it's a tiny packet
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
		endpointPools:    *xsync.NewMap[string, *connectionPool](),
		circuitBreakers:  *xsync.NewMap[string, *circuitBreaker](),
	}

	// setting up object pools to avoid memory allocations during request handling
	// this is critical for high-throughput scenarios where gc pauses would cause jitter
	bufPool, err := pool.NewLitePool(func() *[]byte {
		// we use the configured buffer size to balance memory usage vs syscall frequency
		buf := make([]byte, configuration.StreamBufferSize)
		return &buf
	})
	if err != nil {
		logger.Error("Failed to create buffer pool", "error", err)
		return nil
	}
	service.bufferPool = bufPool

	reqPool, err := pool.NewLitePool(func() *requestContext {
		// pre-allocate request context objects to avoid heap allocations in hot path
		return &requestContext{}
	})
	if err != nil {
		logger.Error("Failed to create request pool", "error", err)
		return nil
	}
	service.requestPool = reqPool

	respPool, err := pool.NewLitePool(func() []byte {
		// 4kb is a good starting size for most response headers
		return make([]byte, 4096)
	})
	if err != nil {
		logger.Error("Failed to create response pool", "error", err)
		return nil
	}
	service.responsePool = respPool

	errPool, err := pool.NewLitePool(func() *errorContext {
		// pooling error contexts helps maintain performance even during error cases
		return &errorContext{}
	})
	if err != nil {
		logger.Error("Failed to create error pool", "error", err)
		return nil
	}
	service.errorPool = errPool

	// initialise min latency to max possible value so first real measurement becomes the min
	// this avoids having to handle special cases for the first request
	atomic.StoreInt64(&service.stats.minLatency, int64(^uint64(0)>>1))

	return service
}

// getOrCreateConnectionPool lazily initialises connection pools per endpoint
// this lets us efficiently manage connections without pre-allocating resources
func (s *OllaProxyService) getOrCreateConnectionPool(endpoint string) *connectionPool {
	epool, ok := s.endpointPools.Load(endpoint)
	if ok && epool != nil {
		// update last used timestamp for pool cleanup decisions
		atomic.StoreInt64(&epool.lastUsed, time.Now().UnixNano())
		return epool
	}

	// create a new pool with a cloned transport to isolate settings
	newPool := &connectionPool{
		transport: s.transport.Clone(),
		lastUsed:  time.Now().UnixNano(),
		healthy:   1,
	}
	// use atomic operation to handle race conditions when multiple goroutines
	// try to create the same pool simultaneously
	actual, _ := s.endpointPools.LoadOrStore(endpoint, newPool)
	if actual != nil {
		atomic.StoreInt64(&actual.lastUsed, time.Now().UnixNano())
		return actual
	}
	return newPool
}

// getCircuitBreaker lazily initialises circuit breakers per endpoint
// this provides failure isolation without pre-allocating resources
func (s *OllaProxyService) getCircuitBreaker(endpoint string) *circuitBreaker {
	cb, ok := s.circuitBreakers.Load(endpoint)
	if ok && cb != nil {
		return cb
	}

	// create a new circuit breaker with default threshold
	newCB := &circuitBreaker{
		threshold: circuitBreakerThreshold,
	}
	// use atomic operation to handle race conditions when multiple goroutines
	// try to create the same circuit breaker simultaneously
	actual, _ := s.circuitBreakers.LoadOrStore(endpoint, newCB)
	if actual != nil {
		return actual
	}
	return newCB // defensive programming - should never happen but avoids nil panic
}

// isOpen implements the circuit breaker state machine logic
// it determines whether requests should be allowed through based on failure history
func (cb *circuitBreaker) isOpen() bool {
	state := atomic.LoadInt64(&cb.state)
	if state == 0 { // circuit is closed - normal operation
		return false
	}

	if state == 1 { // circuit is open - failing fast to prevent cascading failures
		lastFailure := atomic.LoadInt64(&cb.lastFailure)
		// check if enough time has passed to try again
		if time.Since(time.Unix(0, lastFailure)) > circuitBreakerTimeout {
			// attempt to transition to half-open state to test if the endpoint has recovered
			// we use compare-and-swap to handle race conditions with other goroutines
			if atomic.CompareAndSwapInt64(&cb.state, 1, 2) {
				return false // allow one test request through
			}
		}
		return true // still open, reject requests
	}

	return false // circuit is half-open, allowing a test request
}

// recordSuccess resets the circuit breaker after a successful request
// in half-open state, this closes the circuit and resumes normal operation
func (cb *circuitBreaker) recordSuccess() {
	atomic.StoreInt64(&cb.failures, 0) // reset failure counter
	atomic.StoreInt64(&cb.state, 0)    // transition to closed state
}

// recordFailure tracks failures and trips the circuit breaker when threshold is reached
// this prevents overwhelming failing endpoints and gives them time to recover
func (cb *circuitBreaker) recordFailure() {
	// atomically increment failure counter and get new value
	failures := atomic.AddInt64(&cb.failures, 1)
	atomic.StoreInt64(&cb.lastFailure, time.Now().UnixNano())

	// trip the circuit breaker if we've reached the failure threshold
	if failures >= cb.threshold {
		atomic.StoreInt64(&cb.state, 1) // transition to open state
	}
}

// updateStats tracks performance metrics without locks for maximum throughput
// we use atomic operations to avoid mutex contention when updating stats from multiple goroutines
func (s *OllaProxyService) updateStats(success bool, latency time.Duration) {
	atomic.AddInt64(&s.stats.totalRequests, 1)

	if success {
		atomic.AddInt64(&s.stats.successfulRequests, 1)
		latencyMs := latency.Milliseconds()
		atomic.AddInt64(&s.stats.totalLatency, latencyMs)

		// these compare-and-swap loops handle race conditions when multiple goroutines
		// try to update min/max values simultaneously - we keep retrying until either:
		// 1. we find our value isn't a new min/max, or
		// 2. we successfully update the value atomically

		// update min latency with lock-free compare-and-swap
		for {
			current := atomic.LoadInt64(&s.stats.minLatency)
			if latencyMs >= current {
				// not a new minimum, no need to update
				break
			}
			if atomic.CompareAndSwapInt64(&s.stats.minLatency, current, latencyMs) {
				// successfully updated the minimum
				break
			}
			// another goroutine updated the value before us, try again
		}

		// update max latency with lock-free compare-and-swap
		for {
			current := atomic.LoadInt64(&s.stats.maxLatency)
			if latencyMs <= current {
				// not a new maximum, no need to update
				break
			}
			if atomic.CompareAndSwapInt64(&s.stats.maxLatency, current, latencyMs) {
				// successfully updated the maximum
				break
			}
			// another goroutine updated the value before us, try again
		}
	} else {
		atomic.AddInt64(&s.stats.failedRequests, 1)
	}
}

// recordFailure records a failure in stats collector
func (s *OllaProxyService) recordFailure(ctx context.Context, endpoint *domain.Endpoint, duration time.Duration, bytes int64) {
	if endpoint == nil {
		// if we have no endpoint, we can't record it
		return
	}

	// Extract model name from context if available
	modelName := ""
	if model, ok := ctx.Value("model").(string); ok {
		modelName = model
	}

	if modelName != "" {
		s.statsCollector.RecordModelRequest(modelName, endpoint, "failure", duration, bytes)
	} else {
		s.statsCollector.RecordRequest(endpoint, "failure", duration, bytes)
	}
}

// ProxyRequest handles incoming HTTP requests and proxies them to healthy endpoints (regardless of type)
func (s *OllaProxyService) ProxyRequest(ctx context.Context, w http.ResponseWriter, r *http.Request, stats *ports.RequestStats, rlog logger.StyledLogger) error {
	endpoints, err := s.discoveryService.GetHealthyEndpoints(ctx)
	if err != nil {
		return err
	}

	return s.ProxyRequestToEndpoints(ctx, w, r, endpoints, stats, rlog)
}

// ProxyRequestToEndpoints proxies the request to the provided endpoints that are relevant for the request.
func (s *OllaProxyService) ProxyRequestToEndpoints(ctx context.Context, w http.ResponseWriter, r *http.Request, endpoints []*domain.Endpoint, stats *ports.RequestStats, rlog logger.StyledLogger) (err error) {
	// Get pre-allocated request context
	reqCtx := s.requestPool.Get()
	defer s.requestPool.Put(reqCtx)

	reqCtx.requestID = stats.RequestID
	reqCtx.startTime = stats.StartTime
	reqCtx.endpoint = ""
	reqCtx.targetURL = ""

	defer func() {
		if rec := recover(); rec != nil {
			s.updateStats(false, time.Since(reqCtx.startTime))
			s.recordFailure(ctx, nil, time.Since(stats.StartTime), 0)
			err = fmt.Errorf("proxy panic recovered after %.1fs: %v (this is a bug, please report)", time.Since(reqCtx.startTime).Seconds(), rec)
			rlog.Error("Proxy request panic recovered", "panic", rec, "method", r.Method, "path", r.URL.Path)

			if w.Header().Get("Content-Type") == "" {
				http.Error(w, "Internal server error", http.StatusInternalServerError)
			}
		}
	}()

	rlog.Debug("proxy request started", "method", r.Method, "url", r.URL.String())

	if len(endpoints) == 0 {
		// No healthy endpoints available, log and return error
		sinceStart := time.Since(stats.StartTime)
		s.updateStats(false, sinceStart)
		s.recordFailure(ctx, nil, sinceStart, 0)

		rlog.Error("no healthy endpoints available")
		return domain.NewProxyError(stats.RequestID, "", r.Method, r.URL.Path, 0, sinceStart, stats.TotalBytes,
			fmt.Errorf("no healthy AI backends available after %.1fs - all endpoints may be down or still being health checked", sinceStart.Seconds()))
	}

	rlog.Debug("using provided endpoints", "count", len(endpoints))

	// Fast endpoint selection with circuit breaker check
	selectionStart := time.Now()
	var endpoint *domain.Endpoint
	for _, ep := range endpoints {
		cb := s.getCircuitBreaker(ep.Name)
		if !cb.isOpen() {
			endpoint = ep
			break
		}
	}

	if endpoint == nil {
		// All endpoints circuit broken, fall back to selector
		endpoint, err = s.selector.Select(ctx, endpoints)
		if err != nil {
			sinceStart := time.Since(stats.StartTime)
			s.updateStats(false, sinceStart)
			s.recordFailure(ctx, endpoint, sinceStart, 0)
			rlog.Error("failed to select endpoint", "error", err)
			return domain.NewProxyError(stats.RequestID, "", r.Method, r.URL.Path, 0, sinceStart, stats.TotalBytes,
				makeUserFriendlyError(fmt.Errorf("failed to select endpoint: %w", err), sinceStart, "selection", s.configuration.ResponseTimeout))
		}
	}

	selectionEnd := time.Now()
	stats.SelectionMs = selectionEnd.Sub(selectionStart).Milliseconds()
	stats.EndpointName = endpoint.Name
	reqCtx.endpoint = endpoint.Name

	// Strip route prefix from request path for upstream
	targetPath := util.StripRoutePrefix(ctx, r.URL.Path, s.configuration.ProxyPrefix)
	targetURL := endpoint.URL.ResolveReference(&url.URL{Path: targetPath}) // no string allocation
	if r.URL.RawQuery != "" {
		targetURL.RawQuery = r.URL.RawQuery
	}

	stats.TargetUrl = targetURL.String()
	reqCtx.targetURL = targetURL.String()

	rlog.Info("Request dispatching to endpoint", "endpoint", endpoint.Name, "target", stats.TargetUrl, "model", stats.Model)

	// Create upstream context with response timeout
	upstreamCtx := ctx
	var cancel context.CancelFunc
	if s.configuration.ResponseTimeout > 0 {
		upstreamCtx, cancel = context.WithTimeout(ctx, s.configuration.ResponseTimeout)
		defer cancel()
	}

	// Create proxy request with connection pool
	pool := s.getOrCreateConnectionPool(endpoint.Name)
	proxyReq, err := http.NewRequestWithContext(upstreamCtx, r.Method, reqCtx.targetURL, r.Body)
	if err != nil {
		sinceStart := time.Since(stats.StartTime)
		s.updateStats(false, sinceStart)
		s.recordFailure(ctx, endpoint, sinceStart, 0)
		rlog.Error("failed to create proxy request", "error", err)
		return domain.NewProxyError(stats.RequestID, reqCtx.targetURL, r.Method, r.URL.Path, 0, sinceStart, stats.TotalBytes,
			fmt.Errorf("failed to create proxy request after %.1fs: %w", sinceStart.Seconds(), err))
	}

	rlog.Debug("created proxy request")

	// Copy headers efficiently
	headerStart := time.Now()
	s.copyHeaders(proxyReq, r)
	headerEnd := time.Now()
	stats.HeaderProcessingMs = headerEnd.Sub(headerStart).Milliseconds()

	requestProcessingEnd := time.Now()
	stats.RequestProcessingMs = requestProcessingEnd.Sub(stats.StartTime).Milliseconds()

	rlog.Debug("making round-trip request", "target", reqCtx.targetURL)

	// Make request with circuit breaker tracking
	cb := s.getCircuitBreaker(endpoint.Name)
	backendStart := time.Now()
	resp, err := pool.transport.RoundTrip(proxyReq)
	backendEnd := time.Now()
	stats.BackendResponseMs = backendEnd.Sub(backendStart).Milliseconds()

	if err != nil {
		cb.recordFailure()
		sinceStart := time.Since(stats.StartTime)
		s.updateStats(false, sinceStart)
		s.recordFailure(ctx, endpoint, sinceStart, 0)
		rlog.Error("round-trip failed", "error", err)
		return domain.NewProxyError(stats.RequestID, reqCtx.targetURL, r.Method, r.URL.Path, 0, sinceStart, stats.TotalBytes,
			makeUserFriendlyError(err, sinceStart, "backend", s.configuration.ResponseTimeout))
	}
	defer resp.Body.Close()

	rlog.Debug("round-trip success", "status", resp.StatusCode)

	// Record success early
	cb.recordSuccess()

	// Track connection after successful establishment
	s.selector.IncrementConnections(endpoint)
	defer s.selector.DecrementConnections(endpoint)

	// add our custom headers before copying upstream headers
	// this prevents upstream from overriding our routing metadata
	w.Header().Set("X-Olla-Endpoint", endpoint.Name)
	w.Header().Set("X-Olla-Backend-Type", endpoint.Type)
	w.Header().Set("X-Olla-Request-ID", stats.RequestID)

	// add model info if available from context
	if model, ok := ctx.Value("model").(string); ok && model != "" {
		w.Header().Set("X-Olla-Model", model)
	}

	// add standard compatibility header
	w.Header().Set("X-Served-By", fmt.Sprintf("olla/%s", endpoint.Name))

	// Copy response headers with zero allocations where possible
	for k, vals := range resp.Header {
		for _, v := range vals {
			w.Header().Add(k, v)
		}
	}

	// We'll set response time header via trailer if client supports it
	// otherwise it will be missing (can't modify headers after WriteHeader)
	if _, ok := w.(http.Flusher); ok {
		w.Header().Set("Trailer", "X-Olla-Response-Time")
	}

	w.WriteHeader(resp.StatusCode)

	rlog.Debug("starting response stream")

	streamStart := time.Now()
	sumBytes, err := s.streamResponse(ctx, upstreamCtx, w, resp.Body, rlog)
	streamEnd := time.Now()
	stats.StreamingMs = streamEnd.Sub(streamStart).Milliseconds()

	if err != nil {
		sinceStart := time.Since(stats.StartTime)
		s.updateStats(false, sinceStart)
		s.recordFailure(ctx, endpoint, sinceStart, int64(sumBytes))
		rlog.Error("streaming failed", "error", err)
		stats.TotalBytes = sumBytes
		return domain.NewProxyError(stats.RequestID, reqCtx.targetURL, r.Method, r.URL.Path, resp.StatusCode, sinceStart, sumBytes,
			makeUserFriendlyError(err, sinceStart, "streaming", s.configuration.ResponseTimeout))
	}

	// cleanup stats
	stats.TotalBytes = sumBytes
	stats.EndTime = time.Now()
	stats.Latency = stats.EndTime.Sub(stats.StartTime).Milliseconds()
	stats.FirstDataMs = streamStart.Sub(stats.StartTime).Milliseconds()

	// Send response time as trailer if supported
	if flusher, ok := w.(http.Flusher); ok {
		// Trailers can only be sent if we declared them earlier
		w.Header().Set("X-Olla-Response-Time", fmt.Sprintf("%dms", stats.Latency))
		flusher.Flush()
	}

	s.updateStats(true, time.Since(stats.StartTime))

	// Record successful request in stats collector
	// Extract model name from context if available
	modelName := ""
	if model, ok := ctx.Value("model").(string); ok {
		modelName = model
	}

	if modelName != "" {
		s.statsCollector.RecordModelRequest(
			modelName,
			endpoint,
			"success",
			time.Duration(stats.Latency)*time.Millisecond,
			int64(stats.TotalBytes),
		)
	} else {
		s.statsCollector.RecordRequest(
			endpoint,
			"success",
			time.Duration(stats.Latency)*time.Millisecond,
			int64(stats.TotalBytes),
		)
	}

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

// streamResponse handles streaming responses with careful error handling and client disconnect tolerance
// this is the heart of our llm streaming implementation, optimised for low latency token delivery
func (s *OllaProxyService) streamResponse(clientCtx, upstreamCtx context.Context, w http.ResponseWriter, body io.Reader, rlog logger.StyledLogger) (int, error) {
	// Initialise stream resources and settings
	streamState := s.initialiseStreamResponse(w, rlog)
	defer s.bufferPool.Put(streamState.bufPtr)

	// Setup context coordination
	combinedCtx, cancel := s.setupStreamContexts(clientCtx, upstreamCtx)
	defer cancel()

	// Main streaming loop - optimised for minimal allocations and latency
	for {
		// Check for context cancellation first
		done, err, newCtx := s.handleContextCancellation(combinedCtx, clientCtx, upstreamCtx, streamState, rlog)
		if done {
			return streamState.totalBytes, err
		}
		if newCtx != nil {
			// Update the combined context if a new one was returned
			combinedCtx = newCtx
		}

		// Perform read operation with timeout
		readResult, readTimer, readStart := s.performStreamRead(body, streamState)

		// Handle the read result
		done, err = s.handleReadResult(readResult, readTimer, readStart, combinedCtx, clientCtx, upstreamCtx, streamState, w, rlog)
		if done {
			return streamState.totalBytes, err
		}
	}
}

// streamState contains all the state needed for streaming operations
// pooling this struct would add complexity for minimal gain since it's only created once per request
type streamState struct {
	lastReadTime time.Time
	flusher      http.Flusher
	bufPtr       *[]byte
	buf          []byte
	readTimeout  time.Duration
	totalBytes   int
	readCount    int
	canFlush     bool
}

// initialiseStreamResponse sets up all resources needed for streaming
func (s *OllaProxyService) initialiseStreamResponse(w http.ResponseWriter, rlog logger.StyledLogger) *streamState {
	// Reuse buffer from pool to avoid allocations in the hot path
	bufPtr := s.bufferPool.Get()
	buf := *bufPtr

	// Check if we can flush - crucial for streaming responses to reach clients immediately
	flusher, canFlush := w.(http.Flusher)
	readTimeout := s.configuration.ReadTimeout
	if readTimeout == 0 {
		readTimeout = OllaDefaultReadTimeout
	}

	state := &streamState{
		bufPtr:       bufPtr,
		buf:          buf,
		flusher:      flusher,
		canFlush:     canFlush,
		readTimeout:  readTimeout,
		totalBytes:   0,
		readCount:    0,
		lastReadTime: time.Now(),
	}

	rlog.Debug("starting response stream", "read_timeout", readTimeout, "buffer_size", len(buf))
	return state
}

// setupStreamContexts creates a combined context to coordinate cancellation
func (s *OllaProxyService) setupStreamContexts(clientCtx, upstreamCtx context.Context) (context.Context, context.CancelFunc) {
	// We use a combined context to coordinate cancellation between client and upstream
	// This lets us handle both client disconnects and upstream timeouts gracefully
	combinedCtx, cancel := context.WithCancel(context.Background())

	// Goroutine to propagate cancellation between contexts
	// This ensures we clean up resources when either side disconnects
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

	return combinedCtx, cancel
}

// handleContextCancellation checks if the context is cancelled and handles the appropriate response
// Returns true if streaming should stop, along with any error and potentially a new context
func (s *OllaProxyService) handleContextCancellation(combinedCtx, clientCtx, upstreamCtx context.Context, state *streamState, rlog logger.StyledLogger) (bool, error, context.Context) {

	select {
	case <-combinedCtx.Done():
		if clientCtx.Err() != nil {
			return s.handleClientDisconnect(clientCtx, state, rlog)
		} else {
			rlog.Error("upstream timeout exceeded",
				"total_bytes", state.totalBytes,
				"read_count", state.readCount,
				"error", upstreamCtx.Err())
			return true, makeUserFriendlyError(upstreamCtx.Err(), time.Since(state.lastReadTime),
				"streaming", s.configuration.ResponseTimeout), nil
		}
	default:
		return false, nil, nil
	}
}

// handleClientDisconnect manages client disconnection scenarios
// Returns true if streaming should stop, along with any error
// If streaming should continue, also returns a new context to use
func (s *OllaProxyService) handleClientDisconnect(clientCtx context.Context, state *streamState,
	rlog logger.StyledLogger) (bool, error, context.Context) {

	rlog.Info("client disconnected during streaming",
		"total_bytes", state.totalBytes,
		"read_count", state.readCount,
		"error", clientCtx.Err())

	// Special handling for llm streaming - mobile clients often disconnect and reconnect
	// We continue streaming briefly to allow them to reconnect and get the full response
	// This improves user experience on flaky connections without wasting resources
	if s.shouldContinueAfterClientDisconnect(state.totalBytes, time.Since(state.lastReadTime)) {
		rlog.Debug("continuing stream briefly after client disconnect")
		// Create a new context to keep streaming despite client disconnect
		return false, nil, context.Background()
	} else {
		duration := time.Since(state.lastReadTime)
		if duration < 2*time.Second {
			// Immediate disconnection likely indicates a client-side issue
			return true, fmt.Errorf("client disconnected immediately during streaming - possible network issue"), nil
		}
		// Normal disconnection after streaming for a while
		return true, fmt.Errorf("client disconnected after %.1fs during streaming", duration.Seconds()), nil
	}
}

// readResult contains the result of a read operation
type readResult struct {
	err error
	n   int
}

// performStreamRead executes a non-blocking read with timeout
func (s *OllaProxyService) performStreamRead(body io.Reader, state *streamState) (chan readResult, *time.Timer, time.Time) {
	readCh := make(chan readResult, 1)
	readStart := time.Now()

	// Spawn a goroutine for the read operation so we can apply a timeout
	// This lets us detect and handle stalled backends gracefully
	go func() {
		n, err := body.Read(state.buf)
		readCh <- readResult{n: n, err: err}
	}()

	// Set a timer to detect if the read takes too long
	readTimer := time.NewTimer(state.readTimeout)

	return readCh, readTimer, readStart
}

// handleReadResult processes the result of a read operation
// Returns true if streaming should stop, along with any error
func (s *OllaProxyService) handleReadResult(readCh chan readResult, readTimer *time.Timer, readStart time.Time,
	combinedCtx, clientCtx, upstreamCtx context.Context, state *streamState, w http.ResponseWriter, rlog logger.StyledLogger) (bool, error) {

	select {
	case <-combinedCtx.Done():
		readTimer.Stop()
		return s.handleContextCancellationDuringRead(clientCtx, upstreamCtx, readCh, readStart, state, w, rlog)

	case <-readTimer.C:
		return s.handleReadTimeout(state, rlog)

	case result := <-readCh:
		readTimer.Stop()
		return s.processReadData(result, readStart, state, w, rlog)
	}
}

// handleContextCancellationDuringRead handles context cancellation that occurs during a read operation
// Returns true if streaming should stop, along with any error
func (s *OllaProxyService) handleContextCancellationDuringRead(clientCtx, upstreamCtx context.Context,
	readCh chan readResult, readStart time.Time, state *streamState, w http.ResponseWriter, rlog logger.StyledLogger) (bool, error) {

	if clientCtx.Err() != nil {
		rlog.Debug("client context cancelled during read wait",
			"total_bytes", state.totalBytes,
			"read_count", state.readCount)

		// Try to complete current read and send it
		select {
		case result := <-readCh:
			if result.n > 0 {
				if _, err := w.Write(state.buf[:result.n]); err != nil {
					return true, fmt.Errorf("failed to write response after client disconnect: %w", err)
				}
				if state.canFlush {
					state.flusher.Flush()
				}
				state.totalBytes += result.n
			}
		case <-time.After(1 * time.Second):
			// Read taking too long after client disconnect
		}
		return true, makeUserFriendlyError(clientCtx.Err(), time.Since(readStart), "streaming", s.configuration.ResponseTimeout)
	} else {
		rlog.Error("upstream context cancelled during read wait",
			"total_bytes", state.totalBytes,
			"read_count", state.readCount)
		return true, makeUserFriendlyError(upstreamCtx.Err(), time.Since(readStart), "streaming", s.configuration.ResponseTimeout)
	}
}

// handleReadTimeout handles the case when a read operation times out
// Returns true if streaming should stop, along with any error
func (s *OllaProxyService) handleReadTimeout(state *streamState, rlog logger.StyledLogger) (bool, error) {
	rlog.Error("read timeout exceeded between chunks",
		"timeout", state.readTimeout,
		"total_bytes", state.totalBytes,
		"read_count", state.readCount,
		"time_since_last_read", time.Since(state.lastReadTime))
	return true, fmt.Errorf("AI backend stopped responding - no data received for %.1fs (backend may be overloaded)",
		state.readTimeout.Seconds())
}

// processReadData handles successful read data and writes it to the client
// Returns true if streaming should stop, along with any error
func (s *OllaProxyService) processReadData(result readResult, readStart time.Time,
	state *streamState, w http.ResponseWriter, rlog logger.StyledLogger) (bool, error) {

	readDuration := time.Since(readStart)

	if result.n > 0 {
		state.totalBytes += result.n
		state.readCount++
		state.lastReadTime = time.Now()

		rlog.Debug("stream read success",
			"read_num", state.readCount,
			"bytes", result.n,
			"duration_ms", readDuration.Milliseconds(),
			"total_bytes", state.totalBytes)

		if _, err := w.Write(state.buf[:result.n]); err != nil {
			rlog.Error("failed to write response", "error", err)
			return true, fmt.Errorf("failed to write response to client: %w", err)
		}
		if state.canFlush {
			state.flusher.Flush()
		}
	} else if result.n == 0 && result.err == nil {
		rlog.Debug("empty read",
			"read_num", state.readCount+1,
			"duration_ms", readDuration.Milliseconds())
	}

	if result.err != nil {
		if errors.Is(result.err, io.EOF) {
			rlog.Debug("stream ended normally",
				"total_bytes", state.totalBytes,
				"read_count", state.readCount)
			return true, nil
		}
		rlog.Error("stream read error",
			"error", result.err,
			"total_bytes", state.totalBytes,
			"read_count", state.readCount)
		return true, makeUserFriendlyError(result.err, time.Since(state.lastReadTime),
			"streaming", s.configuration.ResponseTimeout)
	}

	return false, nil
}

func (s *OllaProxyService) copyHeaders(proxyReq, originalReq *http.Request) {
	// TODO: copy only safe headers (SECURITY_JUNE2025)
	if len(originalReq.Header) > 0 {
		for k, vals := range originalReq.Header {
			if len(vals) == 1 {
				// optimisation for the common case - most headers have single values
				// using Set() avoids allocating a slice for a single value
				proxyReq.Header.Set(k, vals[0])
			} else {
				// for multi-value headers, we need to copy the entire slice
				// to avoid modifying the original request's headers
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

func (s *OllaProxyService) shouldContinueAfterClientDisconnect(bytesRead int, timeSinceLastRead time.Duration) bool {
	// we only continue streaming after client disconnect if:
	// 1. we've already sent a meaningful amount of data (worth preserving)
	// 2. the client disconnected recently (likely to reconnect)
	// this balances resource usage with improving user experience on flaky connections
	return bytesRead > OllaClientDisconnectionBytesThreshold && timeSinceLastRead < OllaClientDisconnectionTimeThreshold
}

func (s *OllaProxyService) GetStats(ctx context.Context) (ports.ProxyStats, error) {
	return s.statsCollector.GetProxyStats(), nil
}

func (s *OllaProxyService) UpdateConfig(config ports.ProxyConfiguration) {
	newConfig := &OllaConfiguration{
		ProxyPrefix:         config.GetProxyPrefix(),
		ConnectionTimeout:   config.GetConnectionTimeout(),
		ConnectionKeepAlive: config.GetConnectionKeepAlive(),
		ResponseTimeout:     config.GetResponseTimeout(),
		ReadTimeout:         config.GetReadTimeout(),
		StreamBufferSize:    config.GetStreamBufferSize(),
		// preserve our carefully tuned connection settings that aren't exposed
		// in the standard configuration interface to maintain performance
		MaxIdleConns:    s.configuration.MaxIdleConns,
		IdleConnTimeout: s.configuration.IdleConnTimeout,
		MaxConnsPerHost: s.configuration.MaxConnsPerHost,
	}

	// update configuration atomically to avoid race conditions
	// this ensures in-flight requests use consistent settings
	oldConfig := s.configuration
	s.configuration = newConfig

	// only update the transport if connection settings have actually changed
	// this avoids unnecessary disruption to existing connections
	if oldConfig.MaxIdleConns != newConfig.MaxIdleConns ||
		oldConfig.MaxConnsPerHost != newConfig.MaxConnsPerHost ||
		oldConfig.IdleConnTimeout != newConfig.IdleConnTimeout {

		s.transport.MaxIdleConns = newConfig.MaxIdleConns
		s.transport.MaxIdleConnsPerHost = newConfig.MaxConnsPerHost
		s.transport.IdleConnTimeout = newConfig.IdleConnTimeout
	}
}

func (s *OllaProxyService) Cleanup() {
	// close all idle connections across all endpoint pools
	// this ensures we don't leak connections when shutting down or reconfiguring
	s.endpointPools.Range(func(endpoint string, pool *connectionPool) bool {
		pool.transport.CloseIdleConnections()
		return true
	})

	// explicitly trigger garbage collection to clean up any lingering resources
	// this helps ensure a clean shutdown and memory release back to the system
	runtime.GC()
}
