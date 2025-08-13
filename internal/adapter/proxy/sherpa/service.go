package sherpa

//                                       Sherpa Proxy Implementation
//
// The Sherpa proxy implementation is a clean and pragmatic reverse proxy designed for handling AI inference workloads
// such as LLM and embedding requests. It prioritises readability, simplicity and reliability while providing essential
// support for streaming, timeout handling and observability. It was the foundation of the Sherpa AI Tooling which was
// originally based on Scout.
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
// Sherpa is *not* intended for:
// - High-throughput or low-latency scenarios where custom transports or advanced connection pooling are required
// - Complex routing or load balancing needs beyond basic endpoint selection
// - Environments where maximum performance is critical (use Olla Proxy for that)
// - Scenarios requiring advanced features like circuit breaking, rate limiting, etc.
// - Environments where the proxy itself must be highly resilient to failures (use Olla Proxy for that)

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/thushan/olla/internal/core/constants"

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

// Service implements the Sherpa proxy - optimised for simplicity and maintainability
type Service struct {
	*core.BaseProxyComponents

	transport     *http.Transport
	configuration *Configuration
	bufferPool    *pool.Pool[*[]byte]
	retryHandler  *core.RetryHandler
}

// NewService creates a new Sherpa proxy service
func NewService(
	discoveryService ports.DiscoveryService,
	selector domain.EndpointSelector,
	configuration *Configuration,
	statsCollector ports.StatsCollector,
	logger logger.StyledLogger,
) (*Service, error) {

	base := core.NewBaseProxyComponents(discoveryService, selector, statsCollector, logger)

	bufferPool, err := pool.NewLitePool(func() *[]byte {
		buf := make([]byte, configuration.GetStreamBufferSize())
		return &buf
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create buffer pool: %w", err)
	}

	// Create transport with TCP tuning for LLM streaming
	transport := &http.Transport{
		MaxIdleConns:        DefaultMaxIdleConns,
		IdleConnTimeout:     DefaultIdleConnTimeout,
		DisableCompression:  DefaultDisableCompression,
		TLSHandshakeTimeout: DefaultTLSHandshakeTimeout,
		MaxIdleConnsPerHost: DefaultMaxIdleConnsPerHost,
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			dialer := &net.Dialer{
				Timeout:   configuration.GetConnectionTimeout(),
				KeepAlive: configuration.GetConnectionKeepAlive(),
			}
			conn, err := dialer.DialContext(ctx, network, addr)
			if err != nil {
				return nil, err
			}

			// we disable Nagle's algorithm for token streaming
			if tcpConn, ok := conn.(*net.TCPConn); ok {
				if terr := tcpConn.SetNoDelay(DefaultSetNoDelay); terr != nil {
					logger.Warn("failed to set NoDelay", "err", terr)
				}
			}
			return conn, nil
		},
	}

	return &Service{
		BaseProxyComponents: base,
		transport:           transport,
		configuration:       configuration,
		bufferPool:          bufferPool,
		retryHandler:        core.NewRetryHandler(discoveryService, logger),
	}, nil
}

// ProxyRequest handles incoming HTTP requests and proxies them to healthy endpoints
func (s *Service) ProxyRequest(ctx context.Context, w http.ResponseWriter, r *http.Request, stats *ports.RequestStats, rlog logger.StyledLogger) error {
	endpoints, err := s.DiscoveryService.GetHealthyEndpoints(ctx)
	if err != nil {
		return err
	}

	return s.ProxyRequestToEndpoints(ctx, w, r, endpoints, stats, rlog)
}

// ProxyRequestToEndpoints proxies the request to the provided endpoints
func (s *Service) ProxyRequestToEndpoints(ctx context.Context, w http.ResponseWriter, r *http.Request, endpoints []*domain.Endpoint, stats *ports.RequestStats, rlog logger.StyledLogger) error {
	return s.ProxyRequestToEndpointsWithRetry(ctx, w, r, endpoints, stats, rlog)
}

// ProxyRequestToEndpointsLegacy is the original implementation without retry logic
func (s *Service) ProxyRequestToEndpointsLegacy(ctx context.Context, w http.ResponseWriter, r *http.Request, endpoints []*domain.Endpoint, stats *ports.RequestStats, rlog logger.StyledLogger) (err error) {
	// Panic recovery
	defer func() {
		if rec := recover(); rec != nil {
			s.RecordFailure(ctx, nil, time.Since(stats.StartTime), fmt.Errorf("panic: %v", rec))

			err = fmt.Errorf("proxy panic recovered after %.1fs: %v (this is a bug, please report)",
				time.Since(stats.StartTime).Seconds(), rec)
			rlog.Error("proxy request panic recovered",
				"panic", rec,
				"method", r.Method,
				"path", r.URL.Path)

			if w.Header().Get(constants.HeaderContentType) == "" {
				http.Error(w, "Internal server error", http.StatusInternalServerError)
			}
		}
	}()

	s.IncrementRequests()

	// Use context logger if available, fallback to provided logger
	ctxLogger := middleware.GetLogger(ctx)
	if ctxLogger != nil {
		ctxLogger.Debug("Sherpa proxy request started",
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

	// Select endpoint
	startSelect := time.Now()
	endpoint, err := s.Selector.Select(ctx, endpoints)
	stats.SelectionMs = time.Since(startSelect).Milliseconds()

	if err != nil {
		rlog.Error("failed to select endpoint", "error", err)
		s.RecordFailure(ctx, nil, time.Since(stats.StartTime), err)
		return fmt.Errorf("endpoint selection failed: %w", err)
	}

	stats.EndpointName = endpoint.Name

	if ctxLogger != nil {
		ctxLogger.Info("Request dispatching",
			"endpoint", endpoint.Name,
			"target", stats.TargetUrl,
			"model", stats.Model)
	} else {
		rlog.Info("Request dispatching", "endpoint", endpoint.Name, "target", stats.TargetUrl, "model", stats.Model)
	}

	s.Selector.IncrementConnections(endpoint)
	defer s.Selector.DecrementConnections(endpoint)

	// Strip route prefix from request path for upstream
	targetPath := util.StripPrefix(r.URL.Path, s.configuration.GetProxyPrefix())
	targetURL := endpoint.URL.ResolveReference(&url.URL{Path: targetPath})
	if r.URL.RawQuery != "" {
		targetURL.RawQuery = r.URL.RawQuery
	}

	stats.TargetUrl = targetURL.String()

	proxyReq, err := http.NewRequestWithContext(ctx, r.Method, targetURL.String(), r.Body)
	if err != nil {
		s.RecordFailure(ctx, endpoint, time.Since(stats.StartTime), err)
		return fmt.Errorf("failed to create proxy request: %w", err)
	}

	rlog.Debug("created proxy request")

	headerStart := time.Now()
	core.CopyHeaders(proxyReq, r)
	stats.HeaderProcessingMs = time.Since(headerStart).Milliseconds()

	// Add model header if available
	if model, ok := ctx.Value("model").(string); ok && model != "" {
		proxyReq.Header.Set("X-Model", model)
		stats.Model = model
	}

	// we mark the request processing as complete here
	stats.RequestProcessingMs = time.Since(stats.StartTime).Milliseconds()

	rlog.Debug("making round-trip request", "target", targetURL.String())
	backendStart := time.Now()
	resp, err := s.transport.RoundTrip(proxyReq)
	stats.BackendResponseMs = time.Since(backendStart).Milliseconds()

	if err != nil {
		rlog.Error("round-trip failed", "error", err)
		s.RecordFailure(ctx, endpoint, time.Since(stats.StartTime), err)
		duration := time.Since(stats.StartTime)
		return common.MakeUserFriendlyError(err, duration, "backend", s.configuration.GetResponseTimeout())
	}
	defer resp.Body.Close()

	rlog.Debug("round-trip success", "status", resp.StatusCode)

	core.SetResponseHeaders(w, stats, endpoint)

	// Copy response headers
	for key, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}

	w.WriteHeader(resp.StatusCode)

	// stream the response through
	rlog.Debug("starting response stream")
	streamStart := time.Now()
	stats.FirstDataMs = time.Since(stats.StartTime).Milliseconds()

	buffer := s.bufferPool.Get()
	defer s.bufferPool.Put(buffer)

	// Stream with timeout protection - don't let slow clients hang forever
	bytesWritten, streamErr := s.streamResponseWithTimeout(ctx, ctx, w, resp, *buffer, rlog)
	stats.StreamingMs = time.Since(streamStart).Milliseconds()
	stats.TotalBytes = bytesWritten

	if streamErr != nil && !errors.Is(streamErr, context.Canceled) {
		rlog.Error("streaming failed", "error", streamErr)
		s.RecordFailure(ctx, endpoint, time.Since(stats.StartTime), streamErr)
		return common.MakeUserFriendlyError(streamErr, time.Since(stats.StartTime), "streaming", s.configuration.GetResponseTimeout())
	}

	// we've successfully written the response
	duration := time.Since(stats.StartTime)
	s.RecordSuccess(endpoint, duration.Milliseconds(), int64(bytesWritten))

	s.PublishEvent(core.ProxyEvent{
		Type:      core.EventTypeProxySuccess,
		RequestID: stats.RequestID,
		Endpoint:  endpoint.Name,
		Duration:  duration,
		Metadata: core.ProxyEventMetadata{
			BytesSent:  int64(bytesWritten),
			StatusCode: resp.StatusCode,
			Model:      stats.Model,
		},
	})

	// stats update
	stats.EndTime = time.Now()
	stats.Latency = stats.EndTime.Sub(stats.StartTime).Milliseconds()
	stats.TotalBytes = bytesWritten

	// Log detailed completion metrics at Debug level to reduce redundancy
	if ctxLogger != nil {
		ctxLogger.Debug("Sherpa proxy metrics",
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
	}

	s.configuration = newConfig
}

// Cleanup gracefully shuts down the proxy service
func (s *Service) Cleanup() {
	// Close idle connections in the transport
	if s.transport != nil {
		s.transport.CloseIdleConnections()
	}

	// Shutdown base components (EventBus, etc)
	s.BaseProxyComponents.Shutdown()

	s.Logger.Debug("Sherpa proxy service cleaned up")
}
