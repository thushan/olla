package unifier

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/logger"
)

func createTestLogger() logger.StyledLogger {
	loggerCfg := &logger.Config{Level: "error", Theme: "default"}
	log, _, _ := logger.New(loggerCfg)
	return logger.NewPlainStyledLogger(log)
}

func TestDefaultUnifier_UnifyModel(t *testing.T) {
	log := createTestLogger()
	unifier := NewDefaultUnifier(log)
	ctx := context.Background()

	tests := []struct {
		name           string
		sourceModel    *domain.ModelInfo
		endpointURL    string
		expectedID     string
		expectedFamily string
		expectedSize   string
		expectedQuant  string
		shouldError    bool
	}{
		{
			name: "phi4 misclassified as phi3",
			sourceModel: &domain.ModelInfo{
				Name: "phi4:latest",
				Details: &domain.ModelDetails{
					Family:            strPtr("phi3"), // Misclassified
					ParameterSize:     strPtr("14.7B"),
					QuantizationLevel: strPtr("Q4_K_M"),
				},
				Size: 8_000_000_000,
			},
			endpointURL:    "http://localhost:11434",
			expectedID:     "phi/4:14.7b-q4km",
			expectedFamily: "phi",
			expectedSize:   "14.7b",
			expectedQuant:  "q4km",
			shouldError:    false,
		},
		{
			name: "llama3.3 with decimal version",
			sourceModel: &domain.ModelInfo{
				Name: "llama3.3:latest",
				Details: &domain.ModelDetails{
					Family:            strPtr("llama"),
					ParameterSize:     strPtr("70.6B"),
					QuantizationLevel: strPtr("Q4_K_M"),
				},
				Size: 40_000_000_000,
			},
			endpointURL:    "http://localhost:11434",
			expectedID:     "llama/3.3:70.6b-q4km",
			expectedFamily: "llama",
			expectedSize:   "70.6b",
			expectedQuant:  "q4km",
			shouldError:    false,
		},
		{
			name: "huggingface model from ollama",
			sourceModel: &domain.ModelInfo{
				Name: "hf.co/unsloth/Qwen3-32B-GGUF:Q4_K_XL",
				Details: &domain.ModelDetails{
					Family:            strPtr("qwen3"),
					ParameterSize:     strPtr("32.8B"),
					QuantizationLevel: strPtr("unknown"),
				},
				Size: 20_000_000_000,
			},
			endpointURL:    "http://localhost:11434",
			expectedID:     "qwen/3:32.8b-unk",
			expectedFamily: "qwen",
			expectedSize:   "32.8b",
			expectedQuant:  "unk",
			shouldError:    false,
		},
		{
			name: "lm studio model with vendor prefix",
			sourceModel: &domain.ModelInfo{
				Name: "microsoft/phi-4-mini-reasoning",
				Details: &domain.ModelDetails{
					Family:            strPtr("phi3"), // Also misclassified
					QuantizationLevel: strPtr("Q4_K_M"),
					MaxContextLength:  int64Ptr(131072),
					Type:              strPtr("llm"),
				},
				Size: 8_000_000_000,
			},
			endpointURL:    "http://localhost:1234",
			expectedID:     "phi/4:14.7b-q4km", // Special handling for phi-4
			expectedFamily: "phi",
			expectedSize:   "14.7b",
			expectedQuant:  "q4km",
			shouldError:    false,
		},
		{
			name: "model with minimal info",
			sourceModel: &domain.ModelInfo{
				Name: "gemma:2b",
				Size: 1_500_000_000,
			},
			endpointURL:    "http://localhost:11434",
			expectedID:     "gemma:unknown-unk",
			expectedFamily: "gemma",
			expectedSize:   "unknown",
			expectedQuant:  "unk",
			shouldError:    false,
		},
		{
			name:        "nil model",
			sourceModel: nil,
			endpointURL: "http://localhost:11434",
			shouldError: true,
		},
		{
			name: "model with state information",
			sourceModel: &domain.ModelInfo{
				Name: "deepseek/deepseek-r1-0528-qwen3-8b",
				Details: &domain.ModelDetails{
					Family:            strPtr("qwen3"),
					QuantizationLevel: strPtr("Q4_K_M"),
					State:             strPtr("loaded"),
					Type:              strPtr("llm"),
					MaxContextLength:  int64Ptr(32768), // LM Studio provides this
				},
				Size: 5_000_000_000,
			},
			endpointURL:    "http://localhost:1234",
			expectedID:     "qwen/3:8b-q4km",
			expectedFamily: "qwen",
			expectedSize:   "8b",
			expectedQuant:  "q4km",
			shouldError:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			unified, err := unifier.UnifyModel(ctx, tt.sourceModel, tt.endpointURL)

			if tt.shouldError {
				assert.Error(t, err)
				assert.Nil(t, unified)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, unified)

			assert.Equal(t, tt.expectedID, unified.ID)
			assert.Equal(t, tt.expectedFamily, unified.Family)
			assert.Equal(t, tt.expectedSize, unified.ParameterSize)
			assert.Equal(t, tt.expectedQuant, unified.Quantization)

			// Check endpoint information
			assert.Len(t, unified.SourceEndpoints, 1)
			endpoint := unified.SourceEndpoints[0]
			assert.Equal(t, tt.endpointURL, endpoint.EndpointURL)
			assert.Equal(t, tt.sourceModel.Name, endpoint.NativeName)
			if tt.sourceModel.Size > 0 {
				assert.Equal(t, tt.sourceModel.Size, endpoint.DiskSize)
			}

			// Check aliases contain original name
			assert.Contains(t, unified.GetAliasStrings(), tt.sourceModel.Name)

			// Verify caching
			cached, err := unifier.ResolveAlias(ctx, unified.ID)
			assert.NoError(t, err)
			assert.Equal(t, unified.ID, cached.ID)
		})
	}
}

func TestDefaultUnifier_UnifyModels(t *testing.T) {
	log := createTestLogger()
	unifier := NewDefaultUnifier(log)
	ctx := context.Background()

	models := []*domain.ModelInfo{
		{
			Name: "phi4:latest",
			Details: &domain.ModelDetails{
				Family:            strPtr("phi3"),
				ParameterSize:     strPtr("14.7B"),
				QuantizationLevel: strPtr("Q4_K_M"),
			},
			Size: 8_000_000_000,
		},
		{
			Name: "llama3.3:latest",
			Details: &domain.ModelDetails{
				Family:            strPtr("llama"),
				ParameterSize:     strPtr("70.6B"),
				QuantizationLevel: strPtr("Q4_K_M"),
			},
			Size: 40_000_000_000,
		},
		nil, // Should handle nil gracefully
		{
			Name: "qwen3:32b",
			Details: &domain.ModelDetails{
				Family:            strPtr("qwen3"),
				ParameterSize:     strPtr("32B"),
				QuantizationLevel: strPtr("Q3_K_L"),
			},
			Size: 20_000_000_000,
		},
	}

	results, err := unifier.UnifyModels(ctx, models, "http://localhost:11434")
	require.NoError(t, err)

	// Should have 3 results (nil model excluded)
	assert.Len(t, results, 3)

	// Verify each model was unified correctly
	expectedIDs := []string{
		"phi/4:14.7b-q4km",
		"llama/3.3:70.6b-q4km",
		"qwen/3:32b-q3kl",
	}

	for i, result := range results {
		assert.Equal(t, expectedIDs[i], result.ID)
		assert.Len(t, result.SourceEndpoints, 1)
		assert.Equal(t, "http://localhost:11434", result.SourceEndpoints[0].EndpointURL)
	}
}

func TestDefaultUnifier_ResolveAlias(t *testing.T) {
	log := createTestLogger()
	unifier := NewDefaultUnifier(log)
	ctx := context.Background()

	// Create and unify a model
	model := &domain.ModelInfo{
		Name: "phi4:latest",
		Details: &domain.ModelDetails{
			Family:            strPtr("phi3"),
			ParameterSize:     strPtr("14.7B"),
			QuantizationLevel: strPtr("Q4_K_M"),
		},
	}

	_, err := unifier.UnifyModel(ctx, model, "http://localhost:11434")
	require.NoError(t, err)

	tests := []struct {
		name        string
		alias       string
		shouldFind  bool
		expectedID  string
	}{
		{
			name:       "resolve by canonical ID",
			alias:      "phi/4:14.7b-q4km",
			shouldFind: true,
			expectedID: "phi/4:14.7b-q4km",
		},
		{
			name:       "resolve by original name",
			alias:      "phi4:latest",
			shouldFind: true,
			expectedID: "phi/4:14.7b-q4km",
		},
		{
			name:       "non-existent alias",
			alias:      "nonexistent:model",
			shouldFind: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := unifier.ResolveAlias(ctx, tt.alias)

			if tt.shouldFind {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, tt.expectedID, result.ID)
			} else {
				assert.Error(t, err)
				assert.Nil(t, result)
			}
		})
	}

	// Test cache statistics
	stats := unifier.GetStats()
	assert.Greater(t, stats.CacheHits, int64(0))
	assert.Greater(t, stats.TotalUnified, int64(0))
}

func TestDefaultUnifier_MergeUnifiedModels(t *testing.T) {
	log := createTestLogger()
	unifier := NewDefaultUnifier(log)
	ctx := context.Background()

	// Create multiple unified models for the same underlying model
	model1 := &domain.UnifiedModel{
		ID:             "phi/4:14.7b-q4km",
		Family:         "phi",
		Variant:        "4",
		ParameterSize:  "14.7b",
		ParameterCount: 14700000000,
		Quantization:   "q4km",
		Format:         "gguf",
		Aliases:        []domain.AliasEntry{
			{Name: "phi4:latest", Source: "ollama"},
			{Name: "phi4:14.7b", Source: "generated"},
		},
		SourceEndpoints: []domain.SourceEndpoint{
			{
				EndpointURL: "http://localhost:11434",
				NativeName:  "phi4:latest",
				State:       "loaded",
				DiskSize:    8000000000,
			},
		},
		Capabilities:     []string{"chat", "completion"},
		MaxContextLength: int64Ptr(131072),
	}

	model2 := &domain.UnifiedModel{
		ID:             "phi/4:14.7b-q4km",
		Family:         "phi",
		Variant:        "4",
		ParameterSize:  "14.7b",
		ParameterCount: 14700000000,
		Quantization:   "q4km",
		Format:         "gguf",
		Aliases:        []domain.AliasEntry{
			{Name: "microsoft/phi-4", Source: "lmstudio"},
			{Name: "phi-4-mini", Source: "lmstudio"},
		},
		SourceEndpoints: []domain.SourceEndpoint{
			{
				EndpointURL: "http://localhost:1234",
				NativeName:  "microsoft/phi-4",
				State:       "not-loaded",
				DiskSize:    8000000000,
			},
		},
		Capabilities: []string{"chat", "code"},
	}

	merged, err := unifier.MergeUnifiedModels(ctx, []*domain.UnifiedModel{model1, model2})
	require.NoError(t, err)
	require.NotNil(t, merged)

	// Check merged properties
	assert.Equal(t, "phi/4:14.7b-q4km", merged.ID)
	assert.Equal(t, "phi", merged.Family)
	assert.Equal(t, "4", merged.Variant)

	// Check aliases are merged and deduplicated
	assert.Len(t, merged.Aliases, 4)
	aliasStrings := merged.GetAliasStrings()
	assert.Contains(t, aliasStrings, "phi4:latest")
	assert.Contains(t, aliasStrings, "microsoft/phi-4")
	assert.Contains(t, aliasStrings, "phi-4-mini")
	assert.Contains(t, aliasStrings, "phi4:14.7b")

	// Check endpoints are merged
	assert.Len(t, merged.SourceEndpoints, 2)
	
	// Check capabilities are merged and deduplicated
	assert.Len(t, merged.Capabilities, 3)
	assert.Contains(t, merged.Capabilities, "chat")
	assert.Contains(t, merged.Capabilities, "completion")
	assert.Contains(t, merged.Capabilities, "code")

	// Check disk size is recalculated
	assert.Equal(t, int64(16000000000), merged.DiskSize)
}

func TestDefaultUnifier_MergeWithDigestDeduplication(t *testing.T) {
	log := createTestLogger()
	unifier := NewDefaultUnifier(log)
	ctx := context.Background()

	// Create models with same digest (should be deduped)
	model1 := &domain.UnifiedModel{
		ID:             "llama/3:8b-q4km",
		Family:         "llama",
		Variant:        "3",
		ParameterSize:  "8b",
		Quantization:   "q4km",
		Aliases:        []domain.AliasEntry{{Name: "llama3:latest", Source: "ollama"}},
		SourceEndpoints: []domain.SourceEndpoint{{EndpointURL: "http://ollama1:11434"}},
		Metadata:       map[string]interface{}{"digest": "abc123"},
	}

	model2 := &domain.UnifiedModel{
		ID:             "llama/3:8b-q4km",
		Family:         "llama",
		Variant:        "3",
		ParameterSize:  "8b",
		Quantization:   "q4km",
		Aliases:        []domain.AliasEntry{{Name: "llama3:8b", Source: "lmstudio"}},
		SourceEndpoints: []domain.SourceEndpoint{{EndpointURL: "http://lmstudio:1234"}},
		Metadata:       map[string]interface{}{"digest": "abc123"},
	}

	model3 := &domain.UnifiedModel{
		ID:             "llama/3:8b-q4km",
		Family:         "llama",
		Variant:        "3",
		ParameterSize:  "8b",
		Quantization:   "q4km",
		Aliases:        []domain.AliasEntry{{Name: "llama3:latest", Source: "ollama"}},
		SourceEndpoints: []domain.SourceEndpoint{{EndpointURL: "http://ollama2:11434"}},
		Metadata:       map[string]interface{}{"digest": "abc123"},
	}

	// Merge should deduplicate by digest, preferring ollama
	merged, err := unifier.MergeUnifiedModels(ctx, []*domain.UnifiedModel{model1, model2, model3})
	assert.NoError(t, err)
	assert.NotNil(t, merged)

	// Should have only kept one endpoint (Ollama preferred)
	assert.Len(t, merged.SourceEndpoints, 1)
	assert.Contains(t, []string{"http://ollama1:11434", "http://ollama2:11434"}, merged.SourceEndpoints[0].EndpointURL)
}

func TestDefaultUnifier_PromptTemplateAssignment(t *testing.T) {
	log := createTestLogger()
	unifier := NewDefaultUnifier(log)
	ctx := context.Background()

	tests := []struct {
		name     string
		model    *domain.ModelInfo
		expected string
	}{
		{
			name: "chat variant",
			model: &domain.ModelInfo{
				Name: "llama-chat:latest",
				Details: &domain.ModelDetails{
					Family: strPtr("llama"),
				},
			},
			expected: "chatml",
		},
		{
			name: "llama instruct",
			model: &domain.ModelInfo{
				Name: "llama3-instruct:8b",
				Details: &domain.ModelDetails{
					Family: strPtr("llama"),
				},
			},
			expected: "llama3-instruct",
		},
		{
			name: "code model",
			model: &domain.ModelInfo{
				Name: "deepseek-coder:latest",
				Details: &domain.ModelDetails{
					Type: strPtr("code"),
				},
			},
			expected: "plain",
		},
		{
			name: "phi model defaults to chatml",
			model: &domain.ModelInfo{
				Name: "phi4:latest",
				Details: &domain.ModelDetails{
					Family: strPtr("phi"),
				},
			},
			expected: "chatml",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			unified, err := unifier.UnifyModel(ctx, tt.model, "http://localhost:11434")
			assert.NoError(t, err)
			assert.Equal(t, tt.expected, unified.PromptTemplateID)
		})
	}
}

func TestDefaultUnifier_GetStats(t *testing.T) {
	log := createTestLogger()
	unifier := NewDefaultUnifier(log)
	ctx := context.Background()

	// Initial stats should be zero
	stats := unifier.GetStats()
	assert.Equal(t, int64(0), stats.TotalUnified)
	assert.Equal(t, int64(0), stats.TotalErrors)

	// Unify some models
	model := &domain.ModelInfo{
		Name: "test:model",
		Details: &domain.ModelDetails{
			Family:            strPtr("test"),
			ParameterSize:     strPtr("7B"),
			QuantizationLevel: strPtr("Q4_0"),
		},
	}

	_, err := unifier.UnifyModel(ctx, model, "http://localhost:11434")
	require.NoError(t, err)

	// Try to unify a nil model (should error)
	_, err = unifier.UnifyModel(ctx, nil, "http://localhost:11434")
	assert.Error(t, err)

	// Check updated stats
	stats = unifier.GetStats()
	assert.Equal(t, int64(1), stats.TotalUnified)
	assert.Equal(t, int64(1), stats.TotalErrors)
	assert.Greater(t, stats.AverageUnifyTimeMs, float64(0))
	assert.False(t, stats.LastUnificationAt.IsZero())
}

func TestDefaultUnifier_Clear(t *testing.T) {
	log := createTestLogger()
	unifier := NewDefaultUnifier(log)
	ctx := context.Background()

	// Add some models
	model := &domain.ModelInfo{
		Name: "test:model",
		Details: &domain.ModelDetails{
			Family: strPtr("test"),
		},
	}

	unified, err := unifier.UnifyModel(ctx, model, "http://localhost:11434")
	require.NoError(t, err)

	// Verify model is cached
	cached, err := unifier.ResolveAlias(ctx, unified.ID)
	assert.NoError(t, err)
	assert.NotNil(t, cached)

	// Clear cache
	err = unifier.Clear(ctx)
	assert.NoError(t, err)

	// Verify model is no longer cached
	cached, err = unifier.ResolveAlias(ctx, unified.ID)
	assert.Error(t, err)
	assert.Nil(t, cached)
}

func TestDefaultUnifier_RegisterCustomRule(t *testing.T) {
	log := createTestLogger()
	unifier := NewDefaultUnifier(log)
	ctx := context.Background()

	// Create a custom rule that prefixes all model families with "custom-"
	customRule := &testCustomRule{}
	
	err := unifier.RegisterCustomRule("ollama", customRule)
	assert.NoError(t, err)

	// Test that nil rule returns error
	err = unifier.RegisterCustomRule("test", nil)
	assert.Error(t, err)

	// Unify a model that should trigger the custom rule
	model := &domain.ModelInfo{
		Name: "customrule:test",
		Details: &domain.ModelDetails{
			Family: strPtr("test"),
		},
	}

	unified, err := unifier.UnifyModel(ctx, model, "http://localhost:11434")
	require.NoError(t, err)
	assert.Equal(t, "custom-test", unified.Family)
}

func TestDefaultUnifier_EdgeCases(t *testing.T) {
	log := createTestLogger()
	unifier := NewDefaultUnifier(log)
	ctx := context.Background()

	tests := []struct {
		name        string
		sourceModel *domain.ModelInfo
		shouldError bool
	}{
		{
			name: "empty model name",
			sourceModel: &domain.ModelInfo{
				Name: "",
			},
			shouldError: false, // Should handle gracefully
		},
		{
			name: "very long model name",
			sourceModel: &domain.ModelInfo{
				Name: "this-is-a-very-long-model-name-that-might-cause-issues-with-processing-and-should-be-handled-gracefully-by-the-unification-system",
			},
			shouldError: false,
		},
		{
			name: "model with special characters",
			sourceModel: &domain.ModelInfo{
				Name: "model@#$%^&*()_+-=[]{}|;':\",./<>?",
			},
			shouldError: false,
		},
		{
			name: "model with unicode characters",
			sourceModel: &domain.ModelInfo{
				Name: "模型-mô-hình-モデル",
			},
			shouldError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			unified, err := unifier.UnifyModel(ctx, tt.sourceModel, "http://localhost:11434")

			if tt.shouldError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, unified)
				assert.NotEmpty(t, unified.ID)
			}
		})
	}
}

// Test helper functions
func strPtr(s string) *string {
	return &s
}

func int64Ptr(i int64) *int64 {
	return &i
}

// Test custom rule implementation
type testCustomRule struct{}

func (r *testCustomRule) CanHandle(modelInfo *domain.ModelInfo) bool {
	return modelInfo.Name == "customrule:test"
}

func (r *testCustomRule) Apply(modelInfo *domain.ModelInfo) (*domain.UnifiedModel, error) {
	return &domain.UnifiedModel{
		ID:             "custom-test:unknown-unk",
		Family:         "custom-test",
		Variant:        "unknown",
		ParameterSize:  "unknown",
		ParameterCount: 0,
		Quantization:   "unk",
		Format:         "unknown",
		Aliases:        []domain.AliasEntry{{Name: modelInfo.Name, Source: "custom"}},
	}, nil
}

func (r *testCustomRule) GetPriority() int {
	return 1000 // Very high priority
}

func (r *testCustomRule) GetName() string {
	return "test_custom_rule"
}