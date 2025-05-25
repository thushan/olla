package health

import (
	"github.com/thushan/olla/internal/core/domain"
	"sync"
	"sync/atomic"
	"time"
)

// StatusTransitionTracker reduces logging noise by only logging status changes
type StatusTransitionTracker struct {
	entries sync.Map // map[string]*statusEntry
}

type statusEntry struct {
	lastStatus   int32 // atomic access to domain.EndpointStatus as int32
	lastLogTime  int64 // Unix nano timestamp
	errorCount   int64 // atomic counter
}

func NewStatusTransitionTracker() *StatusTransitionTracker {
	return &StatusTransitionTracker{}
}

func (st *StatusTransitionTracker) ShouldLog(endpointURL string, newStatus domain.EndpointStatus, isError bool) (bool, int) {
	value, exists := st.entries.Load(endpointURL)
	if !exists {
		entry := &statusEntry{
			lastStatus:  int32(statusToInt(newStatus)),
			lastLogTime: time.Now().UnixNano(),
		}
		value, _ = st.entries.LoadOrStore(endpointURL, entry)
	}

	entry := value.(*statusEntry)
	oldStatusInt := atomic.LoadInt32(&entry.lastStatus)
	oldStatus := intToStatus(oldStatusInt)

	// Always log status transitions
	if oldStatus != newStatus {
		atomic.StoreInt32(&entry.lastStatus, int32(statusToInt(newStatus)))
		atomic.StoreInt64(&entry.errorCount, 0) // Reset error count on status change
		return true, 0
	}

	// For repeated errors, log every 10th occurrence or every 5 minutes
	if isError {
		count := atomic.AddInt64(&entry.errorCount, 1)
		lastLog := time.Unix(0, atomic.LoadInt64(&entry.lastLogTime))

		if count%10 == 0 || time.Since(lastLog) > 5*time.Minute {
			atomic.StoreInt64(&entry.lastLogTime, time.Now().UnixNano())
			return true, int(count)
		}
	}

	return false, int(atomic.LoadInt64(&entry.errorCount))
}

func (st *StatusTransitionTracker) GetActiveEndpoints() []string {
	var endpoints []string
	st.entries.Range(func(key, value interface{}) bool {
		endpoints = append(endpoints, key.(string))
		return true
	})
	return endpoints
}

func (st *StatusTransitionTracker) CleanupEndpoint(endpointURL string) {
	st.entries.Delete(endpointURL)
}

// Helper functions to convert between status and int for atomic operations
func statusToInt(status domain.EndpointStatus) int {
	switch status {
	case domain.StatusHealthy:
		return 0
	case domain.StatusBusy:
		return 1
	case domain.StatusOffline:
		return 2
	case domain.StatusWarming:
		return 3
	case domain.StatusUnhealthy:
		return 4
	case domain.StatusUnknown:
		return 5
	default:
		return 5
	}
}

func intToStatus(i int32) domain.EndpointStatus {
	switch i {
	case 0:
		return domain.StatusHealthy
	case 1:
		return domain.StatusBusy
	case 2:
		return domain.StatusOffline
	case 3:
		return domain.StatusWarming
	case 4:
		return domain.StatusUnhealthy
	case 5:
		return domain.StatusUnknown
	default:
		return domain.StatusUnknown
	}
}