package core

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/thushan/olla/internal/core/constants"

	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/core/ports"
	"github.com/thushan/olla/internal/logger"
	"github.com/thushan/olla/pkg/eventbus"
)

// EventType represents the type of proxy event
type EventType string

const (
	EventTypeProxyError       EventType = "proxy.error"
	EventTypeProxySuccess     EventType = "proxy.success"
	EventTypeCircuitBreaker   EventType = "circuit_breaker.open"
	EventTypeClientDisconnect EventType = "client.disconnected"
)

// ProxyEventMetadata provides type-safe metadata for events
type ProxyEventMetadata struct {
	Message              string
	Model                string
	BytesSent            int64
	BytesAfterDisconnect int
	Counter              int
	StatusCode           int
}

// ProxyEvent represents events that can be published by the proxy
type ProxyEvent struct {
	Timestamp time.Time
	Error     error
	Type      EventType
	RequestID string
	Endpoint  string
	Metadata  ProxyEventMetadata
	Duration  time.Duration
}

// BaseProxyComponents contains shared components for proxy implementations
type BaseProxyComponents struct {
	DiscoveryService ports.DiscoveryService
	Selector         domain.EndpointSelector
	StatsCollector   ports.StatsCollector
	MetricsExtractor ports.MetricsExtractor
	Logger           logger.StyledLogger
	EventBus         *eventbus.EventBus[ProxyEvent]

	Stats ProxyStats

	// Atomic counters for request tracking
	totalRequests atomic.Int64
}

// NewBaseProxyComponents creates a new BaseProxyComponents instance
func NewBaseProxyComponents(
	discoveryService ports.DiscoveryService,
	selector domain.EndpointSelector,
	statsCollector ports.StatsCollector,
	metricsExtractor ports.MetricsExtractor,
	logger logger.StyledLogger,
) *BaseProxyComponents {
	return &BaseProxyComponents{
		DiscoveryService: discoveryService,
		Selector:         selector,
		StatsCollector:   statsCollector,
		MetricsExtractor: metricsExtractor,
		Logger:           logger,
		EventBus:         eventbus.New[ProxyEvent](),
	}
}

// IncrementRequests increments the total request counter
func (b *BaseProxyComponents) IncrementRequests() {
	b.totalRequests.Add(1)
	atomic.AddInt64(&b.Stats.TotalRequests, 1)
}

// RecordSuccess records a successful request
func (b *BaseProxyComponents) RecordSuccess(endpoint *domain.Endpoint, latency int64, bytes int64) {
	b.Stats.RecordSuccess(latency)

	if b.StatsCollector != nil && endpoint != nil {
		b.StatsCollector.RecordRequest(endpoint, "success", time.Duration(latency)*time.Millisecond, bytes)
	}
}

// RecordFailure records a failed request
func (b *BaseProxyComponents) RecordFailure(ctx context.Context, endpoint *domain.Endpoint, duration time.Duration, err error) {
	b.Stats.RecordFailure()

	if b.StatsCollector != nil && endpoint != nil {
		b.StatsCollector.RecordRequest(endpoint, "error", duration, 0)
	}

	// Publish error event (non-blocking)
	if b.EventBus != nil {
		event := ProxyEvent{
			Type:      EventTypeProxyError,
			Timestamp: time.Now(),
			Error:     err,
			Duration:  duration,
		}

		if endpoint != nil {
			event.Endpoint = endpoint.Name
		}

		// check if we have a request ID in the context
		if reqID, ok := ctx.Value(constants.ContextRequestIdKey).(string); ok {
			event.RequestID = reqID
		}

		b.EventBus.PublishAsync(event)
	}
}

// PublishEvent publishes a custom event (non-blocking)
func (b *BaseProxyComponents) PublishEvent(event ProxyEvent) {
	if b.EventBus != nil {
		event.Timestamp = time.Now()
		b.EventBus.PublishAsync(event)
	}
}

// GetProxyStats returns current proxy statistics
func (b *BaseProxyComponents) GetProxyStats() ports.ProxyStats {
	return b.Stats.GetStats()
}

// Shutdown gracefully shuts down the base components
func (b *BaseProxyComponents) Shutdown() {
	if b.EventBus != nil {
		b.EventBus.Shutdown()
	}
}

// RequestTracker helps track request lifecycle with minimal allocations
type RequestTracker struct {
	StartTime    time.Time
	RequestID    string
	EndpointName string
}

// NewRequestTracker creates a new request tracker
func NewRequestTracker(requestID string) *RequestTracker {
	return &RequestTracker{
		StartTime: time.Now(),
		RequestID: requestID,
	}
}

// Duration returns the duration since request start
func (rt *RequestTracker) Duration() time.Duration {
	return time.Since(rt.StartTime)
}

// DurationMillis returns the duration in milliseconds
func (rt *RequestTracker) DurationMillis() int64 {
	return rt.Duration().Milliseconds()
}
