package unifier_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thushan/olla/internal/adapter/unifier"
	"github.com/thushan/olla/internal/core/domain"
)

func TestFamilyDetectionFixes(t *testing.T) {
	ctx := context.Background()
	unifierInstance := unifier.NewDefaultUnifier(createTestLogger())

	testCases := []struct {
		name           string
		inputModel     *domain.ModelInfo
		expectedFamily string
		expectedID     string
		expectedVariant string
	}{
		{
			name: "mistral devstral model",
			inputModel: &domain.ModelInfo{
				Name: "mistralai/devstral-small-2505",
				Details: &domain.ModelDetails{
					Family:            strPtr("llama"), // Wrong platform detection
					QuantizationLevel: strPtr("Q3_K_L"),
				},
			},
			expectedFamily:  "mistral",
			expectedVariant: "devstral-2505",
			expectedID:      "mistral/devstral-2505:unknown-q3kl",
		},
		{
			name: "mistral devstral latest",
			inputModel: &domain.ModelInfo{
				Name: "devstral:latest",
				Details: &domain.ModelDetails{
					Family:            strPtr("llama"), // Wrong platform detection
					ParameterSize:     strPtr("23.6B"),
					QuantizationLevel: strPtr("Q4_K_M"),
				},
			},
			expectedFamily:  "mistral",
			expectedVariant: "devstral",
			expectedID:      "mistral/devstral:23.6b-q4km",
		},
		{
			name: "mistral magistral model",
			inputModel: &domain.ModelInfo{
				Name: "mistralai/magistral-small",
				Details: &domain.ModelDetails{
					Family:            strPtr("llama"), // Wrong platform detection
					QuantizationLevel: strPtr("Q3_K_L"),
				},
			},
			expectedFamily:  "mistral",
			expectedVariant: "magistral",
			expectedID:      "mistral/magistral:unknown-q3kl",
		},
		{
			name: "nomic embedding model",
			inputModel: &domain.ModelInfo{
				Name: "text-embedding-nomic-embed-text-v1.5",
				Details: &domain.ModelDetails{
					Family:            strPtr("nomic-bert"),
					QuantizationLevel: strPtr("Q4_K_M"),
					Type:              strPtr("embeddings"),
				},
			},
			expectedFamily:  "nomic-bert",
			expectedVariant: "embed-text-1.5",
			expectedID:      "nomic-bert/embed-text-1.5:unknown-q4km",
		},
		{
			name: "codegemma model",
			inputModel: &domain.ModelInfo{
				Name: "codegemma:latest",
				Details: &domain.ModelDetails{
					Family:            strPtr("gemma"),
					ParameterSize:     strPtr("9B"),
					QuantizationLevel: strPtr("Q4_0"),
				},
			},
			expectedFamily:  "gemma",
			expectedVariant: "code",
			expectedID:      "gemma/code:9b-q4",
		},
		{
			name: "google gemma model with publisher",
			inputModel: &domain.ModelInfo{
				Name: "google/gemma-3-12b",
				Details: &domain.ModelDetails{
					Family:            strPtr("unknown"),
					ParameterSize:     strPtr("12B"),
					QuantizationLevel: strPtr("Q4_K_M"),
				},
			},
			expectedFamily:  "gemma",
			expectedVariant: "3",
			expectedID:      "gemma/3:12b-q4km",
		},
		{
			name: "deepseek coder model",
			inputModel: &domain.ModelInfo{
				Name: "deepseek/deepseek-coder-v2",
				Details: &domain.ModelDetails{
					Family:            strPtr("deepseek"),
					ParameterSize:     strPtr("16B"),
					QuantizationLevel: strPtr("Q4_K_M"),
				},
			},
			expectedFamily:  "deepseek",
			expectedVariant: "coder-2",
			expectedID:      "deepseek/coder-2:16b-q4km",
		},
		{
			name: "phi model with microsoft publisher",
			inputModel: &domain.ModelInfo{
				Name: "microsoft/phi-mini-3",
				Details: &domain.ModelDetails{
					Family:            strPtr("unknown"),
					ParameterSize:     strPtr("3.8B"),
					QuantizationLevel: strPtr("Q4_K_M"),
				},
			},
			expectedFamily:  "phi",
			expectedVariant: "mini-3",
			expectedID:      "phi/mini-3:3.8b-q4km",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			unified, err := unifierInstance.UnifyModel(ctx, tc.inputModel, "http://localhost:11434")
			require.NoError(t, err)
			require.NotNil(t, unified)

			assert.Equal(t, tc.expectedFamily, unified.Family, "Family mismatch")
			assert.Equal(t, tc.expectedVariant, unified.Variant, "Variant mismatch")
			assert.Equal(t, tc.expectedID, unified.ID, "ID mismatch")
		})
	}
}

func TestCapabilityInference(t *testing.T) {
	ctx := context.Background()
	unifierInstance := unifier.NewDefaultUnifier(createTestLogger())

	testCases := []struct {
		name                 string
		inputModel           *domain.ModelInfo
		expectedCapabilities []string
	}{
		{
			name: "embedding model capabilities",
			inputModel: &domain.ModelInfo{
				Name: "text-embedding-nomic-embed-text-v1.5",
				Details: &domain.ModelDetails{
					Type: strPtr("embeddings"),
				},
			},
			expectedCapabilities: []string{"embeddings", "text_search"},
		},
		{
			name: "VLM model capabilities",
			inputModel: &domain.ModelInfo{
				Name: "google/gemma-3-12b",
				Details: &domain.ModelDetails{
					Type: strPtr("vlm"),
				},
			},
			expectedCapabilities: []string{"vision", "multimodal", "chat", "completion"},
		},
		{
			name: "code model capabilities from name",
			inputModel: &domain.ModelInfo{
				Name: "codegemma:latest",
				Details: &domain.ModelDetails{
					Type: strPtr("llm"),
				},
			},
			expectedCapabilities: []string{"code", "completion"},
		},
		{
			name: "devstral code capabilities",
			inputModel: &domain.ModelInfo{
				Name: "mistralai/devstral-small-2505",
				Details: &domain.ModelDetails{
					Type: strPtr("llm"),
				},
			},
			expectedCapabilities: []string{"code", "completion"},
		},
		{
			name: "nomic-bert architecture capabilities",
			inputModel: &domain.ModelInfo{
				Name: "nomic-bert-base",
				Details: &domain.ModelDetails{
					Family: strPtr("nomic-bert"),
				},
			},
			expectedCapabilities: []string{"embeddings", "text_search"},
		},
		{
			name: "phi model chat capabilities",
			inputModel: &domain.ModelInfo{
				Name: "phi-3-mini",
				Details: &domain.ModelDetails{
					Family: strPtr("phi3"),
				},
			},
			expectedCapabilities: []string{"chat", "completion"},
		},
		{
			name: "instruct model capabilities",
			inputModel: &domain.ModelInfo{
				Name: "llama-3-instruct",
			},
			expectedCapabilities: []string{"chat", "completion"},
		},
		{
			name: "vision model capabilities from name",
			inputModel: &domain.ModelInfo{
				Name: "llava-vision-model",
			},
			expectedCapabilities: []string{"vision", "multimodal", "chat", "completion"},
		},
		{
			name: "multiple capability patterns",
			inputModel: &domain.ModelInfo{
				Name: "codellama-instruct-vision",
			},
			expectedCapabilities: []string{"code", "chat", "vision", "multimodal", "completion"},
		},
		{
			name: "embeddings from name pattern",
			inputModel: &domain.ModelInfo{
				Name: "bge-large-embedding-v1",
			},
			expectedCapabilities: []string{"embeddings", "text_search"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			unified, err := unifierInstance.UnifyModel(ctx, tc.inputModel, "http://localhost:11434")
			require.NoError(t, err)
			require.NotNil(t, unified)

			// Check that all expected capabilities are present
			for _, expectedCap := range tc.expectedCapabilities {
				assert.Contains(t, unified.Capabilities, expectedCap, "Missing capability: %s", expectedCap)
			}

			// If we expect embeddings, we should NOT have completion
			if contains(tc.expectedCapabilities, "embeddings") {
				assert.NotContains(t, unified.Capabilities, "completion", "Embedding models should not have completion capability")
			}
		})
	}
}

func TestPublisherBasedDetection(t *testing.T) {
	normalizer := unifier.NewModelNormalizer()

	testCases := []struct {
		modelName        string
		platformFamily   string
		expectedFamily   string
		expectedVariant  string
	}{
		{
			modelName:       "mistralai/mixtral-8x7b",
			platformFamily:  "llama", // Wrong
			expectedFamily:  "mistral",
			expectedVariant: "unknown", // No specific pattern for mixtral yet
		},
		{
			modelName:       "google/gemma-2-9b",
			platformFamily:  "",
			expectedFamily:  "gemma",
			expectedVariant: "2",
		},
		{
			modelName:       "microsoft/phi-3.5-mini",
			platformFamily:  "",
			expectedFamily:  "phi",
			expectedVariant: "3.5",
		},
		{
			modelName:       "deepseek/deepseek-r1",
			platformFamily:  "",
			expectedFamily:  "deepseek",
			expectedVariant: "1",
		},
		{
			modelName:       "nomic-ai/nomic-embed-text-v1.5",
			platformFamily:  "",
			expectedFamily:  "nomic-bert",
			expectedVariant: "embed-text-1.5",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.modelName, func(t *testing.T) {
			family, variant := normalizer.NormalizeFamily(tc.modelName, tc.platformFamily)
			assert.Equal(t, tc.expectedFamily, family, "Family mismatch for %s", tc.modelName)
			if tc.expectedVariant != "" {
				assert.Equal(t, tc.expectedVariant, variant, "Variant mismatch for %s", tc.modelName)
			}
		})
	}
}

func TestNamePatternDetection(t *testing.T) {
	normalizer := unifier.NewModelNormalizer()

	testCases := []struct {
		modelName        string
		expectedFamily   string
		expectedVariant  string
	}{
		{
			modelName:       "devstral:latest",
			expectedFamily:  "mistral",
			expectedVariant: "devstral",
		},
		{
			modelName:       "magistral-small",
			expectedFamily:  "mistral",
			expectedVariant: "magistral",
		},
		{
			modelName:       "codegemma:9b",
			expectedFamily:  "gemma",
			expectedVariant: "code",
		},
		{
			modelName:       "codellama-34b",
			expectedFamily:  "llama",
			expectedVariant: "code",
		},
		{
			modelName:       "text-embedding-3-small",
			expectedFamily:  "nomic-bert",
			expectedVariant: "embedding-3",
		},
		{
			modelName:       "nomic-embed-text-v1",
			expectedFamily:  "nomic-bert",
			expectedVariant: "embed-text",
		},
		{
			modelName:       "deepseek-coder-v2",
			expectedFamily:  "deepseek",
			expectedVariant: "coder",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.modelName, func(t *testing.T) {
			family, variant := normalizer.NormalizeFamily(tc.modelName, "")
			assert.Equal(t, tc.expectedFamily, family, "Family mismatch for %s", tc.modelName)
			assert.Equal(t, tc.expectedVariant, variant, "Variant mismatch for %s", tc.modelName)
		})
	}
}

// Helper function
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}