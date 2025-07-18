package unifier

import (
	"fmt"
	"sync"
	"time"

	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/logger"
)

// EndpointManager manages endpoint lifecycle and state
type EndpointManager struct {
	logger            logger.StyledLogger
	endpointStates    map[string]*domain.EndpointStateInfo
	endpointFailures  map[string]int
	circuitBreakers   map[string]*CircuitBreaker
	lastEndpointCheck map[string]time.Time
	config            Config
	mu                sync.RWMutex
}

// NewEndpointManager creates a new endpoint manager
func NewEndpointManager(config Config, logger logger.StyledLogger) *EndpointManager {
	return &EndpointManager{
		endpointStates:    make(map[string]*domain.EndpointStateInfo),
		endpointFailures:  make(map[string]int),
		circuitBreakers:   make(map[string]*CircuitBreaker),
		lastEndpointCheck: make(map[string]time.Time),
		config:            config,
		logger:            logger,
	}
}

// RecordFailure records an endpoint failure
func (m *EndpointManager) RecordFailure(endpointURL string, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.endpointFailures[endpointURL]++
	failures := m.endpointFailures[endpointURL]

	// Update circuit breaker if enabled
	if m.config.CircuitBreaker.Enabled {
		cb := m.getOrCreateCircuitBreakerLocked(endpointURL)
		cb.RecordFailure()
	}

	// Update state
	if failures >= m.config.MaxConsecutiveFailures {
		m.markEndpointOfflineLocked(endpointURL, fmt.Sprintf("Too many failures: %v", err))
	} else {
		m.updateEndpointStateLocked(endpointURL, domain.EndpointStateDegraded, err.Error(), failures)
	}
}

// RecordSuccess records a successful endpoint interaction
func (m *EndpointManager) RecordSuccess(endpointURL string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.endpointFailures[endpointURL] = 0
	m.lastEndpointCheck[endpointURL] = time.Now()

	// Update circuit breaker if enabled
	if m.config.CircuitBreaker.Enabled {
		cb := m.getOrCreateCircuitBreakerLocked(endpointURL)
		cb.RecordSuccess()
	}

	// Update state to online if it was offline
	if state, exists := m.endpointStates[endpointURL]; exists {
		if state.State != domain.EndpointStateOnline {
			state.State = domain.EndpointStateOnline
			state.LastStateChange = time.Now()
			state.ConsecutiveFailures = 0
			state.LastError = ""
		}
	} else {
		m.endpointStates[endpointURL] = &domain.EndpointStateInfo{
			State:               domain.EndpointStateOnline,
			LastStateChange:     time.Now(),
			ConsecutiveFailures: 0,
		}
	}
}

// GetCircuitBreaker returns the circuit breaker for an endpoint
func (m *EndpointManager) GetCircuitBreaker(endpointURL string) *CircuitBreaker {
	if !m.config.CircuitBreaker.Enabled {
		return nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	return m.getOrCreateCircuitBreakerLocked(endpointURL)
}

// GetState returns the current state of an endpoint
func (m *EndpointManager) GetState(endpointURL string) *domain.EndpointStateInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if state, exists := m.endpointStates[endpointURL]; exists {
		// Return a copy to prevent external modification
		stateCopy := *state
		return &stateCopy
	}
	return nil
}

// GetAllStates returns all endpoint states
func (m *EndpointManager) GetAllStates() map[string]*domain.EndpointStateInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	states := make(map[string]*domain.EndpointStateInfo)
	for url, state := range m.endpointStates {
		stateCopy := *state
		states[url] = &stateCopy
	}
	return states
}

// RemoveEndpoint removes all state for an endpoint
func (m *EndpointManager) RemoveEndpoint(endpointURL string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.endpointStates, endpointURL)
	delete(m.endpointFailures, endpointURL)
	delete(m.lastEndpointCheck, endpointURL)

	if cb, exists := m.circuitBreakers[endpointURL]; exists {
		cb.Reset()
		delete(m.circuitBreakers, endpointURL)
	}
}

// CleanupOrphaned removes state for endpoints no longer in use
func (m *EndpointManager) CleanupOrphaned(activeEndpoints map[string]bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for url := range m.endpointStates {
		if !activeEndpoints[url] {
			delete(m.endpointStates, url)
			delete(m.endpointFailures, url)
			delete(m.lastEndpointCheck, url)
			if cb, exists := m.circuitBreakers[url]; exists {
				cb.Reset()
				delete(m.circuitBreakers, url)
			}
		}
	}
}

// GetCircuitBreakerStats returns circuit breaker statistics
func (m *EndpointManager) GetCircuitBreakerStats() map[string]CircuitBreakerStats {
	m.mu.RLock()
	defer m.mu.RUnlock()

	stats := make(map[string]CircuitBreakerStats)
	for url, cb := range m.circuitBreakers {
		stats[url] = cb.GetStats()
	}
	return stats
}

// Helper methods

func (m *EndpointManager) getOrCreateCircuitBreakerLocked(endpointURL string) *CircuitBreaker {
	if cb, exists := m.circuitBreakers[endpointURL]; exists {
		return cb
	}
	cb := NewCircuitBreaker(m.config.CircuitBreaker)
	m.circuitBreakers[endpointURL] = cb
	return cb
}

func (m *EndpointManager) markEndpointOfflineLocked(endpointURL string, reason string) {
	m.updateEndpointStateLocked(endpointURL, domain.EndpointStateOffline, reason, m.endpointFailures[endpointURL])
}

func (m *EndpointManager) updateEndpointStateLocked(endpointURL string, state domain.EndpointState, lastError string, failures int) {
	if existing, exists := m.endpointStates[endpointURL]; exists {
		existing.State = state
		existing.LastStateChange = time.Now()
		existing.LastError = lastError
		existing.ConsecutiveFailures = failures
	} else {
		m.endpointStates[endpointURL] = &domain.EndpointStateInfo{
			State:               state,
			LastStateChange:     time.Now(),
			LastError:           lastError,
			ConsecutiveFailures: failures,
		}
	}
}
