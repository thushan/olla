package ports

import (
	"github.com/thushan/olla/internal/core/domain"
	"time"
)

type StatsCollector interface {
	RecordRequest(endpoint *domain.Endpoint, status string, latency time.Duration, bytes int64)
	RecordConnection(endpoint *domain.Endpoint, delta int) // +1 connect, -1 disconnect
	RecordSecurityViolation(violation SecurityViolation)
	RecordDiscovery(endpoint *domain.Endpoint, success bool, latency time.Duration)

	GetProxyStats() ProxyStats
	GetEndpointStats() map[string]EndpointStats
	GetSecurityStats() SecurityStats
	GetConnectionStats() map[string]int64
}

type EndpointStats struct {
	Name               string    `json:"name"`
	URL                string    `json:"url"`
	ActiveConnections  int64     `json:"active_connections"`
	TotalRequests      int64     `json:"total_requests"`
	SuccessfulRequests int64     `json:"successful_requests"`
	FailedRequests     int64     `json:"failed_requests"`
	TotalBytes         int64     `json:"total_bytes"`
	AverageLatency     int64     `json:"avg_latency_ms"`
	MinLatency         int64     `json:"min_latency_ms"`
	MaxLatency         int64     `json:"max_latency_ms"`
	LastUsed           time.Time `json:"last_used"`
	SuccessRate        float64   `json:"success_rate_percent"`
}

type SecurityStats struct {
	RateLimitViolations  int64 `json:"rate_limit_violations"`
	SizeLimitViolations  int64 `json:"size_limit_violations"`
	UniqueRateLimitedIPs int   `json:"unique_rate_limited_ips"`
}
