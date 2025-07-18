package unifier

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thushan/olla/internal/core/domain"
)

// TestDefaultUnifier_MetadataExtraction validates metadata extraction
func TestDefaultUnifier_MetadataExtraction(t *testing.T) {
	ctx := context.Background()
	unifier := NewDefaultUnifier()

	tests := []struct {
		name          string
		models        []*domain.ModelInfo
		endpoint      *domain.Endpoint
		expectedModel func(*testing.T, *domain.UnifiedModel)
	}{
		{
			name: "Ollama model with full metadata",
			models: []*domain.ModelInfo{
				{
					Name: "gemma3:12b",
					Size: 8149190253,
					Details: &domain.ModelDetails{
						Family:            ptrString("gemma3"),
						Families:          []string{"gemma3"},
						ParameterSize:     ptrString("12.2B"),
						QuantizationLevel: ptrString("Q4_K_M"),
						Format:            ptrString("gguf"),
						Digest:            ptrString("f4031aab637d1ffa37b42570452ae0e4fad0314754d17ded67322e4b95836f8a"),
						ModifiedAt:        ptrTime(time.Now()),
					},
				},
			},
			endpoint: createTestEndpoint("http://localhost:11434", "Ollama"),
			expectedModel: func(t *testing.T, model *domain.UnifiedModel) {
				assert.Equal(t, "gemma3", model.Family)
				assert.Equal(t, "", model.Variant)
				assert.Equal(t, "12.2b", model.ParameterSize)
				assert.Equal(t, int64(12200000000), model.ParameterCount)
				assert.Equal(t, "q4km", model.Quantization)
				assert.Equal(t, "gguf", model.Format)
				assert.Contains(t, model.Capabilities, "text-generation")
				assert.Equal(t, "f4031aab637d1ffa37b42570452ae0e4fad0314754d17ded67322e4b95836f8a", model.Metadata["digest"])
				assert.Equal(t, "12.2B", model.Metadata["parameter_size"])
				assert.Equal(t, "Q4_K_M", model.Metadata["quantization_level"])
			},
		},
		{
			name: "LM Studio model with type and context",
			models: []*domain.ModelInfo{
				{
					Name: "microsoft/phi-4-mini-reasoning",
					Type: "llm",
					Details: &domain.ModelDetails{
						Type:              ptrString("llm"),
						Publisher:         ptrString("microsoft"),
						Family:            ptrString("phi"),
						QuantizationLevel: ptrString("Q4_K_M"),
						MaxContextLength:  ptrInt64(131072),
						State:             ptrString("not-loaded"),
					},
				},
			},
			endpoint: createTestEndpoint("http://localhost:1234", "LM Studio"),
			expectedModel: func(t *testing.T, model *domain.UnifiedModel) {
				assert.Equal(t, "phi", model.Family)
				assert.Equal(t, "", model.Variant) // Variant suppressed for metadata-sourced family
				assert.Equal(t, "q4km", model.Quantization)
				assert.Equal(t, int64(131072), *model.MaxContextLength)
				assert.Contains(t, model.Capabilities, "text-generation")
				assert.Contains(t, model.Capabilities, "chat")
				assert.Contains(t, model.Capabilities, "completion")
				assert.Contains(t, model.Capabilities, "reasoning")
				assert.Contains(t, model.Capabilities, "logic")
				assert.Contains(t, model.Capabilities, "long-context")
				assert.Equal(t, "microsoft", model.Metadata["publisher"])
				assert.Equal(t, "llm", model.Metadata["type"])
			},
		},
		{
			name: "Vision model detection",
			models: []*domain.ModelInfo{
				{
					Name: "llava-v1.6-mistral-7b",
					Type: "vlm",
					Details: &domain.ModelDetails{
						Type:              ptrString("vlm"),
						ParameterSize:     ptrString("7B"),
						QuantizationLevel: ptrString("Q5_K_S"),
						MaxContextLength:  ptrInt64(4096),
					},
				},
			},
			endpoint: createTestEndpoint("http://localhost:1234", "LM Studio"),
			expectedModel: func(t *testing.T, model *domain.UnifiedModel) {
				assert.Equal(t, "7b", model.ParameterSize)
				assert.Equal(t, "q5ks", model.Quantization)
				assert.Contains(t, model.Capabilities, "text-generation")
				assert.Contains(t, model.Capabilities, "vision")
				assert.Contains(t, model.Capabilities, "multimodal")
				assert.Contains(t, model.Capabilities, "image-understanding")
				assert.NotContains(t, model.Capabilities, "embeddings")
			},
		},
		{
			name: "Code model with instruct",
			models: []*domain.ModelInfo{
				{
					Name: "codellama-34b-instruct",
					Details: &domain.ModelDetails{
						ParameterSize:     ptrString("34B"),
						QuantizationLevel: ptrString("Q3_K_L"),
						Family:            ptrString("codellama"),
					},
				},
			},
			endpoint: createTestEndpoint("http://localhost:11434", "Ollama"),
			expectedModel: func(t *testing.T, model *domain.UnifiedModel) {
				assert.Equal(t, "codellama", model.Family)
				assert.Equal(t, "34b", model.ParameterSize)
				assert.Equal(t, "q3kl", model.Quantization)
				assert.Contains(t, model.Capabilities, "code-generation")
				assert.Contains(t, model.Capabilities, "programming")
				assert.Contains(t, model.Capabilities, "instruction-following")
				assert.Contains(t, model.Capabilities, "chat")
			},
		},
		{
			name: "Embedding model",
			models: []*domain.ModelInfo{
				{
					Name: "all-minilm-l6-v2",
					Type: "embeddings",
					Details: &domain.ModelDetails{
						Type:          ptrString("embeddings"),
						ParameterSize: ptrString("22M"),
					},
				},
			},
			endpoint: createTestEndpoint("http://localhost:1234", "LM Studio"),
			expectedModel: func(t *testing.T, model *domain.UnifiedModel) {
				assert.Equal(t, "22m", model.ParameterSize)
				assert.Contains(t, model.Capabilities, "embeddings")
				assert.Contains(t, model.Capabilities, "similarity")
				assert.Contains(t, model.Capabilities, "vector-search")
				assert.NotContains(t, model.Capabilities, "text-generation")
			},
		},
		{
			name: "Ultra long context model",
			models: []*domain.ModelInfo{
				{
					Name: "claude-3-opus",
					Details: &domain.ModelDetails{
						MaxContextLength: ptrInt64(2000000),
						ParameterSize:    ptrString("175B"),
					},
				},
			},
			endpoint: createTestEndpoint("http://localhost:8080", "Custom"),
			expectedModel: func(t *testing.T, model *domain.UnifiedModel) {
				assert.Equal(t, int64(2000000), *model.MaxContextLength)
				assert.Contains(t, model.Capabilities, "ultra-long-context")
				assert.Contains(t, model.Capabilities, "long-context")
			},
		},
		{
			name: "Model with metadata confidence",
			models: []*domain.ModelInfo{
				{
					Name: "unknown-model-7b",
					Size: 7000000000,
					// No details, should have low confidence
				},
			},
			endpoint: createTestEndpoint("http://localhost:11434", "Ollama"),
			expectedModel: func(t *testing.T, model *domain.UnifiedModel) {
				// Confidence score reflects metadata completeness
				confidence, ok := model.Metadata["metadata_confidence"].(float64)
				assert.True(t, ok, "metadata_confidence should be present")
				assert.Less(t, confidence, 0.5, "Low confidence expected for minimal metadata")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			unified, err := unifier.UnifyModels(ctx, tt.models, tt.endpoint)
			require.NoError(t, err)
			require.Len(t, unified, 1)

			tt.expectedModel(t, unified[0])
		})
	}
}

// TestDefaultUnifier_MetadataMerging verifies that metadata from multiple endpoints
// is properly merged without loss, enriching the unified model representation.
func TestDefaultUnifier_MetadataMerging(t *testing.T) {
	ctx := context.Background()
	unifier := NewDefaultUnifier()

	models1 := []*domain.ModelInfo{
		{
			Name: "llama3.2:1b",
			Size: 1000000000,
			Details: &domain.ModelDetails{
				Digest: ptrString("sha256:abc123"),
				Family: ptrString("llama"),
			},
		},
	}
	endpoint1 := createTestEndpoint("http://localhost:11434", "Ollama")
	_, err := unifier.UnifyModels(ctx, models1, endpoint1)
	require.NoError(t, err)

	// Complementary metadata from second endpoint enriches the model
	models2 := []*domain.ModelInfo{
		{
			Name: "llama3.2:1b",
			Type: "llm",
			Details: &domain.ModelDetails{
				Digest:            ptrString("sha256:abc123"),
				ParameterSize:     ptrString("1.2B"),
				QuantizationLevel: ptrString("Q4_K_M"),
				MaxContextLength:  ptrInt64(128000),
				Publisher:         ptrString("meta"),
			},
		},
	}
	endpoint2 := createTestEndpoint("http://localhost:1234", "LM Studio")
	_, err = unifier.UnifyModels(ctx, models2, endpoint2)
	require.NoError(t, err)

	model, err := unifier.(*DefaultUnifier).ResolveModel(ctx, "llama3.2:1b")
	require.NoError(t, err)

	assert.Equal(t, "llama", model.Family)
	assert.Equal(t, "", model.Variant)
	assert.Equal(t, "1.2b", model.ParameterSize)
	assert.Equal(t, "q4km", model.Quantization)
	assert.Equal(t, int64(128000), *model.MaxContextLength)
	assert.Contains(t, model.Capabilities, "long-context")
	assert.Equal(t, "meta", model.Metadata["publisher"])
	assert.Equal(t, 2, len(model.SourceEndpoints))

	// Platform diversity and publisher information preserved through merging
	assert.Contains(t, model.Metadata, "platform")
	assert.Contains(t, model.Metadata, "publisher")
}

func ptrTime(t time.Time) *time.Time {
	return &t
}

func ptrInt64(i int64) *int64 {
	return &i
}
