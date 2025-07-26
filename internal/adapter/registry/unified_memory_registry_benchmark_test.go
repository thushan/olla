package registry

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/thushan/olla/internal/core/domain"
)

func BenchmarkGetHealthyEndpointsForModel(b *testing.B) {
	ctx := context.Background()
	registry := createTestUnifiedRegistry()

	// Create mock endpoint repository
	endpoints := make([]*domain.Endpoint, 100)
	for i := 0; i < 100; i++ {
		status := domain.StatusHealthy
		if i%3 == 0 {
			status = domain.StatusUnhealthy
		}
		endpoints[i] = &domain.Endpoint{
			URLString: fmt.Sprintf("http://localhost:%d", 11434+i),
			Name:      fmt.Sprintf("endpoint-%d", i),
			Status:    status,
		}
	}

	mockRepo := &mockEndpointRepository{endpoints: endpoints}

	// Register models on endpoints
	models := []string{"llama3:8b", "mistral:7b", "phi:3b", "qwen:14b", "deepseek:32b"}
	for i, ep := range endpoints {
		// Register 1-3 models per endpoint
		numModels := (i % 3) + 1
		for j := 0; j < numModels; j++ {
			modelIdx := (i + j) % len(models)
			model := &domain.ModelInfo{
				Name:     models[modelIdx],
				LastSeen: time.Now(),
			}
			registry.RegisterModel(ctx, ep.URLString, model)
		}
	}

	// Benchmark different scenarios
	b.Run("ExistingModel", func(b *testing.B) {
		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			endpoints, err := registry.GetHealthyEndpointsForModel(ctx, "llama3:8b", mockRepo)
			if err != nil {
				b.Fatal(err)
			}
			_ = endpoints
		}
	})

	b.Run("NonExistentModel", func(b *testing.B) {
		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			endpoints, err := registry.GetHealthyEndpointsForModel(ctx, "gpt-4", mockRepo)
			if err != nil {
				b.Fatal(err)
			}
			_ = endpoints
		}
	})

	b.Run("Parallel", func(b *testing.B) {
		b.ResetTimer()
		b.ReportAllocs()
		b.RunParallel(func(pb *testing.PB) {
			modelIdx := 0
			for pb.Next() {
				model := models[modelIdx%len(models)]
				endpoints, err := registry.GetHealthyEndpointsForModel(ctx, model, mockRepo)
				if err != nil {
					b.Fatal(err)
				}
				_ = endpoints
				modelIdx++
			}
		})
	})
}

func BenchmarkGetModelsByCapability(b *testing.B) {
	ctx := context.Background()
	registry := createTestUnifiedRegistry()

	// Create test models with various capabilities
	capabilities := [][]string{
		{"chat", "streaming"},
		{"embeddings"},
		{"chat", "vision", "streaming"},
		{"chat", "code", "function_calling"},
		{"text", "completion"},
		{"chat", "function_calling", "streaming"},
	}

	// Add 1000 models to test at scale
	for i := 0; i < 1000; i++ {
		model := &domain.UnifiedModel{
			ID:           fmt.Sprintf("model-%d", i),
			Family:       fmt.Sprintf("family-%d", i%10),
			Capabilities: capabilities[i%len(capabilities)],
		}
		registry.globalUnified.Store(model.ID, model)
	}

	// Benchmark different capabilities
	testCases := []string{"chat", "embeddings", "vision", "code", "streaming", "unknown"}

	for _, capability := range testCases {
		b.Run(capability, func(b *testing.B) {
			b.ResetTimer()
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				models, err := registry.GetModelsByCapability(ctx, capability)
				if err != nil {
					b.Fatal(err)
				}
				_ = models
			}
		})
	}

	b.Run("Parallel", func(b *testing.B) {
		b.ResetTimer()
		b.ReportAllocs()
		b.RunParallel(func(pb *testing.PB) {
			capIdx := 0
			for pb.Next() {
				capability := testCases[capIdx%len(testCases)]
				models, err := registry.GetModelsByCapability(ctx, capability)
				if err != nil {
					b.Fatal(err)
				}
				_ = models
				capIdx++
			}
		})
	})
}

func BenchmarkRegistryWithXSync(b *testing.B) {
	ctx := context.Background()
	registry := createTestUnifiedRegistry()

	// Benchmark concurrent model registration with xsync
	b.Run("ConcurrentRegistration", func(b *testing.B) {
		b.ResetTimer()
		b.ReportAllocs()
		b.RunParallel(func(pb *testing.PB) {
			i := 0
			for pb.Next() {
				model := &domain.ModelInfo{
					Name:     fmt.Sprintf("model-%d", i),
					LastSeen: time.Now(),
				}
				endpoint := fmt.Sprintf("http://localhost:%d", 11434+(i%100))
				err := registry.RegisterModel(ctx, endpoint, model)
				if err != nil {
					b.Fatal(err)
				}
				i++
			}
		})
	})

	// Benchmark concurrent reads with xsync
	b.Run("ConcurrentReads", func(b *testing.B) {
		// Pre-populate with some data
		for i := 0; i < 100; i++ {
			model := &domain.ModelInfo{
				Name:     fmt.Sprintf("read-model-%d", i),
				LastSeen: time.Now(),
			}
			endpoint := fmt.Sprintf("http://localhost:%d", 11434+i)
			registry.RegisterModel(ctx, endpoint, model)
		}

		b.ResetTimer()
		b.ReportAllocs()
		b.RunParallel(func(pb *testing.PB) {
			i := 0
			for pb.Next() {
				modelName := fmt.Sprintf("read-model-%d", i%100)
				available := registry.IsModelAvailable(ctx, modelName)
				_ = available
				i++
			}
		})
	})
}

func BenchmarkMemoryUsage(b *testing.B) {
	ctx := context.Background()

	// Test memory usage with different numbers of models
	testSizes := []int{100, 1000, 10000}

	for _, size := range testSizes {
		b.Run(fmt.Sprintf("Models_%d", size), func(b *testing.B) {
			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				b.StopTimer()
				registry := createTestUnifiedRegistry()
				b.StartTimer()

				// Register models
				for j := 0; j < size; j++ {
					model := &domain.ModelInfo{
						Name:        fmt.Sprintf("model-%d", j),
						LastSeen:    time.Now(),
						Size:        int64(j * 1024 * 1024), // Simulate different sizes
						Description: fmt.Sprintf("Test model %d with some description text", j),
					}
					endpoint := fmt.Sprintf("http://localhost:%d", 11434+(j%10))
					registry.RegisterModel(ctx, endpoint, model)
				}

				// Force a stats update to measure full memory impact
				_, _ = registry.GetStats(ctx)
			}
		})
	}
}
