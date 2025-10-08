package converter

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thushan/olla/internal/adapter/registry/profile"
	"github.com/thushan/olla/internal/core/constants"
	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/core/ports"
)

func TestNewLemonadeConverter(t *testing.T) {
	converter := NewLemonadeConverter()
	assert.NotNil(t, converter)
	assert.Equal(t, constants.ProviderTypeLemonade, converter.GetFormatName())
}

func TestLemonadeConverter_GetFormatName(t *testing.T) {
	converter := NewLemonadeConverter()
	assert.Equal(t, constants.ProviderTypeLemonade, converter.GetFormatName())
}

func TestLemonadeConverter_ConvertToFormat_EmptyModels(t *testing.T) {
	converter := NewLemonadeConverter()
	result, err := converter.ConvertToFormat([]*domain.UnifiedModel{}, ports.ModelFilters{})

	require.NoError(t, err)
	response, ok := result.(profile.LemonadeResponse)
	require.True(t, ok)

	assert.Equal(t, "list", response.Object)
	assert.Len(t, response.Data, 0)
}

func TestLemonadeConverter_ConvertToFormat_SingleModel(t *testing.T) {
	converter := NewLemonadeConverter()

	model := &domain.UnifiedModel{
		ID: "qwen/2.5:0.5b-instruct-cpu",
		Metadata: map[string]interface{}{
			"checkpoint": "amd/Qwen2.5-0.5B-Instruct-quantized_int4-float16-cpu-onnx",
			"recipe":     "oga-cpu",
		},
		Aliases: []domain.AliasEntry{
			{
				Name:   "Qwen2.5-0.5B-Instruct-CPU",
				Source: constants.ProviderTypeLemonade,
			},
		},
	}

	result, err := converter.ConvertToFormat([]*domain.UnifiedModel{model}, ports.ModelFilters{})

	require.NoError(t, err)
	response, ok := result.(profile.LemonadeResponse)
	require.True(t, ok)

	assert.Equal(t, "list", response.Object)
	require.Len(t, response.Data, 1)

	lemonadeModel := response.Data[0]
	assert.Equal(t, "Qwen2.5-0.5B-Instruct-CPU", lemonadeModel.ID)
	assert.Equal(t, "model", lemonadeModel.Object)
	assert.NotZero(t, lemonadeModel.Created)
	assert.Equal(t, constants.ProviderTypeLemonade, lemonadeModel.OwnedBy) // No slash in model ID, defaults to "lemonade"
	assert.Equal(t, "amd/Qwen2.5-0.5B-Instruct-quantized_int4-float16-cpu-onnx", lemonadeModel.Checkpoint)
	assert.Equal(t, "oga-cpu", lemonadeModel.Recipe)
}

func TestLemonadeConverter_ConvertToFormat_MultipleModels(t *testing.T) {
	converter := NewLemonadeConverter()

	models := []*domain.UnifiedModel{
		{
			ID: "qwen/2.5:0.5b-instruct-cpu",
			Metadata: map[string]interface{}{
				"checkpoint": "amd/Qwen2.5-0.5B-Instruct-quantized_int4-float16-cpu-onnx",
				"recipe":     "oga-cpu",
			},
			Aliases: []domain.AliasEntry{
				{Name: "Qwen2.5-0.5B-Instruct-CPU", Source: constants.ProviderTypeLemonade},
			},
		},
		{
			ID: "llama/3.2:1b-instruct-npu",
			Metadata: map[string]interface{}{
				"checkpoint": "meta-llama/Llama-3.2-1B-Instruct-ONNX",
				"recipe":     "oga-npu",
			},
			Aliases: []domain.AliasEntry{
				{Name: "Llama-3.2-1B-Instruct-NPU", Source: constants.ProviderTypeLemonade},
			},
		},
	}

	result, err := converter.ConvertToFormat(models, ports.ModelFilters{})

	require.NoError(t, err)
	response, ok := result.(profile.LemonadeResponse)
	require.True(t, ok)

	assert.Equal(t, "list", response.Object)
	require.Len(t, response.Data, 2)

	// Check first model (CPU)
	model1 := response.Data[0]
	assert.Equal(t, "Qwen2.5-0.5B-Instruct-CPU", model1.ID)
	assert.Equal(t, constants.ProviderTypeLemonade, model1.OwnedBy) // No slash in model ID, defaults to "lemonade"
	assert.Equal(t, "amd/Qwen2.5-0.5B-Instruct-quantized_int4-float16-cpu-onnx", model1.Checkpoint)
	assert.Equal(t, "oga-cpu", model1.Recipe)

	// Check second model (NPU)
	model2 := response.Data[1]
	assert.Equal(t, "Llama-3.2-1B-Instruct-NPU", model2.ID)
	assert.Equal(t, constants.ProviderTypeLemonade, model2.OwnedBy) // No slash in model ID, defaults to "lemonade"
	assert.Equal(t, "meta-llama/Llama-3.2-1B-Instruct-ONNX", model2.Checkpoint)
	assert.Equal(t, "oga-npu", model2.Recipe)
}

func TestLemonadeConverter_ConvertToFormat_DifferentRecipes(t *testing.T) {
	converter := NewLemonadeConverter()

	recipes := []string{"oga-cpu", "oga-npu", "oga-igpu", "llamacpp", "flm"}
	models := make([]*domain.UnifiedModel, len(recipes))

	for i, recipe := range recipes {
		models[i] = &domain.UnifiedModel{
			ID: "test-model-" + recipe,
			Metadata: map[string]interface{}{
				"checkpoint": "test/checkpoint-" + recipe,
				"recipe":     recipe,
			},
			Aliases: []domain.AliasEntry{
				{Name: "test-model-" + recipe, Source: constants.ProviderTypeLemonade},
			},
		}
	}

	result, err := converter.ConvertToFormat(models, ports.ModelFilters{})

	require.NoError(t, err)
	response, ok := result.(profile.LemonadeResponse)
	require.True(t, ok)

	assert.Len(t, response.Data, len(recipes))

	// Verify each model has the correct recipe
	for i, expectedRecipe := range recipes {
		assert.Equal(t, expectedRecipe, response.Data[i].Recipe,
			"Recipe mismatch for model %d", i)
	}
}

func TestLemonadeConverter_ConvertToFormat_ModelWithCheckpointOnly(t *testing.T) {
	converter := NewLemonadeConverter()

	model := &domain.UnifiedModel{
		ID: "test-checkpoint-only",
		Metadata: map[string]interface{}{
			"checkpoint": "test/model-checkpoint",
		},
		Aliases: []domain.AliasEntry{
			{Name: "test-checkpoint-only", Source: constants.ProviderTypeLemonade},
		},
	}

	result, err := converter.ConvertToFormat([]*domain.UnifiedModel{model}, ports.ModelFilters{})

	require.NoError(t, err)
	response, ok := result.(profile.LemonadeResponse)
	require.True(t, ok)

	require.Len(t, response.Data, 1)
	lemonadeModel := response.Data[0]

	assert.Equal(t, "test-checkpoint-only", lemonadeModel.ID)
	assert.Equal(t, "test/model-checkpoint", lemonadeModel.Checkpoint)
	assert.Empty(t, lemonadeModel.Recipe) // Should be empty when not in metadata
}

func TestLemonadeConverter_ConvertToFormat_ModelWithRecipeOnly(t *testing.T) {
	converter := NewLemonadeConverter()

	model := &domain.UnifiedModel{
		ID: "test-recipe-only",
		Metadata: map[string]interface{}{
			"recipe": "llamacpp",
		},
		Aliases: []domain.AliasEntry{
			{Name: "test-recipe-only", Source: constants.ProviderTypeLemonade},
		},
	}

	result, err := converter.ConvertToFormat([]*domain.UnifiedModel{model}, ports.ModelFilters{})

	require.NoError(t, err)
	response, ok := result.(profile.LemonadeResponse)
	require.True(t, ok)

	require.Len(t, response.Data, 1)
	lemonadeModel := response.Data[0]

	assert.Equal(t, "test-recipe-only", lemonadeModel.ID)
	assert.Empty(t, lemonadeModel.Checkpoint) // Should be empty when not in metadata
	assert.Equal(t, "llamacpp", lemonadeModel.Recipe)
}

func TestLemonadeConverter_ConvertToFormat_FallbackToFirstAlias(t *testing.T) {
	converter := NewLemonadeConverter()

	model := &domain.UnifiedModel{
		ID: "fallback-test-model",
		Aliases: []domain.AliasEntry{
			{Name: "different-name", Source: "different-provider"},
		},
	}

	result, err := converter.ConvertToFormat([]*domain.UnifiedModel{model}, ports.ModelFilters{})

	require.NoError(t, err)
	response, ok := result.(profile.LemonadeResponse)
	require.True(t, ok)

	require.Len(t, response.Data, 1)
	lemonadeModel := response.Data[0]

	// Should use the first alias when no Lemonade-specific alias exists
	assert.Equal(t, "different-name", lemonadeModel.ID)
	assert.Equal(t, constants.ProviderTypeLemonade, lemonadeModel.OwnedBy)
}

func TestLemonadeConverter_ConvertToFormat_NoAliases(t *testing.T) {
	converter := NewLemonadeConverter()

	model := &domain.UnifiedModel{
		ID:      "no-aliases-model",
		Aliases: []domain.AliasEntry{}, // Empty aliases
	}

	result, err := converter.ConvertToFormat([]*domain.UnifiedModel{model}, ports.ModelFilters{})

	require.NoError(t, err)
	response, ok := result.(profile.LemonadeResponse)
	require.True(t, ok)

	require.Len(t, response.Data, 1)
	lemonadeModel := response.Data[0]

	// Should fallback to unified ID when no aliases exist
	assert.Equal(t, "no-aliases-model", lemonadeModel.ID)
	assert.Equal(t, constants.ProviderTypeLemonade, lemonadeModel.OwnedBy)
}

func TestLemonadeConverter_ConvertToFormat_OwnerFromModelID(t *testing.T) {
	converter := NewLemonadeConverter()

	model := &domain.UnifiedModel{
		ID: "test-model",
		Aliases: []domain.AliasEntry{
			{Name: "custom-org/test-model", Source: constants.ProviderTypeLemonade},
		},
	}

	result, err := converter.ConvertToFormat([]*domain.UnifiedModel{model}, ports.ModelFilters{})

	require.NoError(t, err)
	response, ok := result.(profile.LemonadeResponse)
	require.True(t, ok)

	require.Len(t, response.Data, 1)
	lemonadeModel := response.Data[0]

	// Owner should be extracted from model ID with slash
	assert.Equal(t, "custom-org", lemonadeModel.OwnedBy)
}

func TestLemonadeConverter_ConvertToFormat_NoMetadata(t *testing.T) {
	converter := NewLemonadeConverter()

	model := &domain.UnifiedModel{
		ID:       "no-metadata-model",
		Metadata: nil, // No metadata
		Aliases: []domain.AliasEntry{
			{Name: "no-metadata-model", Source: constants.ProviderTypeLemonade},
		},
	}

	result, err := converter.ConvertToFormat([]*domain.UnifiedModel{model}, ports.ModelFilters{})

	require.NoError(t, err)
	response, ok := result.(profile.LemonadeResponse)
	require.True(t, ok)

	require.Len(t, response.Data, 1)
	lemonadeModel := response.Data[0]

	assert.Equal(t, "no-metadata-model", lemonadeModel.ID)
	assert.Empty(t, lemonadeModel.Checkpoint)
	assert.Empty(t, lemonadeModel.Recipe)
}

func TestLemonadeConverter_ConvertToFormat_WithFilters(t *testing.T) {
	converter := NewLemonadeConverter()

	models := []*domain.UnifiedModel{
		{
			ID:     "model1",
			Family: "qwen",
			Aliases: []domain.AliasEntry{
				{Name: "model1", Source: constants.ProviderTypeLemonade},
			},
		},
		{
			ID:     "model2",
			Family: "llama",
			Aliases: []domain.AliasEntry{
				{Name: "model2", Source: constants.ProviderTypeLemonade},
			},
		},
		{
			ID:     "model3",
			Family: "qwen",
			Aliases: []domain.AliasEntry{
				{Name: "model3", Source: constants.ProviderTypeLemonade},
			},
		},
	}

	// Filter to only qwen family models
	filters := ports.ModelFilters{
		Family: "qwen",
	}

	result, err := converter.ConvertToFormat(models, filters)

	require.NoError(t, err)
	response, ok := result.(profile.LemonadeResponse)
	require.True(t, ok)

	assert.Equal(t, "list", response.Object)
	require.Len(t, response.Data, 2)
	assert.Equal(t, "model1", response.Data[0].ID)
	assert.Equal(t, "model3", response.Data[1].ID)
}

func TestLemonadeConverter_FindLemonadeNativeName(t *testing.T) {
	converter := NewLemonadeConverter().(*LemonadeConverter)

	t.Run("finds Lemonade name from aliases when source is lemonade", func(t *testing.T) {
		model := &domain.UnifiedModel{
			ID: "unified/model:tag",
			Aliases: []domain.AliasEntry{
				{Name: "ollama-name", Source: "ollama"},
				{Name: "Qwen2.5-0.5B-Instruct-CPU", Source: constants.ProviderTypeLemonade},
			},
		}

		result := converter.findLemonadeNativeName(model)
		assert.Equal(t, "Qwen2.5-0.5B-Instruct-CPU", result)
	})

	t.Run("returns empty string when no lemonade name found", func(t *testing.T) {
		model := &domain.UnifiedModel{
			ID: "model/without:lemonade",
			Aliases: []domain.AliasEntry{
				{Name: "ollama-name", Source: "ollama"},
				{Name: "vllm-name", Source: "vllm"},
			},
		}

		result := converter.findLemonadeNativeName(model)
		assert.Equal(t, "", result)
	})

	t.Run("only finds lemonade name from correct source", func(t *testing.T) {
		model := &domain.UnifiedModel{
			ID: "test/model",
			Aliases: []domain.AliasEntry{
				{Name: "Qwen-Model-CPU", Source: "openai-compatible"},
			},
			SourceEndpoints: []domain.SourceEndpoint{
				{NativeName: "Qwen-Model-CPU"},
			},
		}

		result := converter.findLemonadeNativeName(model)
		assert.Equal(t, "", result, "Should not pick up names from non-Lemonade sources")
	})
}

func TestLemonadeConverter_DetermineOwner(t *testing.T) {
	converter := &LemonadeConverter{BaseConverter: NewBaseConverter(constants.ProviderTypeLemonade)}

	testCases := []struct {
		modelID  string
		expected string
	}{
		{"amd/Qwen2.5-0.5B-Instruct-CPU", "amd"},
		{"meta-llama/Llama-3.2-1B", "meta-llama"},
		{"microsoft/phi-2", "microsoft"},
		{"simple-model", constants.ProviderTypeLemonade},
		{"", constants.ProviderTypeLemonade},
		{"org/sub/model", "org"}, // Only first part counts
	}

	for _, tc := range testCases {
		t.Run(tc.modelID, func(t *testing.T) {
			result := converter.determineOwner(tc.modelID)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestLemonadeConverter_ApplyMetadataFeatures(t *testing.T) {
	converter := NewLemonadeConverter().(*LemonadeConverter)

	t.Run("applies checkpoint and recipe from metadata", func(t *testing.T) {
		model := &domain.UnifiedModel{
			Metadata: map[string]interface{}{
				"checkpoint": "test/checkpoint-path",
				"recipe":     "oga-npu",
			},
		}

		lemonadeModel := &profile.LemonadeModel{}
		converter.applyMetadataFeatures(model, lemonadeModel)

		assert.Equal(t, "test/checkpoint-path", lemonadeModel.Checkpoint)
		assert.Equal(t, "oga-npu", lemonadeModel.Recipe)
	})

	t.Run("handles nil metadata gracefully", func(t *testing.T) {
		model := &domain.UnifiedModel{
			Metadata: nil,
		}

		lemonadeModel := &profile.LemonadeModel{}
		converter.applyMetadataFeatures(model, lemonadeModel)

		assert.Empty(t, lemonadeModel.Checkpoint)
		assert.Empty(t, lemonadeModel.Recipe)
	})

	t.Run("handles wrong type in metadata", func(t *testing.T) {
		model := &domain.UnifiedModel{
			Metadata: map[string]interface{}{
				"checkpoint": 12345,           // Wrong type
				"recipe":     []string{"oga"}, // Wrong type
			},
		}

		lemonadeModel := &profile.LemonadeModel{}
		converter.applyMetadataFeatures(model, lemonadeModel)

		assert.Empty(t, lemonadeModel.Checkpoint)
		assert.Empty(t, lemonadeModel.Recipe)
	})
}

func BenchmarkLemonadeConverter_ConvertToFormat(b *testing.B) {
	converter := NewLemonadeConverter()

	// Create a large number of models to test performance
	models := make([]*domain.UnifiedModel, 1000)
	for i := 0; i < 1000; i++ {
		models[i] = &domain.UnifiedModel{
			ID: "performance-test-model-" + string(rune(i)),
			Metadata: map[string]interface{}{
				"checkpoint": "test/checkpoint-" + string(rune(i)),
				"recipe":     "oga-cpu",
			},
			Aliases: []domain.AliasEntry{
				{Name: "perf-model-" + string(rune(i)), Source: constants.ProviderTypeLemonade},
			},
		}
	}

	// Report memory allocations
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		result, err := converter.ConvertToFormat(models, ports.ModelFilters{})
		if err != nil {
			b.Fatalf("ConvertToFormat failed: %v", err)
		}

		response, ok := result.(profile.LemonadeResponse)
		if !ok {
			b.Fatal("Result is not LemonadeResponse")
		}

		if len(response.Data) != 1000 {
			b.Fatalf("Expected 1000 models, got %d", len(response.Data))
		}
	}
}
