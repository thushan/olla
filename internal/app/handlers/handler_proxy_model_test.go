package handlers

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thushan/olla/internal/adapter/inspector"
	"github.com/thushan/olla/internal/adapter/registry/profile"
	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/logger"
)

func TestFilterEndpointsByProfile_WithModel(t *testing.T) {
	// Create mock logger
	styledLog := &mockStyledLogger{}

	// Create test endpoints
	endpoint1, _ := url.Parse("http://localhost:11434")
	endpoint2, _ := url.Parse("http://localhost:11435")
	endpoint3, _ := url.Parse("http://localhost:11436")

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
			Type:      domain.ProfileLmStudio,
		},
	}

	// Mock model registry
	modelRegistry := &mockSimpleModelRegistry{
		endpointsForModel: map[string][]string{
			"llama3.1:8b": {"http://localhost:11434", "http://localhost:11435"},
			"codellama":   {"http://localhost:11436"},
		},
	}

	app := &Application{
		modelRegistry: modelRegistry,
		logger:        styledLog,
	}

	t.Run("filters by model when available", func(t *testing.T) {
		profile := &domain.RequestProfile{
			Path:        "/v1/chat/completions",
			ModelName:   "llama3.1:8b",
			SupportedBy: []string{domain.ProfileOllama, domain.ProfileLmStudio},
		}

		filtered := app.filterEndpointsByProfile(endpoints, profile, styledLog)

		// Should only return endpoint1 and endpoint2
		assert.Len(t, filtered, 2)
		assert.Contains(t, filtered, endpoints[0])
		assert.Contains(t, filtered, endpoints[1])
		assert.NotContains(t, filtered, endpoints[2])
	})

	t.Run("returns all compatible when model not found", func(t *testing.T) {
		profile := &domain.RequestProfile{
			Path:        "/v1/chat/completions",
			ModelName:   "gpt-4",
			SupportedBy: []string{domain.ProfileOllama, domain.ProfileLmStudio},
		}

		filtered := app.filterEndpointsByProfile(endpoints, profile, styledLog)

		// Should return all compatible endpoints
		assert.Len(t, filtered, 3)
	})

	t.Run("no model filtering when model not specified", func(t *testing.T) {
		profile := &domain.RequestProfile{
			Path:        "/v1/chat/completions",
			ModelName:   "", // No model specified
			SupportedBy: []string{domain.ProfileOllama, domain.ProfileLmStudio},
		}

		filtered := app.filterEndpointsByProfile(endpoints, profile, styledLog)

		// Should return all compatible endpoints
		assert.Len(t, filtered, 3)
	})
}

func TestBodyInspectorIntegration(t *testing.T) {
	// Create mock logger
	styledLog := &mockStyledLogger{}

	// Create profile factory
	profileFactory, err := profile.NewFactoryWithDefaults()
	require.NoError(t, err)

	// Create inspector chain
	inspectorFactory := inspector.NewFactory(profileFactory, styledLog)
	chain := inspectorFactory.CreateChain()

	// Add inspectors
	pathInspector := inspectorFactory.CreatePathInspector()
	bodyInspector := inspectorFactory.CreateBodyInspector()
	chain.AddInspector(pathInspector)
	chain.AddInspector(bodyInspector)

	tests := []struct {
		name          string
		path          string
		body          string
		expectedModel string
	}{
		{
			name:          "Extracts model from OpenAI format",
			path:          "/v1/chat/completions",
			body:          `{"model": "gpt-4", "messages": []}`,
			expectedModel: "gpt-4",
		},
		{
			name:          "Extracts model from Ollama format",
			path:          "/api/generate",
			body:          `{"model": "llama3.1:8b", "prompt": "Hello"}`,
			expectedModel: "llama3.1:8b",
		},
		{
			name:          "No model in request",
			path:          "/v1/chat/completions",
			body:          `{"messages": []}`,
			expectedModel: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create request
			req := httptest.NewRequest("POST", tt.path, bytes.NewBufferString(tt.body))
			req.Header.Set("Content-Type", "application/json")

			// Inspect
			profile, err := chain.Inspect(context.Background(), req, tt.path)
			require.NoError(t, err)

			// Verify model extraction
			assert.Equal(t, tt.expectedModel, profile.ModelName)

			// Verify body is still readable
			body, err := io.ReadAll(req.Body)
			require.NoError(t, err)
			assert.Equal(t, tt.body, string(body))
		})
	}
}

// Simple mock implementations for testing
type mockStyledLogger struct {
	underlying *slog.Logger
}

func (m *mockStyledLogger) Debug(msg string, args ...any)                                {}
func (m *mockStyledLogger) Info(msg string, args ...any)                                 {}
func (m *mockStyledLogger) Warn(msg string, args ...any)                                 {}
func (m *mockStyledLogger) Error(msg string, args ...any)                                {}
func (m *mockStyledLogger) ResetLine()                                                   {}
func (m *mockStyledLogger) InfoWithStatus(msg string, status string, args ...any)        {}
func (m *mockStyledLogger) InfoWithCount(msg string, count int, args ...any)             {}
func (m *mockStyledLogger) InfoWithEndpoint(msg string, endpoint string, args ...any)    {}
func (m *mockStyledLogger) InfoWithHealthCheck(msg string, endpoint string, args ...any) {}
func (m *mockStyledLogger) InfoWithNumbers(msg string, numbers ...int64)                 {}
func (m *mockStyledLogger) WarnWithEndpoint(msg string, endpoint string, args ...any)    {}
func (m *mockStyledLogger) ErrorWithEndpoint(msg string, endpoint string, args ...any)   {}
func (m *mockStyledLogger) InfoHealthy(msg string, endpoint string, args ...any)         {}
func (m *mockStyledLogger) InfoHealthStatus(msg string, name string, status domain.EndpointStatus, args ...any) {
}
func (m *mockStyledLogger) GetUnderlying() *slog.Logger                                         { return m.underlying }
func (m *mockStyledLogger) WithRequestID(requestID string) logger.StyledLogger                  { return m }
func (m *mockStyledLogger) WithPrefix(prefix string) logger.StyledLogger                        { return m }
func (m *mockStyledLogger) WithAttrs(attrs ...slog.Attr) logger.StyledLogger                    { return m }
func (m *mockStyledLogger) With(args ...any) logger.StyledLogger                                { return m }
func (m *mockStyledLogger) InfoWithContext(msg string, endpoint string, ctx logger.LogContext)  {}
func (m *mockStyledLogger) WarnWithContext(msg string, endpoint string, ctx logger.LogContext)  {}
func (m *mockStyledLogger) ErrorWithContext(msg string, endpoint string, ctx logger.LogContext) {}
func (m *mockStyledLogger) InfoConfigChange(oldName, newName string)                            {}

type mockSimpleModelRegistry struct {
	endpointsForModel map[string][]string
}

func (m *mockSimpleModelRegistry) RegisterModel(ctx context.Context, endpointURL string, model *domain.ModelInfo) error {
	return nil
}

func (m *mockSimpleModelRegistry) RegisterModels(ctx context.Context, endpointURL string, models []*domain.ModelInfo) error {
	return nil
}

func (m *mockSimpleModelRegistry) GetModelsForEndpoint(ctx context.Context, endpointURL string) ([]*domain.ModelInfo, error) {
	return nil, nil
}

func (m *mockSimpleModelRegistry) GetEndpointsForModel(ctx context.Context, modelName string) ([]string, error) {
	if endpoints, ok := m.endpointsForModel[modelName]; ok {
		return endpoints, nil
	}
	return []string{}, nil
}

func (m *mockSimpleModelRegistry) IsModelAvailable(ctx context.Context, modelName string) bool {
	_, ok := m.endpointsForModel[modelName]
	return ok
}

func (m *mockSimpleModelRegistry) GetAllModels(ctx context.Context) (map[string][]*domain.ModelInfo, error) {
	return nil, nil
}

func (m *mockSimpleModelRegistry) GetEndpointModelMap(ctx context.Context) (map[string]*domain.EndpointModels, error) {
	return nil, nil
}

func (m *mockSimpleModelRegistry) RemoveEndpoint(ctx context.Context, endpointURL string) error {
	return nil
}

func (m *mockSimpleModelRegistry) GetStats(ctx context.Context) (domain.RegistryStats, error) {
	return domain.RegistryStats{}, nil
}

func (m *mockSimpleModelRegistry) ModelsToString(models []*domain.ModelInfo) string {
	return ""
}

func (m *mockSimpleModelRegistry) ModelsToStrings(models []*domain.ModelInfo) []string {
	return []string{}
}
