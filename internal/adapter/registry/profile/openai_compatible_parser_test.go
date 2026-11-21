package profile

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpenAIParser_Parse(t *testing.T) {
	t.Parallel()

	parser := &openAIParser{}

	t.Run("captures max_model_len into MaxContextLength", func(t *testing.T) {
		t.Parallel()

		response := `{
			"object": "list",
			"data": [
				{
					"id": "mlx-community/Qwen2.5-7B-Instruct-4bit",
					"object": "model",
					"created": 1700000000,
					"owned_by": "omlx",
					"max_model_len": 32768
				}
			]
		}`

		models, err := parser.Parse([]byte(response))
		require.NoError(t, err)
		require.Len(t, models, 1)

		m := models[0]
		assert.Equal(t, "mlx-community/Qwen2.5-7B-Instruct-4bit", m.Name)
		require.NotNil(t, m.Details)
		require.NotNil(t, m.Details.MaxContextLength)
		assert.Equal(t, int64(32768), *m.Details.MaxContextLength)
	})

	t.Run("max_model_len coexists with created timestamp", func(t *testing.T) {
		t.Parallel()

		response := `{
			"object": "list",
			"data": [
				{
					"id": "some-model",
					"object": "model",
					"created": 1677610602,
					"owned_by": "provider",
					"max_model_len": 131072
				}
			]
		}`

		models, err := parser.Parse([]byte(response))
		require.NoError(t, err)
		require.Len(t, models, 1)

		m := models[0]
		require.NotNil(t, m.Details)
		require.NotNil(t, m.Details.MaxContextLength)
		assert.Equal(t, int64(131072), *m.Details.MaxContextLength)
		// created timestamp must still be mapped
		require.NotNil(t, m.Details.ModifiedAt)
		assert.Equal(t, time.Unix(1677610602, 0), *m.Details.ModifiedAt)
	})

	t.Run("absence of max_model_len leaves details unaffected", func(t *testing.T) {
		t.Parallel()

		response := `{
			"object": "list",
			"data": [
				{
					"id": "gpt-3.5-turbo",
					"object": "model",
					"created": 1677610602,
					"owned_by": "openai"
				}
			]
		}`

		models, err := parser.Parse([]byte(response))
		require.NoError(t, err)
		require.Len(t, models, 1)

		m := models[0]
		// Details may exist (due to created), but MaxContextLength must be nil
		if m.Details != nil {
			assert.Nil(t, m.Details.MaxContextLength)
		}
	})

	t.Run("model without any metadata has nil details", func(t *testing.T) {
		t.Parallel()

		response := `{
			"object": "list",
			"data": [
				{
					"id": "bare-model",
					"object": "model"
				}
			]
		}`

		models, err := parser.Parse([]byte(response))
		require.NoError(t, err)
		require.Len(t, models, 1)

		assert.Nil(t, models[0].Details)
	})

	t.Run("zero max_model_len does not set MaxContextLength", func(t *testing.T) {
		t.Parallel()

		response := `{
			"object": "list",
			"data": [
				{
					"id": "zero-context-model",
					"object": "model",
					"max_model_len": 0
				}
			]
		}`

		models, err := parser.Parse([]byte(response))
		require.NoError(t, err)
		require.Len(t, models, 1)

		// Neither details nor MaxContextLength should be set for a zero value
		m := models[0]
		if m.Details != nil {
			assert.Nil(t, m.Details.MaxContextLength)
		}
	})

	t.Run("skips models without ID", func(t *testing.T) {
		t.Parallel()

		response := `{
			"object": "list",
			"data": [
				{
					"object": "model",
					"max_model_len": 4096
				},
				{
					"id": "valid-model",
					"object": "model"
				}
			]
		}`

		models, err := parser.Parse([]byte(response))
		require.NoError(t, err)
		require.Len(t, models, 1)
		assert.Equal(t, "valid-model", models[0].Name)
	})

	t.Run("handles empty body", func(t *testing.T) {
		t.Parallel()

		models, err := parser.Parse([]byte{})
		require.NoError(t, err)
		assert.Empty(t, models)
	})

	t.Run("returns error for invalid JSON", func(t *testing.T) {
		t.Parallel()

		_, err := parser.Parse([]byte(`{"data": [invalid`))
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to parse OpenAI-compatible response")
	})

	t.Run("owned_by is mapped to Publisher", func(t *testing.T) {
		t.Parallel()

		response := `{
			"object": "list",
			"data": [
				{
					"id": "mlx-community/Llama-3.2-3B-Instruct-4bit",
					"object": "model",
					"owned_by": "omlx"
				}
			]
		}`

		models, err := parser.Parse([]byte(response))
		require.NoError(t, err)
		require.Len(t, models, 1)

		m := models[0]
		require.NotNil(t, m.Details)
		require.NotNil(t, m.Details.Publisher)
		assert.Equal(t, "omlx", *m.Details.Publisher)
	})

	t.Run("empty owned_by does not set Publisher", func(t *testing.T) {
		t.Parallel()

		response := `{
			"object": "list",
			"data": [
				{
					"id": "some-model",
					"object": "model",
					"owned_by": ""
				}
			]
		}`

		models, err := parser.Parse([]byte(response))
		require.NoError(t, err)
		require.Len(t, models, 1)

		// empty string must not produce a Details entry or a Publisher pointer
		m := models[0]
		if m.Details != nil {
			assert.Nil(t, m.Details.Publisher)
		}
	})
}

func TestOpenAIParser_ContextWindow(t *testing.T) {
	t.Parallel()

	// Values are fixtures, not claims about any model's context limit.
	for _, tc := range []struct {
		name   string
		fields string
		want   int64
	}{
		{"Rapid-MLX context_window", `"context_window": 8192`, 8192},
		{"max_model_len takes precedence", `"context_window": 8192, "max_model_len": 4096`, 4096},
		{"zero max_model_len falls back", `"context_window": 8192, "max_model_len": 0`, 8192},
		{"zero context_window ignored", `"context_window": 0`, 0},
		{"negative context_window ignored", `"context_window": -1`, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			parser := &openAIParser{}
			models, err := parser.Parse([]byte(`{"data":[{"id":"mlx-community/Qwen3.5-4B-MLX-4bit","object":"model",` + tc.fields + `}]}`))
			require.NoError(t, err)
			require.Len(t, models, 1)
			assert.Equal(t, "mlx-community/Qwen3.5-4B-MLX-4bit", models[0].Name)
			if tc.want == 0 {
				if models[0].Details != nil {
					assert.Nil(t, models[0].Details.MaxContextLength)
				}
				return
			}
			require.NotNil(t, models[0].Details)
			require.NotNil(t, models[0].Details.MaxContextLength)
			assert.Equal(t, tc.want, *models[0].Details.MaxContextLength)
		})
	}
}
