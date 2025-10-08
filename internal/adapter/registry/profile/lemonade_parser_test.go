package profile

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLemonadeParser_Parse(t *testing.T) {
	parser := &lemonadeParser{}

	t.Run("parses valid Lemonade response with full metadata", func(t *testing.T) {
		response := `{
			"object": "list",
			"data": [
				{
					"id": "Qwen2.5-0.5B-Instruct-CPU",
					"object": "model",
					"created": 1759361710,
					"owned_by": "lemonade",
					"checkpoint": "amd/Qwen2.5-0.5B-Instruct-quantized_int4-float16-cpu-onnx",
					"recipe": "oga-cpu"
				},
				{
					"id": "Llama-3.2-1B-Instruct-NPU",
					"object": "model",
					"created": 1759361720,
					"owned_by": "lemonade",
					"checkpoint": "meta-llama/Llama-3.2-1B-Instruct-quantized-int4-npu-onnx",
					"recipe": "oga-npu"
				}
			]
		}`

		models, err := parser.Parse([]byte(response))
		require.NoError(t, err)
		require.Len(t, models, 2)

		// Verify first model (Qwen CPU)
		qwen := models[0]
		assert.Equal(t, "Qwen2.5-0.5B-Instruct-CPU", qwen.Name)
		assert.Equal(t, "lemonade", qwen.Type)
		assert.NotNil(t, qwen.Details)

		// Check creation time is converted
		require.NotNil(t, qwen.Details.ModifiedAt)
		assert.Equal(t, time.Unix(1759361710, 0), *qwen.Details.ModifiedAt)

		// Check publisher is extracted from checkpoint path
		require.NotNil(t, qwen.Details.Publisher)
		assert.Equal(t, "amd", *qwen.Details.Publisher)

		// Check format is inferred from recipe (oga-* = onnx)
		require.NotNil(t, qwen.Details.Format)
		assert.Equal(t, "onnx", *qwen.Details.Format)

		require.NotNil(t, qwen.Details.Checkpoint)
		assert.Equal(t, "amd/Qwen2.5-0.5B-Instruct-quantized_int4-float16-cpu-onnx", *qwen.Details.Checkpoint)

		require.NotNil(t, qwen.Details.Recipe)
		assert.Equal(t, "oga-cpu", *qwen.Details.Recipe)

		// Verify second model (Llama NPU)
		llama := models[1]
		assert.Equal(t, "Llama-3.2-1B-Instruct-NPU", llama.Name)
		assert.Equal(t, "lemonade", llama.Type)
		assert.NotNil(t, llama.Details)

		// Check publisher extraction
		require.NotNil(t, llama.Details.Publisher)
		assert.Equal(t, "meta-llama", *llama.Details.Publisher)

		// Check format is still onnx for oga-npu recipe
		require.NotNil(t, llama.Details.Format)
		assert.Equal(t, "onnx", *llama.Details.Format)

		require.NotNil(t, llama.Details.Checkpoint)
		assert.Equal(t, "meta-llama/Llama-3.2-1B-Instruct-quantized-int4-npu-onnx", *llama.Details.Checkpoint)

		require.NotNil(t, llama.Details.Recipe)
		assert.Equal(t, "oga-npu", *llama.Details.Recipe)
	})

	t.Run("infers GGUF format from llamacpp recipe", func(t *testing.T) {
		response := `{
			"object": "list",
			"data": [
				{
					"id": "Llama-3.2-3B-Q4-KM",
					"object": "model",
					"created": 1759361730,
					"owned_by": "lemonade",
					"checkpoint": "bartowski/Llama-3.2-3B-Instruct-GGUF",
					"recipe": "llamacpp"
				}
			]
		}`

		models, err := parser.Parse([]byte(response))
		require.NoError(t, err)
		require.Len(t, models, 1)

		model := models[0]
		assert.Equal(t, "Llama-3.2-3B-Q4-KM", model.Name)

		// Check format is inferred as GGUF from llamacpp recipe
		require.NotNil(t, model.Details)
		require.NotNil(t, model.Details.Format)
		assert.Equal(t, "gguf", *model.Details.Format)

		// Check publisher
		require.NotNil(t, model.Details.Publisher)
		assert.Equal(t, "bartowski", *model.Details.Publisher)
	})

	t.Run("infers GGUF format from flm recipe", func(t *testing.T) {
		response := `{
			"object": "list",
			"data": [
				{
					"id": "Phi-3.5-Mini-FLM",
					"object": "model",
					"created": 1759361740,
					"owned_by": "lemonade",
					"checkpoint": "microsoft/Phi-3.5-mini-instruct-gguf",
					"recipe": "flm"
				}
			]
		}`

		models, err := parser.Parse([]byte(response))
		require.NoError(t, err)
		require.Len(t, models, 1)

		model := models[0]

		// Check format is inferred as GGUF from flm recipe
		require.NotNil(t, model.Details)
		require.NotNil(t, model.Details.Format)
		assert.Equal(t, "gguf", *model.Details.Format)

		// Check publisher
		require.NotNil(t, model.Details.Publisher)
		assert.Equal(t, "microsoft", *model.Details.Publisher)
	})

	t.Run("handles models without checkpoint path", func(t *testing.T) {
		response := `{
			"object": "list",
			"data": [
				{
					"id": "local-model",
					"object": "model",
					"created": 1759361750,
					"owned_by": "lemonade",
					"checkpoint": "",
					"recipe": "oga-cpu"
				}
			]
		}`

		models, err := parser.Parse([]byte(response))
		require.NoError(t, err)
		require.Len(t, models, 1)

		model := models[0]
		assert.Equal(t, "local-model", model.Name)

		// Publisher should not be set when checkpoint is empty
		require.NotNil(t, model.Details)
		assert.Nil(t, model.Details.Publisher)

		// Format should still be inferred from recipe
		require.NotNil(t, model.Details.Format)
		assert.Equal(t, "onnx", *model.Details.Format)
	})

	t.Run("handles models with checkpoint but no slash", func(t *testing.T) {
		response := `{
			"object": "list",
			"data": [
				{
					"id": "simple-model",
					"object": "model",
					"created": 1759361760,
					"owned_by": "lemonade",
					"checkpoint": "model-name-without-namespace",
					"recipe": "oga-igpu"
				}
			]
		}`

		models, err := parser.Parse([]byte(response))
		require.NoError(t, err)
		require.Len(t, models, 1)

		model := models[0]
		// Publisher should not be extracted when there's no slash
		require.NotNil(t, model.Details)
		assert.Nil(t, model.Details.Publisher)

		// Format should be inferred from oga-igpu recipe
		require.NotNil(t, model.Details.Format)
		assert.Equal(t, "onnx", *model.Details.Format)
	})

	t.Run("handles models with unknown recipe", func(t *testing.T) {
		response := `{
			"object": "list",
			"data": [
				{
					"id": "future-model",
					"object": "model",
					"created": 1759361770,
					"owned_by": "lemonade",
					"checkpoint": "vendor/future-engine-model",
					"recipe": "future-engine"
				}
			]
		}`

		models, err := parser.Parse([]byte(response))
		require.NoError(t, err)
		require.Len(t, models, 1)

		model := models[0]
		assert.Equal(t, "future-model", model.Name)

		// Format should not be inferred for unknown recipes
		require.NotNil(t, model.Details)
		assert.Nil(t, model.Details.Format)

		// Publisher should still be extracted
		require.NotNil(t, model.Details.Publisher)
		assert.Equal(t, "vendor", *model.Details.Publisher)
	})

	t.Run("handles models without optional fields", func(t *testing.T) {
		response := `{
			"object": "list",
			"data": [
				{
					"id": "minimal-model",
					"object": "model",
					"created": 0,
					"owned_by": "lemonade"
				}
			]
		}`

		models, err := parser.Parse([]byte(response))
		require.NoError(t, err)
		require.Len(t, models, 1)

		model := models[0]
		assert.Equal(t, "minimal-model", model.Name)
		assert.Equal(t, "lemonade", model.Type)

		// Details should be nil when no metadata is present
		assert.Nil(t, model.Details)
	})

	t.Run("skips models without ID", func(t *testing.T) {
		response := `{
			"object": "list",
			"data": [
				{
					"object": "model",
					"created": 1759361780,
					"owned_by": "lemonade",
					"checkpoint": "test/model",
					"recipe": "oga-cpu"
				},
				{
					"id": "valid-model",
					"object": "model",
					"created": 1759361790,
					"owned_by": "lemonade",
					"checkpoint": "test/valid",
					"recipe": "oga-npu"
				}
			]
		}`

		models, err := parser.Parse([]byte(response))
		require.NoError(t, err)
		require.Len(t, models, 1)
		assert.Equal(t, "valid-model", models[0].Name)
	})

	t.Run("handles empty response", func(t *testing.T) {
		models, err := parser.Parse([]byte{})
		require.NoError(t, err)
		assert.Empty(t, models)
	})

	t.Run("handles empty data array", func(t *testing.T) {
		response := `{
			"object": "list",
			"data": []
		}`

		models, err := parser.Parse([]byte(response))
		require.NoError(t, err)
		assert.Empty(t, models)
	})

	t.Run("returns error for invalid JSON", func(t *testing.T) {
		invalidJSON := `{
			"object": "list",
			"data": [
				{
					"id": "test-model",
					"object": "model",
					invalid json here
				}
			]
		}`

		models, err := parser.Parse([]byte(invalidJSON))
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to parse Lemonade response")
		assert.Nil(t, models)
	})

	t.Run("preserves LastSeen timestamp", func(t *testing.T) {
		response := `{
			"object": "list",
			"data": [
				{
					"id": "test-model",
					"object": "model",
					"created": 1759361800,
					"owned_by": "lemonade",
					"checkpoint": "test/model",
					"recipe": "oga-cpu"
				}
			]
		}`

		beforeParse := time.Now()
		models, err := parser.Parse([]byte(response))
		afterParse := time.Now()

		require.NoError(t, err)
		require.Len(t, models, 1)

		model := models[0]
		// LastSeen should be set to current time
		assert.True(t, model.LastSeen.After(beforeParse) || model.LastSeen.Equal(beforeParse))
		assert.True(t, model.LastSeen.Before(afterParse) || model.LastSeen.Equal(afterParse))
	})

	t.Run("handles real Lemonade response from server", func(t *testing.T) {
		// This is the actual response format from Lemonade SDK
		response := `{
			"object": "list",
			"data": [
				{
					"id": "Qwen2.5-0.5B-Instruct-CPU",
					"created": 1759361710,
					"object": "model",
					"owned_by": "lemonade",
					"checkpoint": "amd/Qwen2.5-0.5B-Instruct-quantized_int4-float16-cpu-onnx",
					"recipe": "oga-cpu"
				}
			]
		}`

		models, err := parser.Parse([]byte(response))
		require.NoError(t, err)
		require.Len(t, models, 1)

		model := models[0]
		assert.Equal(t, "Qwen2.5-0.5B-Instruct-CPU", model.Name)
		assert.Equal(t, "lemonade", model.Type)

		// Verify all expected fields are parsed
		require.NotNil(t, model.Details)
		require.NotNil(t, model.Details.ModifiedAt)
		assert.Equal(t, int64(1759361710), model.Details.ModifiedAt.Unix())
		require.NotNil(t, model.Details.Publisher)
		assert.Equal(t, "amd", *model.Details.Publisher)
		require.NotNil(t, model.Details.Format)
		assert.Equal(t, "onnx", *model.Details.Format)
	})
}

func TestLemonadeParser_RecipeInference(t *testing.T) {
	t.Run("infers format correctly for all recipe types", func(t *testing.T) {
		testCases := []struct {
			recipe         string
			expectedFormat string
		}{
			{"oga-cpu", "onnx"},
			{"oga-npu", "onnx"},
			{"oga-igpu", "onnx"},
			{"llamacpp", "gguf"},
			{"flm", "gguf"},
			{"unknown", ""},
		}

		for _, tc := range testCases {
			t.Run(tc.recipe, func(t *testing.T) {
				format := inferFormatFromRecipe(tc.recipe)
				assert.Equal(t, tc.expectedFormat, format, "Recipe %s should infer format %s", tc.recipe, tc.expectedFormat)
			})
		}
	})

}

func TestLemonadeParser_EdgeCases(t *testing.T) {
	parser := &lemonadeParser{}

	t.Run("handles checkpoint with multiple slashes", func(t *testing.T) {
		response := `{
			"object": "list",
			"data": [
				{
					"id": "complex-model",
					"object": "model",
					"created": 1759361810,
					"owned_by": "lemonade",
					"checkpoint": "organization/repo/model/variant",
					"recipe": "oga-cpu"
				}
			]
		}`

		models, err := parser.Parse([]byte(response))
		require.NoError(t, err)
		require.Len(t, models, 1)

		model := models[0]
		// Publisher should be extracted from first part only
		require.NotNil(t, model.Details)
		require.NotNil(t, model.Details.Publisher)
		assert.Equal(t, "organization", *model.Details.Publisher)
	})

	t.Run("handles various checkpoint formats", func(t *testing.T) {
		testCases := []struct {
			checkpoint        string
			expectedPublisher string
		}{
			{"meta-llama/Llama-3.2-3B", "meta-llama"},
			{"microsoft/Phi-3.5-mini", "microsoft"},
			{"amd/Qwen2.5-0.5B", "amd"},
			{"bartowski/model-gguf", "bartowski"},
			{"model-without-slash", ""},
			{"/leading-slash-model", ""},
			{"", ""},
		}

		for _, tc := range testCases {
			response := fmt.Sprintf(`{
				"object": "list",
				"data": [
					{
						"id": "test-model",
						"object": "model",
						"created": 1759361820,
						"owned_by": "lemonade",
						"checkpoint": "%s",
						"recipe": "oga-cpu"
					}
				]
			}`, tc.checkpoint)

			models, err := parser.Parse([]byte(response))
			require.NoError(t, err)
			require.Len(t, models, 1)

			model := models[0]
			if tc.expectedPublisher != "" {
				require.NotNil(t, model.Details)
				require.NotNil(t, model.Details.Publisher)
				assert.Equal(t, tc.expectedPublisher, *model.Details.Publisher)
			} else {
				if model.Details != nil {
					assert.Nil(t, model.Details.Publisher)
				}
			}
		}
	})
}

func TestLemonadeParser_PerformanceConsiderations(t *testing.T) {
	parser := &lemonadeParser{}

	t.Run("handles large model list efficiently", func(t *testing.T) {
		// Generate a response with many models
		modelCount := 100
		modelsJSON := ""
		for i := 0; i < modelCount; i++ {
			if i > 0 {
				modelsJSON += ","
			}
			recipe := "oga-cpu"
			if i%3 == 0 {
				recipe = "oga-npu"
			} else if i%3 == 1 {
				recipe = "llamacpp"
			}
			modelsJSON += fmt.Sprintf(`{
				"id": "model-%d",
				"object": "model",
				"created": %d,
				"owned_by": "lemonade",
				"checkpoint": "vendor-%d/model-%d",
				"recipe": "%s"
			}`, i, 1759361830+i, i%10, i, recipe)
		}

		response := fmt.Sprintf(`{
			"object": "list",
			"data": [%s]
		}`, modelsJSON)

		startTime := time.Now()
		models, err := parser.Parse([]byte(response))
		parseTime := time.Since(startTime)

		require.NoError(t, err)
		assert.Len(t, models, modelCount)

		// Parsing should be fast even with many models
		assert.Less(t, parseTime, 100*time.Millisecond)

		// Verify a sample of models
		assert.Equal(t, "model-0", models[0].Name)
		assert.Equal(t, "model-99", models[99].Name)

		// Check format inference for different recipes
		require.NotNil(t, models[0].Details.Format)
		assert.Equal(t, "onnx", *models[0].Details.Format) // oga-npu
		require.NotNil(t, models[1].Details.Format)
		assert.Equal(t, "gguf", *models[1].Details.Format) // llamacpp
	})
}
