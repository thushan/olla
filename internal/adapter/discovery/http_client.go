package discovery

import (
	"context"
	"errors"
	"fmt"
	"github.com/thushan/olla/internal/version"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/thushan/olla/internal/adapter/registry/profile"
	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/logger"
)

const (
	DefaultTimeout     = 30 * time.Second
	MaxResponseSize    = 10 * 1024 * 1024 // 10MB limit for model responses
	DefaultUserAgent   = "%s-Discovery/%s"
	DefaultContentType = "application/json"

	DefaultMaxIdleConnections        = 10
	DefaultIdleConnTimeout           = 60 * time.Second
	DefaultDisableCompression        = false
	DefaultMaxIdleConnectionsPerHost = 5
)

// HTTPModelDiscoveryClient implements ModelDiscoveryClient using HTTP requests
type HTTPModelDiscoveryClient struct {
	httpClient     *http.Client
	profileFactory *profile.Factory
	parser         *ResponseParser
	logger         *logger.StyledLogger
	metrics        DiscoveryMetrics
	mu             sync.RWMutex
}

func NewHTTPModelDiscoveryClient(profileFactory *profile.Factory, logger *logger.StyledLogger) *HTTPModelDiscoveryClient {
	return &HTTPModelDiscoveryClient{
		httpClient: &http.Client{
			Timeout: DefaultTimeout,
			Transport: &http.Transport{
				MaxIdleConns:        DefaultMaxIdleConnections,
				IdleConnTimeout:     DefaultIdleConnTimeout,
				DisableCompression:  DefaultDisableCompression,
				MaxIdleConnsPerHost: DefaultMaxIdleConnectionsPerHost,
			},
		},
		profileFactory: profileFactory,
		parser:         NewResponseParser(logger),
		logger:         logger,
		metrics: DiscoveryMetrics{
			ErrorsByEndpoint: make(map[string]int64),
		},
	}
}

func (c *HTTPModelDiscoveryClient) DiscoverModels(ctx context.Context, endpoint *domain.Endpoint) ([]*domain.ModelInfo, error) {
	startTime := time.Now()

	c.updateMetrics(func(m *DiscoveryMetrics) {
		atomic.AddInt64(&m.TotalDiscoveries, 1)
	})

	profileType := endpoint.Type
	if profileType == "" || profileType == domain.ProfileAuto {
		return c.discoverWithAutoDetection(ctx, endpoint, startTime)
	}

	platformProfile, err := c.profileFactory.GetProfile(profileType)
	if err != nil {
		c.recordError(endpoint.URLString, err)
		return nil, NewDiscoveryError(endpoint.URLString, profileType, "get_profile", 0, time.Since(startTime), err)
	}

	return c.discoverWithProfile(ctx, endpoint, platformProfile, startTime)
}

// discoverWithAutoDetection tries profiles in order until one succeeds
func (c *HTTPModelDiscoveryClient) discoverWithAutoDetection(ctx context.Context, endpoint *domain.Endpoint, startTime time.Time) ([]*domain.ModelInfo, error) {

	// We're going to try profiles in order:
	//	Ollama → LM Studio → OpenAI Compatible
	// Resolution for this may change in the future as more front-ends appear or are added.
	profileTypes := []string{
		domain.ProfileOllama,
		domain.ProfileLmStudio,
		domain.ProfileOpenAICompatible, /* last ditch effort */
	}

	var lastErr error
	for _, profileType := range profileTypes {
		platformProfile, err := c.profileFactory.GetProfile(profileType)
		if err != nil {
			lastErr = err
			continue
		}

		models, err := c.discoverWithProfile(ctx, endpoint, platformProfile, startTime)
		if err == nil {
			c.logger.InfoWithEndpoint(" Setting up", endpoint.Name, "profile", platformProfile.GetName())
			return models, nil
		}

		// Continue to next profile unless it's a non-recoverable parsing error
		var discErr *DiscoveryError
		if errors.As(err, &discErr) {
			// Stop only on parse errors, continue on HTTP errors (different endpoints)
			// we can't really do much if there's parsing errors
			var parseError *ParseError
			if errors.As(discErr.Err, &parseError) {
				lastErr = err
				break
			}
		}
		lastErr = err
	}

	c.recordError(endpoint.URLString, lastErr)
	return nil, NewDiscoveryError(endpoint.URLString, domain.ProfileAuto, "auto_detect", 0, time.Since(startTime), lastErr)
}

// discoverWithProfile performs discovery using a specific platform profile
func (c *HTTPModelDiscoveryClient) discoverWithProfile(ctx context.Context, endpoint *domain.Endpoint, platformProfile domain.PlatformProfile, startTime time.Time) ([]*domain.ModelInfo, error) {
	discoveryURL := platformProfile.GetModelDiscoveryURL(endpoint.URLString)

	req, err := http.NewRequestWithContext(ctx, "GET", discoveryURL, nil)
	if err != nil {
		return nil, NewDiscoveryError(endpoint.URLString, platformProfile.GetName(), "create_request", 0, time.Since(startTime), err)
	}
	req.Header.Set("User-Agent", fmt.Sprintf(DefaultUserAgent, version.ShortName, version.Version))
	req.Header.Set("Accept", DefaultContentType)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		networkErr := &NetworkError{URL: discoveryURL, Err: err}
		return nil, NewDiscoveryError(endpoint.URLString, platformProfile.GetName(), "http_request", 0, time.Since(startTime), networkErr)
	}
	defer func(Body io.ReadCloser) {
		// dont care about errors
		_ = Body.Close()
	}(resp.Body)

	duration := time.Since(startTime)

	if resp.StatusCode != http.StatusOK {
		err := fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
		return nil, NewDiscoveryError(endpoint.URLString, platformProfile.GetName(), "http_status", resp.StatusCode, duration, err)
	}

	// Limit response size to prevent memory issues
	limitedReader := io.LimitReader(resp.Body, MaxResponseSize)
	body, err := io.ReadAll(limitedReader)
	if err != nil {
		return nil, NewDiscoveryError(endpoint.URLString, platformProfile.GetName(), "read_response", resp.StatusCode, duration, err)
	}

	models, err := c.parser.ParseModelsResponse(body, platformProfile.GetModelResponseFormat())
	if err != nil {
		return nil, NewDiscoveryError(endpoint.URLString, platformProfile.GetName(), "parse_response", resp.StatusCode, duration, err)
	}

	c.updateMetrics(func(m *DiscoveryMetrics) {
		atomic.AddInt64(&m.SuccessfulRequests, 1)
		m.LastDiscoveryTime = time.Now()

		// Update average latency using running average
		if m.SuccessfulRequests == 1 {
			m.AverageLatency = duration
		} else {
			// simplest one for running average for now
			// TODO: Improve this to use a more robust running average algorithm
			m.AverageLatency = time.Duration((int64(m.AverageLatency) + int64(duration)) / 2)
		}
	})

	return models, nil
}

func (c *HTTPModelDiscoveryClient) HealthCheck(ctx context.Context, endpoint *domain.Endpoint) error {
	// Use existing health check URL from endpoint
	req, err := http.NewRequestWithContext(ctx, "GET", endpoint.HealthCheckURLString, nil)
	if err != nil {
		return err
	}

	req.Header.Set("User-Agent", fmt.Sprintf(DefaultUserAgent, version.ShortName, version.Version))

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return &NetworkError{URL: endpoint.HealthCheckURLString, Err: err}
	}
	defer func(Body io.ReadCloser) {
		// dont care about errors here
		_ = Body.Close()
	}(resp.Body)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("health check failed with status %d", resp.StatusCode)
	}

	return nil
}

func (c *HTTPModelDiscoveryClient) GetMetrics() DiscoveryMetrics {
	c.mu.RLock()
	defer c.mu.RUnlock()

	errorsByEndpoint := make(map[string]int64, len(c.metrics.ErrorsByEndpoint))
	for k, v := range c.metrics.ErrorsByEndpoint {
		errorsByEndpoint[k] = v
	}

	return DiscoveryMetrics{
		TotalDiscoveries:   atomic.LoadInt64(&c.metrics.TotalDiscoveries),
		SuccessfulRequests: atomic.LoadInt64(&c.metrics.SuccessfulRequests),
		FailedRequests:     atomic.LoadInt64(&c.metrics.FailedRequests),
		AverageLatency:     c.metrics.AverageLatency,
		LastDiscoveryTime:  c.metrics.LastDiscoveryTime,
		ErrorsByEndpoint:   errorsByEndpoint,
	}
}

func (c *HTTPModelDiscoveryClient) recordError(endpointURL string, err error) {
	c.updateMetrics(func(m *DiscoveryMetrics) {
		atomic.AddInt64(&m.FailedRequests, 1)
		m.ErrorsByEndpoint[endpointURL]++
	})
}

func (c *HTTPModelDiscoveryClient) updateMetrics(updateFn func(*DiscoveryMetrics)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	updateFn(&c.metrics)
}
