package stats

import (
	"fmt"
	"math/rand/v2"
	"testing"
)

var benchPercentileSink int64

// benchLatencyValues returns n latency-shaped values in nanoseconds (a base
// around 1 ms, a ramp to 6 ms that wraps every 632 samples, and every 50th
// value ten times larger as the tail), shuffled with a fixed-seed generator so
// the order is the same on every run. The shuffle matters: GetPercentiles
// sorts a copy of the reservoir, and pdqsort detects an already-sorted input
// and does no swaps, so ascending values would time the sort's best case and
// hide a regression in it.
func benchLatencyValues(n int) []int64 {
	values := make([]int64, n)
	for i := range n {
		v := int64(1_000_000 + (i*7919)%5_000_000)
		if i%50 == 0 {
			v *= 10
		}
		values[i] = v
	}
	rng := rand.New(rand.NewPCG(1, 2))
	rng.Shuffle(len(values), func(i, j int) {
		values[i], values[j] = values[j], values[i]
	})
	return values
}

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
			for _, v := range benchLatencyValues(sampleSize) {
				rs.Add(v)
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
