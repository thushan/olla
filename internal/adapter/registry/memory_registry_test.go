package registry

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/thushan/olla/internal/core/domain"
)

const (
	DefaultModelName  = "llama4:128x17b"
	DefaultModelNameA = "gemma3:12b"
	DefaultModelNameB = "deepseek-r1:32b"
	DefaultModelNameC = "phi4:14b"

	DefaultOllamaEndpointUri = "http://localhost:11434"
)

func TestNewMemoryModelRegistry(t *testing.T) {
	registry := NewMemoryModelRegistry()

	if registry == nil {
		t.Fatal("NewMemoryModelRegistry returned nil")
	}

	if registry.endpointModels == nil {
		t.Error("endpointModels not initialised")
	}

	if registry.modelToEndpoints == nil {
		t.Error("modelToEndpoints not initialised")
	}

	stats, err := registry.GetStats(context.Background())
	if err != nil {
		t.Fatalf("GetStats failed: %v", err)
	}

	if stats.TotalEndpoints != 0 {
		t.Errorf("Expected 0 endpoints, got %d", stats.TotalEndpoints)
	}

	if stats.TotalModels != 0 {
		t.Errorf("Expected 0 models, got %d", stats.TotalModels)
	}
}

func TestRegisterModel_Success(t *testing.T) {
	modelName := DefaultModelName
	registry := NewMemoryModelRegistry()
	ctx := context.Background()

	model := createTestModel(modelName)
	endpointURL := DefaultOllamaEndpointUri

	err := registry.RegisterModel(ctx, endpointURL, model)
	if err != nil {
		t.Fatalf("RegisterModel failed: %v", err)
	}

	models, err := registry.GetModelsForEndpoint(ctx, endpointURL)
	if err != nil {
		t.Fatalf("GetModelsForEndpoint failed: %v", err)
	}

	if len(models) != 1 {
		t.Fatalf("Expected 1 model, got %d", len(models))
	}

	if models[0].Name != modelName {
		t.Errorf("Expected model name '%s', got %s", modelName, models[0].Name)
	}
}

func TestRegisterModel_InvalidInputs(t *testing.T) {
	registry := NewMemoryModelRegistry()
	ctx := context.Background()

	tests := []struct {
		name        string
		endpointURL string
		modelName   string
		expectError bool
	}{
		{"empty endpoint URL", "", "ttest", true},
		{"empty model name", DefaultOllamaEndpointUri, "", true},
		{"whitespace only endpoint", "   ", "test", true},
		{"whitespace only model", DefaultOllamaEndpointUri, "   ", true},
		{"invalid URL", "not-a-url", "test", true},
		{"valid inputs", DefaultOllamaEndpointUri, "test", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testModel := createTestModel(tt.modelName)
			err := registry.RegisterModel(ctx, tt.endpointURL, testModel)

			if tt.expectError && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Expected no error but got: %v", err)
			}
		})
	}
}

func TestRegisterModel_UpdateExisting(t *testing.T) {
	registry := NewMemoryModelRegistry()
	ctx := context.Background()
	endpointURL := DefaultOllamaEndpointUri

	model1 := createTestModel(DefaultModelName)
	model1.Size = 1000
	model1.Description = "Original"

	err := registry.RegisterModel(ctx, endpointURL, model1)
	if err != nil {
		t.Fatalf("First RegisterModel failed: %v", err)
	}

	model2 := createTestModel(DefaultModelName)
	model2.Size = 2000
	model2.Description = "Updated"

	err = registry.RegisterModel(ctx, endpointURL, model2)
	if err != nil {
		t.Fatalf("Second RegisterModel failed: %v", err)
	}

	models, err := registry.GetModelsForEndpoint(ctx, endpointURL)
	if err != nil {
		t.Fatalf("GetModelsForEndpoint failed: %v", err)
	}

	if len(models) != 1 {
		t.Fatalf("Expected 1 model after update, got %d", len(models))
	}

	if models[0].Size != 2000 {
		t.Errorf("Expected updated size 2000, got %d", models[0].Size)
	}

	if models[0].Description != "Updated" {
		t.Errorf("Expected updated description 'Updated', got %s", models[0].Description)
	}
}

func TestRegisterModels_Success(t *testing.T) {
	registry := NewMemoryModelRegistry()
	ctx := context.Background()
	endpointURL := DefaultOllamaEndpointUri

	models := []*domain.ModelInfo{
		createTestModel(DefaultModelName),
		createTestModel(DefaultModelNameA),
		createTestModel(DefaultModelNameB),
	}

	err := registry.RegisterModels(ctx, endpointURL, models)
	if err != nil {
		t.Fatalf("RegisterModels failed: %v", err)
	}

	retrievedModels, err := registry.GetModelsForEndpoint(ctx, endpointURL)
	if err != nil {
		t.Fatalf("GetModelsForEndpoint failed: %v", err)
	}

	if len(retrievedModels) != 3 {
		t.Fatalf("Expected 3 models, got %d", len(retrievedModels))
	}

	modelNames := make(map[string]bool)
	for _, model := range retrievedModels {
		modelNames[model.Name] = true
	}

	expectedNames := []string{DefaultModelName, DefaultModelNameA, DefaultModelNameB}
	for _, name := range expectedNames {
		if !modelNames[name] {
			t.Errorf("Expected model %s not found", name)
		}
	}
}


func createTestModel(name string) *domain.ModelInfo {
	return &domain.ModelInfo{
		Name:        name,
		Size:        1024 * 1024 * 100,
		Type:        "chat",
		Description: fmt.Sprintf("Test model %s", name),
		LastSeen:    time.Now(),
	}
}
