package unifier

import (
	"testing"

	"github.com/thushan/olla/internal/core/constants"
)

func TestDetectPlatform(t *testing.T) {
	tests := []struct {
		name         string
		format       string
		metadata     map[string]interface{}
		endpointType string
		want         string
	}{
		{
			name:         "metadata platform hint",
			metadata:     map[string]interface{}{"platform": "CUSTOM"},
			endpointType: "lmstudio",
			want:         "custom",
		},
		{
			name:         "ollama version in metadata",
			metadata:     map[string]interface{}{"ollama.version": "0.1.0"},
			endpointType: "lmstudio",
			want:         constants.ProviderTypeOllama,
		},
		{
			name:         "lmstudio version in metadata",
			metadata:     map[string]interface{}{"lmstudio.version": "1.0.0"},
			endpointType: "openai",
			want:         constants.ProviderTypeLMStudio,
		},
		{
			name:         "endpoint type used when no metadata hints",
			metadata:     map[string]interface{}{},
			endpointType: "lmstudio",
			want:         "lmstudio",
		},
		{
			name:         "endpoint type lowercase",
			metadata:     map[string]interface{}{},
			endpointType: "LMStudio",
			want:         "lmstudio",
		},
		{
			name:         "endpoint type with hyphen normalized",
			metadata:     map[string]interface{}{},
			endpointType: "lm-studio",
			want:         "lmstudio",
		},
		{
			name:         "endpoint type with underscore normalized",
			metadata:     map[string]interface{}{},
			endpointType: "lm_studio",
			want:         "lmstudio",
		},
		{
			name:         "gguf format defaults to ollama when no endpoint type",
			format:       "gguf",
			metadata:     map[string]interface{}{},
			endpointType: "",
			want:         constants.ProviderTypeOllama,
		},
		{
			name:         "endpoint type overrides gguf format",
			format:       "gguf",
			metadata:     map[string]interface{}{},
			endpointType: "lmstudio",
			want:         "lmstudio",
		},
		{
			name:         "default to openai when no hints",
			format:       "",
			metadata:     map[string]interface{}{},
			endpointType: "",
			want:         constants.ProviderTypeOpenAI,
		},
	}

	extractor := &ModelExtractor{}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractor.DetectPlatform(tt.format, tt.metadata, tt.endpointType)
			if got != tt.want {
				t.Errorf("DetectPlatform() = %v, want %v", got, tt.want)
			}
		})
	}
}
