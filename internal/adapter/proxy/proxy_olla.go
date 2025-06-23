package proxy

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/core/ports"
	"github.com/thushan/olla/internal/logger"
	"github.com/thushan/olla/internal/util"
)

type OllaProxyService struct {
	discoveryService ports.DiscoveryService
	selector         domain.EndpointSelector
	transport        *http.Transport
	configuration    *OllaConfiguration
	logger           logger.StyledLogger
}

func NewOllaService(
	discoveryService ports.DiscoveryService,
	selector domain.EndpointSelector,
	configuration *OllaConfiguration,
	statsCollector ports.StatsCollector,
	logger logger.StyledLogger,
) *OllaProxyService {
	transport := &http.Transport{}

	return &OllaProxyService{
		discoveryService: discoveryService,
		selector:         selector,
		transport:        transport,
		configuration:    configuration,
		logger:           logger,
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
func (s *OllaProxyService) ProxyRequestToEndpoints(ctx context.Context, w http.ResponseWriter, r *http.Request, endpoints []*domain.Endpoint, stats *ports.RequestStats, rlog logger.StyledLogger) error {
	if len(endpoints) == 0 {
		return fmt.Errorf("no healthy endpoints available")
	}

	endpoint, err := s.selector.Select(ctx, endpoints)
	if err != nil {
		return fmt.Errorf("failed to select endpoint: %w", err)
	}

	targetPath := s.stripRoutePrefix(r.Context(), r.URL.Path)
	targetURL := endpoint.URL.ResolveReference(&url.URL{Path: targetPath})
	if r.URL.RawQuery != "" {
		targetURL.RawQuery = r.URL.RawQuery
	}

	stats.EndpointName = endpoint.Name
	stats.TargetUrl = targetURL.String()

	rlog.Info("Proxying request", "endpoint", endpoint.Name, "target", targetURL.String())

	proxyReq, err := http.NewRequestWithContext(ctx, r.Method, targetURL.String(), r.Body)
	if err != nil {
		return fmt.Errorf("failed to create proxy request: %w", err)
	}

	s.copyHeaders(proxyReq, r)

	// Make the request
	resp, err := s.transport.RoundTrip(proxyReq)
	if err != nil {
		return fmt.Errorf("proxy request failed: %w", err)
	}
	defer resp.Body.Close()

	for k, vals := range resp.Header {
		for _, v := range vals {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)

	bytesWritten, err := io.Copy(w, resp.Body)
	if err != nil {
		return fmt.Errorf("failed to copy response: %w", err)
	}

	stats.TotalBytes = int(bytesWritten)
	rlog.Info("Request completed", "bytes", bytesWritten)

	return nil
}

func (s *OllaProxyService) stripRoutePrefix(ctx context.Context, path string) string {
	return util.StripRoutePrefix(ctx, path, s.configuration.ProxyPrefix)
}

func (s *OllaProxyService) copyHeaders(proxyReq, originalReq *http.Request) {

	for k, vals := range originalReq.Header {
		for _, v := range vals {
			proxyReq.Header.Add(k, v)
		}
	}

	proxyReq.Header.Set("X-Forwarded-Host", originalReq.Host)
	if originalReq.TLS != nil {
		proxyReq.Header.Set("X-Forwarded-Proto", "https")
	} else {
		proxyReq.Header.Set("X-Forwarded-Proto", "http")
	}
}

func (s *OllaProxyService) GetStats(ctx context.Context) (ports.ProxyStats, error) {
	// no stats tracking yet
	return ports.ProxyStats{}, nil
}

func (s *OllaProxyService) UpdateConfig(config ports.ProxyConfiguration) {
	// simple config update
	s.configuration.ProxyPrefix = config.GetProxyPrefix()
}
