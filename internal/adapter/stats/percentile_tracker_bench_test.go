package stats

import (
	"fmt"
	"testing"
)

var benchPercentileSink int64

// BenchmarkReservoirSampler_GetPercentiles times the percentile read on a full
// reservoir. BenchmarkReservoirSampler times Add, which never sorts; the sort
// in GetPercentiles runs under the sampler lock on every stats scrape, so this
// is the row that covers the slicessort finding on percentile_tracker.go.
func BenchmarkReservoirSampler_GetPercentiles(b *testing.B) {
	// 100 is the model collector's default reservoir size; 1000 shows how the
	// sort cost grows with the sample size an operator configures.
	for _, sampleSize := range []int{100, 1000} {
		b.Run(fmt.Sprintf("samples-%d", sampleSize), func(b *testing.B) {
			rs := NewReservoirSampler(sampleSize)
			// Latency-shaped values in nanoseconds with a long tail, so the sort
			// sees a realistic distribution rather than an already-ordered run.
			for i := range sampleSize {
				rs.Add(int64(1_000_000 + (i*7919)%50_000_000))
			}

			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				p50, p95, p99 := rs.GetPercentiles()
				benchPercentileSink = p50 + p95 + p99
			}
		})
	}
}
