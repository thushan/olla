package discovery

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/logger"
)

const (
	MaxConsecutiveFailures = 5 // Disable discovery after this many failures
	DefaultCleanupInterval = 10 * time.Minute
)

// ModelDiscoveryService coordinates model discovery across all endpoints
type ModelDiscoveryService struct {
	client            ModelDiscoveryClient
	endpointRepo      domain.EndpointRepository
	modelRegistry     domain.ModelRegistry
	logger            logger.StyledLogger
	stopCh            chan struct{}
	ticker            *time.Ticker
	disabledEndpoints map[string]int // tracks consecutive failures
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
	}
}

func (s *ModelDiscoveryService) Start(ctx context.Context) error {
	if !s.isRunning.CompareAndSwap(false, true) {
		return fmt.Errorf("discovery service is already running")
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
				s.logger.Warn("Regular discovery failed", "error", err)
			}
		}
	}
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

	// FIX: have to filter only active endpoitns here, otherwise we're stuck in a loop
	activeEndpoints := s.filterActiveEndpoints(endpoints)
	if len(activeEndpoints) == 0 {
		s.logger.Debug("No active endpoints available for discovery")
		return nil
	}

	s.logger.InfoWithCount("Starting model discovery on healthy endpoints", len(activeEndpoints))

	// Use worker pool for concurrent discovery
	return s.discoverConcurrently(ctx, activeEndpoints)
}

// DiscoverEndpoint discovers models from a specific endpoint
func (s *ModelDiscoveryService) DiscoverEndpoint(ctx context.Context, endpoint *domain.Endpoint) error {
	// [TF]	Note: Don't skip disabled endpoints in tests, as they might be testing re-enabling
	// 		In release, the filterActiveEndpoints in DiscoverAll handles this nicely

	discoveryCtx, cancel := context.WithTimeout(ctx, s.config.Timeout)
	defer cancel()

	models, err := s.client.DiscoverModels(discoveryCtx, endpoint)
	if err != nil {
		s.handleDiscoveryError(endpoint, err)
		return err
	}

	// Reset failure count on success
	s.resetFailureCount(endpoint.URLString)

	if err := s.modelRegistry.RegisterModels(ctx, endpoint.URLString, models); err != nil {
		s.logger.ErrorWithEndpoint("Failed to register discovered models", endpoint.Name, "error", err)
		return fmt.Errorf("failed to register models: %w", err)
	}

	s.logger.InfoWithEndpoint(" ", endpoint.Name, "models", len(models))
	return nil
}

func (s *ModelDiscoveryService) discoverConcurrently(ctx context.Context, endpoints []*domain.Endpoint) error {
	workerCount := s.config.ConcurrentWorkers
	if workerCount > len(endpoints) {
		workerCount = len(endpoints)
	}

	eg, ctx := errgroup.WithContext(ctx)
	eg.SetLimit(workerCount)

	for _, ep := range endpoints {
		eg.Go(func() error {
			if err := s.DiscoverEndpoint(ctx, ep); err != nil && !errors.Is(err, context.Canceled) {
				return err
			}
			return nil
		})
	}

	if err := eg.Wait(); err != nil {
		return fmt.Errorf("discovery failed: %w", err)
	}

	s.logger.InfoWithCount("Finished model discovery on healthy endpoints", len(endpoints))
	return nil
}

// handleDiscoveryError processes discovery errors and manages endpoint disabling
func (s *ModelDiscoveryService) handleDiscoveryError(endpoint *domain.Endpoint, err error) {
	s.logger.ErrorWithEndpoint("Model discovery failed", endpoint.Name, "error", err)

	if !IsRecoverable(err) {
		s.logger.WarnWithEndpoint("Disabling discovery for endpoint due to non-recoverable error", endpoint.Name)
		s.disableEndpoint(endpoint.URLString)
		return
	}

	s.incrementFailureCount(endpoint.URLString)

	failureCount := s.getFailureCount(endpoint.URLString)
	if failureCount >= MaxConsecutiveFailures {
		s.logger.WarnWithEndpoint("Disabling discovery for endpoint after consequent failures", endpoint.Name, "failures", failureCount)
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
		if failureCount, exists := s.disabledEndpoints[endpoint.URLString]; !exists || failureCount < MaxConsecutiveFailures {
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
	return exists && failureCount >= MaxConsecutiveFailures
}

// disableEndpoint marks an endpoint as disabled for discovery
func (s *ModelDiscoveryService) disableEndpoint(endpointURL string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.disabledEndpoints[endpointURL] = MaxConsecutiveFailures
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
}

// GetMetrics returns combined discovery metrics
func (s *ModelDiscoveryService) GetMetrics() DiscoveryMetrics {
	metrics := s.client.GetMetrics()

	s.mu.RLock()
	disabledCount := len(s.disabledEndpoints)
	s.mu.RUnlock()

	if disabledCount > 0 {
		if metrics.ErrorsByEndpoint == nil {
			metrics.ErrorsByEndpoint = make(map[string]int64)
		}
		metrics.ErrorsByEndpoint["_disabled_endpoints"] = int64(disabledCount)
	}

	return metrics
}
