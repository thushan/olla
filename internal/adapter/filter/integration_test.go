package filter_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thushan/olla/internal/adapter/filter"
	"github.com/thushan/olla/internal/adapter/registry/profile"
	"github.com/thushan/olla/internal/core/domain"
)

func TestProfileFilteringIntegration(t *testing.T) {
	tests := []struct {
		name             string
		filterConfig     *domain.FilterConfig
		expectedProfiles []string
	}{
		{
			name: "exclude vllm profile",
			filterConfig: &domain.FilterConfig{
				Exclude: []string{"vllm"},
			},
			expectedProfiles: []string{
				domain.ProfileOllama,
				domain.ProfileLmStudio,
				domain.ProfileOpenAICompatible,
			},
		},
		{
			name: "include only ollama and lm-studio",
			filterConfig: &domain.FilterConfig{
				Include: []string{"ollama", "lm-studio"},
			},
			expectedProfiles: []string{
				domain.ProfileOllama,
				domain.ProfileLmStudio,
			},
		},
		{
			name: "include all profiles starting with 'o'",
			filterConfig: &domain.FilterConfig{
				Include: []string{"o*"},
			},
			expectedProfiles: []string{
				domain.ProfileOllama,
				domain.ProfileOpenAICompatible,
			},
		},
		{
			name: "exclude profiles containing 'studio'",
			filterConfig: &domain.FilterConfig{
				Exclude: []string{"*studio*"},
			},
			expectedProfiles: []string{
				domain.ProfileOllama,
				domain.ProfileOpenAICompatible,
			},
		},
		{
			name: "include ollama but exclude all",
			filterConfig: &domain.FilterConfig{
				Include: []string{"ollama"},
				Exclude: []string{"*"},
			},
			expectedProfiles: []string{}, // Exclude takes precedence
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create profile loader with filter
			loader := profile.NewProfileLoaderWithFilter(
				"testdata/profiles", // Non-existent dir, will use built-ins
				tt.filterConfig,
				filter.NewGlobFilter(),
			)

			// Load profiles
			err := loader.LoadProfiles()
			require.NoError(t, err)

			// Get all loaded profiles
			profiles := loader.GetAllProfiles()

			// Extract profile names
			var profileNames []string
			for name := range profiles {
				profileNames = append(profileNames, name)
			}

			// Check expected profiles are present
			assert.ElementsMatch(t, tt.expectedProfiles, profileNames)
		})
	}
}

func TestModelFilteringIntegration(t *testing.T) {
	ctx := context.Background()

	// Create sample models
	models := []*domain.ModelInfo{
		{Name: "llama3-8b", Type: "chat"},
		{Name: "deepseek-coder-v2", Type: "code"},
		{Name: "deepseek-r1", Type: "reasoning"},
		{Name: "codellama-7b", Type: "code"},
		{Name: "mistral-7b", Type: "chat"},
		{Name: "qwen2-7b", Type: "chat"},
	}

	tests := []struct {
		name          string
		filterConfig  *domain.FilterConfig
		expectedCount int
		expectedNames []string
	}{
		{
			name: "exclude deepseek models",
			filterConfig: &domain.FilterConfig{
				Exclude: []string{"deepseek*"},
			},
			expectedCount: 4,
			expectedNames: []string{"llama3-8b", "codellama-7b", "mistral-7b", "qwen2-7b"},
		},
		{
			name: "include only llama family models",
			filterConfig: &domain.FilterConfig{
				Include: []string{"*llama*"},
			},
			expectedCount: 2,
			expectedNames: []string{"llama3-8b", "codellama-7b"},
		},
		{
			name: "include 7b models",
			filterConfig: &domain.FilterConfig{
				Include: []string{"*-7b"},
			},
			expectedCount: 3,
			expectedNames: []string{"codellama-7b", "mistral-7b", "qwen2-7b"},
		},
		{
			name: "include llama but exclude code models",
			filterConfig: &domain.FilterConfig{
				Include: []string{"*llama*"},
				Exclude: []string{"code*"},
			},
			expectedCount: 1,
			expectedNames: []string{"llama3-8b"},
		},
		{
			name: "complex filter with multiple patterns",
			filterConfig: &domain.FilterConfig{
				Include: []string{"llama*", "mistral*", "qwen*", "code*"},
				Exclude: []string{"*-8b"},
			},
			expectedCount: 3,
			expectedNames: []string{"codellama-7b", "mistral-7b", "qwen2-7b"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create filter
			f := filter.NewGlobFilter()

			// Apply filter
			result, err := f.Apply(ctx, tt.filterConfig, models, func(item interface{}) string {
				if model, ok := item.(*domain.ModelInfo); ok {
					return model.Name
				}
				return ""
			})
			require.NoError(t, err)

			// Check count
			assert.Equal(t, tt.expectedCount, len(result.Accepted))

			// Extract filtered model names
			var filteredNames []string
			for _, item := range result.Accepted {
				if model, ok := item.(*domain.ModelInfo); ok {
					filteredNames = append(filteredNames, model.Name)
				}
			}

			// Check expected models
			assert.ElementsMatch(t, tt.expectedNames, filteredNames)
		})
	}
}

func TestFilterRepositoryIntegration(t *testing.T) {
	ctx := context.Background()
	repo := filter.NewMemoryFilterRepository().(*filter.MemoryFilterRepository)

	// Test storing and retrieving filter configurations
	t.Run("store and retrieve filter configs", func(t *testing.T) {
		// Create filter configs
		profileFilter := &domain.FilterConfig{
			Include: []string{"ollama", "lmstudio"},
			Exclude: []string{"vllm"},
		}

		modelFilter := &domain.FilterConfig{
			Exclude: []string{"deepseek*", "*-coder*"},
		}

		// Store filters
		err := repo.SetFilterConfig(ctx, "profile-filter", profileFilter)
		require.NoError(t, err)

		err = repo.SetFilterConfig(ctx, "model-filter", modelFilter)
		require.NoError(t, err)

		// Retrieve and verify
		retrieved, err := repo.GetFilterConfig(ctx, "profile-filter")
		require.NoError(t, err)
		assert.Equal(t, profileFilter.Include, retrieved.Include)
		assert.Equal(t, profileFilter.Exclude, retrieved.Exclude)

		retrieved, err = repo.GetFilterConfig(ctx, "model-filter")
		require.NoError(t, err)
		assert.Equal(t, modelFilter.Exclude, retrieved.Exclude)
	})

	// Test validation
	t.Run("validate filter config", func(t *testing.T) {
		invalidFilter := &domain.FilterConfig{
			Include: []string{""},
			Exclude: []string{"  "},
		}

		err := repo.ValidateAndStore(ctx, "invalid-filter", invalidFilter)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid filter configuration")
	})

	// Test concurrent access
	t.Run("concurrent filter operations", func(t *testing.T) {
		done := make(chan bool)

		// Writer goroutine
		go func() {
			for i := 0; i < 100; i++ {
				config := &domain.FilterConfig{
					Include: []string{"pattern" + string(rune(i))},
				}
				_ = repo.SetFilterConfig(ctx, "concurrent-test", config)
				time.Sleep(time.Microsecond)
			}
			done <- true
		}()

		// Reader goroutine
		go func() {
			for i := 0; i < 100; i++ {
				_, _ = repo.GetFilterConfig(ctx, "concurrent-test")
				time.Sleep(time.Microsecond)
			}
			done <- true
		}()

		// Wait for both goroutines
		<-done
		<-done

		// Verify final state
		final, err := repo.GetFilterConfig(ctx, "concurrent-test")
		assert.NoError(t, err)
		assert.NotNil(t, final)
	})
}

func TestFilterConfigOperations(t *testing.T) {
	t.Run("merge filter configs", func(t *testing.T) {
		base := &domain.FilterConfig{
			Include: []string{"pattern1"},
			Exclude: []string{"exclude1"},
		}

		override := &domain.FilterConfig{
			Include: []string{"pattern2"},
			Exclude: []string{"exclude2"},
		}

		merged := base.Merge(override)

		assert.ElementsMatch(t, []string{"pattern1", "pattern2"}, merged.Include)
		assert.ElementsMatch(t, []string{"exclude1", "exclude2"}, merged.Exclude)
	})

	t.Run("clone filter config", func(t *testing.T) {
		original := &domain.FilterConfig{
			Include: []string{"pattern1", "pattern2"},
			Exclude: []string{"exclude1"},
		}

		cloned := original.Clone()

		// Modify original
		original.Include[0] = "modified"

		// Verify clone is independent
		assert.Equal(t, "pattern1", cloned.Include[0])
		assert.NotEqual(t, original.Include[0], cloned.Include[0])
	})

	t.Run("validate patterns", func(t *testing.T) {
		validConfig := &domain.FilterConfig{
			Include: []string{"valid*", "*pattern", "exact"},
			Exclude: []string{"exclude*"},
		}

		err := validConfig.Validate()
		assert.NoError(t, err)

		invalidConfig := &domain.FilterConfig{
			Include: []string{"", "  ", "valid"},
		}

		err = invalidConfig.Validate()
		assert.Error(t, err)
	})
}

func TestEndToEndFiltering(t *testing.T) {
	ctx := context.Background()

	// Simulate a complete filtering workflow
	t.Run("complete filtering workflow", func(t *testing.T) {
		// 1. Create filter repository
		repo := filter.NewMemoryFilterRepository().(*filter.MemoryFilterRepository)

		// 2. Load filter configurations (simulating from config file)
		configs := map[string]*domain.FilterConfig{
			"production-profiles": {
				Include: []string{"ollama", "openai*"},
				Exclude: []string{"*test*", "*debug*"},
			},
			"safe-models": {
				Exclude: []string{"*uncensored*", "*adult*", "*nsfw*"},
			},
			"large-models": {
				Include: []string{"*70b*", "*65b*", "*34b*"},
			},
		}

		err := repo.LoadFromConfig(configs)
		require.NoError(t, err)

		// 3. Retrieve and use filters
		filterImpl := filter.NewGlobFilter()

		// Test profile filtering
		profileFilter, err := repo.GetFilterConfig(ctx, "production-profiles")
		require.NoError(t, err)

		profiles := map[string]interface{}{
			"ollama":       "ollama-profile",
			"openai":       "openai-profile",
			"openai-test":  "openai-test-profile",
			"debug-server": "debug-profile",
			"lmstudio":     "lmstudio-profile",
		}

		filteredProfiles, err := filterImpl.ApplyToMap(ctx, profileFilter, profiles)
		require.NoError(t, err)

		// Should include: ollama, openai
		// Should exclude: openai-test (contains "test"), debug-server (contains "debug"), lmstudio (not in include)
		assert.Equal(t, 2, len(filteredProfiles))
		assert.Contains(t, filteredProfiles, "ollama")
		assert.Contains(t, filteredProfiles, "openai")

		// Test model filtering
		modelFilter, err := repo.GetFilterConfig(ctx, "safe-models")
		require.NoError(t, err)

		models := []*domain.ModelInfo{
			{Name: "llama3-8b"},
			{Name: "llama3-8b-uncensored"},
			{Name: "mistral-7b"},
			{Name: "adult-content-gen"},
		}

		result, err := filterImpl.Apply(ctx, modelFilter, models, func(item interface{}) string {
			if model, ok := item.(*domain.ModelInfo); ok {
				return model.Name
			}
			return ""
		})
		require.NoError(t, err)

		// Should exclude models with uncensored/adult/nsfw in name
		assert.Equal(t, 2, len(result.Accepted))
		assert.Equal(t, 2, len(result.Rejected))

		// Verify accepted models
		acceptedNames := make([]string, 0)
		for _, item := range result.Accepted {
			if model, ok := item.(*domain.ModelInfo); ok {
				acceptedNames = append(acceptedNames, model.Name)
			}
		}
		assert.ElementsMatch(t, []string{"llama3-8b", "mistral-7b"}, acceptedNames)
	})
}

func BenchmarkFilterOperations(b *testing.B) {
	ctx := context.Background()
	f := filter.NewGlobFilter()

	config := &domain.FilterConfig{
		Include: []string{"llama*", "mistral*", "*-7b", "*-8b"},
		Exclude: []string{"*uncensored*", "*test*", "debug*"},
	}

	// Create a large set of models
	models := make([]*domain.ModelInfo, 1000)
	for i := 0; i < 1000; i++ {
		models[i] = &domain.ModelInfo{
			Name: generateModelName(i),
		}
	}

	b.ResetTimer()

	b.Run("filter_1000_models", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_, _ = f.Apply(ctx, config, models, func(item interface{}) string {
				if model, ok := item.(*domain.ModelInfo); ok {
					return model.Name
				}
				return ""
			})
		}
	})

	b.Run("pattern_matching_with_cache", func(b *testing.B) {
		// Prime the cache
		f.Matches(config, "llama3-8b")

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			f.Matches(config, "llama3-8b")
		}
	})

	b.Run("pattern_matching_without_cache", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			// Use different names to avoid cache hits
			f.Matches(config, generateModelName(i))
		}
	})
}

func generateModelName(i int) string {
	prefixes := []string{"llama", "mistral", "qwen", "deepseek", "codellama"}
	sizes := []string{"7b", "8b", "13b", "34b", "70b"}
	suffixes := []string{"", "-instruct", "-chat", "-uncensored", "-test"}

	prefix := prefixes[i%len(prefixes)]
	size := sizes[i%len(sizes)]
	suffix := suffixes[i%len(suffixes)]

	return prefix + "-" + size + suffix
}
