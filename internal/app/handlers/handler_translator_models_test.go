package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

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
	trans, err := anthropic.NewTranslator(app.logger, testConfig)
	if err != nil {
		t.Fatalf("failed to create translator: %v", err)
	}

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
	var response map[string]any
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Verify data field exists
	data, ok := response["data"].([]any)
	if !ok {
		t.Fatal("expected data field to be array")
	}

	// Verify we have models
	if len(data) != 2 {
		t.Errorf("expected 2 models, got %d", len(data))
	}

	// Verify the response carries Anthropic-compliant envelope fields.
	if _, ok := response["has_more"]; !ok {
		t.Error("expected has_more field in response envelope")
	}
	if _, ok := response["first_id"]; !ok {
		t.Error("expected first_id field in response envelope")
	}
	if _, ok := response["last_id"]; !ok {
		t.Error("expected last_id field in response envelope")
	}

	// Verify per-entry shape matches the Anthropic spec wire format.
	if len(data) > 0 {
		model := data[0].(map[string]any)

		// "type" must be "model" — the Anthropic SDK uses it as a deserialise discriminator.
		if modelType, ok := model["type"].(string); !ok || modelType != "model" {
			t.Errorf("expected type \"model\", got %q", model["type"])
		}

		// "display_name" replaces "name".
		if _, ok := model["display_name"]; !ok {
			t.Error("expected display_name field")
		}
		if _, present := model["name"]; present {
			t.Error("unexpected legacy name field; Anthropic spec uses display_name")
		}

		// "created_at" must be an RFC3339 string, not a unix int.
		createdAt, ok := model["created_at"].(string)
		if !ok {
			t.Errorf("expected created_at to be a string, got %T", model["created_at"])
		} else if _, parseErr := time.Parse(time.RFC3339, createdAt); parseErr != nil {
			t.Errorf("created_at %q is not valid RFC3339: %v", createdAt, parseErr)
		}
		if _, present := model["created"]; present {
			t.Error("unexpected legacy created field; Anthropic spec uses created_at (RFC3339 string)")
		}

		// created_at must be stable across repeated calls — it must not re-evaluate
		// time.Now() on every request, which would make caching and idempotency impossible.
		req2 := httptest.NewRequest(http.MethodGet, "/olla/anthropic/v1/models", nil)
		w2 := httptest.NewRecorder()
		handler(w2, req2)

		var response2 map[string]any
		if err2 := json.NewDecoder(w2.Body).Decode(&response2); err2 != nil {
			t.Fatalf("failed to decode second response: %v", err2)
		}
		if data2, ok2 := response2["data"].([]any); ok2 && len(data2) > 0 {
			model2 := data2[0].(map[string]any)
			createdAt2, ok2 := model2["created_at"].(string)
			if !ok2 {
				t.Errorf("expected created_at to be a string on second call, got %T", model2["created_at"])
			} else if createdAt2 != createdAt {
				t.Errorf("created_at changed between calls: %q → %q; must be stable", createdAt, createdAt2)
			}
		}

		// "description" is not in the Anthropic spec.
		if _, present := model["description"]; present {
			t.Error("unexpected description field; not in Anthropic spec")
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
	trans, err := anthropic.NewTranslator(app.logger, testConfig)
	if err != nil {
		t.Fatalf("failed to create translator: %v", err)
	}

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
	var response map[string]any
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Verify data field exists and is empty
	data, ok := response["data"].([]any)
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
	trans, err := anthropic.NewTranslator(app.logger, testConfig)
	if err != nil {
		t.Fatalf("failed to create translator: %v", err)
	}

	// Create request
	req := httptest.NewRequest(http.MethodGet, "/olla/anthropic/v1/models", nil)
	w := httptest.NewRecorder()

	// Call handler
	handler := app.translatorModelsHandler(trans)
	handler(w, req)

	// Verify response
	var response map[string]any
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Verify the Anthropic GET /v1/models wire format.
	data, ok := response["data"].([]any)
	if !ok {
		t.Fatal("expected data field")
	}

	if len(data) == 0 {
		t.Fatal("expected at least one model")
	}

	// Envelope fields.
	if hasMo, ok := response["has_more"].(bool); !ok || hasMo {
		t.Errorf("expected has_more to be false, got %v", response["has_more"])
	}
	if response["first_id"] == nil {
		t.Error("expected first_id to be non-null when data is non-empty")
	}
	if response["last_id"] == nil {
		t.Error("expected last_id to be non-null when data is non-empty")
	}

	model := data[0].(map[string]any)

	if _, ok := model["id"]; !ok {
		t.Error("expected id field")
	}

	// "type" must be "model" per spec.
	if modelType, ok := model["type"].(string); !ok || modelType != "model" {
		t.Errorf("expected type \"model\", got %q", model["type"])
	}

	// "display_name" replaces legacy "name".
	if _, ok := model["display_name"]; !ok {
		t.Error("expected display_name field")
	}

	// "created_at" must be a valid RFC3339 string.
	createdAt, ok := model["created_at"].(string)
	if !ok {
		t.Errorf("expected created_at to be a string, got %T", model["created_at"])
	} else if _, parseErr := time.Parse(time.RFC3339, createdAt); parseErr != nil {
		t.Errorf("created_at %q is not valid RFC3339: %v", createdAt, parseErr)
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
