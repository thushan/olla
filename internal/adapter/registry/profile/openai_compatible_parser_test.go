package profile

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpenAIParser_ParseInfinityTopLevelModels(t *testing.T) {
	parser := &openAIParser{}

	response := `{
		"models": [
			{
				"id": "BAAI/bge-m3",
				"object": "model",
				"created": 1734000007,
				"owned_by": "infinity"
			}
		]
	}`

	models, err := parser.Parse([]byte(response))
	require.NoError(t, err)
	require.Len(t, models, 1)
	assert.Equal(t, "BAAI/bge-m3", models[0].Name)
	assert.Equal(t, "model", models[0].Type)
}
