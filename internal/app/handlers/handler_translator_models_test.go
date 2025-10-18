package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/thushan/olla/internal/adapter/translator/anthropic"
	"github.com/thushan/olla/internal/config"
	"github.com/thushan/olla/internal/core/domain"
)

func TestTranslatorModelsHandler_Success(t *testing.T) {
	// Create test application with models
	mockReg := &mockTranslatorRegistry{
		models: []*domain.UnifiedModel{
			{
				ID: "claude/3:opus",
				Aliases: []domain.AliasEntry{
					{Name: "claude-3-opus-20240229", Source: "anthropic"},
				},
				SourceEndpoints: []domain.SourceEndpoint{
					{EndpointURL: "http://localhost:8080"},
				},
			},
			{
				ID: "claude/3:sonnet",
				Aliases: []domain.AliasEntry{
					{Name: "claude-3-sonnet-20240229", Source: "anthropic"},
				},
				SourceEndpoints: []domain.SourceEndpoint{
					{EndpointURL: "http://localhost:8080"},
				},
			},
		},
	}

	app := &Application{
		logger:           &mockStyledLogger{},
		modelRegistry:    mockReg,
		discoveryService: &mockDiscoveryService{},
		repository:       &mockEndpointRepository{},
	}

	// Create Anthropic translator with test config
	testConfig := config.AnthropicTranslatorConfig{
		Enabled:        true,
		MaxMessageSize: 10 << 20, // 10MB
	}
	trans := anthropic.NewTranslator(app.logger, testConfig)

	// Create request
	req := httptest.NewRequest(http.MethodGet, "/olla/anthropic/v1/models", nil)
	w := httptest.NewRecorder()

	// Call handler
	handler := app.translatorModelsHandler(trans)
	handler(w, req)

	// Verify response
	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	// Verify response format
	var response map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Verify data field exists
	data, ok := response["data"].([]interface{})
	if !ok {
		t.Fatal("expected data field to be array")
	}

	// Verify we have models
	if len(data) != 2 {
		t.Errorf("expected 2 models, got %d", len(data))
	}

	// Verify model format matches Anthropic spec
	if len(data) > 0 {
		model := data[0].(map[string]interface{})
		requiredFields := []string{"id", "name", "created", "description", "type"}
		for _, field := range requiredFields {
			if _, ok := model[field]; !ok {
				t.Errorf("expected field %s in model response", field)
			}
		}

		// Verify type is "chat"
		if modelType, ok := model["type"].(string); ok {
			if modelType != "chat" {
				t.Errorf("expected type 'chat', got '%s'", modelType)
			}
		}
	}
}

func TestTranslatorModelsHandler_EmptyRegistry(t *testing.T) {
	// Create test application with empty registry
	app := createTranslatorTestApp(t)

	// Create Anthropic translator with test config
	testConfig := config.AnthropicTranslatorConfig{
		Enabled:        true,
		MaxMessageSize: 10 << 20, // 10MB
	}
	trans := anthropic.NewTranslator(app.logger, testConfig)

	// Create request
	req := httptest.NewRequest(http.MethodGet, "/olla/anthropic/v1/models", nil)
	w := httptest.NewRecorder()

	// Call handler
	handler := app.translatorModelsHandler(trans)
	handler(w, req)

	// Verify response
	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	// Verify response format
	var response map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Verify data field exists and is empty
	data, ok := response["data"].([]interface{})
	if !ok {
		t.Fatal("expected data field to be array")
	}

	if len(data) != 0 {
		t.Errorf("expected 0 models, got %d", len(data))
	}
}

func TestTranslatorModelsHandler_ResponseFormat(t *testing.T) {
	// Create test application with a single model
	mockReg := &mockTranslatorRegistry{
		models: []*domain.UnifiedModel{
			{
				ID: "test/model:v1",
				Aliases: []domain.AliasEntry{
					{Name: "test-model-v1", Source: "test"},
				},
				SourceEndpoints: []domain.SourceEndpoint{
					{EndpointURL: "http://localhost:8080"},
				},
			},
		},
	}

	app := &Application{
		logger:           &mockStyledLogger{},
		modelRegistry:    mockReg,
		discoveryService: &mockDiscoveryService{},
		repository:       &mockEndpointRepository{},
	}

	// Create Anthropic translator with test config
	testConfig := config.AnthropicTranslatorConfig{
		Enabled:        true,
		MaxMessageSize: 10 << 20, // 10MB
	}
	trans := anthropic.NewTranslator(app.logger, testConfig)

	// Create request
	req := httptest.NewRequest(http.MethodGet, "/olla/anthropic/v1/models", nil)
	w := httptest.NewRecorder()

	// Call handler
	handler := app.translatorModelsHandler(trans)
	handler(w, req)

	// Verify response
	var response map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Verify structure matches Python reference
	// {data: [{id, name, created, description, type}]}
	data, ok := response["data"].([]interface{})
	if !ok {
		t.Fatal("expected data field")
	}

	if len(data) == 0 {
		t.Fatal("expected at least one model")
	}

	model := data[0].(map[string]interface{})

	// Verify model has correct fields
	if _, ok := model["id"]; !ok {
		t.Error("expected id field")
	}

	// Verify created is unix timestamp
	if created, ok := model["created"].(float64); ok {
		if created <= 0 {
			t.Error("expected created to be positive unix timestamp")
		}
	} else {
		t.Error("expected created to be number")
	}
}

// mockTranslatorRegistry implements ModelRegistry for translator tests
type mockTranslatorRegistry struct {
	baseMockRegistry
	models []*domain.UnifiedModel
}

func (m *mockTranslatorRegistry) GetUnifiedModels(ctx context.Context) ([]*domain.UnifiedModel, error) {
	return m.models, nil
}

// createTranslatorTestApp creates a minimal test application for translator handler testing
func createTranslatorTestApp(t *testing.T) *Application {
	return &Application{
		logger:           &mockStyledLogger{},
		modelRegistry:    &mockTranslatorRegistry{models: []*domain.UnifiedModel{}},
		discoveryService: &mockDiscoveryService{},
		repository:       &mockEndpointRepository{},
	}
}
