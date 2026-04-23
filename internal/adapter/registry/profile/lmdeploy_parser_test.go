package profile

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLMDeployParser_Parse(t *testing.T) {
	t.Parallel()

	parser := &lmdeployParser{}

	t.Run("parses valid response with full metadata", func(t *testing.T) {
		t.Parallel()

		response := `{
			"object": "list",
			"data": [
				{
					"id": "internlm/internlm2_5-7b-chat",
					"object": "model",
					"created": 1754535984,
					"owned_by": "lmdeploy",
					"root": "internlm/internlm2_5-7b-chat",
					"parent": null,
					"permission": []
				},
				{
					"id": "meta-llama/Meta-Llama-3.1-8B-Instruct",
					"object": "model",
					"created": 1754535985,
					"owned_by": "meta-llama",
					"root": "meta-llama/Meta-Llama-3.1-8B-Instruct",
					"parent": null,
					"permission": []
				}
			]
		}`

		models, err := parser.Parse([]byte(response))
		require.NoError(t, err)
		require.Len(t, models, 2)

		internlm := models[0]
		assert.Equal(t, "internlm/internlm2_5-7b-chat", internlm.Name)
		assert.Equal(t, "lmdeploy", internlm.Type)
		// owned_by "lmdeploy" is the default — publisher should not be set
		require.NotNil(t, internlm.Details)
		assert.Nil(t, internlm.Details.Publisher)
		require.NotNil(t, internlm.Details.ModifiedAt)
		assert.Equal(t, time.Unix(1754535984, 0), *internlm.Details.ModifiedAt)

		llama := models[1]
		assert.Equal(t, "meta-llama/Meta-Llama-3.1-8B-Instruct", llama.Name)
		assert.Equal(t, "lmdeploy", llama.Type)
		require.NotNil(t, llama.Details)
		require.NotNil(t, llama.Details.Publisher)
		assert.Equal(t, "meta-llama", *llama.Details.Publisher)
	})

	t.Run("handles fine-tuned model with parent", func(t *testing.T) {
		t.Parallel()

		response := `{
			"object": "list",
			"data": [
				{
					"id": "custom/fine-tuned-internlm",
					"object": "model",
					"created": 1754535986,
					"owned_by": "custom-org",
					"parent": "internlm/internlm2_5-7b-chat"
				}
			]
		}`

		models, err := parser.Parse([]byte(response))
		require.NoError(t, err)
		require.Len(t, models, 1)

		model := models[0]
		require.NotNil(t, model.Details)
		require.NotNil(t, model.Details.ParentModel)
		assert.Equal(t, "internlm/internlm2_5-7b-chat", *model.Details.ParentModel)
		require.NotNil(t, model.Details.Publisher)
		assert.Equal(t, "custom-org", *model.Details.Publisher)
	})

	t.Run("skips models without ID", func(t *testing.T) {
		t.Parallel()

		response := `{
			"object": "list",
			"data": [
				{
					"object": "model",
					"created": 1754535987,
					"owned_by": "lmdeploy"
				},
				{
					"id": "valid-model",
					"object": "model",
					"created": 1754535988,
					"owned_by": "lmdeploy"
				}
			]
		}`

		models, err := parser.Parse([]byte(response))
		require.NoError(t, err)
		require.Len(t, models, 1)
		assert.Equal(t, "valid-model", models[0].Name)
	})

	t.Run("handles empty response bytes", func(t *testing.T) {
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

	t.Run("returns error for malformed JSON", func(t *testing.T) {
		t.Parallel()

		invalidJSON := `{"object": "list", "data": [{"id": "m", invalid}]}`
		models, err := parser.Parse([]byte(invalidJSON))
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to parse LMDeploy response")
		assert.Nil(t, models)
	})

	t.Run("details nil when no metadata beyond default owned_by", func(t *testing.T) {
		t.Parallel()

		response := `{
			"object": "list",
			"data": [
				{
					"id": "simple-model",
					"object": "model",
					"created": 0,
					"owned_by": "lmdeploy"
				}
			]
		}`

		models, err := parser.Parse([]byte(response))
		require.NoError(t, err)
		require.Len(t, models, 1)
		assert.Equal(t, "simple-model", models[0].Name)
		// No created timestamp and owned_by is the default — details should be nil
		assert.Nil(t, models[0].Details)
	})

	t.Run("no max_model_len field — LMDeploy differs from vLLM here", func(t *testing.T) {
		t.Parallel()

		// LMDeploy /v1/models does not include max_model_len; verify we tolerate
		// the field being absent (and don't panic or error if it somehow appears).
		response := `{
			"object": "list",
			"data": [
				{
					"id": "qwen/Qwen2-7B-Instruct",
					"object": "model",
					"created": 1754535990,
					"owned_by": "lmdeploy"
				}
			]
		}`

		models, err := parser.Parse([]byte(response))
		require.NoError(t, err)
		require.Len(t, models, 1)
		model := models[0]
		assert.Equal(t, "qwen/Qwen2-7B-Instruct", model.Name)
		// MaxContextLength is never populated from the wire response for LMDeploy
		if model.Details != nil {
			assert.Nil(t, model.Details.MaxContextLength)
		}
	})

	t.Run("preserves LastSeen timestamp", func(t *testing.T) {
		t.Parallel()

		response := `{
			"object": "list",
			"data": [
				{
					"id": "test-model",
					"object": "model",
					"created": 1754535991,
					"owned_by": "lmdeploy"
				}
			]
		}`

		before := time.Now()
		models, err := parser.Parse([]byte(response))
		after := time.Now()

		require.NoError(t, err)
		require.Len(t, models, 1)
		assert.True(t, !models[0].LastSeen.Before(before))
		assert.True(t, !models[0].LastSeen.After(after))
	})
}
