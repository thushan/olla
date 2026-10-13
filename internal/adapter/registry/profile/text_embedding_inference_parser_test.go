package profile

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/thushan/olla/internal/core/constants"
	"github.com/thushan/olla/internal/core/domain"
)

// teiInfoSample mirrors a real TEI /info payload: the served name is the
// Hugging Face repo ID while model_id is the on-disk checkpoint path.
const teiInfoSample = `{
	"model_id": "/data/snowflake-arctic-embed-l-v2.0",
	"model_sha": null,
	"model_dtype": "float16",
	"served_model_name": "Snowflake/snowflake-arctic-embed-l-v2.0",
	"max_input_length": 8192,
	"model_type": {
		"embedding": {
			"pooling": "cls"
		}
	}
}`

func TestTextEmbeddingInferenceParser_Parse(t *testing.T) {
	t.Parallel()

	parser := &textEmbeddingInferenceParser{}

	t.Run("prefers served_model_name over model_id", func(t *testing.T) {
		t.Parallel()

		models, err := parser.Parse([]byte(teiInfoSample))
		require.NoError(t, err)
		require.Len(t, models, 1)
		assert.Equal(t, "Snowflake/snowflake-arctic-embed-l-v2.0", models[0].Name)
	})

	t.Run("falls back to model_id when served_model_name is absent", func(t *testing.T) {
		t.Parallel()

		response := `{"model_id": "/data/sentence-transformers/all-MiniLM-L6-v2"}`
		models, err := parser.Parse([]byte(response))
		require.NoError(t, err)
		require.Len(t, models, 1)
		assert.Equal(t, "/data/sentence-transformers/all-MiniLM-L6-v2", models[0].Name)
	})

	t.Run("falls back to model_id when served_model_name is empty", func(t *testing.T) {
		t.Parallel()

		response := `{"model_id": "/data/nomic-embed-text-v1.5", "served_model_name": ""}`
		models, err := parser.Parse([]byte(response))
		require.NoError(t, err)
		require.Len(t, models, 1)
		assert.Equal(t, "/data/nomic-embed-text-v1.5", models[0].Name)
	})

	t.Run("returns error when neither field is present", func(t *testing.T) {
		t.Parallel()

		// Error rather than an empty list: an empty registration would silently
		// deregister the endpoint's model, while an error feeds the discovery
		// failure count and pause machinery.
		_, err := parser.Parse([]byte(`{"model_dtype": "float16"}`))
		assert.Error(t, err)
	})

	t.Run("returns error for empty body", func(t *testing.T) {
		t.Parallel()

		// Unlike the list parsers (where an empty list is a legitimate state),
		// a TEI instance always serves exactly one model, so an empty /info
		// response is a failure worth surfacing.
		_, err := parser.Parse([]byte{})
		assert.Error(t, err)
	})

	t.Run("returns error for invalid JSON", func(t *testing.T) {
		t.Parallel()

		_, err := parser.Parse([]byte(`{"served_model_name": `))
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to parse Text Embedding Inference response")
	})
}

func TestGetParserForFormat_TextEmbeddingInference(t *testing.T) {
	t.Parallel()

	parser := getParserForFormat(constants.ProviderTypeTextEmbeddingInference)
	require.IsType(t, &textEmbeddingInferenceParser{}, parser)

	models, err := parser.Parse([]byte(teiInfoSample))
	require.NoError(t, err)
	require.Len(t, models, 1)
	assert.Equal(t, "Snowflake/snowflake-arctic-embed-l-v2.0", models[0].Name)
}

// TestShippedTextEmbeddingInferenceProfile loads the real shipped YAML (not a
// fixture) and asserts the wiring that makes TEI discovery work end to end:
// /info as the discovery path, /health for liveness, OpenAI compatibility for
// /v1/embeddings traffic, embeddings-only capabilities, and the dedicated
// parser selected by response_format.
func TestShippedTextEmbeddingInferenceProfile(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile(shippedProfilesDir + "text-embedding-inference.yaml")
	require.NoError(t, err, "shipped profile file must exist")

	var cfg domain.ProfileConfig
	require.NoError(t, yaml.Unmarshal(data, &cfg), "shipped profile must be valid YAML")

	profile := NewConfigurableProfile(&cfg)

	assert.Equal(t, constants.ProviderTypeTextEmbeddingInference, profile.GetName())
	assert.True(t, profile.IsOpenAPICompatible(), "TEI /v1/embeddings is OpenAI-compatible")
	assert.Equal(t, "/health", profile.GetHealthCheckPath())
	assert.Equal(t, "http://127.0.0.1:80/info", profile.GetModelDiscoveryURL("http://127.0.0.1:80"))

	caps := profile.GetModelCapabilities("Snowflake/snowflake-arctic-embed-l-v2.0", nil)
	assert.True(t, caps.Embeddings, "TEI models must be flagged as embedding models")
	assert.False(t, caps.ChatCompletion, "TEI models must not be offered for chat")
	assert.False(t, caps.TextGeneration, "TEI models must not be offered for text generation")
	assert.False(t, caps.FunctionCalling, "TEI models must not be offered for function calling")

	models, err := profile.ParseModelsResponse([]byte(teiInfoSample))
	require.NoError(t, err)
	require.Len(t, models, 1)
	assert.Equal(t, "Snowflake/snowflake-arctic-embed-l-v2.0", models[0].Name)
}
