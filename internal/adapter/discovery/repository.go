package discovery

import (
	"context"
	"fmt"
	"net/url"
	"sync"
	"time"

	"github.com/thushan/olla/internal/adapter/registry/profile"
	"github.com/thushan/olla/internal/config"
	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/util"
)

const (
	MinHealthCheckInterval = 1 * time.Second
	MaxHealthCheckTimeout  = 30 * time.Second
)

type StaticEndpointRepository struct {
	endpoints      map[string]*domain.Endpoint
	profileFactory *profile.Factory
	mu             sync.RWMutex
}

func NewStaticEndpointRepository() *StaticEndpointRepository {
	profileFactory, err := profile.NewFactoryWithDefaults()
	if err != nil {
		// For tests, use empty profile dir to get built-in profiles
		profileFactory, _ = profile.NewFactory("")
	}
	return &StaticEndpointRepository{
		endpoints:      make(map[string]*domain.Endpoint),
		profileFactory: profileFactory,
	}
}

func NewStaticEndpointRepositoryWithFactory(factory *profile.Factory) *StaticEndpointRepository {
	return &StaticEndpointRepository{
		endpoints:      make(map[string]*domain.Endpoint),
		profileFactory: factory,
	}
}

func (r *StaticEndpointRepository) GetAll(ctx context.Context) ([]*domain.Endpoint, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if len(r.endpoints) == 0 {
		return []*domain.Endpoint{}, nil
	}

	endpoints := make([]*domain.Endpoint, 0, len(r.endpoints))
	for _, endpoint := range r.endpoints {
		endpointCopy := *endpoint
		endpoints = append(endpoints, &endpointCopy)
	}
	return endpoints, nil
}

func (r *StaticEndpointRepository) GetHealthy(ctx context.Context) ([]*domain.Endpoint, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Pre-allocate with capacity of all endpoints (worst case)
	healthy := make([]*domain.Endpoint, 0, len(r.endpoints))
	for _, endpoint := range r.endpoints {
		if endpoint.Status == domain.StatusHealthy {
			healthyCopy := *endpoint
			healthy = append(healthy, &healthyCopy)
		}
	}

	return healthy, nil
}

func (r *StaticEndpointRepository) GetRoutable(ctx context.Context) ([]*domain.Endpoint, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Pre-allocate with capacity of all endpoints (worst case)
	routable := make([]*domain.Endpoint, 0, len(r.endpoints))
	for _, endpoint := range r.endpoints {
		if endpoint.Status.IsRoutable() {
			routableCopy := *endpoint
			routable = append(routable, &routableCopy)
		}
	}

	return routable, nil
}

func (r *StaticEndpointRepository) UpdateEndpoint(ctx context.Context, endpoint *domain.Endpoint) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := endpoint.URL.String()
	existing, exists := r.endpoints[key]
	if !exists {
		return &domain.EndpointNotFoundError{URL: key}
	}

	existing.Status = endpoint.Status
	existing.LastChecked = endpoint.LastChecked
	existing.ConsecutiveFailures = endpoint.ConsecutiveFailures
	existing.BackoffMultiplier = endpoint.BackoffMultiplier
	existing.NextCheckTime = endpoint.NextCheckTime
	existing.LastLatency = endpoint.LastLatency

	return nil
}

func (r *StaticEndpointRepository) Exists(ctx context.Context, endpointURL *url.URL) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	key := endpointURL.String()
	_, exists := r.endpoints[key]
	return exists
}

func (r *StaticEndpointRepository) LoadFromConfig(ctx context.Context, configs []config.EndpointConfig) error {
	if len(configs) == 0 {
		r.mu.Lock()
		r.endpoints = make(map[string]*domain.Endpoint)
		r.mu.Unlock()
		return nil
	}

	now := time.Now()
	newEndpoints := make(map[string]*domain.Endpoint, len(configs))

	for _, cfg := range configs {
		if err := r.validateEndpointConfig(cfg); err != nil {
			return fmt.Errorf("invalid endpoint config for %q: %w", cfg.Name, err)
		}

		endpointURL, err := url.Parse(cfg.URL)
		if err != nil {
			return fmt.Errorf("invalid endpoint URL %q: %w", cfg.URL, err)
		}

		urlString := endpointURL.String()

		// Resolve URL defaults using fallback hierarchy: explicit config → profile defaults → universal defaults
		healthCheckPath, modelPath := r.resolveURLDefaults(cfg)

		// Build health check and model URLs using ResolveURLPath to preserve
		// the base URL's path prefix. This handles both relative paths and absolute URLs correctly,
		// preserving nested paths like http://localhost:12434/engines/llama.cpp/
		healthCheckURLString := util.ResolveURLPath(urlString, healthCheckPath)
		modelURLString := util.ResolveURLPath(urlString, modelPath)

		healthCheckURL, err := url.Parse(healthCheckURLString)
		if err != nil {
			return fmt.Errorf("invalid health check URL %q: %w", healthCheckURLString, err)
		}

		modelURL, err := url.Parse(modelURLString)
		if err != nil {
			return fmt.Errorf("invalid model URL %q: %w", modelURLString, err)
		}

		newEndpoint := &domain.Endpoint{
			Name:                  cfg.Name,
			URL:                   endpointURL,
			Type:                  cfg.Type,
			Priority:              cfg.Priority,
			HealthCheckURL:        healthCheckURL,
			ModelUrl:              modelURL,
			ModelFilter:           cfg.ModelFilter,
			CheckInterval:         cfg.CheckInterval,
			CheckTimeout:          cfg.CheckTimeout,
			Status:                domain.StatusUnknown,
			URLString:             urlString,
			HealthCheckPathString: healthCheckPath,
			HealthCheckURLString:  healthCheckURLString,
			ModelURLString:        modelURLString,
			BackoffMultiplier:     1,
			NextCheckTime:         now,
			PreservePath:          cfg.PreservePath,
		}

		newEndpoints[urlString] = newEndpoint
	}

	r.mu.Lock()
	r.endpoints = newEndpoints
	r.mu.Unlock()

	return nil
}

// resolveURLDefaults determines health check and model paths using fallback hierarchy:
// explicit config > profile defaults > universal defaults ("/", "/v1/models")
func (r *StaticEndpointRepository) resolveURLDefaults(cfg config.EndpointConfig) (healthCheckPath, modelPath string) {
	healthCheckPath = cfg.HealthCheckURL
	modelPath = cfg.ModelURL

	if healthCheckPath == "" || modelPath == "" {
		profileHealthPath, profileModelPath := r.applyProfileDefaults(cfg.Type, healthCheckPath, modelPath)
		if healthCheckPath == "" {
			healthCheckPath = profileHealthPath
		}
		if modelPath == "" {
			modelPath = profileModelPath
		}
	}

	return healthCheckPath, modelPath
}

// applyProfileDefaults retrieves defaults from profile configuration or returns universal fallbacks.
// Returns health check path (default "/") and model path (default "/v1/models").
func (r *StaticEndpointRepository) applyProfileDefaults(endpointType, healthCheckPath, modelPath string) (string, string) {
	// Try to get defaults from profile if a known type is specified (not empty or "auto")
	if endpointType != "" && endpointType != domain.ProfileAuto {
		profile, err := r.profileFactory.GetProfile(endpointType)
		if err == nil {
			if healthCheckPath == "" {
				healthCheckPath = profile.GetHealthCheckPath()
			}
			if modelPath == "" {
				if profileCfg := profile.GetConfig(); profileCfg != nil {
					modelPath = profileCfg.API.ModelDiscoveryPath
				}
			}
		}
	}

	// apply universal defaults for any remaining empty paths
	// (for "auto" type, unknown types, or if profile lookup failed)
	if healthCheckPath == "" {
		healthCheckPath = "/"
	}
	if modelPath == "" {
		modelPath = "/v1/models"
	}

	return healthCheckPath, modelPath
}

func (r *StaticEndpointRepository) validateEndpointConfig(cfg config.EndpointConfig) error {
	if cfg.URL == "" {
		return fmt.Errorf("endpoint URL cannot be empty")
	}

	// Allow empty health check and model URLs - they will get defaults from profile or fallback values
	// in LoadFromConfig. This enables simpler configuration when using known profile types.

	if cfg.CheckInterval < MinHealthCheckInterval {
		return fmt.Errorf("check_interval too short: minimum %v, got %v", MinHealthCheckInterval, cfg.CheckInterval)
	}

	if cfg.CheckTimeout >= cfg.CheckInterval {
		return fmt.Errorf("check_timeout (%v) must be less than check_interval (%v)", cfg.CheckTimeout, cfg.CheckInterval)
	}

	if cfg.CheckTimeout > MaxHealthCheckTimeout {
		return fmt.Errorf("check_timeout too long: maximum %v, got %v", MaxHealthCheckTimeout, cfg.CheckTimeout)
	}

	if cfg.Priority < 0 {
		return fmt.Errorf("priority must be non-negative, got %d", cfg.Priority)
	}

	if cfg.Type != "" {
		if !r.profileFactory.ValidateProfileType(cfg.Type) {
			availableTypes := r.profileFactory.GetAvailableProfiles()
			availableTypes = append(availableTypes, domain.ProfileAuto)
			return fmt.Errorf("unsupported endpoint type: %s, supported types: %v", cfg.Type, availableTypes)
		}
	}

	return nil
}
