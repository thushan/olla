package domain

import "testing"

func TestModelKindFromMetadata(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		metadata map[string]any
		want     string
	}{
		{"nil metadata", nil, ""},
		{"model_type alone", map[string]any{"model_type": "vlm"}, "vlm"},
		{"type alone", map[string]any{"type": "embeddings"}, "embeddings"},
		{"specific model_type beats generic model marker", map[string]any{"type": "model", "model_type": "embeddings"}, "embeddings"},
		{"model_type beats a differing type", map[string]any{"type": "embeddings", "model_type": "reranker"}, "reranker"},
		{"generic marker alone is no kind", map[string]any{"type": "model"}, ""},
		{"empty model_type falls back to type", map[string]any{"type": "llm", "model_type": ""}, "llm"},
		{"non-string values are ignored", map[string]any{"type": 3, "model_type": true}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := ModelKindFromMetadata(tt.metadata); got != tt.want {
				t.Errorf("ModelKindFromMetadata(%v) = %q, want %q", tt.metadata, got, tt.want)
			}
		})
	}
}
