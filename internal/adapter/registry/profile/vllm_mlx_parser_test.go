package profile

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thushan/olla/internal/core/constants"
)

func TestVLLMMLXParser_Parse(t *testing.T) {
	t.Parallel()

	parser := &vllmMLXParser{}

	t.Run("parses valid response with single model", func(t *testing.T) {
		t.Parallel()
		response := `{
			"object": "list",
			"data": [
				{
					"id": "mlx-community/Llama-3.2-3B-Instruct-4bit",
					"object": "model",
					"created": 1734000000,
					"owned_by": "mlx-community"
				}
			]
		}`

		models, err := parser.Parse([]byte(response))
		require.NoError(t, err)
		require.Len(t, models, 1)

		model := models[0]
		assert.Equal(t, "mlx-community/Llama-3.2-3B-Instruct-4bit", model.Name)
		assert.Equal(t, constants.ProviderTypeVLLMMLX, model.Type)
		require.NotNil(t, model.Details)
		require.NotNil(t, model.Details.ModifiedAt)
		assert.Equal(t, int64(1734000000), model.Details.ModifiedAt.Unix())

		// "mlx-community" is a real HuggingFace organisation — publisher attribution should be preserved.
		require.NotNil(t, model.Details.Publisher)
		assert.Equal(t, "mlx-community", *model.Details.Publisher)

		require.NotNil(t, model.Details.Format)
		assert.Equal(t, constants.RecipeMLX, *model.Details.Format)
	})

	t.Run("parses multiple models", func(t *testing.T) {
		t.Parallel()
		response := `{
			"object": "list",
			"data": [
				{
					"id": "mlx-community/Llama-3.2-3B-Instruct-4bit",
					"object": "model",
					"created": 1734000000,
					"owned_by": "mlx-community"
				},
				{
					"id": "mlx-community/Qwen3-4B-4bit",
					"object": "model",
					"created": 1734000001,
					"owned_by": "mlx-community"
				}
			]
		}`

		models, err := parser.Parse([]byte(response))
		require.NoError(t, err)
		require.Len(t, models, 2)

		assert.Equal(t, "mlx-community/Llama-3.2-3B-Instruct-4bit", models[0].Name)
		assert.Equal(t, "mlx-community/Qwen3-4B-4bit", models[1].Name)
	})

	t.Run("handles empty byte slice", func(t *testing.T) {
		t.Parallel()
		models, err := parser.Parse([]byte{})
		require.NoError(t, err)
		assert.Empty(t, models)
	})

	t.Run("handles empty data array", func(t *testing.T) {
		t.Parallel()
		response := `{"object": "list", "data": []}`

		models, err := parser.Parse([]byte(response))
		require.NoError(t, err)
		assert.Empty(t, models)
	})

	t.Run("skips models without ID", func(t *testing.T) {
		t.Parallel()
		response := `{
			"object": "list",
			"data": [
				{
					"id": "",
					"object": "model",
					"created": 1734000002,
					"owned_by": "mlx-community"
				},
				{
					"id": "mlx-community/Qwen3-4B-4bit",
					"object": "model",
					"created": 1734000003,
					"owned_by": "mlx-community"
				}
			]
		}`

		models, err := parser.Parse([]byte(response))
		require.NoError(t, err)
		require.Len(t, models, 1)
		assert.Equal(t, "mlx-community/Qwen3-4B-4bit", models[0].Name)
	})

	t.Run("returns error for invalid JSON", func(t *testing.T) {
		t.Parallel()
		models, err := parser.Parse([]byte(`{"object": "list", "data": [not valid]}`))
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to parse vLLM-MLX response")
		assert.Nil(t, models)
	})

	t.Run("does not set publisher when owned_by is default", func(t *testing.T) {
		t.Parallel()

		defaultOwners := []string{"vllm-mlx", "vllm", ""}

		for _, owner := range defaultOwners {
			owner := owner
			t.Run(owner, func(t *testing.T) {
				t.Parallel()
				response := `{
					"object": "list",
					"data": [
						{
							"id": "mlx-community/Llama-3.2-3B-Instruct-4bit",
							"object": "model",
							"created": 1734000004,
							"owned_by": "` + owner + `"
						}
					]
				}`

				models, err := parser.Parse([]byte(response))
				require.NoError(t, err)
				require.Len(t, models, 1)

				// Default owned_by values carry no meaningful publisher attribution.
				require.NotNil(t, models[0].Details)
				assert.Nil(t, models[0].Details.Publisher)
			})
		}
	})

	t.Run("sets MLX format in details", func(t *testing.T) {
		t.Parallel()
		response := `{
			"object": "list",
			"data": [
				{
					"id": "mlx-community/Llama-3.2-3B-Instruct-4bit",
					"object": "model",
					"created": 1734000005,
					"owned_by": "vllm-mlx"
				}
			]
		}`

		models, err := parser.Parse([]byte(response))
		require.NoError(t, err)
		require.Len(t, models, 1)

		require.NotNil(t, models[0].Details)
		require.NotNil(t, models[0].Details.Format)
		assert.Equal(t, constants.RecipeMLX, *models[0].Details.Format)
	})

	t.Run("preserves LastSeen timestamp within parse window", func(t *testing.T) {
		t.Parallel()
		response := `{
			"object": "list",
			"data": [
				{
					"id": "mlx-community/Llama-3.2-3B-Instruct-4bit",
					"object": "model",
					"created": 1734000006,
					"owned_by": "vllm-mlx"
				}
			]
		}`

		before := time.Now()
		models, err := parser.Parse([]byte(response))
		after := time.Now()

		require.NoError(t, err)
		require.Len(t, models, 1)

		ls := models[0].LastSeen
		assert.True(t, !ls.Before(before) && !ls.After(after), "LastSeen should fall within parse window")
	})
}

func TestVLLMMLXParser_HuggingFaceModelIDFormats(t *testing.T) {
	t.Parallel()

	parser := &vllmMLXParser{}

	cases := []struct {
		modelID string
	}{
		{"mlx-community/Llama-3.2-3B-Instruct-4bit"}, // standard HuggingFace namespace/model
		{"org/model-name"},                           // simple namespace
		{"simple-model"},                             // no namespace
		{"org/sub/deep-model"},                       // nested path — full ID preserved as Name
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.modelID, func(t *testing.T) {
			t.Parallel()
			response := `{"object":"list","data":[{"id":"` + tc.modelID + `","object":"model","created":1734000007,"owned_by":"vllm-mlx"}]}`

			models, err := parser.Parse([]byte(response))
			require.NoError(t, err)
			require.Len(t, models, 1)
			assert.Equal(t, tc.modelID, models[0].Name)
		})
	}
}
