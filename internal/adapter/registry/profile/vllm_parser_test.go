package profile

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVLLMParser_Parse(t *testing.T) {
	parser := &vllmParser{}

	t.Run("parses valid vLLM response with full metadata", func(t *testing.T) {
		response := `{
			"object": "list",
			"data": [
				{
					"id": "TinyLlama/TinyLlama-1.1B-Chat-v1.0",
					"object": "model",
					"created": 1754535984,
					"owned_by": "vllm",
					"root": "TinyLlama/TinyLlama-1.1B-Chat-v1.0",
					"parent": null,
					"max_model_len": 2048,
					"permission": [
						{
							"id": "modelperm-abc123",
							"object": "model_permission",
							"created": 1754535984,
							"allow_create_engine": false,
							"allow_sampling": true,
							"allow_logprobs": true,
							"allow_search_indices": false,
							"allow_view": true,
							"allow_fine_tuning": false,
							"organization": "*",
							"group": null,
							"is_blocking": false
						}
					]
				},
				{
					"id": "meta-llama/Meta-Llama-3.1-8B-Instruct",
					"object": "model",
					"created": 1754535985,
					"owned_by": "meta-llama",
					"root": "meta-llama/Meta-Llama-3.1-8B-Instruct",
					"parent": null,
					"max_model_len": 131072,
					"permission": []
				}
			]
		}`

		models, err := parser.Parse([]byte(response))
		require.NoError(t, err)
		require.Len(t, models, 2)

		// Verify first model (TinyLlama)
		tinyllama := models[0]
		assert.Equal(t, "TinyLlama/TinyLlama-1.1B-Chat-v1.0", tinyllama.Name)
		assert.Equal(t, "vllm", tinyllama.Type)
		assert.NotNil(t, tinyllama.Details)

		// Check max context length is captured
		require.NotNil(t, tinyllama.Details.MaxContextLength)
		assert.Equal(t, int64(2048), *tinyllama.Details.MaxContextLength)

		// Check creation time is converted
		require.NotNil(t, tinyllama.Details.ModifiedAt)
		assert.Equal(t, time.Unix(1754535984, 0), *tinyllama.Details.ModifiedAt)

		// Verify second model (Llama 3.1)
		llama := models[1]
		assert.Equal(t, "meta-llama/Meta-Llama-3.1-8B-Instruct", llama.Name)
		assert.Equal(t, "vllm", llama.Type)
		assert.NotNil(t, llama.Details)

		// Check larger context length
		require.NotNil(t, llama.Details.MaxContextLength)
		assert.Equal(t, int64(131072), *llama.Details.MaxContextLength)

		// Check publisher is extracted from owned_by when not "vllm"
		require.NotNil(t, llama.Details.Publisher)
		assert.Equal(t, "meta-llama", *llama.Details.Publisher)
	})

	t.Run("handles models with fine-tuning parent", func(t *testing.T) {
		response := `{
			"object": "list",
			"data": [
				{
					"id": "custom/fine-tuned-model",
					"object": "model",
					"created": 1754535986,
					"owned_by": "custom-org",
					"root": "base-model",
					"parent": "meta-llama/Meta-Llama-3.1-8B-Instruct",
					"max_model_len": 8192
				}
			]
		}`

		models, err := parser.Parse([]byte(response))
		require.NoError(t, err)
		require.Len(t, models, 1)

		model := models[0]
		assert.Equal(t, "custom/fine-tuned-model", model.Name)
		assert.NotNil(t, model.Details)

		// Check parent model is captured
		require.NotNil(t, model.Details.ParentModel)
		assert.Equal(t, "meta-llama/Meta-Llama-3.1-8B-Instruct", *model.Details.ParentModel)

		// Check publisher
		require.NotNil(t, model.Details.Publisher)
		assert.Equal(t, "custom-org", *model.Details.Publisher)
	})

	t.Run("handles models without optional fields", func(t *testing.T) {
		response := `{
			"object": "list",
			"data": [
				{
					"id": "simple-model",
					"object": "model",
					"created": 0,
					"owned_by": "vllm"
				}
			]
		}`

		models, err := parser.Parse([]byte(response))
		require.NoError(t, err)
		require.Len(t, models, 1)

		model := models[0]
		assert.Equal(t, "simple-model", model.Name)
		assert.Equal(t, "vllm", model.Type)

		// Details should be nil when no metadata is present
		assert.Nil(t, model.Details)
	})

	t.Run("skips models without ID", func(t *testing.T) {
		response := `{
			"object": "list",
			"data": [
				{
					"object": "model",
					"created": 1754535987,
					"owned_by": "test"
				},
				{
					"id": "valid-model",
					"object": "model",
					"created": 1754535988,
					"owned_by": "test"
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
		assert.Contains(t, err.Error(), "failed to parse vLLM response")
		assert.Nil(t, models)
	})

	t.Run("handles zero max_model_len", func(t *testing.T) {
		response := `{
			"object": "list",
			"data": [
				{
					"id": "model-with-zero-context",
					"object": "model",
					"created": 1754535989,
					"owned_by": "test",
					"max_model_len": 0
				}
			]
		}`

		models, err := parser.Parse([]byte(response))
		require.NoError(t, err)
		require.Len(t, models, 1)

		model := models[0]
		// Details should exist due to other metadata, but MaxContextLength should be nil when it's 0
		assert.NotNil(t, model.Details)
		assert.Nil(t, model.Details.MaxContextLength)
	})

	t.Run("handles negative max_model_len", func(t *testing.T) {
		response := `{
			"object": "list",
			"data": [
				{
					"id": "model-with-negative-context",
					"object": "model",
					"created": 1754535990,
					"owned_by": "test",
					"max_model_len": -1
				}
			]
		}`

		models, err := parser.Parse([]byte(response))
		require.NoError(t, err)
		require.Len(t, models, 1)

		model := models[0]
		// Details should exist due to other metadata, but MaxContextLength should be nil when it's negative
		assert.NotNil(t, model.Details)
		assert.Nil(t, model.Details.MaxContextLength)
	})

	t.Run("preserves LastSeen timestamp", func(t *testing.T) {
		response := `{
			"object": "list",
			"data": [
				{
					"id": "test-model",
					"object": "model",
					"created": 1754535991,
					"owned_by": "test"
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

	t.Run("handles real TinyLlama response from vLLM server", func(t *testing.T) {
		// This is the actual response from the vLLM server at 192.168.0.1:8000
		response := `{
			"object": "list",
			"data": [
				{
					"id": "TinyLlama/TinyLlama-1.1B-Chat-v1.0",
					"object": "model",
					"created": 1754535984,
					"owned_by": "vllm",
					"root": "TinyLlama/TinyLlama-1.1B-Chat-v1.0",
					"parent": null,
					"max_model_len": 2048,
					"permission": [
						{
							"id": "modelperm-ca8a321b35824411baf0fbbe4719c498",
							"object": "model_permission",
							"created": 1754535984,
							"allow_create_engine": false,
							"allow_sampling": true,
							"allow_logprobs": true,
							"allow_search_indices": false,
							"allow_view": true,
							"allow_fine_tuning": false,
							"organization": "*",
							"group": null,
							"is_blocking": false
						}
					]
				}
			]
		}`

		models, err := parser.Parse([]byte(response))
		require.NoError(t, err)
		require.Len(t, models, 1)

		model := models[0]
		assert.Equal(t, "TinyLlama/TinyLlama-1.1B-Chat-v1.0", model.Name)
		assert.Equal(t, "vllm", model.Type)

		// Verify all expected fields are parsed
		require.NotNil(t, model.Details)
		require.NotNil(t, model.Details.MaxContextLength)
		assert.Equal(t, int64(2048), *model.Details.MaxContextLength)
		require.NotNil(t, model.Details.ModifiedAt)
		assert.Equal(t, int64(1754535984), model.Details.ModifiedAt.Unix())
	})
}

func TestVLLMParser_EdgeCases(t *testing.T) {
	parser := &vllmParser{}

	t.Run("handles null parent field", func(t *testing.T) {
		response := `{
			"object": "list",
			"data": [
				{
					"id": "model-with-null-parent",
					"object": "model",
					"created": 1754535992,
					"owned_by": "test",
					"parent": null
				}
			]
		}`

		models, err := parser.Parse([]byte(response))
		require.NoError(t, err)
		require.Len(t, models, 1)

		model := models[0]
		// ParentModel should not be set when parent is null
		if model.Details != nil {
			assert.Nil(t, model.Details.ParentModel)
		}
	})

	t.Run("handles owned_by as vllm", func(t *testing.T) {
		response := `{
			"object": "list",
			"data": [
				{
					"id": "model-owned-by-vllm",
					"object": "model",
					"created": 1754535993,
					"owned_by": "vllm",
					"max_model_len": 4096
				}
			]
		}`

		models, err := parser.Parse([]byte(response))
		require.NoError(t, err)
		require.Len(t, models, 1)

		model := models[0]
		assert.NotNil(t, model.Details)
		// Publisher should not be set when owned_by is "vllm"
		assert.Nil(t, model.Details.Publisher)
	})

	t.Run("handles various organisation formats", func(t *testing.T) {
		testCases := []struct {
			ownedBy           string
			expectedPublisher string
		}{
			{"meta-llama", "meta-llama"},
			{"TinyLlama", "TinyLlama"},
			{"custom-org", "custom-org"},
			{"", ""}, // Empty owned_by
		}

		for _, tc := range testCases {
			response := fmt.Sprintf(`{
				"object": "list",
				"data": [
					{
						"id": "test-model",
						"object": "model",
						"created": 1754535994,
						"owned_by": "%s",
						"max_model_len": 1024
					}
				]
			}`, tc.ownedBy)

			models, err := parser.Parse([]byte(response))
			require.NoError(t, err)
			require.Len(t, models, 1)

			model := models[0]
			if tc.expectedPublisher != "" && tc.expectedPublisher != "vllm" {
				require.NotNil(t, model.Details)
				require.NotNil(t, model.Details.Publisher)
				assert.Equal(t, tc.expectedPublisher, *model.Details.Publisher)
			}
		}
	})
}

func TestVLLMParser_PerformanceConsiderations(t *testing.T) {
	parser := &vllmParser{}

	t.Run("handles large model list efficiently", func(t *testing.T) {
		// Generate a response with many models
		modelCount := 100
		modelsJSON := ""
		for i := 0; i < modelCount; i++ {
			if i > 0 {
				modelsJSON += ","
			}
			modelsJSON += fmt.Sprintf(`{
				"id": "model-%d",
				"object": "model",
				"created": %d,
				"owned_by": "test",
				"max_model_len": %d
			}`, i, 1754535995+i, 1024*(i+1))
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
		assert.Equal(t, int64(1024), *models[0].Details.MaxContextLength)
		assert.Equal(t, int64(102400), *models[99].Details.MaxContextLength)
	})
}
