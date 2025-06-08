package ports

import (
	"github.com/thushan/olla/internal/core/constants"
	"sync"
	"time"
)

// MockStatsCollector provides a working implementation for faking the StatsCollector interface
// it's a bit sketchy, however it allows us to test the code that uses StatsCollector
type MockStatsCollector struct {
	connections          map[string]int64
	rateLimitViolations  int64
	sizeLimitViolations  int64
	uniqueRateLimitedIPs map[string]time.Time
	mu                   sync.RWMutex
}

func NewMockStatsCollector() *MockStatsCollector {
	return &MockStatsCollector{
		connections:          make(map[string]int64),
		uniqueRateLimitedIPs: make(map[string]time.Time),
	}
}

func (m *MockStatsCollector) RecordRequest(endpoint, status string, latency time.Duration, bytes int64) {
}
func (m *MockStatsCollector) RecordDiscovery(endpoint string, success bool, latency time.Duration) {}

func (m *MockStatsCollector) RecordConnection(endpoint string, delta int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	current := m.connections[endpoint]
	newVal := current + int64(delta)
	if newVal < 0 {
		newVal = 0
	}
	m.connections[endpoint] = newVal
}

func (m *MockStatsCollector) RecordSecurityViolation(violation SecurityViolation) {
	m.mu.Lock()
	defer m.mu.Unlock()

	switch violation.ViolationType {
	case constants.ViolationRateLimit:
		m.rateLimitViolations++
		m.uniqueRateLimitedIPs[violation.ClientID] = time.Now()

		// Clean up old IPs (simplified for testing)
		cutoff := time.Now().Add(-time.Hour)
		for ip, timestamp := range m.uniqueRateLimitedIPs {
			if timestamp.Before(cutoff) {
				delete(m.uniqueRateLimitedIPs, ip)
			}
		}
	case constants.ViolationSizeLimit:
		m.sizeLimitViolations++
	}
}

func (m *MockStatsCollector) GetProxyStats() ProxyStats {
	return ProxyStats{}
}

func (m *MockStatsCollector) GetEndpointStats() map[string]EndpointStats {
	return make(map[string]EndpointStats)
}

func (m *MockStatsCollector) GetSecurityStats() SecurityStats {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return SecurityStats{
		RateLimitViolations:  m.rateLimitViolations,
		SizeLimitViolations:  m.sizeLimitViolations,
		UniqueRateLimitedIPs: len(m.uniqueRateLimitedIPs),
	}
}

func (m *MockStatsCollector) GetConnectionStats() map[string]int64 {
	m.mu.RLock()
	defer m.mu.RUnlock()

	stats := make(map[string]int64, len(m.connections))
	for endpoint, count := range m.connections {
		stats[endpoint] = count
	}
	return stats
}
