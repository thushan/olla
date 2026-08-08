package balancer

import (
	"context"
	"errors"
	"sync/atomic"

	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/core/ports"
)

// LeastConnectionsSelector implements a load balancer that selects the endpoint with the least number of active connections.
type LeastConnectionsSelector struct {
	statsCollector ports.StatsCollector
	// cursor rotates the tie-break among endpoints sharing the minimum
	// connection count. A strict less-than comparison alone always keeps the
	// first tied candidate in registration order, so a recovered endpoint
	// tied with an already-healthy one would never receive traffic under
	// sequential requests - it's permanently shadowed by whichever endpoint
	// happens to sort first. Plain atomic counter, no mutex: Select is a
	// per-request hot path and the cursor only needs to move, not be exact.
	cursor atomic.Uint64
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
	//
	// tied collects every endpoint sharing the current minimum, not just the
	// first one found, so a rotating cursor can distribute selection among
	// them instead of a strict less-than always favouring registration order.
	tied := make([]*domain.Endpoint, 0, len(routable))
	minConnections := int64(-1)

	for _, endpoint := range routable {
		connections := l.statsCollector.GetConnectionCount(endpoint.URLString)

		switch {
		case minConnections == -1 || connections < minConnections:
			minConnections = connections
			tied = tied[:0]
			tied = append(tied, endpoint)
		case connections == minConnections:
			tied = append(tied, endpoint)
		}
	}

	if len(tied) == 1 {
		return tied[0], nil
	}

	idx := l.cursor.Add(1) - 1
	return tied[idx%uint64(len(tied))], nil
}

func (l *LeastConnectionsSelector) IncrementConnections(endpoint *domain.Endpoint) {
	l.statsCollector.RecordConnection(endpoint, 1)
}

func (l *LeastConnectionsSelector) DecrementConnections(endpoint *domain.Endpoint) {
	l.statsCollector.RecordConnection(endpoint, -1)
}
