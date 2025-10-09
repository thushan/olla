package unifier

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/thushan/olla/internal/core/domain"
)

// BenchmarkCatalogStore_GetModel measures read performance
func BenchmarkCatalogStore_GetModel(b *testing.B) {
	store := NewCatalogStore(5 * time.Minute)

	// Pre-populate store with test models
	for i := 0; i < 100; i++ {
		model := &domain.UnifiedModel{
			ID:     fmt.Sprintf("model-%d", i),
			Family: fmt.Sprintf("family-%d", i%10),
			Aliases: []domain.AliasEntry{
				{Name: fmt.Sprintf("alias-%d", i), Source: "ollama"},
			},
			SourceEndpoints: []domain.SourceEndpoint{
				{
					EndpointURL:  fmt.Sprintf("http://localhost:%d", 11434+i),
					EndpointName: fmt.Sprintf("endpoint-%d", i),
					NativeName:   fmt.Sprintf("model-%d", i),
					DiskSize:     int64(i * 1024 * 1024),
					LastSeen:     time.Now(),
				},
			},
			Capabilities: []string{"chat", "streaming", "function_calling"},
			Metadata:     map[string]interface{}{"digest": fmt.Sprintf("sha256:%d", i)},
			LastSeen:     time.Now(),
		}
		store.PutModel(model)
	}

	b.Run("Sequential", func(b *testing.B) {
		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_, _ = store.GetModel(fmt.Sprintf("model-%d", i%100))
		}
	})

	b.Run("Parallel", func(b *testing.B) {
		b.ResetTimer()
		b.ReportAllocs()
		b.RunParallel(func(pb *testing.PB) {
			i := 0
			for pb.Next() {
				_, _ = store.GetModel(fmt.Sprintf("model-%d", i%100))
				i++
			}
		})
	})
}

// BenchmarkCatalogStore_PutModel measures write performance
func BenchmarkCatalogStore_PutModel(b *testing.B) {
	store := NewCatalogStore(5 * time.Minute)

	createModel := func(i int) *domain.UnifiedModel {
		return &domain.UnifiedModel{
			ID:     fmt.Sprintf("model-%d", i),
			Family: fmt.Sprintf("family-%d", i%10),
			Aliases: []domain.AliasEntry{
				{Name: fmt.Sprintf("alias-%d", i), Source: "ollama"},
			},
			SourceEndpoints: []domain.SourceEndpoint{
				{
					EndpointURL:  fmt.Sprintf("http://localhost:%d", 11434+i),
					EndpointName: fmt.Sprintf("endpoint-%d", i),
					NativeName:   fmt.Sprintf("model-%d", i),
					DiskSize:     int64(i * 1024 * 1024),
					LastSeen:     time.Now(),
				},
			},
			Capabilities: []string{"chat", "streaming", "function_calling"},
			Metadata:     map[string]interface{}{"digest": fmt.Sprintf("sha256:%d", i)},
			LastSeen:     time.Now(),
		}
	}

	b.Run("Sequential", func(b *testing.B) {
		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			model := createModel(i)
			store.PutModel(model)
		}
	})

	b.Run("Parallel", func(b *testing.B) {
		b.ResetTimer()
		b.ReportAllocs()
		b.RunParallel(func(pb *testing.PB) {
			i := 0
			for pb.Next() {
				model := createModel(i)
				store.PutModel(model)
				i++
			}
		})
	})
}

// BenchmarkCatalogStore_GetAllModels measures GetAllModels performance
func BenchmarkCatalogStore_GetAllModels(b *testing.B) {
	sizes := []int{10, 100, 1000}

	for _, size := range sizes {
		b.Run(fmt.Sprintf("Size_%d", size), func(b *testing.B) {
			store := NewCatalogStore(5 * time.Minute)

			// Pre-populate
			for i := 0; i < size; i++ {
				model := &domain.UnifiedModel{
					ID:     fmt.Sprintf("model-%d", i),
					Family: fmt.Sprintf("family-%d", i%10),
					Aliases: []domain.AliasEntry{
						{Name: fmt.Sprintf("alias-%d", i), Source: "ollama"},
					},
					SourceEndpoints: []domain.SourceEndpoint{
						{
							EndpointURL:  fmt.Sprintf("http://localhost:%d", 11434+i),
							EndpointName: fmt.Sprintf("endpoint-%d", i),
							NativeName:   fmt.Sprintf("model-%d", i),
							DiskSize:     int64(i * 1024 * 1024),
							LastSeen:     time.Now(),
						},
					},
					Capabilities: []string{"chat", "streaming"},
					Metadata:     map[string]interface{}{"digest": fmt.Sprintf("sha256:%d", i)},
					LastSeen:     time.Now(),
				}
				store.PutModel(model)
			}

			b.ResetTimer()
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				models := store.GetAllModels()
				_ = models
			}
		})
	}
}

// BenchmarkCatalogStore_ResolveByName measures name resolution performance
func BenchmarkCatalogStore_ResolveByName(b *testing.B) {
	store := NewCatalogStore(5 * time.Minute)

	// Pre-populate with models
	for i := 0; i < 100; i++ {
		model := &domain.UnifiedModel{
			ID:     fmt.Sprintf("model-%d", i),
			Family: fmt.Sprintf("family-%d", i%10),
			Aliases: []domain.AliasEntry{
				{Name: fmt.Sprintf("alias-%d", i), Source: "ollama"},
				{Name: fmt.Sprintf("alt-%d", i), Source: "lmstudio"},
			},
			SourceEndpoints: []domain.SourceEndpoint{
				{
					EndpointURL:  fmt.Sprintf("http://localhost:%d", 11434+i),
					EndpointName: fmt.Sprintf("endpoint-%d", i),
					NativeName:   fmt.Sprintf("model-%d", i),
					DiskSize:     int64(i * 1024 * 1024),
					LastSeen:     time.Now(),
				},
			},
			Capabilities: []string{"chat", "streaming"},
			Metadata:     map[string]interface{}{"digest": fmt.Sprintf("sha256:%d", i)},
			LastSeen:     time.Now(),
		}
		store.PutModel(model)
	}

	b.Run("DirectID", func(b *testing.B) {
		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_, _ = store.ResolveByName(fmt.Sprintf("model-%d", i%100))
		}
	})

	b.Run("ByAlias", func(b *testing.B) {
		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_, _ = store.ResolveByName(fmt.Sprintf("alias-%d", i%100))
		}
	})

	b.Run("Parallel", func(b *testing.B) {
		b.ResetTimer()
		b.ReportAllocs()
		b.RunParallel(func(pb *testing.PB) {
			i := 0
			for pb.Next() {
				_, _ = store.ResolveByName(fmt.Sprintf("model-%d", i%100))
				i++
			}
		})
	})
}

// BenchmarkCatalogStore_MixedWorkload simulates realistic read/write ratios
func BenchmarkCatalogStore_MixedWorkload(b *testing.B) {
	store := NewCatalogStore(5 * time.Minute)

	// Pre-populate
	for i := 0; i < 100; i++ {
		model := &domain.UnifiedModel{
			ID:     fmt.Sprintf("model-%d", i),
			Family: fmt.Sprintf("family-%d", i%10),
			Aliases: []domain.AliasEntry{
				{Name: fmt.Sprintf("alias-%d", i), Source: "ollama"},
			},
			SourceEndpoints: []domain.SourceEndpoint{
				{
					EndpointURL:  fmt.Sprintf("http://localhost:%d", 11434+i),
					EndpointName: fmt.Sprintf("endpoint-%d", i),
					NativeName:   fmt.Sprintf("model-%d", i),
					DiskSize:     int64(i * 1024 * 1024),
					LastSeen:     time.Now(),
				},
			},
			Capabilities: []string{"chat", "streaming"},
			Metadata:     map[string]interface{}{"digest": fmt.Sprintf("sha256:%d", i)},
			LastSeen:     time.Now(),
		}
		store.PutModel(model)
	}

	// 90/10 read/write ratio (typical for production)
	b.Run("90_10_ReadWrite", func(b *testing.B) {
		b.ResetTimer()
		b.ReportAllocs()
		b.RunParallel(func(pb *testing.PB) {
			i := 0
			for pb.Next() {
				if i%10 == 0 {
					// 10% writes
					model := &domain.UnifiedModel{
						ID:       fmt.Sprintf("model-%d", i%100),
						Family:   "updated",
						LastSeen: time.Now(),
					}
					store.PutModel(model)
				} else {
					// 90% reads
					_, _ = store.GetModel(fmt.Sprintf("model-%d", i%100))
				}
				i++
			}
		})
	})

	// Heavy read workload (99/1 ratio - closer to actual usage)
	b.Run("99_1_ReadWrite", func(b *testing.B) {
		b.ResetTimer()
		b.ReportAllocs()
		b.RunParallel(func(pb *testing.PB) {
			i := 0
			for pb.Next() {
				if i%100 == 0 {
					// 1% writes
					model := &domain.UnifiedModel{
						ID:       fmt.Sprintf("model-%d", i%100),
						Family:   "updated",
						LastSeen: time.Now(),
					}
					store.PutModel(model)
				} else {
					// 99% reads
					_, _ = store.GetModel(fmt.Sprintf("model-%d", i%100))
				}
				i++
			}
		})
	})
}

// BenchmarkDefaultUnifier_UnifyModels benchmarks the full unification flow
func BenchmarkDefaultUnifier_UnifyModels(b *testing.B) {
	unifier := NewDefaultUnifier().(*DefaultUnifier)
	ctx := context.Background()

	endpoint := &domain.Endpoint{
		URLString: "http://localhost:11434",
		Name:      "test-endpoint",
		Type:      "ollama",
	}

	// Create realistic model data
	models := make([]*domain.ModelInfo, 10)
	for i := 0; i < 10; i++ {
		digest := fmt.Sprintf("sha256:abc%d", i)
		format := "gguf"
		paramSize := "7B"
		models[i] = &domain.ModelInfo{
			Name: fmt.Sprintf("llama3-%d:latest", i),
			Size: int64(i * 4 * 1024 * 1024 * 1024),
			Details: &domain.ModelDetails{
				Digest:        &digest,
				Format:        &format,
				ParameterSize: &paramSize,
			},
			LastSeen: time.Now(),
		}
	}

	b.Run("FirstUnification", func(b *testing.B) {
		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			b.StopTimer()
			unifier.Clear(ctx)
			b.StartTimer()
			_, _ = unifier.UnifyModels(ctx, models, endpoint)
		}
	})

	b.Run("UpdateExisting", func(b *testing.B) {
		// Pre-populate
		unifier.UnifyModels(ctx, models, endpoint)

		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_, _ = unifier.UnifyModels(ctx, models, endpoint)
		}
	})
}
