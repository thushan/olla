package profile

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thushan/olla/internal/core/constants"
)

func TestLlamaCppParser_Parse(t *testing.T) {
	parser := &llamaCppParser{}

	t.Run("parses standard llama.cpp response with single model", func(t *testing.T) {
		response := `{
			"object": "list",
			"data": [
				{
					"id": "llama-3.1-8b-instruct-q4_k_m.gguf",
					"object": "model",
					"created": 1704067200,
					"owned_by": "meta-llama"
				}
			]
		}`

		models, err := parser.Parse([]byte(response))
		require.NoError(t, err)
		require.Len(t, models, 1)

		model := models[0]
		assert.Equal(t, "llama-3.1-8b-instruct-q4_k_m.gguf", model.Name)
		assert.Equal(t, "llamacpp", model.Type)
		require.NotNil(t, model.Details)

		// Check timestamp is converted correctly
		require.NotNil(t, model.Details.ModifiedAt)
		assert.Equal(t, time.Unix(1704067200, 0), *model.Details.ModifiedAt)

		// Check publisher is extracted when not "llamacpp"
		require.NotNil(t, model.Details.Publisher)
		assert.Equal(t, "meta-llama", *model.Details.Publisher)

		// CRITICAL: GGUF format MUST always be set for llama.cpp models
		require.NotNil(t, model.Details.Format)
		assert.Equal(t, constants.RecipeGGUF, *model.Details.Format)
	})

	t.Run("always sets GGUF format for all models", func(t *testing.T) {
		// Test multiple models to ensure EVERY model gets GGUF format
		// This is the most critical test - llama.cpp EXCLUSIVELY serves GGUF
		response := `{
			"object": "list",
			"data": [
				{
					"id": "mistral-7b-v0.1-q4_0.gguf",
					"object": "model",
					"created": 1704067200,
					"owned_by": "mistralai"
				},
				{
					"id": "tinyllama-1.1b-chat-v1.0.Q4_K_M.gguf",
					"object": "model",
					"created": 1704067300,
					"owned_by": "llamacpp"
				},
				{
					"id": "nomic-embed-text-v1.5.Q4_K_M.gguf",
					"object": "model",
					"created": 1704067400,
					"owned_by": "nomic-ai"
				}
			]
		}`

		models, err := parser.Parse([]byte(response))
		require.NoError(t, err)
		require.Len(t, models, 3)

		// Verify EVERY single model has GGUF format set
		for i, model := range models {
			require.NotNil(t, model.Details, "Model %d (%s) should have Details", i, model.Name)
			require.NotNil(t, model.Details.Format, "Model %d (%s) must have Format set", i, model.Name)
			assert.Equal(t, constants.RecipeGGUF, *model.Details.Format, "Model %d (%s) must have GGUF format", i, model.Name)
		}
	})

	t.Run("skips publisher when owned_by is llamacpp", func(t *testing.T) {
		// "llamacpp" is the default value, not a real publisher
		response := `{
			"object": "list",
			"data": [
				{
					"id": "test-model.gguf",
					"object": "model",
					"created": 1704067200,
					"owned_by": "llamacpp"
				}
			]
		}`

		models, err := parser.Parse([]byte(response))
		require.NoError(t, err)
		require.Len(t, models, 1)

		model := models[0]
		require.NotNil(t, model.Details)
		// Publisher should be nil when owned_by is "llamacpp"
		assert.Nil(t, model.Details.Publisher)
		// But format should still be set
		require.NotNil(t, model.Details.Format)
		assert.Equal(t, constants.RecipeGGUF, *model.Details.Format)
	})

	t.Run("sets publisher when owned_by is organisation", func(t *testing.T) {
		testCases := []struct {
			ownedBy           string
			expectedPublisher string
		}{
			{"meta-llama", "meta-llama"},
			{"mistralai", "mistralai"},
			{"microsoft", "microsoft"},
			{"nomic-ai", "nomic-ai"},
		}

		for _, tc := range testCases {
			t.Run(tc.ownedBy, func(t *testing.T) {
				response := fmt.Sprintf(`{
					"object": "list",
					"data": [
						{
							"id": "test-model.gguf",
							"object": "model",
							"created": 1704067200,
							"owned_by": "%s"
						}
					]
				}`, tc.ownedBy)

				models, err := parser.Parse([]byte(response))
				require.NoError(t, err)
				require.Len(t, models, 1)

				model := models[0]
				require.NotNil(t, model.Details)
				require.NotNil(t, model.Details.Publisher)
				assert.Equal(t, tc.expectedPublisher, *model.Details.Publisher)
			})
		}
	})

	t.Run("handles empty response array", func(t *testing.T) {
		response := `{
			"object": "list",
			"data": []
		}`

		models, err := parser.Parse([]byte(response))
		require.NoError(t, err)
		assert.Empty(t, models)
		assert.NotNil(t, models) // Should be empty slice, not nil
	})

	t.Run("handles empty data input", func(t *testing.T) {
		// Test with empty byte slice
		models, err := parser.Parse([]byte{})
		require.NoError(t, err)
		assert.Empty(t, models)
		assert.NotNil(t, models) // Should be empty slice, not nil
	})

	t.Run("returns error for malformed JSON", func(t *testing.T) {
		invalidJSON := `{
			"object": "list",
			"data": [
				{
					"id": "test-model.gguf",
					invalid json here
				}
			]
		}`

		models, err := parser.Parse([]byte(invalidJSON))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to parse llama.cpp response")
		assert.Nil(t, models)
	})

	t.Run("skips models with empty ID", func(t *testing.T) {
		response := `{
			"object": "list",
			"data": [
				{
					"id": "",
					"object": "model",
					"created": 1704067200,
					"owned_by": "llamacpp"
				},
				{
					"id": "valid-model.gguf",
					"object": "model",
					"created": 1704067300,
					"owned_by": "llamacpp"
				}
			]
		}`

		models, err := parser.Parse([]byte(response))
		require.NoError(t, err)
		require.Len(t, models, 1)
		assert.Equal(t, "valid-model.gguf", models[0].Name)
	})

	t.Run("preserves timestamps correctly", func(t *testing.T) {
		// Use specific timestamp (2024-01-01 00:00:00 UTC)
		timestamp := int64(1704067200)
		response := fmt.Sprintf(`{
			"object": "list",
			"data": [
				{
					"id": "test-model.gguf",
					"object": "model",
					"created": %d,
					"owned_by": "llamacpp"
				}
			]
		}`, timestamp)

		models, err := parser.Parse([]byte(response))
		require.NoError(t, err)
		require.Len(t, models, 1)

		model := models[0]
		require.NotNil(t, model.Details)
		require.NotNil(t, model.Details.ModifiedAt)
		assert.Equal(t, timestamp, model.Details.ModifiedAt.Unix())
	})

	t.Run("handles zero timestamp correctly", func(t *testing.T) {
		// Zero timestamps should NOT set ModifiedAt field
		response := `{
			"object": "list",
			"data": [
				{
					"id": "test-model.gguf",
					"object": "model",
					"created": 0,
					"owned_by": "llamacpp"
				}
			]
		}`

		models, err := parser.Parse([]byte(response))
		require.NoError(t, err)
		require.Len(t, models, 1)

		model := models[0]
		// Details should exist (because Format is always set)
		require.NotNil(t, model.Details)
		// But ModifiedAt should be nil when timestamp is 0
		assert.Nil(t, model.Details.ModifiedAt)
		// Format should still be set
		require.NotNil(t, model.Details.Format)
		assert.Equal(t, constants.RecipeGGUF, *model.Details.Format)
	})

	t.Run("handles multiple models with different publishers", func(t *testing.T) {
		// Edge case: llama.cpp typically serves one model, but parser must handle multiple
		response := `{
			"object": "list",
			"data": [
				{
					"id": "llama-3.1-8b-instruct.gguf",
					"object": "model",
					"created": 1704067200,
					"owned_by": "meta-llama"
				},
				{
					"id": "mistral-7b-v0.1.gguf",
					"object": "model",
					"created": 1704067300,
					"owned_by": "mistralai"
				},
				{
					"id": "deepseek-coder-6.7b-instruct.Q5_K_M.gguf",
					"object": "model",
					"created": 1704067400,
					"owned_by": "deepseek-ai"
				}
			]
		}`

		models, err := parser.Parse([]byte(response))
		require.NoError(t, err)
		require.Len(t, models, 3)

		// Verify each model
		assert.Equal(t, "llama-3.1-8b-instruct.gguf", models[0].Name)
		require.NotNil(t, models[0].Details.Publisher)
		assert.Equal(t, "meta-llama", *models[0].Details.Publisher)

		assert.Equal(t, "mistral-7b-v0.1.gguf", models[1].Name)
		require.NotNil(t, models[1].Details.Publisher)
		assert.Equal(t, "mistralai", *models[1].Details.Publisher)

		assert.Equal(t, "deepseek-coder-6.7b-instruct.Q5_K_M.gguf", models[2].Name)
		require.NotNil(t, models[2].Details.Publisher)
		assert.Equal(t, "deepseek-ai", *models[2].Details.Publisher)
	})

	t.Run("type field is always set to llamacpp", func(t *testing.T) {
		response := `{
			"object": "list",
			"data": [
				{
					"id": "test-model-1.gguf",
					"object": "model",
					"created": 1704067200,
					"owned_by": "publisher-1"
				},
				{
					"id": "test-model-2.gguf",
					"object": "model",
					"created": 1704067300,
					"owned_by": "llamacpp"
				}
			]
		}`

		models, err := parser.Parse([]byte(response))
		require.NoError(t, err)
		require.Len(t, models, 2)

		// Verify type is set correctly for all models
		for i, model := range models {
			assert.Equal(t, "llamacpp", model.Type, "Model %d type must be 'llamacpp'", i)
		}
	})

	t.Run("handles empty owned_by field", func(t *testing.T) {
		response := `{
			"object": "list",
			"data": [
				{
					"id": "test-model.gguf",
					"object": "model",
					"created": 1704067200,
					"owned_by": ""
				}
			]
		}`

		models, err := parser.Parse([]byte(response))
		require.NoError(t, err)
		require.Len(t, models, 1)

		model := models[0]
		require.NotNil(t, model.Details)
		// Publisher should not be set when owned_by is empty
		assert.Nil(t, model.Details.Publisher)
		// Format should still be set
		require.NotNil(t, model.Details.Format)
		assert.Equal(t, constants.RecipeGGUF, *model.Details.Format)
	})

	t.Run("preserves LastSeen timestamp", func(t *testing.T) {
		response := `{
			"object": "list",
			"data": [
				{
					"id": "test-model.gguf",
					"object": "model",
					"created": 1704067200,
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
		// LastSeen should be set to current time during parsing
		assert.True(t, model.LastSeen.After(beforeParse) || model.LastSeen.Equal(beforeParse))
		assert.True(t, model.LastSeen.Before(afterParse) || model.LastSeen.Equal(afterParse))
	})
}

func TestLlamaCppParser_RealWorldModelNames(t *testing.T) {
	parser := &llamaCppParser{}

	t.Run("handles real GGUF model names", func(t *testing.T) {
		testCases := []struct {
			modelName string
			publisher string
		}{
			{"llama-3.1-8b-instruct-q4_k_m.gguf", "meta-llama"},
			{"mistral-7b-v0.1-q4_0.gguf", "mistralai"},
			{"tinyllama-1.1b-chat-v1.0.Q4_K_M.gguf", "TinyLlama"},
			{"nomic-embed-text-v1.5.Q4_K_M.gguf", "nomic-ai"},
			{"deepseek-coder-6.7b-instruct.Q5_K_M.gguf", "deepseek-ai"},
		}

		for _, tc := range testCases {
			t.Run(tc.modelName, func(t *testing.T) {
				response := fmt.Sprintf(`{
					"object": "list",
					"data": [
						{
							"id": "%s",
							"object": "model",
							"created": 1704067200,
							"owned_by": "%s"
						}
					]
				}`, tc.modelName, tc.publisher)

				models, err := parser.Parse([]byte(response))
				require.NoError(t, err)
				require.Len(t, models, 1)

				model := models[0]
				assert.Equal(t, tc.modelName, model.Name)
				assert.Equal(t, "llamacpp", model.Type)
				require.NotNil(t, model.Details)
				require.NotNil(t, model.Details.Format)
				assert.Equal(t, constants.RecipeGGUF, *model.Details.Format)
				require.NotNil(t, model.Details.Publisher)
				assert.Equal(t, tc.publisher, *model.Details.Publisher)
			})
		}
	})
}

func TestLlamaCppParser_EdgeCases(t *testing.T) {
	parser := &llamaCppParser{}

	t.Run("handles model without optional fields", func(t *testing.T) {
		// Minimal valid response with only required fields
		response := `{
			"object": "list",
			"data": [
				{
					"id": "minimal-model.gguf",
					"object": "model"
				}
			]
		}`

		models, err := parser.Parse([]byte(response))
		require.NoError(t, err)
		require.Len(t, models, 1)

		model := models[0]
		assert.Equal(t, "minimal-model.gguf", model.Name)
		assert.Equal(t, "llamacpp", model.Type)
		// Details should exist because Format is always set
		require.NotNil(t, model.Details)
		require.NotNil(t, model.Details.Format)
		assert.Equal(t, constants.RecipeGGUF, *model.Details.Format)
	})

	t.Run("handles negative timestamp", func(t *testing.T) {
		// Negative timestamps should be treated like zero (not set)
		response := `{
			"object": "list",
			"data": [
				{
					"id": "test-model.gguf",
					"object": "model",
					"created": -1,
					"owned_by": "llamacpp"
				}
			]
		}`

		models, err := parser.Parse([]byte(response))
		require.NoError(t, err)
		require.Len(t, models, 1)

		model := models[0]
		require.NotNil(t, model.Details)
		// Negative timestamp should not be set as ModifiedAt
		assert.Nil(t, model.Details.ModifiedAt)
	})

	t.Run("handles model with meta field", func(t *testing.T) {
		// Meta field exists in response but is not processed in Phase 3
		response := `{
			"object": "list",
			"data": [
				{
					"id": "model-with-meta.gguf",
					"object": "model",
					"created": 1704067200,
					"owned_by": "llamacpp",
					"meta": {
						"vocab_type": 1,
						"n_vocab": 32000,
						"n_ctx_train": 4096,
						"n_embd": 4096,
						"n_params": 6738415616,
						"size": 4080000000
					}
				}
			]
		}`

		models, err := parser.Parse([]byte(response))
		require.NoError(t, err)
		require.Len(t, models, 1)

		model := models[0]
		assert.Equal(t, "model-with-meta.gguf", model.Name)
		// Meta field is parsed but not processed in Phase 3
		require.NotNil(t, model.Details)
		require.NotNil(t, model.Details.Format)
		assert.Equal(t, constants.RecipeGGUF, *model.Details.Format)
	})

	t.Run("handles models array in dual format response", func(t *testing.T) {
		// llama.cpp can return both 'data' and 'models' arrays (Ollama compatibility)
		// Parser should only use 'data' array
		response := `{
			"object": "list",
			"data": [
				{
					"id": "data-model.gguf",
					"object": "model",
					"created": 1704067200,
					"owned_by": "llamacpp"
				}
			],
			"models": [
				{
					"id": "models-model.gguf",
					"object": "model",
					"created": 1704067300,
					"owned_by": "llamacpp"
				}
			]
		}`

		models, err := parser.Parse([]byte(response))
		require.NoError(t, err)
		// Should only parse from 'data' array, not 'models' array
		require.Len(t, models, 1)
		assert.Equal(t, "data-model.gguf", models[0].Name)
	})
}

func TestLlamaCppParser_PerformanceConsiderations(t *testing.T) {
	parser := &llamaCppParser{}

	t.Run("handles large model list efficiently", func(t *testing.T) {
		// Although llama.cpp typically serves one model,
		// parser must handle multiple models efficiently
		modelCount := 50
		modelsJSON := ""
		for i := 0; i < modelCount; i++ {
			if i > 0 {
				modelsJSON += ","
			}
			modelsJSON += fmt.Sprintf(`{
				"id": "model-%d.gguf",
				"object": "model",
				"created": %d,
				"owned_by": "publisher-%d"
			}`, i, 1704067200+i, i%5)
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
		assert.Equal(t, "model-0.gguf", models[0].Name)
		assert.Equal(t, "model-49.gguf", models[49].Name)
		// All should have GGUF format
		for _, model := range models {
			require.NotNil(t, model.Details)
			require.NotNil(t, model.Details.Format)
			assert.Equal(t, constants.RecipeGGUF, *model.Details.Format)
		}
	})

	t.Run("LastSeen timestamp is captured once for efficiency", func(t *testing.T) {
		// All models in a single parse should have the same LastSeen timestamp
		// This verifies the optimization of capturing timestamp once
		response := `{
			"object": "list",
			"data": [
				{
					"id": "model-1.gguf",
					"object": "model",
					"created": 1704067200,
					"owned_by": "llamacpp"
				},
				{
					"id": "model-2.gguf",
					"object": "model",
					"created": 1704067300,
					"owned_by": "llamacpp"
				},
				{
					"id": "model-3.gguf",
					"object": "model",
					"created": 1704067400,
					"owned_by": "llamacpp"
				}
			]
		}`

		models, err := parser.Parse([]byte(response))
		require.NoError(t, err)
		require.Len(t, models, 3)

		// All models should have identical LastSeen timestamps
		// (captured once at parse time, not per model)
		firstTimestamp := models[0].LastSeen
		for i, model := range models {
			assert.Equal(t, firstTimestamp, model.LastSeen, "Model %d should have same LastSeen timestamp", i)
		}
	})
}
