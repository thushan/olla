package health

import (
	"github.com/thushan/olla/internal/core/domain"
	"sync"
	"time"
)

// StatusTransitionTracker reduces logging noise by only logging status changes
type StatusTransitionTracker struct {
	mu          sync.RWMutex
	lastStatus  map[string]domain.EndpointStatus
	lastLogTime map[string]time.Time
	errorCounts map[string]int
}

func NewStatusTransitionTracker() *StatusTransitionTracker {
	return &StatusTransitionTracker{
		lastStatus:  make(map[string]domain.EndpointStatus),
		lastLogTime: make(map[string]time.Time),
		errorCounts: make(map[string]int),
	}
}

func (st *StatusTransitionTracker) ShouldLog(endpointURL string, newStatus domain.EndpointStatus, isError bool) (bool, int) {
	st.mu.Lock()
	defer st.mu.Unlock()

	key := endpointURL
	oldStatus := st.lastStatus[key]

	// Always log status transitions
	if oldStatus != newStatus {
		st.lastStatus[key] = newStatus
		st.errorCounts[key] = 0 // Reset error count on status change
		return true, 0
	}

	// For repeated errors, log every 10th occurrence or every 5 minutes
	if isError {
		st.errorCounts[key]++
		count := st.errorCounts[key]
		lastLog := st.lastLogTime[key]

		if count%10 == 0 || time.Since(lastLog) > 5*time.Minute {
			st.lastLogTime[key] = time.Now()
			return true, count
		}
	}

	return false, st.errorCounts[key]
}

func (st *StatusTransitionTracker) GetActiveEndpoints() []string {
	st.mu.RLock()
	defer st.mu.RUnlock()

	endpoints := make([]string, 0, len(st.lastStatus))
	for url := range st.lastStatus {
		endpoints = append(endpoints, url)
	}
	return endpoints
}

func (st *StatusTransitionTracker) CleanupEndpoint(endpointURL string) {
	st.mu.Lock()
	defer st.mu.Unlock()
	delete(st.lastStatus, endpointURL)
	delete(st.lastLogTime, endpointURL)
	delete(st.errorCounts, endpointURL)
}
