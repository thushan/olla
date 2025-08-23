package converter

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/thushan/olla/internal/core/domain"
)

func TestBaseConverter_FindProviderAlias(t *testing.T) {
	base := NewBaseConverter("ollama")

	tests := []struct {
		name     string
		model    *domain.UnifiedModel
		wantName string
		wantOk   bool
	}{
		{
			name: "finds ollama alias",
			model: &domain.UnifiedModel{
				Aliases: []domain.AliasEntry{
					{Name: "llama3:latest", Source: "ollama"},
					{Name: "llama3:70b", Source: "generated"},
				},
			},
			wantName: "llama3:latest",
			wantOk:   true,
		},
		{
			name: "no ollama alias",
			model: &domain.UnifiedModel{
				Aliases: []domain.AliasEntry{
					{Name: "microsoft/phi-4", Source: "lmstudio"},
				},
			},
			wantName: "",
			wantOk:   false,
		},
		{
			name: "empty aliases",
			model: &domain.UnifiedModel{
				Aliases: []domain.AliasEntry{},
			},
			wantName: "",
			wantOk:   false,
		},
		{
			name: "nil aliases",
			model: &domain.UnifiedModel{
				Aliases: nil,
			},
			wantName: "",
			wantOk:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotName, gotOk := base.FindProviderAlias(tt.model)
			assert.Equal(t, tt.wantName, gotName)
			assert.Equal(t, tt.wantOk, gotOk)
		})
	}
}

func TestBaseConverter_FindProviderEndpoint(t *testing.T) {
	base := NewBaseConverter("ollama")

	tests := []struct {
		name         string
		model        *domain.UnifiedModel
		providerName string
		wantNil      bool
		wantEndpoint string
	}{
		{
			name: "finds matching endpoint",
			model: &domain.UnifiedModel{
				SourceEndpoints: []domain.SourceEndpoint{
					{
						EndpointURL: "http://localhost:11434",
						NativeName:  "llama3:latest",
						State:       "loaded",
					},
					{
						EndpointURL: "http://localhost:1234",
						NativeName:  "something-else",
						State:       "loaded",
					},
				},
			},
			providerName: "llama3:latest",
			wantNil:      false,
			wantEndpoint: "http://localhost:11434",
		},
		{
			name: "no matching endpoint",
			model: &domain.UnifiedModel{
				SourceEndpoints: []domain.SourceEndpoint{
					{
						EndpointURL: "http://localhost:1234",
						NativeName:  "microsoft/phi-4",
						State:       "loaded",
					},
				},
			},
			providerName: "llama3:latest",
			wantNil:      true,
		},
		{
			name: "empty provider name",
			model: &domain.UnifiedModel{
				SourceEndpoints: []domain.SourceEndpoint{
					{
						EndpointURL: "http://localhost:11434",
						NativeName:  "llama3:latest",
						State:       "loaded",
					},
				},
			},
			providerName: "",
			wantNil:      true,
		},
		{
			name: "empty endpoints",
			model: &domain.UnifiedModel{
				SourceEndpoints: []domain.SourceEndpoint{},
			},
			providerName: "llama3:latest",
			wantNil:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := base.FindProviderEndpoint(tt.model, tt.providerName)
			if tt.wantNil {
				assert.Nil(t, got)
			} else {
				assert.NotNil(t, got)
				assert.Equal(t, tt.wantEndpoint, got.EndpointURL)
			}
		})
	}
}

func TestBaseConverter_ExtractMetadata(t *testing.T) {
	base := NewBaseConverter("test")

	metadata := map[string]interface{}{
		"string_val": "test",
		"int_val":    42,
		"float_val":  42.5,
		"bool_val":   true,
		"nil_val":    nil,
		"wrong_type": []string{"array"},
	}

	t.Run("extract string", func(t *testing.T) {
		assert.Equal(t, "test", base.ExtractMetadataString(metadata, "string_val"))
		assert.Equal(t, "", base.ExtractMetadataString(metadata, "missing"))
		assert.Equal(t, "", base.ExtractMetadataString(metadata, "int_val"))
		assert.Equal(t, "", base.ExtractMetadataString(nil, "string_val"))
	})

	t.Run("extract int", func(t *testing.T) {
		assert.Equal(t, 42, base.ExtractMetadataInt(metadata, "int_val"))
		assert.Equal(t, 42, base.ExtractMetadataInt(metadata, "float_val")) // Converts float64
		assert.Equal(t, 0, base.ExtractMetadataInt(metadata, "missing"))
		assert.Equal(t, 0, base.ExtractMetadataInt(metadata, "string_val"))
		assert.Equal(t, 0, base.ExtractMetadataInt(nil, "int_val"))
	})

	t.Run("extract bool", func(t *testing.T) {
		assert.Equal(t, true, base.ExtractMetadataBool(metadata, "bool_val"))
		assert.Equal(t, false, base.ExtractMetadataBool(metadata, "missing"))
		assert.Equal(t, false, base.ExtractMetadataBool(metadata, "string_val"))
		assert.Equal(t, false, base.ExtractMetadataBool(nil, "bool_val"))
	})
}

func TestConversionHelper(t *testing.T) {
	base := NewBaseConverter("ollama")
	now := time.Now()

	t.Run("with valid ollama model", func(t *testing.T) {
		model := &domain.UnifiedModel{
			ID:            "llama/3:70b-q4km",
			Family:        "llama",
			ParameterSize: "70b",
			Quantization:  "q4km",
			Aliases: []domain.AliasEntry{
				{Name: "llama3:latest", Source: "ollama"},
			},
			SourceEndpoints: []domain.SourceEndpoint{
				{
					EndpointURL: "http://localhost:11434",
					NativeName:  "llama3:latest",
					State:       "loaded",
					DiskSize:    40000000000,
				},
			},
			DiskSize: 35000000000, // Different from endpoint
			LastSeen: now,
			Metadata: map[string]interface{}{
				"digest": "sha256:abc123",
				"type":   "chat",
			},
		}

		helper := base.NewConversionHelper(model)

		assert.False(t, helper.ShouldSkip())
		assert.Equal(t, "llama3:latest", helper.Alias)
		assert.NotNil(t, helper.Endpoint)
		assert.Equal(t, int64(40000000000), helper.GetDiskSize()) // Uses endpoint size
		assert.Equal(t, "loaded", helper.GetState("unknown"))
		assert.Equal(t, "chat", helper.GetModelType("llm"))
		assert.Equal(t, "sha256:abc123", helper.GetMetadataString("digest"))
	})

	t.Run("with non-ollama model", func(t *testing.T) {
		model := &domain.UnifiedModel{
			ID: "phi/4:14.7b-q4km",
			Aliases: []domain.AliasEntry{
				{Name: "microsoft/phi-4", Source: "lmstudio"},
			},
			SourceEndpoints: []domain.SourceEndpoint{
				{
					EndpointURL: "http://localhost:1234",
					NativeName:  "microsoft/phi-4",
					State:       "loaded",
				},
			},
		}

		helper := base.NewConversionHelper(model)

		assert.True(t, helper.ShouldSkip())
		assert.Equal(t, "", helper.Alias)
		assert.Nil(t, helper.Endpoint)
	})

	t.Run("with model but no endpoint", func(t *testing.T) {
		model := &domain.UnifiedModel{
			ID: "llama/3:70b-q4km",
			Aliases: []domain.AliasEntry{
				{Name: "llama3:latest", Source: "ollama"},
			},
			SourceEndpoints: []domain.SourceEndpoint{}, // No endpoints
			DiskSize:        35000000000,
		}

		helper := base.NewConversionHelper(model)

		assert.False(t, helper.ShouldSkip()) // Has alias
		assert.Equal(t, "llama3:latest", helper.Alias)
		assert.Nil(t, helper.Endpoint)
		assert.Equal(t, int64(35000000000), helper.GetDiskSize()) // Uses model size
		assert.Equal(t, "unknown", helper.GetState("unknown"))    // Uses default
	})
}

func TestBaseConverter_DetermineModelType(t *testing.T) {
	base := NewBaseConverter("test")

	tests := []struct {
		name        string
		model       *domain.UnifiedModel
		defaultType string
		want        string
	}{
		{
			name: "from metadata type",
			model: &domain.UnifiedModel{
				Metadata: map[string]interface{}{
					"type": "chat",
				},
			},
			defaultType: "llm",
			want:        "chat",
		},
		{
			name: "vision capability",
			model: &domain.UnifiedModel{
				Capabilities: []string{"vision", "chat"},
			},
			defaultType: "llm",
			want:        "vlm",
		},
		{
			name: "embedding capability",
			model: &domain.UnifiedModel{
				Capabilities: []string{"embedding"},
			},
			defaultType: "llm",
			want:        "embeddings",
		},
		{
			name: "embeddings capability plural",
			model: &domain.UnifiedModel{
				Capabilities: []string{"embeddings"},
			},
			defaultType: "llm",
			want:        "embeddings",
		},
		{
			name:        "uses default",
			model:       &domain.UnifiedModel{},
			defaultType: "llm",
			want:        "llm",
		},
		{
			name: "metadata takes precedence over capabilities",
			model: &domain.UnifiedModel{
				Metadata: map[string]interface{}{
					"type": "custom",
				},
				Capabilities: []string{"vision"},
			},
			defaultType: "llm",
			want:        "custom",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := base.DetermineModelType(tt.model, tt.defaultType)
			assert.Equal(t, tt.want, got)
		})
	}
}
