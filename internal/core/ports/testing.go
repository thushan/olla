package ports

import (
	"sync"
	"time"
)

// MockStatsCollector provides a working implementation for faking the StatsCollector interface
// it's a bit sketchy, however it allows us to test the code that uses StatsCollector
type MockStatsCollector struct {
	connections map[string]int64
	mu          sync.RWMutex
}

func NewMockStatsCollector() *MockStatsCollector {
	return &MockStatsCollector{
		connections: make(map[string]int64),
	}
}

func (m *MockStatsCollector) RecordRequest(endpoint, status string, latency time.Duration, bytes int64) {
}
func (m *MockStatsCollector) RecordSecurityViolation(violation SecurityViolation)                  {}
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

func (m *MockStatsCollector) GetProxyStats() ProxyStats {
	return ProxyStats{}
}

func (m *MockStatsCollector) GetEndpointStats() map[string]EndpointStats {
	return make(map[string]EndpointStats)
}

func (m *MockStatsCollector) GetSecurityStats() SecurityStats {
	return SecurityStats{}
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
