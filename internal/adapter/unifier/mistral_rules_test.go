package unifier

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thushan/olla/internal/core/domain"
)

func TestMistralModelRule(t *testing.T) {
	normalizer := NewModelNormalizer()
	rule := &mistralModelRule{normalizer: normalizer}

	tests := []struct {
		name           string
		modelInfo      *domain.ModelInfo
		canHandle      bool
		expectedID     string
		expectedFamily string
		expectedVariant string
		expectedSize   string
		expectedQuant  string
		shouldError    bool
	}{
		{
			name: "devstral-small-2505",
			modelInfo: &domain.ModelInfo{
				Name: "mistralai/devstral-small-2505",
				Details: &domain.ModelDetails{
					Family:            strPtr("llama"), // Wrong family
					QuantizationLevel: strPtr("Q3_K_L"),
					MaxContextLength:  int64Ptr(131072),
					Type:              strPtr("llm"),
				},
			},
			canHandle:       true,
			expectedID:      "mistral/devstral:8b-q3kl",
			expectedFamily:  "mistral",
			expectedVariant: "devstral",
			expectedSize:    "8b",
			expectedQuant:   "q3kl",
			shouldError:     false,
		},
		{
			name: "magistral-small",
			modelInfo: &domain.ModelInfo{
				Name: "mistralai/magistral-small",
				Details: &domain.ModelDetails{
					Family:            strPtr("llama"),
					QuantizationLevel: strPtr("Q3_K_L"),
					MaxContextLength:  int64Ptr(49152),
					Type:              strPtr("llm"),
				},
			},
			canHandle:       true,
			expectedID:      "mistral/magistral:8b-q3kl",
			expectedFamily:  "mistral",
			expectedVariant: "magistral",
			expectedSize:    "8b",
			expectedQuant:   "q3kl",
			shouldError:     false,
		},
		{
			name: "mixtral-8x7b",
			modelInfo: &domain.ModelInfo{
				Name: "mistralai/mixtral-8x7b-instruct",
				Details: &domain.ModelDetails{
					QuantizationLevel: strPtr("Q4_K_M"),
					Type:              strPtr("llm"),
				},
			},
			canHandle:       true,
			expectedID:      "mistral/mixtral:46.7b-q4km",
			expectedFamily:  "mistral",
			expectedVariant: "mixtral",
			expectedSize:    "46.7b",
			expectedQuant:   "q4km",
			shouldError:     false,
		},
		{
			name: "mistral with numeric size",
			modelInfo: &domain.ModelInfo{
				Name: "mistral:7b",
				Details: &domain.ModelDetails{
					ParameterSize:     strPtr("7B"),
					QuantizationLevel: strPtr("Q4_0"),
				},
			},
			canHandle:       true,
			expectedID:      "mistral:7b-q4",
			expectedFamily:  "mistral",
			expectedVariant: "",
			expectedSize:    "7b",
			expectedQuant:   "q4",
			shouldError:     false,
		},
		{
			name: "non-mistral model",
			modelInfo: &domain.ModelInfo{
				Name: "llama3:latest",
				Details: &domain.ModelDetails{
					Family: strPtr("llama"),
				},
			},
			canHandle: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			canHandle := rule.CanHandle(tt.modelInfo)
			assert.Equal(t, tt.canHandle, canHandle)

			if canHandle {
				unified, err := rule.Apply(tt.modelInfo)
				
				if tt.shouldError {
					assert.Error(t, err)
				} else {
					require.NoError(t, err)
					require.NotNil(t, unified)
					assert.Equal(t, tt.expectedID, unified.ID)
					assert.Equal(t, tt.expectedFamily, unified.Family)
					assert.Equal(t, tt.expectedVariant, unified.Variant)
					assert.Equal(t, tt.expectedSize, unified.ParameterSize)
					assert.Equal(t, tt.expectedQuant, unified.Quantization)
					
					// Check capabilities
					if tt.expectedVariant == "devstral" {
						assert.Contains(t, unified.Capabilities, "code")
					}
					if tt.expectedVariant == "magistral" {
						assert.Contains(t, unified.Capabilities, "chat")
						assert.Contains(t, unified.Capabilities, "reasoning")
					}
					
					// Check metadata
					assert.Equal(t, "mistral", unified.Metadata["source"])
					if tt.modelInfo.Name == "mistralai/mixtral-8x7b-instruct" {
						assert.Equal(t, "moe", unified.Metadata["architecture"])
					}
					
					// Check aliases are well-formed
					for _, alias := range unified.Aliases {
						// Ensure no malformed aliases like "mistralmagistral"
						assert.NotContains(t, alias.Name, "mistralmagistral")
						assert.NotContains(t, alias.Name, "mistraldevstral")
					}
				}
			}
		})
	}

	// Test rule properties
	assert.Equal(t, 95, rule.GetPriority())
	assert.Equal(t, "mistral_model_rule", rule.GetName())
}