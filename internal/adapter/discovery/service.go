package discovery

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/thushan/olla/internal/adapter/filter"
	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/core/ports"
	"github.com/thushan/olla/internal/logger"
)

const (
	MaxConsecutiveFailures   = 5 // Pause discovery after this many failures
	DefaultDiscoveryCooldown = 5 * time.Minute
	DefaultCleanupInterval   = 10 * time.Minute
)

// ModelDiscoveryService coordinates model discovery across all endpoints
type ModelDiscoveryService struct {
	client            ModelDiscoveryClient
	endpointRepo      domain.EndpointRepository
	modelRegistry     domain.ModelRegistry
	logger            logger.StyledLogger
	modelFilter       ports.Filter
	stopCh            chan struct{}
	ticker            *time.Ticker
	retryAfter        map[string]time.Time
	disabledEndpoints map[string]int                  // tracks consecutive failures
	endpointFilters   map[string]*domain.FilterConfig // per-endpoint filter configs
	config            DiscoveryConfig
	mu                sync.RWMutex
	isRunning         atomic.Bool
}

// DiscoveryConfig holds configuration for the discovery service
type DiscoveryConfig struct {
	Interval          time.Duration // How often to discover models
	Timeout           time.Duration // Timeout for individual discovery requests
	ConcurrentWorkers int           // Max concurrent discovery operations
	RetryAttempts     int           // Number of retry attempts for failed discoveries
	RetryBackoff      time.Duration // Backoff between retries
}

func NewModelDiscoveryService(client ModelDiscoveryClient, endpointRepo domain.EndpointRepository, modelRegistry domain.ModelRegistry, config DiscoveryConfig, logger logger.StyledLogger) *ModelDiscoveryService {
	return &ModelDiscoveryService{
		client:            client,
		endpointRepo:      endpointRepo,
		modelRegistry:     modelRegistry,
		logger:            logger,
		config:            config,
		stopCh:            make(chan struct{}),
		disabledEndpoints: make(map[string]int),
		retryAfter:        make(map[string]time.Time),
		modelFilter:       filter.NewGlobFilter(),
		endpointFilters:   make(map[string]*domain.FilterConfig),
	}
}

func (s *ModelDiscoveryService) Start(ctx context.Context) error {
	if !s.isRunning.CompareAndSwap(false, true) {
		return errors.New("discovery service is already running")
	}

	// Guard against a zero interval from user config overrides — time.NewTicker panics on <= 0.
	if s.config.Interval <= 0 {
		s.logger.Warn("Discovery interval is zero or negative, defaulting to 5 minutes", "configured", s.config.Interval)
		s.config.Interval = 5 * time.Minute
	}

	s.logger.Info("Initialising model discovery service", "interval", s.config.Interval)

	s.ticker = time.NewTicker(s.config.Interval)

	go s.discoveryLoop(ctx)

	return nil
}

func (s *ModelDiscoveryService) Stop(ctx context.Context) error {
	if !s.isRunning.CompareAndSwap(true, false) {
		return nil // looks like this has stopped earlier?
	}

	s.logger.Info("Stopping model discovery service")

	if s.ticker != nil {
		s.ticker.Stop()
	}

	close(s.stopCh)
	return nil
}

// discoveryLoop runs the periodic discovery process
func (s *ModelDiscoveryService) discoveryLoop(ctx context.Context) {
	defer s.isRunning.Store(false)

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.stopCh:
			return
		case <-s.ticker.C:
			if err := s.DiscoverAll(ctx); err != nil {
				s.LogErrorsWithContext(err)
			}
		}
	}
}
func (s *ModelDiscoveryService) LogErrorsWithContext(err error) {
	userMsg := GetUserFriendlyMessage(err)
	logCtx := logger.LogContext{
		UserArgs:     []interface{}{},
		DetailedArgs: []interface{}{"detailed_error", err},
	}
	s.logger.WarnWithContext(fmt.Sprintf("Regular discovery failed (%s)", userMsg), "", logCtx)
}

// DiscoverAll discovers models from all healthy endpoints
func (s *ModelDiscoveryService) DiscoverAll(ctx context.Context) error {
	endpoints, err := s.endpointRepo.GetHealthy(ctx)
	if err != nil {
		return fmt.Errorf("failed to get healthy endpoints: %w", err)
	}

	if len(endpoints) == 0 {
		s.logger.Debug("No healthy endpoints available for discovery")
		return nil
	}

	// Failed endpoints are retried once their cooldown expires.
	activeEndpoints := s.filterActiveEndpoints(endpoints)
	if len(activeEndpoints) == 0 {
		s.logger.Debug("No active endpoints available for discovery")
		return nil
	}

	s.logger.InfoWithCount("Starting model discovery on healthy endpoints", len(activeEndpoints))

	// Use worker pool for concurrent discovery
	return s.discoverConcurrently(ctx, activeEndpoints)
}

// endpointModelRegistrar is implemented by registries that register models with the
// endpoint object rather than just its URL.
type endpointModelRegistrar interface {
	RegisterModelsWithEndpoint(ctx context.Context, endpoint *domain.Endpoint, models []*domain.ModelInfo) error
}

// DiscoverEndpoint discovers models from a specific endpoint
func (s *ModelDiscoveryService) DiscoverEndpoint(ctx context.Context, endpoint *domain.Endpoint) error {
	// Explicit discovery bypasses the periodic cooldown.
	if contextErr := ctx.Err(); contextErr != nil {
		return contextErr
	}

	models, err := s.fetchModelsWithRetry(ctx, endpoint)
	if err != nil {
		if ctx.Err() == nil && !errors.Is(err, errDiscoveryStopped) {
			s.handleDiscoveryError(endpoint, err)
		}
		return err
	}

	if contextErr := ctx.Err(); contextErr != nil {
		return contextErr
	}

	// Apply model filtering if configured for this endpoint
	filteredModels := models
	if filterConfig := s.getEndpointFilterConfig(endpoint); filterConfig != nil {
		filteredModels, err = s.applyModelFilter(ctx, models, filterConfig)
		if err != nil {
			s.logger.ErrorWithEndpoint("Failed to apply model filter", endpoint.Name, "error", err)
			// Continue with unfiltered models rather than failing completely
			filteredModels = models
		} else if len(filteredModels) < len(models) {
			s.logger.InfoWithEndpoint("Applied model filter", endpoint.Name,
				"original_count", len(models),
				"filtered_count", len(filteredModels))
		}
	}

	// Registries that take the endpoint object also learn its type, which scopes
	// per-platform name rules such as Ollama's default tag
	if endpointRegistry, ok := s.modelRegistry.(endpointModelRegistrar); ok {
		if err := endpointRegistry.RegisterModelsWithEndpoint(ctx, endpoint, filteredModels); err != nil {
			s.logger.ErrorWithEndpoint("Failed to register discovered models", endpoint.Name, "error", err)
			return fmt.Errorf("failed to register models: %w", err)
		}
	} else {
		// Fall back to string-based registration
		if err := s.modelRegistry.RegisterModels(ctx, endpoint.URLString, filteredModels); err != nil {
			s.logger.ErrorWithEndpoint("Failed to register discovered models", endpoint.Name, "error", err)
			return fmt.Errorf("failed to register models: %w", err)
		}
	}

	s.resetFailureCount(endpoint.URLString)
	s.logger.InfoWithEndpoint(" ", endpoint.Name, "models", len(filteredModels))
	return nil
}

// maxRetryBackoff caps the doubling so a generous retry_attempts cannot park a
// discovery worker for minutes inside one cycle.
const maxRetryBackoff = 30 * time.Second

// errDiscoveryStopped is returned when the service is stopped mid-retry. It wraps
// context.Canceled so callers treat it like any other cancellation, and
// DiscoverEndpoint matches it to avoid charging the endpoint for our shutdown.
var errDiscoveryStopped = fmt.Errorf("discovery service stopped: %w", context.Canceled)

// nextRetryBackoff doubles the wait up to maxRetryBackoff.
func nextRetryBackoff(d time.Duration) time.Duration {
	return min(d*2, maxRetryBackoff)
}

// fetchModelsWithRetry asks the backend for its models, retrying transient
// failures up to RetryAttempts more times with RetryBackoff doubling between
// them. All attempts and backoffs share one Timeout budget, so a single cycle
// (including the synchronous pass at startup) takes no longer than a single
// request always did, whatever retry_attempts is set to. A retry whose backoff
// would outlast what is left of the budget is not started.
//
// The retries ride out a blip inside one discovery cycle, so the whole sequence
// counts as a single failure towards MaxConsecutiveFailures (handled by the
// caller once this gives up). Counting each attempt would trip the five-failure
// cooldown, which pauses the endpoint for minutes, off one brief outage.
func (s *ModelDiscoveryService) fetchModelsWithRetry(ctx context.Context, endpoint *domain.Endpoint) ([]*domain.ModelInfo, error) {
	retries := max(s.config.RetryAttempts, 0)
	// config validation rejects negatives; clamp anyway for callers that build the config directly
	backoff := min(max(s.config.RetryBackoff, 0), maxRetryBackoff)

	cycleCtx, cancel := context.WithTimeout(ctx, s.config.Timeout)
	defer cancel()
	deadline, _ := cycleCtx.Deadline()

	for attempt := 0; ; attempt++ {
		models, err := s.client.DiscoverModels(cycleCtx, endpoint)

		if err == nil || attempt >= retries || ctx.Err() != nil || !isRetryableWithinCycle(err) {
			return models, err
		}
		if time.Until(deadline) <= backoff {
			return models, err
		}
		// Checked here as well as in the wait so a zero backoff still honours Stop.
		select {
		case <-s.stopCh:
			return nil, errDiscoveryStopped
		default:
		}

		s.logger.Debug("Model discovery attempt failed, retrying",
			"endpoint", endpoint.Name, "attempt", attempt+1, "of", retries+1, "backoff", backoff, "error", err)

		if backoff > 0 {
			timer := time.NewTimer(backoff)
			select {
			case <-timer.C:
			case <-ctx.Done():
				timer.Stop()
				return nil, ctx.Err()
			case <-s.stopCh:
				timer.Stop()
				return nil, errDiscoveryStopped
			}
			backoff = nextRetryBackoff(backoff)
		}
	}
}

// isRetryableWithinCycle separates failures a second request moments later can
// fix (connection trouble, 5xx, a truncated body) from ones it cannot. A parse
// failure is recoverable across cycles (IsRecoverable) because a backend upgrade
// can repair it, but the same body comes back seconds later, as does a 4xx.
func isRetryableWithinCycle(err error) bool {
	var parseErr *ParseError
	if errors.As(err, &parseErr) {
		return false
	}
	var profileErr *ProfileNotFoundError
	if errors.As(err, &profileErr) {
		return false
	}
	// An oversized listing is the backend's steady state, not a blip.
	if errors.Is(err, ErrResponseTooLarge) {
		return false
	}
	// A certificate the client rejects is rejected again on the next handshake.
	var unknownAuthority x509.UnknownAuthorityError
	var certInvalid x509.CertificateInvalidError
	var hostname x509.HostnameError
	var verifyErr *tls.CertificateVerificationError
	if errors.As(err, &verifyErr) || errors.As(err, &unknownAuthority) || errors.As(err, &certInvalid) || errors.As(err, &hostname) {
		return false
	}
	return IsRecoverable(err)
}

func (s *ModelDiscoveryService) discoverConcurrently(ctx context.Context, endpoints []*domain.Endpoint) error {
	workerCount := s.config.ConcurrentWorkers
	// Guard against zero from user config overrides — errgroup.SetLimit panics on <= 0.
	if workerCount <= 0 {
		s.logger.Warn("ConcurrentWorkers is zero or negative, defaulting to 1", "configured", workerCount)
		workerCount = 1
	}
	if workerCount > len(endpoints) {
		workerCount = len(endpoints)
	}

	// Endpoint failures must not cancel discovery on unrelated backends.
	var eg errgroup.Group
	eg.SetLimit(workerCount)

	for _, ep := range endpoints {
		eg.Go(func() error {
			if err := s.DiscoverEndpoint(ctx, ep); err != nil {
				return err
			}
			return nil
		})
	}

	if err := errors.Join(eg.Wait(), ctx.Err()); err != nil {
		return fmt.Errorf("discovery failed: %w", err)
	}

	s.logger.InfoWithCount("Finished model discovery on healthy endpoints", len(endpoints))
	return nil
}

// handleDiscoveryError processes discovery errors and manages endpoint disabling
func (s *ModelDiscoveryService) handleDiscoveryError(endpoint *domain.Endpoint, err error) {
	// Create user-friendly message for console output and detailed error for logs
	userMsg := GetUserFriendlyMessage(err)

	logCtx := logger.LogContext{
		UserArgs:     []interface{}{},
		DetailedArgs: []interface{}{"detailed_error", err},
	}

	s.logger.ErrorWithContext(fmt.Sprintf("Model discovery failed (%s)", userMsg), endpoint.Name, logCtx)

	if !IsRecoverable(err) {
		s.logger.WarnWithEndpoint("Pausing discovery for endpoint until cooldown expires", endpoint.Name)
		s.disableEndpoint(endpoint.URLString)
		return
	}

	s.incrementFailureCount(endpoint.URLString)

	failureCount := s.getFailureCount(endpoint.URLString)
	if failureCount >= MaxConsecutiveFailures {
		s.logger.WarnWithEndpoint("Pausing discovery for endpoint after consecutive failures", endpoint.Name, "failures", failureCount)
		s.disableEndpoint(endpoint.URLString)
	}
}

// filterActiveEndpoints removes disabled endpoints from the list
func (s *ModelDiscoveryService) filterActiveEndpoints(endpoints []*domain.Endpoint) []*domain.Endpoint {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if len(s.disabledEndpoints) == 0 {
		return endpoints
	}

	active := make([]*domain.Endpoint, 0, len(endpoints))
	for _, endpoint := range endpoints {
		if failureCount, exists := s.disabledEndpoints[endpoint.URLString]; !exists || failureCount < MaxConsecutiveFailures || !time.Now().Before(s.retryAfter[endpoint.URLString]) {
			active = append(active, endpoint)
		}
	}

	return active
}

// isEndpointDisabled checks if an endpoint is disabled for discovery
func (s *ModelDiscoveryService) isEndpointDisabled(endpointURL string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	failureCount, exists := s.disabledEndpoints[endpointURL]
	return exists && failureCount >= MaxConsecutiveFailures && time.Now().Before(s.retryAfter[endpointURL])
}

// disableEndpoint marks an endpoint as disabled for discovery
func (s *ModelDiscoveryService) disableEndpoint(endpointURL string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.disabledEndpoints[endpointURL] = MaxConsecutiveFailures
	cooldown := max(DefaultDiscoveryCooldown, s.config.Interval)
	s.retryAfter[endpointURL] = time.Now().Add(cooldown)
}

// incrementFailureCount increments the failure count for an endpoint
func (s *ModelDiscoveryService) incrementFailureCount(endpointURL string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.disabledEndpoints[endpointURL]++
}

// getFailureCount returns the current failure count for an endpoint
func (s *ModelDiscoveryService) getFailureCount(endpointURL string) int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.disabledEndpoints[endpointURL]
}

// resetFailureCount resets the failure count for an endpoint
func (s *ModelDiscoveryService) resetFailureCount(endpointURL string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.disabledEndpoints, endpointURL)
	delete(s.retryAfter, endpointURL)
}

// GetMetrics returns combined discovery metrics
func (s *ModelDiscoveryService) GetMetrics() DiscoveryMetrics {
	metrics := s.client.GetMetrics()

	s.mu.RLock()
	disabledCount := 0
	for endpointURL, failures := range s.disabledEndpoints {
		if failures >= MaxConsecutiveFailures && time.Now().Before(s.retryAfter[endpointURL]) {
			disabledCount++
		}
	}
	s.mu.RUnlock()

	if disabledCount > 0 {
		if metrics.ErrorsByEndpoint == nil {
			metrics.ErrorsByEndpoint = make(map[string]int64)
		}
		metrics.ErrorsByEndpoint["_disabled_endpoints"] = int64(disabledCount)
	}

	return metrics
}

// SetEndpointFilterConfig sets a filter configuration for a specific endpoint
func (s *ModelDiscoveryService) SetEndpointFilterConfig(endpointName string, filterConfig *domain.FilterConfig) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if filterConfig != nil {
		s.endpointFilters[endpointName] = filterConfig
	} else {
		delete(s.endpointFilters, endpointName)
	}
}

// getEndpointFilterConfig retrieves the filter configuration for an endpoint
func (s *ModelDiscoveryService) getEndpointFilterConfig(endpoint *domain.Endpoint) *domain.FilterConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Check by endpoint name first
	if config, exists := s.endpointFilters[endpoint.Name]; exists {
		return config
	}

	// Fallback to URL string if name doesn't match
	if config, exists := s.endpointFilters[endpoint.URLString]; exists {
		return config
	}

	// Use the endpoint's own ModelFilter if no override has been set
	if endpoint.ModelFilter != nil {
		return endpoint.ModelFilter
	}

	return nil
}

// applyModelFilter applies filtering to discovered models
func (s *ModelDiscoveryService) applyModelFilter(ctx context.Context, models []*domain.ModelInfo, filterConfig *domain.FilterConfig) ([]*domain.ModelInfo, error) {
	if filterConfig == nil || filterConfig.IsEmpty() {
		return models, nil
	}

	// Use the filter to process models
	result, err := s.modelFilter.Apply(ctx, filterConfig, models, func(item interface{}) string {
		if model, ok := item.(*domain.ModelInfo); ok {
			return model.Name
		}
		return ""
	})

	if err != nil {
		return nil, fmt.Errorf("failed to apply model filter: %w", err)
	}

	// Convert filtered results back to ModelInfo slice
	filtered := make([]*domain.ModelInfo, 0, len(result.Accepted))
	for _, item := range result.Accepted {
		if model, ok := item.(*domain.ModelInfo); ok {
			filtered = append(filtered, model)
		}
	}

	return filtered, nil
}
