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
}

// NewService creates a new Sherpa proxy service
func NewService(
	discoveryService ports.DiscoveryService,
	selector domain.EndpointSelector,
	configuration *Configuration,
	statsCollector ports.StatsCollector,
	metricsExtractor ports.MetricsExtractor,
	logger logger.StyledLogger,
) (*Service, error) {

	base := core.NewBaseProxyComponents(discoveryService, selector, statsCollector, metricsExtractor, logger)

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
func (s *Service) ProxyRequestToEndpoints(ctx context.Context, w http.ResponseWriter, r *http.Request, endpoints []*domain.Endpoint, stats *ports.RequestStats, rlog logger.StyledLogger) (err error) {
	// Setup panic recovery
	defer s.handlePanic(&err, stats, w, r, rlog)

	s.IncrementRequests()

	// Validate endpoints availability
	if valErr := s.validateEndpoints(ctx, endpoints, stats, rlog); valErr != nil {
		return valErr
	}

	// Select endpoint
	endpoint, err := s.selectEndpoint(ctx, endpoints, stats, rlog)
	if err != nil {
		return err
	}

	s.Selector.IncrementConnections(endpoint)
	defer s.Selector.DecrementConnections(endpoint)

	// Create and execute proxy request
	proxyReq, targetURL, err := s.createProxyRequest(ctx, r, endpoint, stats, rlog)
	if err != nil {
		return err
	}

	// Execute backend request
	resp, err := s.executeBackendRequest(ctx, proxyReq, targetURL, endpoint, stats, rlog)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Handle response
	return s.handleResponse(ctx, w, resp, endpoint, stats, rlog)
}

// handlePanic recovers from panics during request processing
func (s *Service) handlePanic(err *error, stats *ports.RequestStats, w http.ResponseWriter, r *http.Request, rlog logger.StyledLogger) {
	if rec := recover(); rec != nil {
		s.RecordFailure(context.Background(), nil, time.Since(stats.StartTime), fmt.Errorf("panic: %v", rec))

		*err = fmt.Errorf("proxy panic recovered after %.1fs: %v (this is a bug, please report)",
			time.Since(stats.StartTime).Seconds(), rec)
		rlog.Error("proxy request panic recovered",
			"panic", rec,
			"method", r.Method,
			"path", r.URL.Path)

		if w.Header().Get(constants.HeaderContentType) == "" {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
		}
	}
}

// validateEndpoints checks if endpoints are available
func (s *Service) validateEndpoints(ctx context.Context, endpoints []*domain.Endpoint, stats *ports.RequestStats, rlog logger.StyledLogger) error {
	ctxLogger := middleware.GetLogger(ctx)
	if ctxLogger != nil {
		ctxLogger.Debug("Sherpa proxy request started",
			"endpoint_count", len(endpoints))
	} else {
		rlog.Debug("proxy request started", "endpoint_count", len(endpoints))
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

	return nil
}

// selectEndpoint selects an endpoint from the available list
func (s *Service) selectEndpoint(ctx context.Context, endpoints []*domain.Endpoint, stats *ports.RequestStats, rlog logger.StyledLogger) (*domain.Endpoint, error) {
	startSelect := time.Now()
	endpoint, err := s.Selector.Select(ctx, endpoints)
	stats.SelectionMs = time.Since(startSelect).Milliseconds()

	if err != nil {
		rlog.Error("failed to select endpoint", "error", err)
		s.RecordFailure(ctx, nil, time.Since(stats.StartTime), err)
		return nil, fmt.Errorf("endpoint selection failed: %w", err)
	}

	stats.EndpointName = endpoint.Name

	ctxLogger := middleware.GetLogger(ctx)
	if ctxLogger != nil {
		ctxLogger.Info("Request dispatching",
			"endpoint", endpoint.Name,
			"target", stats.TargetUrl,
			"model", stats.Model)
	} else {
		rlog.Info("Request dispatching", "endpoint", endpoint.Name, "target", stats.TargetUrl, "model", stats.Model)
	}

	return endpoint, nil
}

// createProxyRequest creates the proxy request
func (s *Service) createProxyRequest(ctx context.Context, r *http.Request, endpoint *domain.Endpoint, stats *ports.RequestStats, rlog logger.StyledLogger) (*http.Request, *url.URL, error) {
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
		return nil, nil, fmt.Errorf("failed to create proxy request: %w", err)
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

	// Mark request processing as complete
	stats.RequestProcessingMs = time.Since(stats.StartTime).Milliseconds()

	return proxyReq, targetURL, nil
}

// executeBackendRequest executes the request to the backend
func (s *Service) executeBackendRequest(ctx context.Context, proxyReq *http.Request, targetURL *url.URL, endpoint *domain.Endpoint, stats *ports.RequestStats, rlog logger.StyledLogger) (*http.Response, error) {
	rlog.Debug("making round-trip request", "target", targetURL.String())
	backendStart := time.Now()
	resp, err := s.transport.RoundTrip(proxyReq)
	stats.BackendResponseMs = time.Since(backendStart).Milliseconds()

	if err != nil {
		rlog.Error("round-trip failed", "error", err)
		s.RecordFailure(ctx, endpoint, time.Since(stats.StartTime), err)
		duration := time.Since(stats.StartTime)
		return nil, common.MakeUserFriendlyError(err, duration, "backend", s.configuration.GetResponseTimeout())
	}

	rlog.Debug("round-trip success", "status", resp.StatusCode)
	return resp, nil
}

// handleResponse handles the response from the backend
func (s *Service) handleResponse(ctx context.Context, w http.ResponseWriter, resp *http.Response, endpoint *domain.Endpoint, stats *ports.RequestStats, rlog logger.StyledLogger) error {
	core.SetResponseHeaders(w, stats, endpoint)

	// Copy response headers
	for key, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}

	w.WriteHeader(resp.StatusCode)

	// Stream the response
	bytesWritten, lastChunk, err := s.streamResponse(ctx, w, resp, endpoint, stats, rlog)
	if err != nil {
		return err
	}

	// Record success
	s.recordRequestSuccess(ctx, endpoint, resp.StatusCode, bytesWritten, lastChunk, stats, rlog)

	return nil
}

// streamResponse streams the response body to the client
func (s *Service) streamResponse(ctx context.Context, w http.ResponseWriter, resp *http.Response, endpoint *domain.Endpoint, stats *ports.RequestStats, rlog logger.StyledLogger) (int, []byte, error) {
	rlog.Debug("starting response stream")
	streamStart := time.Now()
	stats.FirstDataMs = time.Since(stats.StartTime).Milliseconds()

	buffer := s.bufferPool.Get()
	defer s.bufferPool.Put(buffer)

	// Stream with timeout protection
	bytesWritten, lastChunk, streamErr := s.streamResponseWithTimeout(ctx, ctx, w, resp, *buffer, rlog)
	stats.StreamingMs = time.Since(streamStart).Milliseconds()
	stats.TotalBytes = bytesWritten

	if streamErr != nil && !errors.Is(streamErr, context.Canceled) {
		rlog.Error("streaming failed", "error", streamErr)
		s.RecordFailure(ctx, endpoint, time.Since(stats.StartTime), streamErr)
		return 0, nil, common.MakeUserFriendlyError(streamErr, time.Since(stats.StartTime), "streaming", s.configuration.GetResponseTimeout())
	}

	return bytesWritten, lastChunk, nil
}

// recordRequestSuccess records successful request completion
func (s *Service) recordRequestSuccess(ctx context.Context, endpoint *domain.Endpoint, statusCode int, bytesWritten int, lastChunk []byte, stats *ports.RequestStats, rlog logger.StyledLogger) {
	duration := time.Since(stats.StartTime)
	s.RecordSuccess(endpoint, duration.Milliseconds(), int64(bytesWritten))

	s.PublishEvent(core.ProxyEvent{
		Type:      core.EventTypeProxySuccess,
		RequestID: stats.RequestID,
		Endpoint:  endpoint.Name,
		Duration:  duration,
		Metadata: core.ProxyEventMetadata{
			BytesSent:  int64(bytesWritten),
			StatusCode: statusCode,
			Model:      stats.Model,
		},
	})

	// Update stats
	stats.EndTime = time.Now()
	stats.Latency = stats.EndTime.Sub(stats.StartTime).Milliseconds()
	stats.TotalBytes = bytesWritten

	// Extract metrics from response if available
	if s.MetricsExtractor != nil && len(lastChunk) > 0 && endpoint != nil && endpoint.Type != "" {
		rlog.Debug("Attempting metrics extraction", 
			"extractor_available", s.MetricsExtractor != nil,
			"chunk_size", len(lastChunk),
			"endpoint_type", endpoint.Type)
		stats.ProviderMetrics = s.MetricsExtractor.ExtractFromChunk(ctx, lastChunk, endpoint.Type)
		if stats.ProviderMetrics != nil {
			rlog.Debug("Metrics extracted successfully",
				"input_tokens", stats.ProviderMetrics.InputTokens,
				"output_tokens", stats.ProviderMetrics.OutputTokens)
		} else {
			rlog.Debug("No metrics extracted from response")
		}
	} else {
		rlog.Debug("Metrics extraction skipped",
			"extractor", s.MetricsExtractor != nil,
			"chunk_size", len(lastChunk),
			"has_endpoint", endpoint != nil,
			"endpoint_type", endpoint.Type)
	}

	// Log completion metrics
	s.logCompletionMetrics(ctx, endpoint, statusCode, stats, rlog)
}

// logCompletionMetrics logs detailed completion metrics
func (s *Service) logCompletionMetrics(ctx context.Context, endpoint *domain.Endpoint, statusCode int, stats *ports.RequestStats, rlog logger.StyledLogger) {
	logFields := []interface{}{
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
		"status", statusCode,
	}

	// Add provider metrics if available
	if stats.ProviderMetrics != nil {
		logFields = s.appendProviderMetrics(logFields, stats.ProviderMetrics)
	}

	ctxLogger := middleware.GetLogger(ctx)
	if ctxLogger != nil {
		logFields = append(logFields, "request_id", middleware.GetRequestID(ctx))
		ctxLogger.Debug("Sherpa proxy metrics", logFields...)
	} else {
		rlog.Debug("proxy request completed", logFields...)
	}
}

// appendProviderMetrics appends provider metrics to log fields
func (s *Service) appendProviderMetrics(logFields []interface{}, pm *domain.ProviderMetrics) []interface{} {
	if pm.InputTokens > 0 {
		logFields = append(logFields, "input_tokens", pm.InputTokens)
	}
	if pm.OutputTokens > 0 {
		logFields = append(logFields, "output_tokens", pm.OutputTokens)
	}
	if pm.TokensPerSecond > 0 {
		logFields = append(logFields, "tokens_per_sec", fmt.Sprintf("%.1f", pm.TokensPerSecond))
	}
	if pm.TTFTMs > 0 {
		logFields = append(logFields, "ttft_ms", pm.TTFTMs)
	}
	return logFields
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
