package balancer

import (
	"context"
	"errors"

	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/core/ports"
)

// LeastConnectionsSelector implements a load balancer that selects the endpoint with the least number of active connections.
type LeastConnectionsSelector struct {
	statsCollector ports.StatsCollector
}

func NewLeastConnectionsSelector(statsCollector ports.StatsCollector) *LeastConnectionsSelector {
	return &LeastConnectionsSelector{
		statsCollector: statsCollector,
	}
}

func (l *LeastConnectionsSelector) Name() string {
	return DefaultBalancerLeastConnections
}

func (l *LeastConnectionsSelector) Select(ctx context.Context, endpoints []*domain.Endpoint) (*domain.Endpoint, error) {
	if len(endpoints) == 0 {
		return nil, errors.New("no endpoints available")
	}

	routable := make([]*domain.Endpoint, 0, len(endpoints))
	for _, endpoint := range endpoints {
		if endpoint.Status.IsRoutable() {
			routable = append(routable, endpoint)
		}
	}

	if len(routable) == 0 {
		return nil, errors.New("no routable endpoints available")
	}

	// Read each candidate's count directly instead of building a map of every
	// tracked endpoint via GetConnectionStats - Select runs per request, and
	// most of that map would be discarded unread.
	var selected *domain.Endpoint
	minConnections := int64(-1)

	for _, endpoint := range routable {
		connections := l.statsCollector.GetConnectionCount(endpoint.URLString)

		if minConnections == -1 || connections < minConnections {
			minConnections = connections
			selected = endpoint
		}
	}

	return selected, nil
}

func (l *LeastConnectionsSelector) IncrementConnections(endpoint *domain.Endpoint) {
	l.statsCollector.RecordConnection(endpoint, 1)
}

func (l *LeastConnectionsSelector) DecrementConnections(endpoint *domain.Endpoint) {
	l.statsCollector.RecordConnection(endpoint, -1)
}
