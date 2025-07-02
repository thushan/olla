package services

import (
	"context"
	"fmt"
	"sync"

	"github.com/thushan/olla/internal/logger"
)

// ManagedService defines the contract for services participating in the orchestration
// lifecycle. Services must be idempotent for Start/Stop operations and explicitly
// declare their dependencies to enable proper initialisation ordering.
type ManagedService interface {
	// Name returns the unique name of the service
	Name() string

	// Start initialises and starts the service
	Start(ctx context.Context) error

	// Stop gracefully shuts down the service
	Stop(ctx context.Context) error

	// Dependencies returns the names of services this service depends on
	Dependencies() []string
}

// ServiceManager orchestrates service lifecycle using topological sorting to resolve
// dependencies. It ensures services start in the correct order and shutdown gracefully
// in reverse order, handling partial startup failures with appropriate cleanup.
type ServiceManager struct {
	services   map[string]ManagedService
	registry   *ServiceRegistry
	logger     logger.StyledLogger
	startOrder []string // dependency-resolved start order
	mu         sync.RWMutex
}

// NewServiceManager creates a new service manager
func NewServiceManager(logger logger.StyledLogger) *ServiceManager {
	return &ServiceManager{
		services: make(map[string]ManagedService),
		registry: NewServiceRegistry(),
		logger:   logger,
	}
}

func (sm *ServiceManager) Register(service ManagedService) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	name := service.Name()
	if _, exists := sm.services[name]; exists {
		return fmt.Errorf("service %s already registered", name)
	}

	sm.services[name] = service
	sm.registry.Register(name, service)
	sm.logger.Debug("Service registered", "name", name)
	return nil
}

// resolveDependencies implements Kahn's algorithm for topological sorting to determine
// the correct service startup order. Returns an error if circular dependencies are
// detected or if a service declares a dependency on a non-existent service.
func (sm *ServiceManager) resolveDependencies() ([]string, error) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	dependencies := make(map[string][]string)
	inDegree := make(map[string]int)

	for name, service := range sm.services {
		dependencies[name] = service.Dependencies()
		inDegree[name] = 0
	}

	for _, deps := range dependencies {
		for _, dep := range deps {
			if _, exists := sm.services[dep]; !exists {
				return nil, fmt.Errorf("dependency %s not registered", dep)
			}
			inDegree[dep]++
		}
	}

	var order []string
	queue := make([]string, 0)

	for name, degree := range inDegree {
		if degree == 0 {
			queue = append(queue, name)
		}
	}

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		order = append(order, current)

		for _, dep := range dependencies[current] {
			inDegree[dep]--
			if inDegree[dep] == 0 {
				queue = append(queue, dep)
			}
		}
	}

	if len(order) != len(sm.services) {
		return nil, fmt.Errorf("circular dependency detected")
	}

	// Reverse to ensure dependencies start before dependants
	for i, j := 0, len(order)-1; i < j; i, j = i+1, j-1 {
		order[i], order[j] = order[j], order[i]
	}

	return order, nil
}

// Start orchestrates service initialisation in dependency order. If any service fails
// to start, all previously started services are stopped in reverse order to maintain
// system consistency. This ensures no partial startup states persist.
func (sm *ServiceManager) Start(ctx context.Context) error {
	order, err := sm.resolveDependencies()
	if err != nil {
		return fmt.Errorf("failed to resolve dependencies: %w", err)
	}

	sm.mu.Lock()
	sm.startOrder = order
	sm.mu.Unlock()

	sm.logger.Debug("Starting services", "count", len(order))

	started := make([]string, 0, len(order))
	for _, name := range order {
		service := sm.services[name]
		sm.logger.Debug("Starting service", "name", name, "dependencies", service.Dependencies())

		if err := service.Start(ctx); err != nil {
			sm.logger.Error("Failed to start service", "name", name, "error", err)
			sm.stopServices(ctx, started)
			return fmt.Errorf("failed to start service %s: %w", name, err)
		}

		started = append(started, name)
		sm.logger.Debug("Service started", "name", name)
	}

	sm.logger.Debug("All services started successfully")
	return nil
}

// Stop gracefully shuts down all services in reverse dependency order, ensuring
// dependants stop before their dependencies. This prevents resource access violations
// during shutdown.
func (sm *ServiceManager) Stop(ctx context.Context) error {
	sm.mu.RLock()
	order := make([]string, len(sm.startOrder))
	copy(order, sm.startOrder)
	sm.mu.RUnlock()

	for i, j := 0, len(order)-1; i < j; i, j = i+1, j-1 {
		order[i], order[j] = order[j], order[i]
	}

	sm.logger.Debug("Stopping services", "count", len(order))
	return sm.stopServices(ctx, order)
}

// stopServices stops the given services in order
func (sm *ServiceManager) stopServices(ctx context.Context, names []string) error {
	var firstErr error

	for _, name := range names {
		service, exists := sm.services[name]
		if !exists {
			continue
		}

		sm.logger.Debug("Stopping service", "name", name)
		if err := service.Stop(ctx); err != nil {
			sm.logger.Error("Failed to stop service", "name", name, "error", err)
			if firstErr == nil {
				firstErr = err
			}
		} else {
			sm.logger.Debug("Service stopped", "name", name)
		}
	}

	return firstErr
}

func (sm *ServiceManager) Get(name string) (ManagedService, bool) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	service, exists := sm.services[name]
	return service, exists
}

func (sm *ServiceManager) GetRegistry() *ServiceRegistry {
	return sm.registry
}
