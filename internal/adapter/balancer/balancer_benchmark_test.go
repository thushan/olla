package balancer

import (
	"context"
	"fmt"
	"net/url"
	"testing"

	"github.com/thushan/olla/internal/adapter/stats"
	"github.com/thushan/olla/internal/logger"

	"github.com/thushan/olla/internal/core/domain"
)

func createTestFactoryLogger() logger.StyledLogger {
	loggerCfg := &logger.Config{Level: "error", Theme: "default"}
	log, _, _ := logger.New(loggerCfg)
	return logger.NewPlainStyledLogger(log)
}
func NewTestStatsCollector() *stats.Collector {
	return stats.NewCollector(createTestFactoryLogger())
}
func BenchmarkFactory_Create(b *testing.B) {
	factory := NewFactory(NewTestStatsCollector())

	b.ResetTimer()
	b.ReportAllocs()

	for range b.N {
		selector, err := factory.Create(DefaultBalancerPriority)
		if err != nil {
			b.Fatal(err)
		}
		_ = selector
	}
}

func BenchmarkPrioritySelector_Select(b *testing.B) {
	selector := NewPrioritySelector(NewTestStatsCollector())
	ctx := context.Background()

	// Create test endpoints with different prior
	endpoints := make([]*domain.Endpoint, 10)
	for i := range 10 {
		endpoints[i] = createBenchEndpoint(fmt.Sprintf("endpoint-%d", i), 11434+i, domain.StatusHealthy, 100+i*10)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for range b.N {
		_, err := selector.Select(ctx, endpoints)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkPrioritySelector_SelectSamePriority tests weighted selection
func BenchmarkPrioritySelector_SelectSamePriority(b *testing.B) {
	selector := NewPrioritySelector(NewTestStatsCollector())
	ctx := context.Background()

	// Same priority, different statuses for weighted selection
	endpoints := []*domain.Endpoint{
		createBenchEndpoint("healthy", 11434, domain.StatusHealthy, 100),
		createBenchEndpoint("busy", 11435, domain.StatusBusy, 100),
		createBenchEndpoint("warming", 11436, domain.StatusWarming, 100),
	}

	b.ResetTimer()
	b.ReportAllocs()

	for range b.N {
		_, err := selector.Select(ctx, endpoints)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkRoundRobinSelector_Select tests round-robin performance
func BenchmarkRoundRobinSelector_Select(b *testing.B) {
	selector := NewRoundRobinSelector(NewTestStatsCollector())
	ctx := context.Background()

	endpoints := make([]*domain.Endpoint, 10)
	for i := range 10 {
		endpoints[i] = createBenchEndpoint(fmt.Sprintf("endpoint-%d", i), 11434+i, domain.StatusHealthy, 100)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for range b.N {
		_, err := selector.Select(ctx, endpoints)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkLeastConnectionsSelector_Select tests lease connections performance
func BenchmarkLeastConnectionsSelector_Select(b *testing.B) {
	selector := NewLeastConnectionsSelector(NewTestStatsCollector())
	ctx := context.Background()

	endpoints := make([]*domain.Endpoint, 10)
	for i := range 10 {
		endpoints[i] = createBenchEndpoint(fmt.Sprintf("endpoint-%d", i), 11434+i, domain.StatusHealthy, 100)
	}

	// Add some connection counts to make it realistic
	// faking it till we make it :D
	for i, endpoint := range endpoints {
		for range i {
			selector.IncrementConnections(endpoint)
		}
	}

	b.ResetTimer()
	b.ReportAllocs()

	for range b.N {
		_, err := selector.Select(ctx, endpoints)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkConnectionTracking tests connection increment/decrement performance
func BenchmarkConnectionTracking(b *testing.B) {
	mockStatsCollector := NewTestStatsCollector()
	selectors := map[string]domain.EndpointSelector{
		DefaultBalancerPriority:         NewPrioritySelector(mockStatsCollector),
		DefaultBalancerRoundRobin:       NewRoundRobinSelector(mockStatsCollector),
		DefaultBalancerLeastConnections: NewLeastConnectionsSelector(mockStatsCollector),
	}

	endpoint := createBenchEndpoint("test", 11434, domain.StatusHealthy, 100)

	for name, selector := range selectors {
		b.Run(name, func(b *testing.B) {
			b.ResetTimer()
			b.ReportAllocs()

			for range b.N {
				selector.IncrementConnections(endpoint)
				selector.DecrementConnections(endpoint)
			}
		})
	}
}

// BenchmarkConcurrentSelection tests concurrent selection performance
func BenchmarkConcurrentSelection(b *testing.B) {
	mockStatsCollector := NewTestStatsCollector()
	selectors := map[string]domain.EndpointSelector{
		DefaultBalancerPriority:         NewPrioritySelector(mockStatsCollector),
		DefaultBalancerRoundRobin:       NewRoundRobinSelector(mockStatsCollector),
		DefaultBalancerLeastConnections: NewLeastConnectionsSelector(mockStatsCollector),
	}

	endpoints := make([]*domain.Endpoint, 5)
	for i := range 5 {
		endpoints[i] = createBenchEndpoint(fmt.Sprintf("endpoint-%d", i), 11434+i, domain.StatusHealthy, 100+i*50)
	}

	ctx := context.Background()

	for name, selector := range selectors {
		b.Run(name, func(b *testing.B) {
			b.ResetTimer()
			b.ReportAllocs()

			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					_, err := selector.Select(ctx, endpoints)
					if err != nil {
						b.Fatal(err)
					}
				}
			})
		})
	}
}

// BenchmarkLargeEndpointSet tests performance with many endpoints
func BenchmarkLargeEndpointSet(b *testing.B) {
	sizes := []int{10, 50, 100, 500}
	mockStatsCollector := NewTestStatsCollector()

	for _, size := range sizes {
		b.Run(fmt.Sprintf("size-%d", size), func(b *testing.B) {
			selectors := map[string]domain.EndpointSelector{
				DefaultBalancerPriority:         NewPrioritySelector(mockStatsCollector),
				DefaultBalancerRoundRobin:       NewRoundRobinSelector(mockStatsCollector),
				DefaultBalancerLeastConnections: NewLeastConnectionsSelector(mockStatsCollector),
			}

			endpoints := make([]*domain.Endpoint, size)
			for i := range size {
				status := domain.StatusHealthy
				if i%4 == 0 {
					status = domain.StatusBusy
				} else if i%5 == 0 {
					status = domain.StatusWarming
				}
				endpoints[i] = createBenchEndpoint(fmt.Sprintf("endpoint-%d", i), 11434+i, status, 100+i)
			}

			ctx := context.Background()

			for selectorName, selector := range selectors {
				b.Run(selectorName, func(b *testing.B) {
					b.ResetTimer()
					b.ReportAllocs()

					for range b.N {
						_, err := selector.Select(ctx, endpoints)
						if err != nil {
							b.Fatal(err)
						}
					}
				})
			}
		})
	}
}

// BenchmarkFilteringRoutableEndpoints tests the cost of filtering routable endpoints
func BenchmarkFilteringRoutableEndpoints(b *testing.B) {
	// Mix of routable and non-routable endpoints
	endpoints := make([]*domain.Endpoint, 20)
	statuses := []domain.EndpointStatus{
		domain.StatusHealthy, domain.StatusBusy, domain.StatusWarming, // Routable
		domain.StatusOffline, domain.StatusUnhealthy, domain.StatusUnknown, // Not routable
	}

	for i := range 20 {
		endpoints[i] = createBenchEndpoint(
			fmt.Sprintf("endpoint-%d", i),
			11434+i,
			statuses[i%len(statuses)],
			100+i*10,
		)
	}

	mockStatsCollector := NewTestStatsCollector()
	selectors := map[string]domain.EndpointSelector{
		DefaultBalancerPriority:         NewPrioritySelector(mockStatsCollector),
		DefaultBalancerRoundRobin:       NewRoundRobinSelector(mockStatsCollector),
		DefaultBalancerLeastConnections: NewLeastConnectionsSelector(mockStatsCollector),
	}

	ctx := context.Background()

	for name, selector := range selectors {
		b.Run(name, func(b *testing.B) {
			b.ResetTimer()
			b.ReportAllocs()

			for range b.N {
				_, err := selector.Select(ctx, endpoints)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkMemoryUsage tests memory efficiency
func BenchmarkMemoryUsage(b *testing.B) {
	b.Run("factory-creation", func(b *testing.B) {
		b.ReportAllocs()

		for range b.N {
			factory := NewFactory(NewTestStatsCollector())
			_ = factory
		}
	})

	b.Run("selector-creation", func(b *testing.B) {
		factory := NewFactory(NewTestStatsCollector())

		b.ResetTimer()
		b.ReportAllocs()

		for range b.N {
			priority, _ := factory.Create(DefaultBalancerPriority)
			roundRobin, _ := factory.Create(DefaultBalancerRoundRobin)
			leastConn, _ := factory.Create(DefaultBalancerLeastConnections)

			_ = priority
			_ = roundRobin
			_ = leastConn
		}
	})

	b.Run("connection-tracking", func(b *testing.B) {
		selector := NewPrioritySelector(NewTestStatsCollector())
		endpoints := make([]*domain.Endpoint, 100)

		for i := range 100 {
			endpoints[i] = createBenchEndpoint(fmt.Sprintf("endpoint-%d", i), 11434+i, domain.StatusHealthy, 100)
		}

		b.ResetTimer()
		b.ReportAllocs()

		for i := range b.N {
			endpoint := endpoints[i%len(endpoints)]
			selector.IncrementConnections(endpoint)
		}
	})
}

// BenchmarkPrioritySelector_ConnectionStats tests stats performance
func BenchmarkPrioritySelector_ConnectionStats(b *testing.B) {
	selector := NewPrioritySelector(NewTestStatsCollector())

	// Add connections to various endpoints
	for i := range 50 {
		endpoint := createBenchEndpoint(fmt.Sprintf("endpoint-%d", i), 11434+i, domain.StatusHealthy, 100)
		for range i % 10 {
			selector.IncrementConnections(endpoint)
		}
	}

	b.ResetTimer()
	b.ReportAllocs()

	for range b.N {
		stats := selector.statsCollector.GetConnectionStats()
		_ = stats
	}
}

// BenchmarkConcurrentConnectionTracking tests concurrent connection updates
func BenchmarkConcurrentConnectionTracking(b *testing.B) {
	selector := NewPrioritySelector(NewTestStatsCollector())
	endpoints := make([]*domain.Endpoint, 10)

	for i := range 10 {
		endpoints[i] = createBenchEndpoint(fmt.Sprintf("endpoint-%d", i), 11434+i, domain.StatusHealthy, 100)
	}

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			endpoint := endpoints[0] // All goroutines update same endpoint
			selector.IncrementConnections(endpoint)
			selector.DecrementConnections(endpoint)
		}
	})
}

// BenchmarkRealWorldScenario simulates realistic usage patterns
func BenchmarkRealWorldScenario(b *testing.B) {
	factory := NewFactory(NewTestStatsCollector())
	selector, _ := factory.Create("priority")

	// Create endpoints with realistic distribution
	endpoints := []*domain.Endpoint{
		createBenchEndpoint("primary", 11434, domain.StatusHealthy, 300),   // High priority, healthy
		createBenchEndpoint("secondary", 11435, domain.StatusHealthy, 200), // Medium priority, healthy
		createBenchEndpoint("tertiary", 11436, domain.StatusBusy, 100),     // Low priority, busy
		createBenchEndpoint("backup", 11437, domain.StatusWarming, 50),     // Lowest priority, warming
		createBenchEndpoint("offline", 11438, domain.StatusOffline, 400),   // Highest priority but offline
	}

	ctx := context.Background()

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			endpoint, err := selector.Select(ctx, endpoints)
			if err != nil {
				b.Fatal(err)
			}

			selector.IncrementConnections(endpoint)

			// Simulate some worky work (occasionally decrement)
			if b.N%10 == 0 {
				selector.DecrementConnections(endpoint)
			}
		}
	})
}

func createBenchEndpoint(name string, port int, status domain.EndpointStatus, priority int) *domain.Endpoint {
	testURL, _ := url.Parse(fmt.Sprintf("http://localhost:%d", port))
	healthURL, _ := url.Parse(fmt.Sprintf("http://localhost:%d/health", port))
	return &domain.Endpoint{
		Name:           name,
		URL:            testURL,
		HealthCheckURL: healthURL,
		Status:         status,
		Priority:       priority,
	}
}
