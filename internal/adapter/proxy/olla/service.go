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
	"sync"
	"sync/atomic"
	"time"

	"github.com/thushan/olla/internal/core/constants"

	"github.com/puzpuzpuz/xsync/v4"

	"github.com/thushan/olla/internal/adapter/health"
	proxyconfig "github.com/thushan/olla/internal/adapter/proxy/config"
	"github.com/thushan/olla/internal/adapter/proxy/core"
	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/core/ports"
	"github.com/thushan/olla/internal/logger"
	"github.com/thushan/olla/pkg/pool"
)

const (
	// Olla-specific constants (others are in proxy package)
	DefaultTLSHandshakeTimeout = 10 * time.Second
	DefaultSetNoDelay          = true

	ClientDisconnectionBytesThreshold = 1024
	ClientDisconnectionTimeThreshold  = 5 * time.Second

	// Circuit breaker threshold higher than health checker for tolerance
	circuitBreakerThreshold = 5 // vs health.DefaultCircuitBreakerThreshold (3)

	// halfOpenStaleness bounds how long a single in-flight half-open probe can
	// gate out further probes. Matches health.CircuitBreaker's hardcoded 1s.
	halfOpenStaleness = time.Second
)

// Service implements the Olla proxy - optimised for high performance and resilience
type Service struct {
	*core.BaseProxyComponents

	// Buffer pool for streaming
	bufferPool *pool.Pool[*[]byte]

	transport     *http.Transport
	configuration atomic.Pointer[Configuration]
	retryHandler  *core.RetryHandler

	cleanupTicker *time.Ticker
	cleanupStop   chan struct{}

	// Per-endpoint connection pools and circuit breakers
	endpointPools   xsync.Map[string, *connectionPool]
	circuitBreakers xsync.Map[string, *circuitBreaker]

	// Cleanup management
	cleanupOnce sync.Once
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
	lastAttempt int64 // atomic: half-open single-flight gate (unix nanos), reset by RecordSuccess/RecordFailure
	state       int64 // atomic: 0=closed, 1=open, 2=half-open
	threshold   int64
}

// NewService creates a new Olla proxy service
func NewService(
	discoveryService ports.DiscoveryService,
	selector domain.EndpointSelector,
	configuration *Configuration,
	statsCollector ports.StatsCollector,
	metricsExtractor ports.MetricsExtractor,
	logger logger.StyledLogger,
) (*Service, error) {

	if configuration.StreamBufferSize == 0 {
		configuration.StreamBufferSize = proxyconfig.OllaDefaultStreamBufferSize
	}
	if configuration.MaxIdleConns == 0 {
		configuration.MaxIdleConns = proxyconfig.OllaDefaultMaxIdleConns
	}
	if configuration.MaxConnsPerHost == 0 {
		configuration.MaxConnsPerHost = proxyconfig.OllaDefaultMaxConnsPerHost
	}
	if configuration.MaxIdleConnsPerHost == 0 {
		configuration.MaxIdleConnsPerHost = proxyconfig.OllaDefaultMaxIdleConnsPerHost
	}
	if configuration.IdleConnTimeout == 0 {
		configuration.IdleConnTimeout = proxyconfig.OllaDefaultIdleConnTimeout
	}
	if configuration.ReadTimeout == 0 {
		configuration.ReadTimeout = proxyconfig.OllaDefaultReadTimeout
	}

	base := core.NewBaseProxyComponents(discoveryService, selector, statsCollector, metricsExtractor, logger)

	bufferPool, err := pool.NewLitePool(func() *[]byte {
		buf := make([]byte, configuration.StreamBufferSize)
		return &buf
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create buffer pool: %w", err)
	}

	transport := createOptimisedTransport(configuration)

	service := &Service{
		BaseProxyComponents: base,
		bufferPool:          bufferPool,
		transport:           transport,
		retryHandler:        core.NewRetryHandler(discoveryService, logger),
		circuitBreakers:     *xsync.NewMap[string, *circuitBreaker](),
		endpointPools:       *xsync.NewMap[string, *connectionPool](),
		cleanupTicker:       time.NewTicker(5 * time.Minute),
		cleanupStop:         make(chan struct{}),
	}
	service.configuration.Store(configuration)

	// Start cleanup goroutine
	go service.cleanupLoop()

	return service, nil
}

// createOptimisedTransport creates an HTTP transport optimised for AI workloads
func createOptimisedTransport(config *Configuration) *http.Transport {
	return &http.Transport{
		MaxIdleConns:          config.MaxIdleConns,
		MaxIdleConnsPerHost:   config.MaxIdleConnsPerHost,
		MaxConnsPerHost:       config.MaxConnsPerHost,
		IdleConnTimeout:       config.IdleConnTimeout,
		TLSHandshakeTimeout:   config.GetTLSHandshakeTimeout(),
		DisableCompression:    true,
		ForceAttemptHTTP2:     true,
		ResponseHeaderTimeout: config.GetResponseHeaderTimeout(),
		// Olla targets local inference backends; outbound proxy env vars are not
		// honoured here because they would route credentialled requests through an
		// intermediary on plain HTTP. Health probes (no credentials) keep the proxy
		// so corporate monitoring infra still works for connectivity checks.
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

// Configuration returns the effective, fully-defaulted configuration currently in
// use. Exposed for diagnostics and for tests that verify factory-supplied defaults
// actually reach the engine rather than getting overwritten upstream.
func (s *Service) Configuration() *Configuration {
	return s.configuration.Load()
}

// getOrCreateEndpointPool returns a connection pool for the endpoint.
// LoadOrCompute guarantees the transport is constructed at most once per endpoint key,
// preventing wasted allocations when multiple goroutines race on first use.
func (s *Service) getOrCreateEndpointPool(endpoint string) *connectionPool {
	cfg := s.configuration.Load()
	pool, _ := s.endpointPools.LoadOrCompute(endpoint, func() (*connectionPool, bool) {
		return &connectionPool{
			transport: createOptimisedTransport(cfg),
			lastUsed:  time.Now().UnixNano(),
			healthy:   1,
		}, false
	})
	atomic.StoreInt64(&pool.lastUsed, time.Now().UnixNano())
	return pool
}

// GetCircuitBreaker returns the circuit breaker for an endpoint (exported for testing).
// LoadOrCompute guarantees exactly one circuitBreaker is created per endpoint even
// under concurrent first-use, avoiding a redundant allocation race.
func (s *Service) GetCircuitBreaker(endpoint string) *circuitBreaker {
	cb, _ := s.circuitBreakers.LoadOrCompute(endpoint, func() (*circuitBreaker, bool) {
		return &circuitBreaker{
			threshold: circuitBreakerThreshold,
			state:     0, // closed
		}, false
	})
	return cb
}

// Circuit breaker methods
func (cb *circuitBreaker) IsOpen() bool {
	state := atomic.LoadInt64(&cb.state)
	if state == 0 {
		return false
	}

	if state == 1 {
		// Check if the recovery timeout has passed
		lastFailure := atomic.LoadInt64(&cb.lastFailure)
		if time.Since(time.Unix(0, lastFailure)) <= health.DefaultCircuitBreakerTimeout {
			return true
		}
		// Timeout elapsed: attempt Open -> Half-open. Every caller that reaches
		// here (whether it won this CAS or another goroutine already flipped it)
		// falls through to the half-open single-flight gate below.
		atomic.CompareAndSwapInt64(&cb.state, 1, 2)
	}

	// Half-open: admit exactly one probe. lastAttempt is the single-flight gate -
	// only the goroutine that wins the 0->now CAS is let through immediately;
	// every other concurrent request is rejected until RecordSuccess/RecordFailure
	// resolves the probe and resets it. If the outstanding probe is stale (older
	// than halfOpenStaleness - e.g. a hung request that never resolved), a
	// further caller is admitted too rather than being wedged for however long
	// the hang lasts. Mirrors health.CircuitBreaker's lastAttempt gate.
	if atomic.CompareAndSwapInt64(&cb.lastAttempt, 0, time.Now().UnixNano()) {
		return false
	}
	lastAttempt := atomic.LoadInt64(&cb.lastAttempt)
	return time.Since(time.Unix(0, lastAttempt)) < halfOpenStaleness
}

func (cb *circuitBreaker) RecordSuccess() {
	atomic.StoreInt64(&cb.failures, 0)
	atomic.StoreInt64(&cb.lastAttempt, 0)
	atomic.StoreInt64(&cb.state, 0) // closed
}

func (cb *circuitBreaker) RecordFailure() {
	failures := atomic.AddInt64(&cb.failures, 1)
	atomic.StoreInt64(&cb.lastFailure, time.Now().UnixNano())
	atomic.StoreInt64(&cb.lastAttempt, 0)

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

// prepareProxyRequest creates and prepares the proxy request with headers.
// endpoint is passed through so CopyHeaders can apply per-endpoint auth and custom headers.
func (s *Service) prepareProxyRequest(ctx context.Context, r *http.Request, targetURL *url.URL, endpoint *domain.Endpoint, stats *ports.RequestStats) (*http.Request, error) {
	proxyReq, err := http.NewRequestWithContext(ctx, r.Method, targetURL.String(), r.Body)
	if err != nil {
		return nil, err
	}

	// Copy headers
	headerStart := time.Now()
	core.CopyHeaders(proxyReq, r, endpoint)
	stats.HeaderProcessingMs = time.Since(headerStart).Milliseconds()

	// Add model header
	if model, ok := ctx.Value(constants.ContextModelKey).(string); ok && model != "" {
		proxyReq.Header.Set(constants.HeaderXModel, model)
		stats.Model = model
	}

	// Mark request processing complete
	stats.RequestProcessingMs = time.Since(stats.StartTime).Milliseconds()

	return proxyReq, nil
}

// streamResponse performs buffered streaming with backpressure handling
func (s *Service) streamResponse(clientCtx, upstreamCtx context.Context, w http.ResponseWriter, resp *http.Response, buffer []byte, rlog logger.StyledLogger) (int, []byte, error) {
	state := &streamState{}
	// Snapshot configuration once for the lifetime of this stream so all reads
	// within the loop see a coherent config even if UpdateConfig runs concurrently.
	cfg := s.configuration.Load()
	readTimeout := cfg.GetReadTimeout()

	// Use http.ResponseController for modern flush handling (Go 1.20+)
	// Provides better error handling and cleaner API than type assertion
	rc := http.NewResponseController(w)
	isStreaming := core.AutoDetectStreamingMode(clientCtx, resp, cfg.GetProxyProfile())

	// Pre-allocate timer to avoid allocations in hot path
	readDeadline := time.NewTimer(readTimeout)
	defer readDeadline.Stop()

	for {
		// Check for context cancellation
		if err := s.checkContexts(clientCtx, upstreamCtx, readDeadline, readTimeout, state, rlog); err != nil {
			return state.totalBytes, state.lastChunk, err
		}

		// Reset timer for next read (drain if already fired to prevent race)
		// this only applies to Olla as Sherpa has a different streaming model
		// which creates a new timer, instead of resetting the existing one
		if !readDeadline.Stop() {
			// Timer already expired, drain the channel
			select {
			case <-readDeadline.C:
			default:
			}
		}
		readDeadline.Reset(readTimeout)

		// Read and process data
		if err := s.processStreamData(resp, buffer, state, w, isStreaming, rc, rlog); err != nil {
			if errors.Is(err, io.EOF) {
				return state.totalBytes, state.lastChunk, nil
			}
			rlog.Debug("read error during streaming", "error", err, "bytes_read", state.totalBytes)
			return state.totalBytes, state.lastChunk, err
		}
	}
}

// GetStats returns current proxy statistics
func (s *Service) GetStats(ctx context.Context) (ports.ProxyStats, error) {
	return s.GetProxyStats(), nil
}

// UpdateConfig updates the proxy configuration.
// The swap is atomic so in-flight requests always read a complete, consistent
// snapshot — never a partially-written config.
func (s *Service) UpdateConfig(config ports.ProxyConfiguration) {
	newConfig := &Configuration{}
	newConfig.ProxyPrefix = config.GetProxyPrefix()
	newConfig.ConnectionTimeout = config.GetConnectionTimeout()
	newConfig.ConnectionKeepAlive = config.GetConnectionKeepAlive()
	newConfig.ResponseTimeout = config.GetResponseTimeout()
	// Getter-based defaults, same as every other field above. Overridden below
	// with the raw (possibly zero) value when config is concretely an
	// *olla.Configuration, so an unset field there resolves through Olla's own
	// default instead of getting permanently stuck on whatever this generic
	// getter defaulted to - the same class of bug factory.go's raw copy fixes
	// for construction (F1). A foreign config type has no way to expose an
	// undefaulted value, so it keeps using the getter here.
	newConfig.ReadTimeout = config.GetReadTimeout()
	newConfig.StreamBufferSize = config.GetStreamBufferSize()
	newConfig.Profile = config.GetProxyProfile()

	// Snapshot the current config once before deciding what to preserve.
	// This single Load ensures the fallback branch reads a coherent value
	// even if another UpdateConfig is racing concurrently.
	current := s.configuration.Load()

	// we try to get Olla-specific fields from incoming config if it's an *olla.Configuration
	if ollaConfig, ok := config.(*Configuration); ok && ollaConfig != nil {
		newConfig.ReadTimeout = ollaConfig.ReadTimeout
		newConfig.StreamBufferSize = ollaConfig.StreamBufferSize
		newConfig.MaxIdleConns = ollaConfig.MaxIdleConns
		newConfig.IdleConnTimeout = ollaConfig.IdleConnTimeout
		newConfig.MaxConnsPerHost = ollaConfig.MaxConnsPerHost
		newConfig.MaxIdleConnsPerHost = ollaConfig.MaxIdleConnsPerHost
		newConfig.ResponseHeaderTimeout = ollaConfig.ResponseHeaderTimeout
		newConfig.TLSHandshakeTimeout = ollaConfig.TLSHandshakeTimeout
	} else if current != nil {
		// fallback: preserve current Olla-specific pool tunables for non-Olla
		// configs. Guard against a nil current pointer — only reachable if
		// UpdateConfig is called on a zero-value Service (e.g. in tests)
		// before NewService stores the initial configuration.
		newConfig.MaxIdleConns = current.MaxIdleConns
		newConfig.IdleConnTimeout = current.IdleConnTimeout
		newConfig.MaxConnsPerHost = current.MaxConnsPerHost
		newConfig.MaxIdleConnsPerHost = current.MaxIdleConnsPerHost
		newConfig.ResponseHeaderTimeout = current.ResponseHeaderTimeout
		newConfig.TLSHandshakeTimeout = current.TLSHandshakeTimeout
	}

	s.configuration.Store(newConfig)
}

// cleanupLoop periodically cleans up unused endpoint pools and circuit breakers.
func (s *Service) cleanupLoop() {
	// Function-level safety net in case something escapes the per-tick recover.
	defer func() {
		if r := recover(); r != nil {
			s.Logger.Error("cleanupLoop exited unexpectedly", "panic", r)
		}
	}()

	for {
		select {
		case <-s.cleanupStop:
			return
		case <-s.cleanupTicker.C:
			// Wrap per-tick work so a panic in cleanupUnusedResources does not
			// kill the goroutine — the loop continues and cleans up next tick.
			func() {
				defer func() {
					if r := recover(); r != nil {
						s.Logger.Error("cleanupLoop tick panic recovered, loop continues",
							"panic", r)
					}
				}()
				s.cleanupUnusedResources()
			}()
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

// Cleanup cleans up resources. Safe to call more than once.
func (s *Service) Cleanup() {
	s.cleanupOnce.Do(func() {
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

		s.Logger.Debug("Olla proxy service cleaned up")
	})
}
