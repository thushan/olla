package profile

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOllamaParser_CapabilitiesTyping(t *testing.T) {
	t.Parallel()

	parser := &ollamaParser{}

	tests := []struct {
		name     string
		body     string
		wantType string // empty means Type must be nil
		wantCaps []string
	}{
		{
			name:     "embedding only is typed embeddings",
			body:     `{"models":[{"name":"all-minilm:latest","details":{"family":"bert"},"capabilities":["embedding"]}]}`,
			wantType: "embeddings",
		},
		{
			name:     "completion vision tools is a vlm with vision and function calling",
			body:     `{"models":[{"name":"qwen3-vl:8b","details":{"family":"qwen3vl"},"capabilities":["completion","vision","tools","thinking"]}]}`,
			wantType: "vlm",
			wantCaps: []string{"vision", "function_calling"},
		},
		{
			name:     "completion plus vision is a generative vlm",
			body:     `{"models":[{"name":"llava:13b","capabilities":["completion","vision"]}]}`,
			wantType: "vlm",
			wantCaps: []string{"vision"},
		},
		{
			name:     "vision without a completion entry is still a vlm",
			body:     `{"models":[{"name":"vision-only:latest","capabilities":["vision"]}]}`,
			wantType: "vlm",
			wantCaps: []string{"vision"},
		},
		{
			name:     "completion plus tools has the capability but no kind",
			body:     `{"models":[{"name":"llama3.1:8b","capabilities":["completion","tools"]}]}`,
			wantCaps: []string{"function_calling"},
		},
		{
			name:     "embedding plus vision stays embedding only",
			body:     `{"models":[{"name":"odd:latest","capabilities":["embedding","vision"]}]}`,
			wantType: "embeddings",
		},
		{
			name: "missing capabilities (older Ollama) stays untyped",
			body: `{"models":[{"name":"llama3:latest","digest":"abc","details":{"family":"llama"}}]}`,
		},
		{
			name: "embedding plus completion stays untyped",
			body: `{"models":[{"name":"hybrid:latest","details":{"family":"x"},"capabilities":["embedding","completion"]}]}`,
		},
		{
			name:     "embedding plus unknown value is still embedding only",
			body:     `{"models":[{"name":"rerank:latest","capabilities":["embedding","rerank"]}]}`,
			wantType: "embeddings",
		},
		{
			name: "only unknown values stays untyped",
			body: `{"models":[{"name":"foo:latest","capabilities":["foo"]}]}`,
		},
		{
			name: "empty capabilities stays untyped",
			body: `{"models":[{"name":"x:latest","details":{"family":"x"},"capabilities":[]}]}`,
		},
		{
			name:     "capabilities alone still create details",
			body:     `{"models":[{"name":"nomic-embed-text:latest","capabilities":["embedding"]}]}`,
			wantType: "embeddings",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			models, err := parser.Parse([]byte(tt.body))
			require.NoError(t, err)
			require.Len(t, models, 1)

			if tt.wantType == "" {
				if models[0].Details != nil {
					assert.Nil(t, models[0].Details.Type)
				}
			} else {
				require.NotNil(t, models[0].Details)
				require.NotNil(t, models[0].Details.Type)
				assert.Equal(t, tt.wantType, *models[0].Details.Type)
			}
			if len(tt.wantCaps) == 0 {
				if models[0].Details != nil {
					assert.Empty(t, models[0].Details.Capabilities)
				}
				return
			}
			require.NotNil(t, models[0].Details)
			assert.ElementsMatch(t, tt.wantCaps, models[0].Details.Capabilities)
		})
	}
}

func TestOllamaParser_CapabilitiesLeaveOtherDetailsAlone(t *testing.T) {
	t.Parallel()

	body := `{"models":[{"name":"all-minilm:latest","digest":"d1","details":{"family":"bert","parameter_size":"23M","quantization_level":"F16"},"capabilities":["embedding"]}]}`
	models, err := (&ollamaParser{}).Parse([]byte(body))
	require.NoError(t, err)
	require.Len(t, models, 1)
	d := models[0].Details
	require.NotNil(t, d)
	assert.Equal(t, "bert", *d.Family)
	assert.Equal(t, "23M", *d.ParameterSize)
	assert.Equal(t, "F16", *d.QuantizationLevel)
	assert.Equal(t, "d1", *d.Digest)
}

func TestOllamaParser_NoCapabilitiesNoDetails(t *testing.T) {
	t.Parallel()

	models, err := (&ollamaParser{}).Parse([]byte(`{"models":[{"name":"bare"}]}`))
	require.NoError(t, err)
	require.Len(t, models, 1)
	assert.Nil(t, models[0].Details)
}
