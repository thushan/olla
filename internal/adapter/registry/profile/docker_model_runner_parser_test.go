package profile

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thushan/olla/internal/core/constants"
)

func TestDockerModelRunnerParser_Parse(t *testing.T) {
	t.Parallel()

	parser := &dockerModelRunnerParser{}

	t.Run("parses valid response with single model", func(t *testing.T) {
		t.Parallel()
		response := `{
			"object": "list",
			"data": [
				{
					"id": "ai/smollm2",
					"object": "model",
					"created": 1734000000,
					"owned_by": "docker"
				}
			]
		}`

		models, err := parser.Parse([]byte(response))
		require.NoError(t, err)
		require.Len(t, models, 1)

		model := models[0]
		assert.Equal(t, "ai/smollm2", model.Name)
		assert.Equal(t, constants.ProviderTypeDockerMR, model.Type)
		require.NotNil(t, model.Details)
		require.NotNil(t, model.Details.ModifiedAt)
		assert.Equal(t, int64(1734000000), model.Details.ModifiedAt.Unix())
	})

	t.Run("parses multiple models", func(t *testing.T) {
		t.Parallel()
		response := `{
			"object": "list",
			"data": [
				{
					"id": "ai/smollm2",
					"object": "model",
					"created": 1734000000,
					"owned_by": "docker"
				},
				{
					"id": "docker/llama3.2",
					"object": "model",
					"created": 1734000001,
					"owned_by": "meta"
				}
			]
		}`

		models, err := parser.Parse([]byte(response))
		require.NoError(t, err)
		require.Len(t, models, 2)

		assert.Equal(t, "ai/smollm2", models[0].Name)
		assert.Equal(t, "docker/llama3.2", models[1].Name)

		// "meta" is a real publisher — should be preserved
		require.NotNil(t, models[1].Details.Publisher)
		assert.Equal(t, "meta", *models[1].Details.Publisher)
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
					"object": "model",
					"created": 1734000002,
					"owned_by": "test"
				},
				{
					"id": "valid/model",
					"object": "model",
					"created": 1734000003,
					"owned_by": "test"
				}
			]
		}`

		models, err := parser.Parse([]byte(response))
		require.NoError(t, err)
		require.Len(t, models, 1)
		assert.Equal(t, "valid/model", models[0].Name)
	})

	t.Run("returns error for invalid JSON", func(t *testing.T) {
		t.Parallel()
		models, err := parser.Parse([]byte(`{"object": "list", "data": [not valid]}`))
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to parse Docker Model Runner response")
		assert.Nil(t, models)
	})

	t.Run("does not set publisher when owned_by is docker", func(t *testing.T) {
		t.Parallel()
		response := `{
			"object": "list",
			"data": [
				{
					"id": "ai/test-model",
					"object": "model",
					"created": 1734000004,
					"owned_by": "docker"
				}
			]
		}`

		models, err := parser.Parse([]byte(response))
		require.NoError(t, err)
		require.Len(t, models, 1)

		// "docker" is the default owned_by — not a meaningful publisher attribution
		require.NotNil(t, models[0].Details)
		assert.Nil(t, models[0].Details.Publisher)
	})

	t.Run("sets GGUF format in details", func(t *testing.T) {
		t.Parallel()
		response := `{
			"object": "list",
			"data": [
				{
					"id": "ai/smollm2",
					"object": "model",
					"created": 1734000005,
					"owned_by": "docker"
				}
			]
		}`

		models, err := parser.Parse([]byte(response))
		require.NoError(t, err)
		require.Len(t, models, 1)

		require.NotNil(t, models[0].Details)
		require.NotNil(t, models[0].Details.Format)
		assert.Equal(t, constants.RecipeGGUF, *models[0].Details.Format)
	})

	t.Run("preserves LastSeen timestamp within parse window", func(t *testing.T) {
		t.Parallel()
		response := `{
			"object": "list",
			"data": [
				{
					"id": "ai/test-model",
					"object": "model",
					"created": 1734000006,
					"owned_by": "docker"
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

func TestDockerModelRunnerParser_NamespaceFormats(t *testing.T) {
	t.Parallel()

	parser := &dockerModelRunnerParser{}

	cases := []struct {
		modelID string
	}{
		{"ai/smollm2"},
		{"docker/llama3.2"},
		{"huggingface/meta-llama-3.1-8b"},
		{"simple-model"},  // no namespace
		{"org/sub/model"}, // nested path
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.modelID, func(t *testing.T) {
			t.Parallel()
			response := `{"object":"list","data":[{"id":"` + tc.modelID + `","object":"model","created":1734000007,"owned_by":"docker"}]}`

			models, err := parser.Parse([]byte(response))
			require.NoError(t, err)
			require.Len(t, models, 1)
			assert.Equal(t, tc.modelID, models[0].Name)
		})
	}
}
