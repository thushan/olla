package discovery

import (
	"context"
	"fmt"
	"io"
	"maps"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tidwall/gjson"

	"github.com/thushan/olla/internal/core/constants"

	"github.com/thushan/olla/internal/version"

	"github.com/thushan/olla/internal/adapter/factory"
	"github.com/thushan/olla/internal/adapter/registry/profile"
	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/logger"
)

const (
	DefaultTimeout     = 30 * time.Second
	MaxResponseSize    = 10 * 1024 * 1024 // 10MB limit for model responses
	DefaultUserAgent   = "%s-Discovery/%s"
	DefaultContentType = constants.DefaultContentTypeJSON

	DefaultMaxIdleConnections        = 10
	DefaultIdleConnTimeout           = 60 * time.Second
	DefaultMaxIdleConnectionsPerHost = 5
)

// HTTPModelDiscoveryClient implements ModelDiscoveryClient using HTTP requests
type HTTPModelDiscoveryClient struct {
	httpClient     *http.Client
	profileFactory *profile.Factory
	logger         logger.StyledLogger
	metrics        DiscoveryMetrics
	mu             sync.RWMutex
}

// NewHTTPModelDiscoveryClient uses httpClient verbatim. Discovery sends the
// endpoint's auth and custom headers, so production callers must build the
// client with factory.NewBackendHTTPClient (no environment proxy, no
// cross-origin redirects).
func NewHTTPModelDiscoveryClient(profileFactory *profile.Factory, logger logger.StyledLogger, httpClient *http.Client) *HTTPModelDiscoveryClient {
	return &HTTPModelDiscoveryClient{
		httpClient:     httpClient,
		profileFactory: profileFactory,
		logger:         logger,
		metrics: DiscoveryMetrics{
			ErrorsByEndpoint: make(map[string]int64),
		},
	}
}

func NewHTTPModelDiscoveryClientWithDefaults(profileFactory *profile.Factory, logger logger.StyledLogger) *HTTPModelDiscoveryClient {
	return &HTTPModelDiscoveryClient{
		httpClient: factory.NewBackendHTTPClient(factory.BackendClientOptions{
			Timeout:             DefaultTimeout,
			MaxIdleConns:        DefaultMaxIdleConnections,
			IdleConnTimeout:     DefaultIdleConnTimeout,
			MaxIdleConnsPerHost: DefaultMaxIdleConnectionsPerHost,
		}),
		profileFactory: profileFactory,
		logger:         logger,
		metrics: DiscoveryMetrics{
			ErrorsByEndpoint: make(map[string]int64),
		},
	}
}

func (c *HTTPModelDiscoveryClient) DiscoverModels(ctx context.Context, endpoint *domain.Endpoint) (models []*domain.ModelInfo, err error) {
	defer func() {
		if err != nil {
			c.recordError(endpoint.URLString)
		}
	}()
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
		return nil, NewDiscoveryError(endpoint.URLString, profileType, "get_profile", 0, time.Since(startTime), err)
	}

	return c.discoverWithProfile(ctx, endpoint, platformProfile, startTime)
}

// discoverWithAutoDetection tries profiles in order until one succeeds
func (c *HTTPModelDiscoveryClient) discoverWithAutoDetection(ctx context.Context, endpoint *domain.Endpoint, startTime time.Time) ([]*domain.ModelInfo, error) {
	// We're going to try profiles in order:
	//	Ollama → llama.cpp → LM Studio → vLLM → OpenAI Compatible (as a last resort)
	// Resolution for this may change in the future as more front-ends appear or are added.
	profileTypes := []string{
		domain.ProfileOllama,
		domain.ProfileLlamaCpp,
		domain.ProfileLmStudio,
		domain.ProfileVLLM,
		domain.ProfileRapidMLX,
		domain.ProfileOMLX,             // Apple Silicon only; tried after other common servers
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

		if ctx.Err() != nil {
			return nil, err
		}

		lastErr = err
	}

	return nil, NewDiscoveryError(endpoint.URLString, domain.ProfileAuto, "auto_detect", 0, time.Since(startTime), lastErr)
}

// discoverWithProfile performs discovery using a specific platform profile
func (c *HTTPModelDiscoveryClient) discoverWithProfile(ctx context.Context, endpoint *domain.Endpoint, platformProfile domain.PlatformProfile, startTime time.Time) ([]*domain.ModelInfo, error) {
	// For specific profiles like OpenAI-compatible, skip model_url override by default
	// to avoid breaking existing configurations that rely on profile defaults
	// Issue #86 fix applies mainly to LlamaCpp, Ollama, etc. where users need custom paths
	var discoveryURL string

	// Skip model_url override for OpenAI-compatible unless it's a custom path
	// This prevents issues with endpoints like Docker that have /models returning HTML
	if platformProfile.GetName() == domain.ProfileOpenAICompatible {
		if endpoint.ModelURLString != "" {
			// Only use override if it's not a common problematic path
			// /models often returns HTML in Docker/OpenWebUI setups, not model data
			if endpoint.ModelURLString != "/v1/models" && endpoint.ModelURLString != "/models" {
				discoveryURL = endpoint.ModelURLString
			}
			// If it's /models, use profile default (/v1/models) instead
		}
		// For all other cases, use profile default
	} else {
		// For other profiles (Ollama, LlamaCpp, etc.), respect the override
		// This fixes Issue #86 where users need custom discovery paths
		if endpoint.ModelURLString != "" {
			discoveryURL = endpoint.ModelURLString
		}
	}

	if discoveryURL == "" {
		discoveryURL = platformProfile.GetModelDiscoveryURL(endpoint.URLString)
	}

	req, err := http.NewRequestWithContext(ctx, "GET", discoveryURL, http.NoBody)
	if err != nil {
		return nil, NewDiscoveryError(endpoint.URLString, platformProfile.GetName(), "create_request", 0, time.Since(startTime), err)
	}
	req.Header.Set("User-Agent", fmt.Sprintf(DefaultUserAgent, version.ShortName, version.Version))
	req.Header.Set("Accept", DefaultContentType)

	// Apply any per-endpoint custom headers then auth, mirroring what CopyHeaders does on
	// the proxy path. Without this, authenticated backends 401 during model discovery.
	for name, value := range endpoint.Headers {
		req.Header.Set(name, value)
	}
	if endpoint.AuthHeaderName != "" {
		req.Header.Set(endpoint.AuthHeaderName, endpoint.AuthHeaderValue)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		networkErr := &NetworkError{URL: discoveryURL, Err: err}
		return nil, NewDiscoveryError(endpoint.URLString, platformProfile.GetName(), "http_request", 0, time.Since(startTime), networkErr)
	}
	defer resp.Body.Close()

	duration := time.Since(startTime)

	if resp.StatusCode != http.StatusOK {
		return nil, NewDiscoveryError(endpoint.URLString, platformProfile.GetName(), "http_status", resp.StatusCode, duration, fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status))
	}

	// Limit response size to prevent memory issues
	limitedReader := io.LimitReader(resp.Body, MaxResponseSize+1)
	body, err := io.ReadAll(limitedReader)
	if err != nil {
		return nil, NewDiscoveryError(endpoint.URLString, platformProfile.GetName(), "read_response", resp.StatusCode, duration, err)
	}

	if len(body) > MaxResponseSize {
		return nil, NewDiscoveryError(discoveryURL, platformProfile.GetName(), "read_response", resp.StatusCode, duration, fmt.Errorf("model response exceeds %d bytes: %w", MaxResponseSize, ErrResponseTooLarge))
	}

	if endpoint.Type == "" || endpoint.Type == domain.ProfileAuto {
		if aerr := checkAutoDetectShape(body, platformProfile.GetName()); aerr != nil {
			return nil, NewDiscoveryError(discoveryURL, platformProfile.GetName(), "auto_detect", resp.StatusCode, duration, aerr)
		}
	}

	models, err := platformProfile.ParseModelsResponse(body)
	if err != nil {
		// Include the actual discovery URL in the error for debugging
		return nil, NewDiscoveryError(discoveryURL, platformProfile.GetName(), "parse_response", resp.StatusCode, duration, &ParseError{Format: platformProfile.GetName(), Err: err})
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

// checkAutoDetectShape stops a candidate winning auto-detection on a response that is not
// really its own. The parsers tolerate any JSON, so without this an unrelated envelope would
// be accepted as an empty model list. Ollama and LM Studio must have their own envelope; the
// OpenAI look-alikes share one shape, so they must also identify themselves via owned_by.
func checkAutoDetectShape(body []byte, profileName string) error {
	switch profileName {
	case domain.ProfileOllama:
		// llama.cpp's /v1/models also carries a top-level `models` array (an Ollama-style
		// compat block) next to its OpenAI `data` envelope. /api/tags never has `data`, so
		// its presence means the body is an OpenAI-style server, not Ollama.
		if !gjson.GetBytes(body, "models").IsArray() || gjson.GetBytes(body, "data").IsArray() {
			return fmt.Errorf("model response is not in %s format", profileName)
		}
	case domain.ProfileLmStudio:
		if !isLMStudioModelList(body) {
			return fmt.Errorf("model response is not in %s format", profileName)
		}
	case domain.ProfileLlamaCpp, domain.ProfileVLLM, domain.ProfileRapidMLX, domain.ProfileOMLX:
		if !hasModelOwner(body, profileName) {
			return fmt.Errorf("model response does not identify %s", profileName)
		}
	}
	return nil
}

// lmStudioFields are model fields only LM Studio's /api/v0/models emits.
var lmStudioFields = []string{"type", "state", "arch"}

// isLMStudioModelList accepts LM Studio's /api/v0/models shape: a `data` array whose models
// carry its own `type`, `state` or `arch` fields. Plain OpenAI envelopes (vLLM, Rapid-MLX, oMLX,
// generic) have none of them, and used to win here and lose their context metadata. An empty list
// is accepted as before since it gives no evidence either way.
func isLMStudioModelList(body []byte) bool {
	data := gjson.GetBytes(body, "data")
	if !data.IsArray() {
		return false
	}
	models := data.Array()
	if len(models) == 0 {
		return true
	}
	for _, model := range models {
		if model.Get("id").String() == "" {
			continue
		}
		for _, field := range lmStudioFields {
			if model.Get(field).String() != "" {
				return true
			}
		}
	}
	return false
}

func hasModelOwner(body []byte, owner string) bool {
	for _, model := range gjson.GetBytes(body, "data").Array() {
		if model.Get("id").String() != "" && model.Get("owned_by").String() == owner {
			return true
		}
	}
	return false
}

func (c *HTTPModelDiscoveryClient) HealthCheck(ctx context.Context, endpoint *domain.Endpoint) error {
	// Use existing health check URL from endpoint
	req, err := http.NewRequestWithContext(ctx, "GET", endpoint.HealthCheckURLString, http.NoBody)
	if err != nil {
		return err
	}

	req.Header.Set("User-Agent", fmt.Sprintf(DefaultUserAgent, version.ShortName, version.Version))

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return &NetworkError{URL: endpoint.HealthCheckURLString, Err: err}
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("health check failed with status %d", resp.StatusCode)
	}

	return nil
}

func (c *HTTPModelDiscoveryClient) GetMetrics() DiscoveryMetrics {
	c.mu.RLock()
	defer c.mu.RUnlock()

	errorsByEndpoint := make(map[string]int64, len(c.metrics.ErrorsByEndpoint))
	maps.Copy(errorsByEndpoint, c.metrics.ErrorsByEndpoint)

	return DiscoveryMetrics{
		TotalDiscoveries:   atomic.LoadInt64(&c.metrics.TotalDiscoveries),
		SuccessfulRequests: atomic.LoadInt64(&c.metrics.SuccessfulRequests),
		FailedRequests:     atomic.LoadInt64(&c.metrics.FailedRequests),
		AverageLatency:     c.metrics.AverageLatency,
		LastDiscoveryTime:  c.metrics.LastDiscoveryTime,
		ErrorsByEndpoint:   errorsByEndpoint,
	}
}

func (c *HTTPModelDiscoveryClient) recordError(endpointURL string) {
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

func (c *HTTPModelDiscoveryClient) Close() error {
	c.httpClient.CloseIdleConnections()
	return nil
}
