package stats

import (
	"math/rand/v2"
	"sort"
	"sync"
)

// PercentileTracker is an interface for tracking latencies and calculating percentiles
type PercentileTracker interface {
	Add(value int64)
	GetPercentiles() (p50, p95, p99 int64)
	Count() int64
	Reset()
}

// ReservoirSampler implements reservoir sampling for memory-efficient percentile calculation
// It maintains a fixed-size sample of values, providing good statistical accuracy
// with bounded memory usage (typically 100-200 samples vs 1000)
type ReservoirSampler struct {
	samples     []int64
	sampleSize  int
	count       int64
	mu          sync.Mutex
	initialized bool
}

// NewReservoirSampler creates a new reservoir sampler with the specified sample size
func NewReservoirSampler(sampleSize int) *ReservoirSampler {
	if sampleSize <= 0 {
		sampleSize = 100 // Default to 100 samples
	}
	return &ReservoirSampler{
		sampleSize: sampleSize,
		samples:    make([]int64, 0, sampleSize), // Start with empty slice, grow as needed
	}
}

// Add adds a new latency value to the sampler
func (rs *ReservoirSampler) Add(value int64) {
	rs.mu.Lock()
	defer rs.mu.Unlock()

	rs.count++

	// Fill reservoir array until it's full
	if len(rs.samples) < rs.sampleSize {
		rs.samples = append(rs.samples, value)
		return
	}

	// Use reservoir sampling algorithm for subsequent values
	// This gives each value an equal probability of being included
	j := rand.Int64N(rs.count) //nolint:gosec // Statistical sampling doesn't require crypto rand
	if j < int64(rs.sampleSize) {
		rs.samples[j] = value
	}
}

// GetPercentiles returns the 50th, 95th, and 99th percentiles
func (rs *ReservoirSampler) GetPercentiles() (p50, p95, p99 int64) {
	rs.mu.Lock()
	defer rs.mu.Unlock()

	if len(rs.samples) == 0 {
		return 0, 0, 0
	}

	// Create a copy to avoid modifying the original samples
	sorted := make([]int64, len(rs.samples))
	copy(sorted, rs.samples)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i] < sorted[j]
	})

	// Calculate percentile indices
	p50Idx := len(sorted) * 50 / 100
	p95Idx := len(sorted) * 95 / 100
	p99Idx := len(sorted) * 99 / 100

	// Ensure indices are within bounds
	if p50Idx >= len(sorted) {
		p50Idx = len(sorted) - 1
	}
	if p95Idx >= len(sorted) {
		p95Idx = len(sorted) - 1
	}
	if p99Idx >= len(sorted) {
		p99Idx = len(sorted) - 1
	}

	return sorted[p50Idx], sorted[p95Idx], sorted[p99Idx]
}

// Count returns the total number of values added
func (rs *ReservoirSampler) Count() int64 {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	return rs.count
}

// Reset clears all samples and resets the counter
func (rs *ReservoirSampler) Reset() {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	rs.samples = rs.samples[:0] // Keep capacity, clear length
	rs.count = 0
}

// SimpleStatsTracker tracks basic statistics without storing individual samples
// This is the most memory-efficient option when percentiles aren't critical
type SimpleStatsTracker struct {
	count   int64
	sum     int64
	min     int64
	max     int64
	sumOfSq int64 // For standard deviation calculation if needed
	mu      sync.Mutex
}

// NewSimpleStatsTracker creates a new simple statistics tracker
func NewSimpleStatsTracker() *SimpleStatsTracker {
	return &SimpleStatsTracker{
		min: -1, // Sentinel value for uninitialized
	}
}

// Add adds a new value to the tracker
func (st *SimpleStatsTracker) Add(value int64) {
	st.mu.Lock()
	defer st.mu.Unlock()

	st.count++
	st.sum += value
	st.sumOfSq += value * value

	if st.min == -1 || value < st.min {
		st.min = value
	}
	if value > st.max {
		st.max = value
	}
}

// GetPercentiles returns approximated percentiles based on min/max/average
// This is less accurate but uses minimal memory
func (st *SimpleStatsTracker) GetPercentiles() (p50, p95, p99 int64) {
	st.mu.Lock()
	defer st.mu.Unlock()

	if st.count == 0 {
		return 0, 0, 0
	}

	avg := st.sum / st.count

	// Simple approximation: use average for p50, interpolate for p95/p99
	p50 = avg
	p95 = avg + (st.max-avg)*50/100
	p99 = avg + (st.max-avg)*90/100

	return p50, p95, p99
}

// Count returns the total number of values added
func (st *SimpleStatsTracker) Count() int64 {
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.count
}

// Reset clears all statistics
func (st *SimpleStatsTracker) Reset() {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.count = 0
	st.sum = 0
	st.min = -1
	st.max = 0
	st.sumOfSq = 0
}

// GetAverage returns the average value
func (st *SimpleStatsTracker) GetAverage() int64 {
	st.mu.Lock()
	defer st.mu.Unlock()

	if st.count == 0 {
		return 0
	}
	return st.sum / st.count
}

// GetMin returns the minimum value
func (st *SimpleStatsTracker) GetMin() int64 {
	st.mu.Lock()
	defer st.mu.Unlock()

	if st.min == -1 {
		return 0
	}
	return st.min
}

// GetMax returns the maximum value
func (st *SimpleStatsTracker) GetMax() int64 {
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.max
}
