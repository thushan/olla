package stats

import (
	"testing"
)

func TestReservoirSampler(t *testing.T) {
	t.Run("Basic functionality", func(t *testing.T) {
		rs := NewReservoirSampler(10)

		// Add some values
		for i := int64(1); i <= 20; i++ {
			rs.Add(i * 10)
		}

		if rs.Count() != 20 {
			t.Errorf("Expected count 20, got %d", rs.Count())
		}

		p50, p95, p99 := rs.GetPercentiles()
		if p50 == 0 || p95 == 0 || p99 == 0 {
			t.Error("Percentiles should not be zero")
		}

		// P50 should be less than or equal to P95 which should be less than or equal to P99
		// With small sample sizes, these might be equal
		if p50 > p95 || p95 > p99 {
			t.Errorf("Invalid percentile ordering: p50=%d, p95=%d, p99=%d", p50, p95, p99)
		}
	})

	t.Run("Empty sampler", func(t *testing.T) {
		rs := NewReservoirSampler(10)

		p50, p95, p99 := rs.GetPercentiles()
		if p50 != 0 || p95 != 0 || p99 != 0 {
			t.Error("Empty sampler should return zero percentiles")
		}
	})

	t.Run("Single value", func(t *testing.T) {
		rs := NewReservoirSampler(10)
		rs.Add(100)

		p50, p95, p99 := rs.GetPercentiles()
		if p50 != 100 || p95 != 100 || p99 != 100 {
			t.Error("Single value should return same value for all percentiles")
		}
	})

	t.Run("Reset functionality", func(t *testing.T) {
		rs := NewReservoirSampler(10)

		for i := 0; i < 100; i++ {
			rs.Add(int64(i))
		}

		rs.Reset()

		if rs.Count() != 0 {
			t.Error("Count should be 0 after reset")
		}

		p50, p95, p99 := rs.GetPercentiles()
		if p50 != 0 || p95 != 0 || p99 != 0 {
			t.Error("Percentiles should be 0 after reset")
		}
	})
}

func TestSimpleStatsTracker(t *testing.T) {
	t.Run("Basic functionality", func(t *testing.T) {
		st := NewSimpleStatsTracker()

		// Add values 10, 20, 30, 40, 50
		values := []int64{10, 20, 30, 40, 50}
		for _, v := range values {
			st.Add(v)
		}

		if st.Count() != 5 {
			t.Errorf("Expected count 5, got %d", st.Count())
		}

		if avg := st.GetAverage(); avg != 30 {
			t.Errorf("Expected average 30, got %d", avg)
		}

		if min := st.GetMin(); min != 10 {
			t.Errorf("Expected min 10, got %d", min)
		}

		if max := st.GetMax(); max != 50 {
			t.Errorf("Expected max 50, got %d", max)
		}
	})

	t.Run("Empty tracker", func(t *testing.T) {
		st := NewSimpleStatsTracker()

		if st.Count() != 0 {
			t.Error("Empty tracker should have count 0")
		}

		if avg := st.GetAverage(); avg != 0 {
			t.Error("Empty tracker should have average 0")
		}

		p50, p95, p99 := st.GetPercentiles()
		if p50 != 0 || p95 != 0 || p99 != 0 {
			t.Error("Empty tracker should return zero percentiles")
		}
	})

	t.Run("Reset functionality", func(t *testing.T) {
		st := NewSimpleStatsTracker()

		for i := 0; i < 100; i++ {
			st.Add(int64(i))
		}

		st.Reset()

		if st.Count() != 0 {
			t.Error("Count should be 0 after reset")
		}

		if st.GetMin() != 0 || st.GetMax() != 0 {
			t.Error("Min/Max should be reset")
		}
	})
}

func BenchmarkReservoirSampler(b *testing.B) {
	rs := NewReservoirSampler(100)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rs.Add(int64(i % 1000))
	}
}

func BenchmarkSimpleStatsTracker(b *testing.B) {
	st := NewSimpleStatsTracker()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		st.Add(int64(i % 1000))
	}
}

func BenchmarkArrayImplementation(b *testing.B) {
	// Simulate current implementation with 1000-element array
	latencies := make([]int64, 1000)
	index := 0

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		latencies[index] = int64(i % 1000)
		index = (index + 1) % 1000
	}
}

// Memory allocation benchmark
func BenchmarkMemoryAllocation(b *testing.B) {
	b.Run("Current_1000_Array", func(b *testing.B) {
		b.ReportAllocs()
		var sink interface{}
		for i := 0; i < b.N; i++ {
			sink = make([]int64, 1000)
		}
		_ = sink
	})

	b.Run("ReservoirSampler_100", func(b *testing.B) {
		b.ReportAllocs()
		var sink interface{}
		for i := 0; i < b.N; i++ {
			sink = NewReservoirSampler(100)
		}
		_ = sink
	})

	b.Run("SimpleStatsTracker", func(b *testing.B) {
		b.ReportAllocs()
		var sink interface{}
		for i := 0; i < b.N; i++ {
			sink = NewSimpleStatsTracker()
		}
		_ = sink
	})
}

// Size comparison benchmark
func BenchmarkMemorySize(b *testing.B) {
	b.Run("ModelData_Old_100Models", func(b *testing.B) {
		b.ReportAllocs()
		// Simulate old model data structure with 100 models
		models := make([]struct {
			latencies []int64
			other     [100]byte // Simulate other fields
		}, 100)

		for i := range models {
			models[i].latencies = make([]int64, 1000)
		}
		b.ReportMetric(float64(len(models)*1000*8), "bytes/total")
	})

	b.Run("ModelData_New_100Models", func(b *testing.B) {
		b.ReportAllocs()
		// Simulate new model data structure with 100 models
		models := make([]struct {
			tracker PercentileTracker
			other   [100]byte // Simulate other fields
		}, 100)

		for i := range models {
			models[i].tracker = NewReservoirSampler(100)
		}
		b.ReportMetric(float64(len(models)*100*8), "bytes/total")
	})
}
