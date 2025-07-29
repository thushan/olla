package sherpa

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/thushan/olla/internal/adapter/proxy/common"
	"github.com/thushan/olla/internal/adapter/proxy/core"
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

// Configuration holds proxy settings
type Configuration struct {
	ProxyPrefix         string
	ConnectionTimeout   time.Duration
	ConnectionKeepAlive time.Duration
	ResponseTimeout     time.Duration
	ReadTimeout         time.Duration
	StreamBufferSize    int
}

func (c *Configuration) GetProxyPrefix() string {
	if c.ProxyPrefix == "" {
		return "/olla"
	}
	return c.ProxyPrefix
}

func (c *Configuration) GetConnectionTimeout() time.Duration {
	if c.ConnectionTimeout == 0 {
		return DefaultTimeout
	}
	return c.ConnectionTimeout
}

func (c *Configuration) GetConnectionKeepAlive() time.Duration {
	if c.ConnectionKeepAlive == 0 {
		return DefaultKeepAlive
	}
	return c.ConnectionKeepAlive
}

func (c *Configuration) GetResponseTimeout() time.Duration {
	return c.ResponseTimeout
}

func (c *Configuration) GetReadTimeout() time.Duration {
	if c.ReadTimeout == 0 {
		return DefaultReadTimeout
	}
	return c.ReadTimeout
}

func (c *Configuration) GetStreamBufferSize() int {
	if c.StreamBufferSize == 0 {
		return DefaultStreamBufferSize
	}
	return c.StreamBufferSize
}

// NewService creates a new Sherpa proxy service
func NewService(
	discoveryService ports.DiscoveryService,
	selector domain.EndpointSelector,
	configuration *Configuration,
	statsCollector ports.StatsCollector,
	logger logger.StyledLogger,
) (*Service, error) {
	// Create base components
	base := core.NewBaseProxyComponents(discoveryService, selector, statsCollector, logger)

	// Create buffer pool using LitePool
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
			// Disable Nagle's algorithm for token streaming
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

			if w.Header().Get("Content-Type") == "" {
				http.Error(w, "Internal server error", http.StatusInternalServerError)
			}
		}
	}()

	s.IncrementRequests()

	rlog.Debug("proxy request started", "method", r.Method, "url", r.URL.String())

	if len(endpoints) == 0 {
		rlog.Error("no healthy endpoints available")
		s.RecordFailure(ctx, nil, time.Since(stats.StartTime), common.ErrNoHealthyEndpoints)
		return common.ErrNoHealthyEndpoints
	}

	rlog.Debug("using provided endpoints", "count", len(endpoints))

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

	// Log dispatch
	rlog.Info("Request dispatching to endpoint", "endpoint", endpoint.Name, "target", stats.TargetUrl, "model", stats.Model)

	// Track connections
	s.Selector.IncrementConnections(endpoint)
	defer s.Selector.DecrementConnections(endpoint)

	// Strip route prefix from request path for upstream
	targetPath := util.StripPrefix(r.URL.Path, s.configuration.GetProxyPrefix())
	targetURL := endpoint.URL.ResolveReference(&url.URL{Path: targetPath})
	if r.URL.RawQuery != "" {
		targetURL.RawQuery = r.URL.RawQuery
	}

	stats.TargetUrl = targetURL.String()

	// Create proxy request
	proxyReq, err := http.NewRequestWithContext(ctx, r.Method, targetURL.String(), r.Body)
	if err != nil {
		s.RecordFailure(ctx, endpoint, time.Since(stats.StartTime), err)
		return fmt.Errorf("failed to create proxy request: %w", err)
	}

	rlog.Debug("created proxy request")

	// Copy headers
	headerStart := time.Now()
	core.CopyHeaders(proxyReq, r)
	stats.HeaderProcessingMs = time.Since(headerStart).Milliseconds()

	// Add model header if available
	if model, ok := ctx.Value("model").(string); ok && model != "" {
		proxyReq.Header.Set("X-Model", model)
		stats.Model = model
	}

	// Mark request processing complete
	stats.RequestProcessingMs = time.Since(stats.StartTime).Milliseconds()

	// Execute request
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

	// Set response headers
	core.SetResponseHeaders(w, stats, endpoint)

	// Copy response headers
	for key, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}

	// Write status code
	w.WriteHeader(resp.StatusCode)

	// Stream response
	rlog.Debug("starting response stream")
	streamStart := time.Now()
	stats.FirstDataMs = time.Since(stats.StartTime).Milliseconds()

	buffer := s.bufferPool.Get()
	defer s.bufferPool.Put(buffer)

	// Use timeout-aware streaming for edge server reliability
	bytesWritten, streamErr := s.streamResponseWithTimeout(ctx, ctx, w, resp.Body, *buffer, rlog)
	stats.StreamingMs = time.Since(streamStart).Milliseconds()
	stats.TotalBytes = bytesWritten

	if streamErr != nil && !errors.Is(streamErr, context.Canceled) {
		rlog.Error("streaming failed", "error", streamErr)
		s.RecordFailure(ctx, endpoint, time.Since(stats.StartTime), streamErr)
		return common.MakeUserFriendlyError(streamErr, time.Since(stats.StartTime), "streaming", s.configuration.GetResponseTimeout())
	}

	// Record success
	duration := time.Since(stats.StartTime)
	s.RecordSuccess(endpoint, duration.Milliseconds(), int64(bytesWritten))

	// Publish success event
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

	// Update final stats
	stats.EndTime = time.Now()
	stats.Latency = stats.EndTime.Sub(stats.StartTime).Milliseconds()
	stats.TotalBytes = bytesWritten

	// Log detailed completion metrics
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

	return nil
}

// streamResponse is now implemented in service_streaming.go with timeout support

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
