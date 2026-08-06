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
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/thushan/olla/internal/adapter/proxy/config"
	"github.com/thushan/olla/internal/adapter/proxy/core"
	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/core/ports"
	"github.com/thushan/olla/internal/logger"
	"github.com/thushan/olla/pkg/pool"
)

const (
	// Sherpa-specific constants that aren't in the base proxy package
	DefaultSetNoDelay         = true
	DefaultDisableCompression = false

	DefaultMaxIdleConns        = 20
	DefaultMaxIdleConnsPerHost = 5

	DefaultIdleConnTimeout            = 90 * time.Second
	DefaultTLSHandshakeTimeout        = 10 * time.Second
	ClientDisconnectionBytesThreshold = 1024
	ClientDisconnectionTimeThreshold  = 5 * time.Second
)

// DefaultResponseHeaderTimeout re-exports the shared constant for Sherpa callers.
const DefaultResponseHeaderTimeout = config.DefaultResponseHeaderTimeout

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
		MaxIdleConns:          DefaultMaxIdleConns,
		IdleConnTimeout:       DefaultIdleConnTimeout,
		DisableCompression:    DefaultDisableCompression,
		TLSHandshakeTimeout:   configuration.GetTLSHandshakeTimeout(),
		MaxIdleConnsPerHost:   DefaultMaxIdleConnsPerHost,
		ResponseHeaderTimeout: configuration.GetResponseHeaderTimeout(),
		// Olla targets local inference backends; outbound proxy env vars are not
		// honoured here because they would route credentialled requests through an
		// intermediary on plain HTTP. Health probes (no credentials) keep the proxy
		// so corporate monitoring infra still works for connectivity checks.
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

	return s.ProxyRequestToEndpointsWithRetry(ctx, w, r, endpoints, stats, rlog)
}

// ProxyRequestToEndpoints proxies the request to the provided endpoints with automatic retry on connection failures
func (s *Service) ProxyRequestToEndpoints(ctx context.Context, w http.ResponseWriter, r *http.Request, endpoints []*domain.Endpoint, stats *ports.RequestStats, rlog logger.StyledLogger) error {
	return s.ProxyRequestToEndpointsWithRetry(ctx, w, r, endpoints, stats, rlog)
}

// GetStats returns current proxy statistics
func (s *Service) GetStats(ctx context.Context) (ports.ProxyStats, error) {
	return s.GetProxyStats(), nil
}

// Configuration returns the effective, fully-defaulted configuration currently in
// use. Exposed for diagnostics and for tests that verify factory-supplied defaults
// actually reach the engine rather than getting overwritten upstream.
func (s *Service) Configuration() *Configuration {
	return s.configuration
}

// UpdateConfig updates the proxy configuration
func (s *Service) UpdateConfig(config ports.ProxyConfiguration) {
	newConfig := &Configuration{}
	newConfig.ProxyPrefix = config.GetProxyPrefix()
	newConfig.ConnectionTimeout = config.GetConnectionTimeout()
	newConfig.ConnectionKeepAlive = config.GetConnectionKeepAlive()
	newConfig.ResponseTimeout = config.GetResponseTimeout()
	// Getter-based defaults for a foreign config type, same as every other
	// field above. Overridden below with the raw value when config is
	// concretely a *sherpa.Configuration, preserving the distinction between
	// "operator left this unset" and "operator set it to the default" in the
	// stored config rather than baking the getter's resolved value into the
	// raw field. Mirrors factory.go's raw copy for construction (F1) and
	// olla.Service's UpdateConfig (F12).
	newConfig.ReadTimeout = config.GetReadTimeout()
	newConfig.StreamBufferSize = config.GetStreamBufferSize()
	newConfig.Profile = config.GetProxyProfile()

	if sherpaConfig, ok := config.(*Configuration); ok && sherpaConfig != nil {
		newConfig.ReadTimeout = sherpaConfig.ReadTimeout
		newConfig.StreamBufferSize = sherpaConfig.StreamBufferSize
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
