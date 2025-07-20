package handlers

import (
	"context"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/thushan/olla/internal/core/domain"
)

func TestFilterEndpointsByCapabilities(t *testing.T) {
	// Create mock logger
	styledLog := &mockStyledLogger{}

	// Create test endpoints
	endpoint1, _ := url.Parse("http://localhost:11434")
	endpoint2, _ := url.Parse("http://localhost:11435")
	endpoint3, _ := url.Parse("http://localhost:11436")
	endpoint4, _ := url.Parse("http://localhost:11437")

	endpoints := []*domain.Endpoint{
		{
			Name:      "endpoint1",
			URL:       endpoint1,
			URLString: "http://localhost:11434",
			Type:      domain.ProfileOllama,
		},
		{
			Name:      "endpoint2",
			URL:       endpoint2,
			URLString: "http://localhost:11435",
			Type:      domain.ProfileOllama,
		},
		{
			Name:      "endpoint3",
			URL:       endpoint3,
			URLString: "http://localhost:11436",
			Type:      domain.ProfileOpenAICompatible,
		},
		{
			Name:      "endpoint4",
			URL:       endpoint4,
			URLString: "http://localhost:11437",
			Type:      domain.ProfileOpenAICompatible,
		},
	}

	// Mock model registry with capability support
	modelRegistry := &mockCapabilityModelRegistry{
		modelsByCapability: map[string][]*domain.UnifiedModel{
			"vision": {
				{
					ID:           "llava:13b",
					Capabilities: []string{"vision", "chat", "streaming"},
					SourceEndpoints: []domain.SourceEndpoint{
						{EndpointURL: "http://localhost:11434", LastSeen: time.Now()},
						{EndpointURL: "http://localhost:11435", LastSeen: time.Now()},
					},
				},
				{
					ID:           "gpt-4-vision",
					Capabilities: []string{"vision", "chat", "function_calling"},
					SourceEndpoints: []domain.SourceEndpoint{
						{EndpointURL: "http://localhost:11436", LastSeen: time.Now()},
					},
				},
			},
			"function_calling": {
				{
					ID:           "gpt-4-turbo",
					Capabilities: []string{"chat", "function_calling", "streaming"},
					SourceEndpoints: []domain.SourceEndpoint{
						{EndpointURL: "http://localhost:11436", LastSeen: time.Now()},
						{EndpointURL: "http://localhost:11437", LastSeen: time.Now()},
					},
				},
				{
					ID:           "gpt-4-vision",
					Capabilities: []string{"vision", "chat", "function_calling"},
					SourceEndpoints: []domain.SourceEndpoint{
						{EndpointURL: "http://localhost:11436", LastSeen: time.Now()},
					},
				},
			},
			"tools": {
				{
					ID:           "gpt-4-turbo",
					Capabilities: []string{"chat", "function_calling", "streaming"},
					SourceEndpoints: []domain.SourceEndpoint{
						{EndpointURL: "http://localhost:11436", LastSeen: time.Now()},
						{EndpointURL: "http://localhost:11437", LastSeen: time.Now()},
					},
				},
			},
			"embeddings": {
				{
					ID:           "text-embedding-ada-002",
					Capabilities: []string{"embeddings"},
					SourceEndpoints: []domain.SourceEndpoint{
						{EndpointURL: "http://localhost:11437", LastSeen: time.Now()},
					},
				},
			},
			"code": {
				{
					ID:           "codellama:34b",
					Capabilities: []string{"code", "chat", "streaming"},
					SourceEndpoints: []domain.SourceEndpoint{
						{EndpointURL: "http://localhost:11434", LastSeen: time.Now()},
					},
				},
			},
		},
	}

	app := &Application{
		modelRegistry: modelRegistry,
		logger:        styledLog,
	}

	tests := []struct {
		name              string
		capabilities      *domain.ModelCapabilities
		expectedEndpoints []*domain.Endpoint
		expectedCount     int
		description       string
	}{
		{
			name: "vision request filters to vision-capable endpoints",
			capabilities: &domain.ModelCapabilities{
				VisionUnderstanding: true,
				ChatCompletion:      true,
				TextGeneration:      true,
				StreamingSupport:    true,
			},
			expectedCount: 3, // endpoints 1, 2, 3 have vision models
			description:   "Should filter to endpoints with vision-capable models",
		},
		{
			name: "function calling request",
			capabilities: &domain.ModelCapabilities{
				FunctionCalling:  true,
				ChatCompletion:   true,
				TextGeneration:   true,
				StreamingSupport: true,
			},
			expectedCount: 2, // endpoints 3, 4 have function calling models
			description:   "Should filter to endpoints with function calling models",
		},
		{
			name: "embeddings request",
			capabilities: &domain.ModelCapabilities{
				Embeddings:       true,
				ChatCompletion:   false,
				TextGeneration:   false,
				StreamingSupport: true,
			},
			expectedCount: 1, // only endpoint 4 has embeddings
			description:   "Should filter to endpoints with embedding models",
		},
		{
			name: "code generation request",
			capabilities: &domain.ModelCapabilities{
				CodeGeneration:   true,
				ChatCompletion:   true,
				TextGeneration:   true,
				StreamingSupport: true,
			},
			expectedCount: 1, // only endpoint 1 has code models
			description:   "Should filter to endpoints with code generation models",
		},
		{
			name: "combined vision and function calling",
			capabilities: &domain.ModelCapabilities{
				VisionUnderstanding: true,
				FunctionCalling:     true,
				ChatCompletion:      true,
				TextGeneration:      true,
				StreamingSupport:    true,
			},
			expectedCount: 1, // only endpoint 3 has both capabilities (gpt-4-vision)
			description:   "Should filter to endpoints with both vision and function calling",
		},
		{
			name: "no special capabilities",
			capabilities: &domain.ModelCapabilities{
				ChatCompletion:   true,
				TextGeneration:   true,
				StreamingSupport: true,
			},
			expectedCount: 4, // all endpoints returned when no special capabilities
			description:   "Should return all endpoints when no special capabilities required",
		},
		{
			name:          "nil capabilities",
			capabilities:  nil,
			expectedCount: 4, // all endpoints returned
			description:   "Should return all endpoints when capabilities is nil",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			profile := &domain.RequestProfile{
				Path:              "/v1/chat/completions",
				ModelCapabilities: tt.capabilities,
				SupportedBy:       []string{domain.ProfileOllama, domain.ProfileOpenAICompatible},
			}

			filtered := app.filterEndpointsByCapabilities(endpoints, profile, styledLog)

			assert.Len(t, filtered, tt.expectedCount, tt.description)
		})
	}
}

func TestFilterEndpointsByProfile_WithCapabilities(t *testing.T) {
	// Create mock logger
	styledLog := &mockStyledLogger{}

	// Create test endpoints
	endpoint1, _ := url.Parse("http://localhost:11434")
	endpoint2, _ := url.Parse("http://localhost:11435")
	endpoint3, _ := url.Parse("http://localhost:11436")

	endpoints := []*domain.Endpoint{
		{
			Name:      "ollama-vision",
			URL:       endpoint1,
			URLString: "http://localhost:11434",
			Type:      domain.ProfileOllama,
		},
		{
			Name:      "ollama-basic",
			URL:       endpoint2,
			URLString: "http://localhost:11435",
			Type:      domain.ProfileOllama,
		},
		{
			Name:      "openai",
			URL:       endpoint3,
			URLString: "http://localhost:11436",
			Type:      domain.ProfileOpenAICompatible,
		},
	}

	// Mock model registry with both model and capability support
	modelRegistry := &mockFullModelRegistry{
		endpointsForModel: map[string][]string{
			"llava:13b":    {"http://localhost:11434"},
			"llama3.1:8b":  {"http://localhost:11435"},
			"gpt-4-vision": {"http://localhost:11436"},
		},
		modelsByCapability: map[string][]*domain.UnifiedModel{
			"vision": {
				{
					ID:           "llava:13b",
					Capabilities: []string{"vision", "chat"},
					SourceEndpoints: []domain.SourceEndpoint{
						{EndpointURL: "http://localhost:11434", LastSeen: time.Now()},
					},
				},
				{
					ID:           "gpt-4-vision",
					Capabilities: []string{"vision", "chat", "function_calling"},
					SourceEndpoints: []domain.SourceEndpoint{
						{EndpointURL: "http://localhost:11436", LastSeen: time.Now()},
					},
				},
			},
		},
	}

	app := &Application{
		modelRegistry: modelRegistry,
		logger:        styledLog,
	}

	t.Run("capability filtering takes precedence over model name", func(t *testing.T) {
		profile := &domain.RequestProfile{
			Path:      "/v1/chat/completions",
			ModelName: "llava:13b",
			ModelCapabilities: &domain.ModelCapabilities{
				VisionUnderstanding: true,
				ChatCompletion:      true,
			},
			SupportedBy: []string{domain.ProfileOllama, domain.ProfileOpenAICompatible},
		}

		filtered := app.filterEndpointsByProfile(endpoints, profile, styledLog)

		// Should return only endpoint1 (has llava:13b with vision)
		assert.Len(t, filtered, 1)
		assert.Equal(t, "ollama-vision", filtered[0].Name)
	})

	t.Run("falls back to profile filtering when no capability match", func(t *testing.T) {
		profile := &domain.RequestProfile{
			Path: "/v1/chat/completions",
			ModelCapabilities: &domain.ModelCapabilities{
				Embeddings: true, // No endpoints have embedding models
			},
			SupportedBy: []string{domain.ProfileOllama, domain.ProfileOpenAICompatible},
		}

		filtered := app.filterEndpointsByProfile(endpoints, profile, styledLog)

		// Should return all compatible endpoints since no capability match
		assert.Len(t, filtered, 3)
	})
}

// Mock implementations for capability testing
type mockCapabilityModelRegistry struct {
	mockSimpleModelRegistry
	modelsByCapability map[string][]*domain.UnifiedModel
}

func (m *mockCapabilityModelRegistry) GetModelsByCapability(ctx context.Context, capability string) ([]*domain.UnifiedModel, error) {
	if models, ok := m.modelsByCapability[capability]; ok {
		return models, nil
	}
	return []*domain.UnifiedModel{}, nil
}

// Full mock with both model and capability support
type mockFullModelRegistry struct {
	endpointsForModel  map[string][]string
	modelsByCapability map[string][]*domain.UnifiedModel
}

func (m *mockFullModelRegistry) RegisterModel(ctx context.Context, endpointURL string, model *domain.ModelInfo) error {
	return nil
}

func (m *mockFullModelRegistry) RegisterModels(ctx context.Context, endpointURL string, models []*domain.ModelInfo) error {
	return nil
}

func (m *mockFullModelRegistry) GetModelsForEndpoint(ctx context.Context, endpointURL string) ([]*domain.ModelInfo, error) {
	return nil, nil
}

func (m *mockFullModelRegistry) GetEndpointsForModel(ctx context.Context, modelName string) ([]string, error) {
	if endpoints, ok := m.endpointsForModel[modelName]; ok {
		return endpoints, nil
	}
	return []string{}, nil
}

func (m *mockFullModelRegistry) IsModelAvailable(ctx context.Context, modelName string) bool {
	_, ok := m.endpointsForModel[modelName]
	return ok
}

func (m *mockFullModelRegistry) GetAllModels(ctx context.Context) (map[string][]*domain.ModelInfo, error) {
	return nil, nil
}

func (m *mockFullModelRegistry) GetEndpointModelMap(ctx context.Context) (map[string]*domain.EndpointModels, error) {
	return nil, nil
}

func (m *mockFullModelRegistry) RemoveEndpoint(ctx context.Context, endpointURL string) error {
	return nil
}

func (m *mockFullModelRegistry) GetStats(ctx context.Context) (domain.RegistryStats, error) {
	return domain.RegistryStats{}, nil
}

func (m *mockFullModelRegistry) ModelsToString(models []*domain.ModelInfo) string {
	return ""
}

func (m *mockFullModelRegistry) ModelsToStrings(models []*domain.ModelInfo) []string {
	return []string{}
}

func (m *mockFullModelRegistry) GetModelsByCapability(ctx context.Context, capability string) ([]*domain.UnifiedModel, error) {
	if models, ok := m.modelsByCapability[capability]; ok {
		return models, nil
	}
	return []*domain.UnifiedModel{}, nil
}

// Ensure our mocks implement the interface
var _ domain.ModelRegistry = (*mockCapabilityModelRegistry)(nil)
var _ domain.ModelRegistry = (*mockFullModelRegistry)(nil)
