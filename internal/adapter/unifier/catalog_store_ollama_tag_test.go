package unifier

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/core/ports"
)

func unifyOn(t *testing.T, u ports.ModelUnifier, endpointType, url string, names ...string) {
	t.Helper()
	models := make([]*domain.ModelInfo, 0, len(names))
	for _, n := range names {
		models = append(models, &domain.ModelInfo{Name: n})
	}
	_, err := u.UnifyModels(context.Background(), models, &domain.Endpoint{URLString: url, Name: url, Type: endpointType})
	require.NoError(t, err)
}

// Exact first, Ollama only: a bare name must never reach a model another
// platform serves, because that platform would be sent a name it does not know.
func TestCatalogResolveByName_OllamaDefaultTag(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		query  string
		wantID string // empty means no match
		setup  func(t *testing.T, u ports.ModelUnifier)
	}{
		{"bare resolves to ollama latest", "all-minilm", "all-minilm:latest", func(t *testing.T, u ports.ModelUnifier) {
			unifyOn(t, u, "ollama", "http://o:11434", "all-minilm:latest")
		}},
		{"upper case resolves", "ALL-MINILM", "all-minilm:latest", func(t *testing.T, u ports.ModelUnifier) {
			unifyOn(t, u, "ollama", "http://o:11434", "all-minilm:latest")
		}},
		{"tagged is unchanged", "all-minilm:latest", "all-minilm:latest", func(t *testing.T, u ports.ModelUnifier) {
			unifyOn(t, u, "ollama", "http://o:11434", "all-minilm:latest")
		}},
		{"other tags are not latest", "qwen2.5", "", func(t *testing.T, u ports.ModelUnifier) {
			unifyOn(t, u, "ollama", "http://o:11434", "qwen2.5:0.5b")
		}},
		{"vllm latest does not answer a bare name", "foo", "", func(t *testing.T, u ports.ModelUnifier) {
			unifyOn(t, u, "vllm", "http://v:8000", "foo:latest")
		}},
		{"vllm bare does not answer latest", "foo:latest", "", func(t *testing.T, u ports.ModelUnifier) {
			unifyOn(t, u, "vllm", "http://v:8000", "foo")
		}},
		{"exact vllm bare wins over ollama latest", "foo", "foo", func(t *testing.T, u ports.ModelUnifier) {
			unifyOn(t, u, "ollama", "http://o:11434", "foo:latest")
			unifyOn(t, u, "vllm", "http://v:8000", "foo")
		}},
		{"a merged model also served by ollama answers the bare name", "foo", "foo:latest", func(t *testing.T, u ports.ModelUnifier) {
			unifyOn(t, u, "vllm", "http://v:8000", "foo:latest")
			unifyOn(t, u, "ollama", "http://o:11434", "foo:latest")
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			u := NewDefaultUnifier()
			tt.setup(t, u)

			got, err := u.ResolveAlias(context.Background(), tt.query)
			if tt.wantID == "" {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantID, got.ID)
		})
	}
}
