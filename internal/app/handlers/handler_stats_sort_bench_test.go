package handlers

import (
	"fmt"
	"testing"
	"time"

	"github.com/thushan/olla/internal/core/ports"
)

// benchModelStats builds the collector maps behind /internal/stats/models for
// n models. Every seventh model shares a request count so the sort key ties
// often, as it does in production where most models see little traffic. The
// maps are rebuilt per iteration because buildModelStats sorts the slice it
// appends from a map, and Go randomises map iteration order, so the sort input
// is shuffled the way it is on a live scrape.
func benchModelStats(n int) (map[string]ports.ModelStats, map[string]map[string]ports.EndpointModelStats) {
	modelStats := make(map[string]ports.ModelStats, n)
	modelEndpointStats := make(map[string]map[string]ports.EndpointModelStats, n)

	for i := range n {
		name := fmt.Sprintf("models/%s-%d", []string{"llama", "qwen", "mistral", "gemma"}[i%4], i)
		requests := int64((i / 7) * 13)
		modelStats[name] = ports.ModelStats{
			Name:               name,
			TotalRequests:      requests,
			SuccessfulRequests: requests - (requests % 11),
			FailedRequests:     requests % 11,
			TotalBytes:         requests * 4096,
			AverageLatency:     int64(requests%97) * int64(time.Millisecond),
			P95Latency:         int64(requests%97+40) * int64(time.Millisecond),
			P99Latency:         int64(requests%97+90) * int64(time.Millisecond),
			UniqueClients:      int64(i % 5),
			LastRequested:      time.Now().Add(-time.Duration(i) * time.Minute),
			RoutingHits:        requests,
			RoutingMisses:      int64(i % 3),
			RoutingFallbacks:   int64(i % 2),
		}
		if i%4 == 0 {
			perEndpoint := make(map[string]ports.EndpointModelStats, 3)
			for e := range 3 {
				perEndpoint[fmt.Sprintf("endpoint-%d", e)] = ports.EndpointModelStats{
					EndpointName:      fmt.Sprintf("endpoint-%d", e),
					RequestCount:      requests / 3,
					SuccessRate:       0.98,
					AverageLatency:    int64(requests%97) * int64(time.Millisecond),
					LastUsed:          time.Now().Add(-time.Duration(i) * time.Minute),
					ConsecutiveErrors: 0,
				}
			}
			modelEndpointStats[name] = perEndpoint
		}
	}
	return modelStats, modelEndpointStats
}

// BenchmarkBuildModelStats_Sort covers the sort.Slice in buildModelStats,
// the larger of the two stats handler sorts: ModelStats is a wide struct with
// a map and eight strings, so the swap cost dominates the comparator.
func BenchmarkBuildModelStats_Sort(b *testing.B) {
	app := &Application{}

	for _, size := range []int{500, 5000} {
		b.Run(fmt.Sprintf("models-%d", size), func(b *testing.B) {
			modelStats, modelEndpointStats := benchModelStats(size)
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				models, _ := app.buildModelStats(modelStats, modelEndpointStats, true)
				benchHandlerSortSink = models[0].Name
			}
		})
	}
}

// BenchmarkBuildTranslatorStats_Sort covers the sort.Slice in
// buildTranslatorStats. Production has a handful of translators; the large row
// exists so the sort dominates the formatting work and a change to the
// comparator is visible.
func BenchmarkBuildTranslatorStats_Sort(b *testing.B) {
	app := &Application{}

	for _, size := range []int{3, 1000} {
		b.Run(fmt.Sprintf("translators-%d", size), func(b *testing.B) {
			translatorStats := make(map[string]ports.TranslatorStats, size)
			for i := range size {
				requests := int64((i / 5) * 17)
				name := fmt.Sprintf("translator-%d", i)
				translatorStats[name] = ports.TranslatorStats{
					TranslatorName:       name,
					TotalRequests:        requests,
					SuccessfulRequests:   requests - (requests % 13),
					FailedRequests:       requests % 13,
					PassthroughRequests:  requests * 4 / 5,
					TranslationRequests:  requests / 5,
					StreamingRequests:    requests / 2,
					NonStreamingRequests: requests / 2,
					AverageLatency:       int64(requests%89) * int64(time.Millisecond),
				}
			}

			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				translators := app.buildTranslatorStats(translatorStats)
				benchHandlerSortSink = translators[0].TranslatorName
			}
		})
	}
}

var benchHandlerSortSink string
