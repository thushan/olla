package unifier

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thushan/olla/internal/core/domain"
)

func TestOllamaPhiFamilyRule(t *testing.T) {
	normalizer := NewModelNormalizer()
	rule := &ollamaPhiFamilyRule{normalizer: normalizer}

	tests := []struct {
		name        string
		modelInfo   *domain.ModelInfo
		canHandle   bool
		expectedID  string
		shouldError bool
	}{
		{
			name: "phi4 misclassified as phi3",
			modelInfo: &domain.ModelInfo{
				Name: "phi4:latest",
				Details: &domain.ModelDetails{
					Family:            strPtr("phi3"),
					ParameterSize:     strPtr("14.7B"),
					QuantizationLevel: strPtr("Q4_K_M"),
					Digest:            strPtr("abc123"),
				},
			},
			canHandle:   true,
			expectedID:  "phi/4:14.7b-q4km",
			shouldError: false,
		},
		{
			name: "phi-4 with hyphen misclassified",
			modelInfo: &domain.ModelInfo{
				Name: "phi-4:latest",
				Details: &domain.ModelDetails{
					Family:            strPtr("phi3"),
					ParameterSize:     strPtr("14.7B"),
					QuantizationLevel: strPtr("Q4_K_M"),
				},
			},
			canHandle:   true,
			expectedID:  "phi/4:14.7b-q4km",
			shouldError: false,
		},
		{
			name: "phi4 correctly classified",
			modelInfo: &domain.ModelInfo{
				Name: "phi4:latest",
				Details: &domain.ModelDetails{
					Family:            strPtr("phi4"),
					ParameterSize:     strPtr("14.7B"),
					QuantizationLevel: strPtr("Q4_K_M"),
				},
			},
			canHandle: false, // Not misclassified, so rule doesn't apply
		},
		{
			name: "non-phi model",
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
					assert.Equal(t, "phi", unified.Family)
					assert.Equal(t, "4", unified.Variant)
					assert.Contains(t, unified.Capabilities, "chat")
					assert.Contains(t, unified.Capabilities, "completion")
					
					// Check metadata
					if tt.modelInfo.Details.Digest != nil {
						assert.Equal(t, *tt.modelInfo.Details.Digest, unified.Metadata["digest"])
					}
				}
			}
		})
	}

	// Test rule properties
	assert.Equal(t, 100, rule.GetPriority())
	assert.Equal(t, "ollama_phi_family_fix", rule.GetName())
}

func TestOllamaHuggingFaceRule(t *testing.T) {
	normalizer := NewModelNormalizer()
	rule := &ollamaHuggingFaceRule{normalizer: normalizer}

	tests := []struct {
		name        string
		modelInfo   *domain.ModelInfo
		canHandle   bool
		expectedID  string
		shouldError bool
	}{
		{
			name: "valid huggingface model",
			modelInfo: &domain.ModelInfo{
				Name: "hf.co/unsloth/Qwen3-32B-GGUF:Q4_K_XL",
				Details: &domain.ModelDetails{
					Family:            strPtr("qwen3"),
					ParameterSize:     strPtr("32.8B"),
					QuantizationLevel: strPtr("unknown"),
				},
			},
			canHandle:   true,
			expectedID:  "qwen/3:32.8b-q4kxl",
			shouldError: false,
		},
		{
			name: "huggingface model without quantization in name",
			modelInfo: &domain.ModelInfo{
				Name: "hf.co/microsoft/phi-4",
				Details: &domain.ModelDetails{
					Family:            strPtr("phi"),
					ParameterSize:     strPtr("14.7B"),
					QuantizationLevel: strPtr("Q4_K_M"),
				},
			},
			canHandle:   true,
			expectedID:  "phi/4:14.7b-q4km",
			shouldError: false,
		},
		{
			name: "invalid huggingface format",
			modelInfo: &domain.ModelInfo{
				Name: "hf.co/invalid",
			},
			canHandle:   true,
			shouldError: true,
		},
		{
			name: "non-huggingface model",
			modelInfo: &domain.ModelInfo{
				Name: "llama3:latest",
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
					assert.Equal(t, "huggingface", unified.Metadata["source"])
					assert.NotEmpty(t, unified.Metadata["organization"])
					assert.Contains(t, unified.GetAliasStrings(), tt.modelInfo.Name)
				}
			}
		})
	}

	assert.Equal(t, 90, rule.GetPriority())
	assert.Equal(t, "ollama_huggingface", rule.GetName())
}

func TestLMStudioVendorPrefixRule(t *testing.T) {
	normalizer := NewModelNormalizer()
	rule := &lmstudioVendorPrefixRule{normalizer: normalizer}

	tests := []struct {
		name        string
		modelInfo   *domain.ModelInfo
		canHandle   bool
		expectedID  string
		shouldError bool
	}{
		{
			name: "microsoft vendor prefix",
			modelInfo: &domain.ModelInfo{
				Name: "microsoft/phi-4-mini-reasoning",
				Details: &domain.ModelDetails{
					Family:            strPtr("phi3"),
					QuantizationLevel: strPtr("Q4_K_M"),
					MaxContextLength:  int64Ptr(131072),
					Type:              strPtr("llm"),
				},
			},
			canHandle:   true,
			expectedID:  "phi/4:14.7b-q4km",
			shouldError: false,
		},
		{
			name: "deepseek model with size in name",
			modelInfo: &domain.ModelInfo{
				Name: "deepseek/deepseek-r1-0528-qwen3-8b",
				Details: &domain.ModelDetails{
					Family:            strPtr("qwen3"),
					QuantizationLevel: strPtr("Q4_K_M"),
					State:             strPtr("loaded"),
					Type:              strPtr("llm"),
				},
			},
			canHandle:   true,
			expectedID:  "qwen/3:8b-q4km",
			shouldError: false,
		},
		{
			name: "vision model",
			modelInfo: &domain.ModelInfo{
				Name: "vendor/vision-model",
				Details: &domain.ModelDetails{
					Type:              strPtr("vlm"),
					QuantizationLevel: strPtr("Q4_0"),
				},
			},
			canHandle:   true,
			expectedID:  "vision:unknown-q4",
			shouldError: false,
		},
		{
			name: "model without vendor prefix",
			modelInfo: &domain.ModelInfo{
				Name: "simple-model",
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
					assert.NotEmpty(t, unified.Metadata["vendor"])
					assert.Contains(t, unified.GetAliasStrings(), tt.modelInfo.Name)
					
					// Check vision capability for vlm models
					if tt.modelInfo.Details != nil && tt.modelInfo.Details.Type != nil && *tt.modelInfo.Details.Type == "vlm" {
						assert.Contains(t, unified.Capabilities, "vision")
					}
					
					// Check context length preservation
					if tt.modelInfo.Details != nil && tt.modelInfo.Details.MaxContextLength != nil {
						assert.Equal(t, tt.modelInfo.Details.MaxContextLength, unified.MaxContextLength)
					}
				}
			}
		})
	}

	assert.Equal(t, 80, rule.GetPriority())
	assert.Equal(t, "lmstudio_vendor_prefix", rule.GetName())
}

func TestGenericModelRule(t *testing.T) {
	normalizer := NewModelNormalizer()
	rule := &genericModelRule{normalizer: normalizer}

	tests := []struct {
		name        string
		modelInfo   *domain.ModelInfo
		expectedID  string
		shouldError bool
	}{
		{
			name: "basic model",
			modelInfo: &domain.ModelInfo{
				Name: "llama3:latest",
				Details: &domain.ModelDetails{
					Family:            strPtr("llama"),
					ParameterSize:     strPtr("70B"),
					QuantizationLevel: strPtr("Q4_K_M"),
				},
			},
			expectedID:  "llama/3:70b-q4km",
			shouldError: false,
		},
		{
			name: "model with minimal info",
			modelInfo: &domain.ModelInfo{
				Name: "unknown-model",
			},
			expectedID:  "unknown:unknown-unk",
			shouldError: false,
		},
		{
			name: "model with all metadata",
			modelInfo: &domain.ModelInfo{
				Name: "test:model",
				Details: &domain.ModelDetails{
					Family:            strPtr("test"),
					ParameterSize:     strPtr("7B"),
					QuantizationLevel: strPtr("Q8_0"),
					MaxContextLength:  int64Ptr(4096),
					Digest:            strPtr("xyz789"),
					Publisher:         strPtr("test-org"),
					Type:              strPtr("llm"),
					State:             strPtr("loaded"),
					ParentModel:       strPtr("parent-test"),
				},
			},
			expectedID:  "test:7b-q8",
			shouldError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Generic rule handles everything
			assert.True(t, rule.CanHandle(tt.modelInfo))

			unified, err := rule.Apply(tt.modelInfo)
			
			if tt.shouldError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				require.NotNil(t, unified)
				assert.Equal(t, tt.expectedID, unified.ID)
				assert.Contains(t, unified.GetAliasStrings(), tt.modelInfo.Name)
				
				// Check all metadata is preserved
				if tt.modelInfo.Details != nil {
					if tt.modelInfo.Details.MaxContextLength != nil {
						assert.Equal(t, tt.modelInfo.Details.MaxContextLength, unified.MaxContextLength)
					}
					if tt.modelInfo.Details.Digest != nil {
						assert.Equal(t, *tt.modelInfo.Details.Digest, unified.Metadata["digest"])
					}
					if tt.modelInfo.Details.Publisher != nil {
						assert.Equal(t, *tt.modelInfo.Details.Publisher, unified.Metadata["publisher"])
					}
					if tt.modelInfo.Details.Type != nil {
						assert.Equal(t, *tt.modelInfo.Details.Type, unified.Metadata["model_type"])
					}
					if tt.modelInfo.Details.State != nil {
						assert.Equal(t, *tt.modelInfo.Details.State, unified.Metadata["state"])
					}
					if tt.modelInfo.Details.ParentModel != nil {
						assert.Equal(t, *tt.modelInfo.Details.ParentModel, unified.Metadata["parent_model"])
					}
				}
			}
		})
	}

	assert.Equal(t, 10, rule.GetPriority())
	assert.Equal(t, "generic_model", rule.GetName())
}

func TestInferCapabilitiesFromName(t *testing.T) {
	tests := []struct {
		name         string
		modelName    string
		expected     []string
	}{
		{
			name:      "chat model",
			modelName: "llama-3-chat",
			expected:  []string{"chat"},
		},
		{
			name:      "instruct model",
			modelName: "phi-4-instruct",
			expected:  []string{"chat"},
		},
		{
			name:      "code model",
			modelName: "deepseek-coder",
			expected:  []string{"code"},
		},
		{
			name:      "vision model",
			modelName: "llava-vision",
			expected:  []string{"vision"},
		},
		{
			name:      "vlm model",
			modelName: "qwen-vlm",
			expected:  []string{"vision"},
		},
		{
			name:      "embedding model",
			modelName: "bge-embed-large",
			expected:  []string{"embedding"},
		},
		{
			name:      "reranking model",
			modelName: "bge-rerank-v2",
			expected:  []string{"reranking"},
		},
		{
			name:      "multi-capability model",
			modelName: "codellama-instruct",
			expected:  []string{"chat", "code"},
		},
		{
			name:      "generic model",
			modelName: "llama-3-base",
			expected:  []string{"completion"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			capabilities := inferCapabilitiesFromName(tt.modelName)
			
			// Check that all expected capabilities are present
			for _, expected := range tt.expected {
				// Handle the embedding/embeddings mismatch
				if expected == "embedding" {
					hasEmbedding := false
					for _, cap := range capabilities {
						if cap == "embedding" || cap == "embeddings" {
							hasEmbedding = true
							break
						}
					}
					assert.True(t, hasEmbedding, "Expected embedding capability")
				} else {
					assert.Contains(t, capabilities, expected)
				}
			}
		})
	}
}

func TestModelNormalizer(t *testing.T) {
	normalizer := NewModelNormalizer()

	t.Run("NormalizeFamily", func(t *testing.T) {
		tests := []struct {
			name           string
			modelName      string
			platformFamily string
			expectedFamily string
			expectedVariant string
		}{
			{
				name:            "phi4 from name",
				modelName:       "phi4:latest",
				platformFamily:  "phi3",
				expectedFamily:  "phi",
				expectedVariant: "4",
			},
			{
				name:            "llama3.3 with decimal",
				modelName:       "llama3.3:70b",
				platformFamily:  "",
				expectedFamily:  "llama",
				expectedVariant: "3.3",
			},
			{
				name:            "qwen with hyphen",
				modelName:       "qwen-3:32b",
				platformFamily:  "qwen3",
				expectedFamily:  "qwen",
				expectedVariant: "3",
			},
			{
				name:            "deepseek with r prefix",
				modelName:       "deepseek-r1",
				platformFamily:  "",
				expectedFamily:  "deepseek",
				expectedVariant: "1",
			},
			{
				name:            "unknown model",
				modelName:       "some-random-model",
				platformFamily:  "",
				expectedFamily:  "some",
				expectedVariant: "unknown",
			},
			{
				name:            "use platform family",
				modelName:       "custom-name",
				platformFamily:  "mixtral",
				expectedFamily:  "mixtral",
				expectedVariant: "unknown",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				family, variant := normalizer.NormalizeFamily(tt.modelName, tt.platformFamily)
				assert.Equal(t, tt.expectedFamily, family)
				assert.Equal(t, tt.expectedVariant, variant)
			})
		}
	})

	t.Run("NormalizeSize", func(t *testing.T) {
		tests := []struct {
			name               string
			size               string
			expectedNormalized string
			expectedCount      int64
		}{
			{
				name:               "billions with B",
				size:               "14.7B",
				expectedNormalized: "14.7b",
				expectedCount:      14700000000,
			},
			{
				name:               "billions lowercase",
				size:               "70.6b",
				expectedNormalized: "70.6b",
				expectedCount:      70600000000,
			},
			{
				name:               "integer billions",
				size:               "32B",
				expectedNormalized: "32b",
				expectedCount:      32000000000,
			},
			{
				name:               "millions",
				size:               "350M",
				expectedNormalized: "0.3b",
				expectedCount:      350000000,
			},
			{
				name:               "thousands",
				size:               "125K",
				expectedNormalized: "0.0b",
				expectedCount:      125000,
			},
			{
				name:               "no unit assumes billions",
				size:               "7",
				expectedNormalized: "7b",
				expectedCount:      7000000000,
			},
			{
				name:               "empty string",
				size:               "",
				expectedNormalized: "unknown",
				expectedCount:      0,
			},
			{
				name:               "invalid format",
				size:               "large",
				expectedNormalized: "large",
				expectedCount:      0,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				normalized, count := normalizer.NormalizeSize(tt.size)
				assert.Equal(t, tt.expectedNormalized, normalized)
				assert.Equal(t, tt.expectedCount, count)
			})
		}
	})

	t.Run("NormalizeQuantization", func(t *testing.T) {
		tests := []struct {
			name     string
			quant    string
			expected string
		}{
			{"Q4_K_M", "Q4_K_M", "q4km"},
			{"Q4_K_S", "Q4_K_S", "q4ks"},
			{"Q3_K_L", "Q3_K_L", "q3kl"},
			{"Q5_K_M", "Q5_K_M", "q5km"},
			{"Q6_K", "Q6_K", "q6k"},
			{"Q8_0", "Q8_0", "q8"},
			{"Q4_0", "Q4_0", "q4"},
			{"F16", "F16", "f16"},
			{"F32", "F32", "f32"},
			{"unknown", "unknown", "unk"},
			{"", "", "unk"},
			{"Q4-K-M", "Q4-K-M", "q4km"}, // With hyphens
			{"q4_k_xl", "q4_k_xl", "q4kxl"}, // Custom format
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				result := normalizer.NormalizeQuantization(tt.quant)
				assert.Equal(t, tt.expected, result)
			})
		}
	})

	t.Run("GenerateCanonicalID", func(t *testing.T) {
		tests := []struct {
			name     string
			family   string
			variant  string
			size     string
			quant    string
			expected string
		}{
			{
				name:     "full ID",
				family:   "phi",
				variant:  "4",
				size:     "14.7b",
				quant:    "q4km",
				expected: "phi/4:14.7b-q4km",
			},
			{
				name:     "no variant",
				family:   "gemma",
				variant:  "unknown",
				size:     "2b",
				quant:    "q4",
				expected: "gemma:2b-q4",
			},
			{
				name:     "empty variant",
				family:   "llama",
				variant:  "",
				size:     "7b",
				quant:    "f16",
				expected: "llama:7b-f16",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				result := normalizer.GenerateCanonicalID(tt.family, tt.variant, tt.size, tt.quant)
				assert.Equal(t, tt.expected, result)
			})
		}
	})

	t.Run("GenerateAliases", func(t *testing.T) {
		unified := &domain.UnifiedModel{
			Family:        "phi",
			Variant:       "4",
			ParameterSize: "14.7b",
			Quantization:  "q4km",
			Metadata:      map[string]interface{}{"publisher": "microsoft"},
		}

		tests := []struct {
			name         string
			platformType string
			nativeName   string
			expected     []string
		}{
			{
				name:         "ollama platform",
				platformType: "ollama",
				nativeName:   "phi4:latest",
				expected: []string{
					"phi4:latest",
					"phi4:14.7b",
					"phi4:14.7b-q4km",
					"phi:14.7b",
				},
			},
			{
				name:         "lmstudio platform",
				platformType: "lmstudio",
				nativeName:   "microsoft/phi-4",
				expected: []string{
					"microsoft/phi-4",
					"phi4:14.7b",
					"phi4:14.7b-q4km",
					"microsoft/phi4",
					"microsoft/phi4-14.7b",
				},
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				aliases := normalizer.GenerateAliases(unified, tt.platformType, tt.nativeName)
				
				// Check native name is always first
				assert.Equal(t, tt.nativeName, aliases[0].Name)
				
				// Convert to string slice for easier testing
				aliasStrings := make([]string, len(aliases))
				for i, a := range aliases {
					aliasStrings[i] = a.Name
				}
				
				// Check all expected aliases are present
				for _, expected := range tt.expected {
					assert.Contains(t, aliasStrings, expected)
				}
				
				// Check no duplicates
				seen := make(map[string]bool)
				for _, alias := range aliases {
					assert.False(t, seen[alias.Name], "duplicate alias: %s", alias.Name)
					seen[alias.Name] = true
				}
			})
		}
	})
}

func TestPlatformDetector(t *testing.T) {
	detector := NewPlatformDetector()

	tests := []struct {
		name         string
		modelInfo    *domain.ModelInfo
		expected     string
	}{
		{
			name: "huggingface with hf.co",
			modelInfo: &domain.ModelInfo{
				Name: "hf.co/org/model",
			},
			expected: "huggingface",
		},
		{
			name: "huggingface with org/model format",
			modelInfo: &domain.ModelInfo{
				Name: "microsoft/phi-4",
			},
			expected: "huggingface",
		},
		{
			name: "lmstudio with type indicators",
			modelInfo: &domain.ModelInfo{
				Name: "some-model",
				Details: &domain.ModelDetails{
					Type:             strPtr("llm"),
					MaxContextLength: int64Ptr(4096),
				},
			},
			expected: "lmstudio",
		},
		{
			name: "lmstudio vlm model",
			modelInfo: &domain.ModelInfo{
				Name: "vision-model",
				Details: &domain.ModelDetails{
					Type:             strPtr("vlm"),
					MaxContextLength: int64Ptr(8192),
				},
			},
			expected: "lmstudio",
		},
		{
			name: "ollama with :latest tag",
			modelInfo: &domain.ModelInfo{
				Name: "llama3:latest",
			},
			expected: "ollama",
		},
		{
			name: "ollama with version tag",
			modelInfo: &domain.ModelInfo{
				Name: "phi4:14.7b",
			},
			expected: "ollama",
		},
		{
			name: "default to ollama",
			modelInfo: &domain.ModelInfo{
				Name: "some-model",
			},
			expected: "ollama",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			platform := detector.DetectPlatform(tt.modelInfo)
			assert.Equal(t, tt.expected, platform)
		})
	}
}