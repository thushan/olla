package registry

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/thushan/olla/internal/logger"
	"github.com/thushan/olla/theme"

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
	registry := NewMemoryModelRegistry(createTestLogger())

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
	registry := NewMemoryModelRegistry(createTestLogger())
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
	registry := NewMemoryModelRegistry(createTestLogger())
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
	registry := NewMemoryModelRegistry(createTestLogger())
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
	registry := NewMemoryModelRegistry(createTestLogger())
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

func TestRegisterModels_ReplaceExisting(t *testing.T) {
	registry := NewMemoryModelRegistry(createTestLogger())
	ctx := context.Background()
	endpointURL := DefaultOllamaEndpointUri

	initialModels := []*domain.ModelInfo{
		createTestModel(DefaultModelName),
		createTestModel(DefaultModelNameC),
	}

	err := registry.RegisterModels(ctx, endpointURL, initialModels)
	if err != nil {
		t.Fatalf("Initial RegisterModels failed: %v", err)
	}

	newModels := []*domain.ModelInfo{
		createTestModel(DefaultModelNameB),
	}

	err = registry.RegisterModels(ctx, endpointURL, newModels)
	if err != nil {
		t.Fatalf("Second RegisterModels failed: %v", err)
	}

	retrievedModels, err := registry.GetModelsForEndpoint(ctx, endpointURL)
	if err != nil {
		t.Fatalf("GetModelsForEndpoint failed: %v", err)
	}

	if len(retrievedModels) != 1 {
		t.Fatalf("Expected 1 model after replacement, got %d", len(retrievedModels))
	}

	if retrievedModels[0].Name != DefaultModelNameB {
		t.Errorf("Expected model 'mistral:7b', got %s", retrievedModels[0].Name)
	}

	if registry.IsModelAvailable(ctx, DefaultModelName) {
		t.Error("Old model should no longer be available")
	}

	if !registry.IsModelAvailable(ctx, DefaultModelNameB) {
		t.Error("New model should be available")
	}
}

func TestRegisterModels_EmptyList(t *testing.T) {
	registry := NewMemoryModelRegistry(createTestLogger())
	ctx := context.Background()
	endpointURL := DefaultOllamaEndpointUri

	initialModels := []*domain.ModelInfo{
		createTestModel(DefaultModelName),
	}

	err := registry.RegisterModels(ctx, endpointURL, initialModels)
	if err != nil {
		t.Fatalf("Initial RegisterModels failed: %v", err)
	}

	err = registry.RegisterModels(ctx, endpointURL, []*domain.ModelInfo{})
	if err != nil {
		t.Fatalf("RegisterModels with empty list failed: %v", err)
	}

	retrievedModels, err := registry.GetModelsForEndpoint(ctx, endpointURL)
	if err != nil {
		t.Fatalf("GetModelsForEndpoint failed: %v", err)
	}

	if len(retrievedModels) != 0 {
		t.Fatalf("Expected 0 models after empty registration, got %d", len(retrievedModels))
	}

	if registry.IsModelAvailable(ctx, DefaultModelName) {
		t.Error("Model should no longer be available")
	}
}

func TestGetModelsForEndpoint_NonExistent(t *testing.T) {
	registry := NewMemoryModelRegistry(createTestLogger())
	ctx := context.Background()

	models, err := registry.GetModelsForEndpoint(ctx, "http://nonexistent:11434")
	if err != nil {
		t.Fatalf("GetModelsForEndpoint failed: %v", err)
	}

	if len(models) != 0 {
		t.Errorf("Expected empty slice for nonexistent endpoint, got %d models", len(models))
	}
}

func TestGetEndpointsForModel_Success(t *testing.T) {
	registry := NewMemoryModelRegistry(createTestLogger())
	ctx := context.Background()

	model := createTestModel(DefaultModelNameC)

	endpoints := []string{
		DefaultOllamaEndpointUri,
		"http://localhost:11435",
		"http://localhost:11436",
	}

	for _, endpoint := range endpoints {
		err := registry.RegisterModel(ctx, endpoint, model)
		if err != nil {
			t.Fatalf("RegisterModel failed for %s: %v", endpoint, err)
		}
	}

	retrievedEndpoints, err := registry.GetEndpointsForModel(ctx, DefaultModelNameC)
	if err != nil {
		t.Fatalf("GetEndpointsForModel failed: %v", err)
	}

	if len(retrievedEndpoints) != 3 {
		t.Fatalf("Expected 3 endpoints, got %d", len(retrievedEndpoints))
	}

	endpointMap := make(map[string]bool)
	for _, endpoint := range retrievedEndpoints {
		endpointMap[endpoint] = true
	}

	for _, expected := range endpoints {
		if !endpointMap[expected] {
			t.Errorf("Expected endpoint %s not found", expected)
		}
	}
}

func TestGetEndpointsForModel_NonExistent(t *testing.T) {
	registry := NewMemoryModelRegistry(createTestLogger())
	ctx := context.Background()

	endpoints, err := registry.GetEndpointsForModel(ctx, "nonexistent-model")
	if err != nil {
		t.Fatalf("GetEndpointsForModel failed: %v", err)
	}

	if len(endpoints) != 0 {
		t.Errorf("Expected empty slice for nonexistent model, got %d endpoints", len(endpoints))
	}
}

func TestIsModelAvailable(t *testing.T) {
	registry := NewMemoryModelRegistry(createTestLogger())
	ctx := context.Background()

	if registry.IsModelAvailable(ctx, "nonexistent") {
		t.Error("Nonexistent model should not be available")
	}

	model := createTestModel(DefaultModelNameC)
	err := registry.RegisterModel(ctx, DefaultOllamaEndpointUri, model)
	if err != nil {
		t.Fatalf("RegisterModel failed: %v", err)
	}

	if !registry.IsModelAvailable(ctx, DefaultModelNameC) {
		t.Error("Registered model should be available")
	}

	if registry.IsModelAvailable(ctx, "") {
		t.Error("Empty model name should not be available")
	}
}

func TestGetAllModels(t *testing.T) {
	registry := NewMemoryModelRegistry(createTestLogger())
	ctx := context.Background()

	endpoint1 := DefaultOllamaEndpointUri
	endpoint2 := "http://localhost:11435"

	models1 := []*domain.ModelInfo{
		createTestModel(DefaultModelName),
		createTestModel(DefaultModelNameA),
	}

	models2 := []*domain.ModelInfo{
		createTestModel(DefaultModelNameB),
	}

	err := registry.RegisterModels(ctx, endpoint1, models1)
	if err != nil {
		t.Fatalf("RegisterModels failed for endpoint1: %v", err)
	}

	err = registry.RegisterModels(ctx, endpoint2, models2)
	if err != nil {
		t.Fatalf("RegisterModels failed for endpoint2: %v", err)
	}

	allModels, err := registry.GetAllModels(ctx)
	if err != nil {
		t.Fatalf("GetAllModels failed: %v", err)
	}

	if len(allModels) != 2 {
		t.Fatalf("Expected 2 endpoints, got %d", len(allModels))
	}

	if len(allModels[endpoint1]) != 2 {
		t.Errorf("Expected 2 models for endpoint1, got %d", len(allModels[endpoint1]))
	}

	if len(allModels[endpoint2]) != 1 {
		t.Errorf("Expected 1 model for endpoint2, got %d", len(allModels[endpoint2]))
	}
}

func TestRemoveEndpoint(t *testing.T) {
	registry := NewMemoryModelRegistry(createTestLogger())
	ctx := context.Background()

	endpoint1 := DefaultOllamaEndpointUri
	endpoint2 := "http://localhost:11435"

	model1 := createTestModel(DefaultModelName)
	model2 := createTestModel(DefaultModelNameC)

	err := registry.RegisterModel(ctx, endpoint1, model1)
	if err != nil {
		t.Fatalf("RegisterModel failed: %v", err)
	}

	err = registry.RegisterModel(ctx, endpoint2, model1)
	if err != nil {
		t.Fatalf("RegisterModel failed: %v", err)
	}

	err = registry.RegisterModel(ctx, endpoint1, model2)
	if err != nil {
		t.Fatalf("RegisterModel failed: %v", err)
	}

	err = registry.RemoveEndpoint(ctx, endpoint1)
	if err != nil {
		t.Fatalf("RemoveEndpoint failed: %v", err)
	}

	models, err := registry.GetModelsForEndpoint(ctx, endpoint1)
	if err != nil {
		t.Fatalf("GetModelsForEndpoint failed: %v", err)
	}

	if len(models) != 0 {
		t.Errorf("Expected 0 models for removed endpoint, got %d", len(models))
	}

	endpoints, err := registry.GetEndpointsForModel(ctx, DefaultModelName)
	if err != nil {
		t.Fatalf("GetEndpointsForModel failed: %v", err)
	}

	if len(endpoints) != 1 || endpoints[0] != endpoint2 {
		t.Error("Model should still be available on endpoint2")
	}

	if registry.IsModelAvailable(ctx, DefaultModelNameC) {
		t.Error("Model should no longer be available after endpoint removal")
	}
}

func TestGetStats(t *testing.T) {
	registry := NewMemoryModelRegistry(createTestLogger())
	ctx := context.Background()

	endpoint1 := DefaultOllamaEndpointUri
	endpoint2 := "http://localhost:11435"

	models1 := []*domain.ModelInfo{
		createTestModel(DefaultModelName),
		createTestModel(DefaultModelNameA),
	}

	models2 := []*domain.ModelInfo{
		createTestModel(DefaultModelName),
		createTestModel(DefaultModelNameB),
	}

	err := registry.RegisterModels(ctx, endpoint1, models1)
	if err != nil {
		t.Fatalf("RegisterModels failed: %v", err)
	}

	err = registry.RegisterModels(ctx, endpoint2, models2)
	if err != nil {
		t.Fatalf("RegisterModels failed: %v", err)
	}

	stats, err := registry.GetStats(ctx)
	if err != nil {
		t.Fatalf("GetStats failed: %v", err)
	}

	if stats.TotalEndpoints != 2 {
		t.Errorf("Expected 2 endpoints, got %d", stats.TotalEndpoints)
	}

	if stats.TotalModels != 3 {
		t.Errorf("Expected 3 unique models, got %d", stats.TotalModels)
	}

	if stats.ModelsPerEndpoint[endpoint1] != 2 {
		t.Errorf("Expected 2 models for endpoint1, got %d", stats.ModelsPerEndpoint[endpoint1])
	}

	if stats.ModelsPerEndpoint[endpoint2] != 2 {
		t.Errorf("Expected 2 models for endpoint2, got %d", stats.ModelsPerEndpoint[endpoint2])
	}
}

func TestContextCancellation(t *testing.T) {
	registry := NewMemoryModelRegistry(createTestLogger())

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	model := createTestModel("test")

	err := registry.RegisterModel(ctx, DefaultOllamaEndpointUri, model)
	if err == nil {
		t.Error("Expected error due to cancelled context")
	}

	_, err = registry.GetModelsForEndpoint(ctx, DefaultOllamaEndpointUri)
	if err == nil {
		t.Error("Expected error due to cancelled context")
	}

	_, err = registry.GetEndpointsForModel(ctx, "test")
	if err == nil {
		t.Error("Expected error due to cancelled context")
	}

	available := registry.IsModelAvailable(ctx, "test")
	if available {
		t.Error("Expected false due to cancelled context")
	}
}

func TestConcurrentAccess(t *testing.T) {
	registry := NewMemoryModelRegistry(createTestLogger())
	ctx := context.Background()

	var wg sync.WaitGroup
	errors := make(chan error, 200)

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			endpoint := fmt.Sprintf("http://localhost:%d", 11434+id)
			model := createTestModel(fmt.Sprintf("model-%d", id))

			for j := 0; j < 10; j++ {
				if err := registry.RegisterModel(ctx, endpoint, model); err != nil {
					errors <- err
					return
				}
			}
		}(i)
	}

	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 5; j++ {
				_, err := registry.GetAllModels(ctx)
				if err != nil {
					errors <- err
					return
				}

				_, err = registry.GetStats(ctx)
				if err != nil {
					errors <- err
					return
				}
			}
		}(i)
	}

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			modelName := fmt.Sprintf("model-%d", id%5)
			for j := 0; j < 5; j++ {
				registry.IsModelAvailable(ctx, modelName)
				_, _ = registry.GetEndpointsForModel(ctx, modelName)
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	for err := range errors {
		t.Errorf("Concurrent access error: %v", err)
	}
}

func TestDataIsolation(t *testing.T) {
	registry := NewMemoryModelRegistry(createTestLogger())
	ctx := context.Background()

	model := createTestModel(DefaultModelName)
	model.Size = 1000

	err := registry.RegisterModel(ctx, DefaultOllamaEndpointUri, model)
	if err != nil {
		t.Fatalf("RegisterModel failed: %v", err)
	}

	retrievedModels, err := registry.GetModelsForEndpoint(ctx, DefaultOllamaEndpointUri)
	if err != nil {
		t.Fatalf("GetModelsForEndpoint failed: %v", err)
	}

	retrievedModels[0].Size = 2000

	models2, err := registry.GetModelsForEndpoint(ctx, DefaultOllamaEndpointUri)
	if err != nil {
		t.Fatalf("Second GetModelsForEndpoint failed: %v", err)
	}

	if models2[0].Size != 1000 {
		t.Error("External modification affected internal data")
	}
}

func BenchmarkModelLookup(b *testing.B) {
	registry := NewMemoryModelRegistry(createTestLogger())
	ctx := context.Background()

	for i := 0; i < 100; i++ {
		endpoint := fmt.Sprintf("http://localhost:%d", 11434+i)
		models := make([]*domain.ModelInfo, 10)
		for j := 0; j < 10; j++ {
			models[j] = createTestModel(fmt.Sprintf("model-%d-%d", i, j))
		}
		registry.RegisterModels(ctx, endpoint, models)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			endpoint := fmt.Sprintf("http://localhost:%d", 11434+(b.N%100))
			_, _ = registry.GetModelsForEndpoint(ctx, endpoint)
		}
	})
}

func BenchmarkModelRegistration(b *testing.B) {
	registry := NewMemoryModelRegistry(createTestLogger())
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		endpoint := fmt.Sprintf("http://localhost:%d", 11434+(i%10))
		model := createTestModel(fmt.Sprintf("model-%d", i))
		registry.RegisterModel(ctx, endpoint, model)
	}
}

func BenchmarkConcurrentAccess(b *testing.B) {
	registry := NewMemoryModelRegistry(createTestLogger())
	ctx := context.Background()

	for i := 0; i < 50; i++ {
		endpoint := fmt.Sprintf("http://localhost:%d", 11434+i)
		models := make([]*domain.ModelInfo, 5)
		for j := 0; j < 5; j++ {
			models[j] = createTestModel(fmt.Sprintf("model-%d-%d", i, j))
		}
		registry.RegisterModels(ctx, endpoint, models)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			switch b.N % 4 {
			case 0:
				endpoint := fmt.Sprintf("http://localhost:%d", 11434+(b.N%50))
				_, _ = registry.GetModelsForEndpoint(ctx, endpoint)
			case 1:
				model := fmt.Sprintf("model-%d-0", b.N%50)
				_, _ = registry.GetEndpointsForModel(ctx, model)
			case 2:
				model := fmt.Sprintf("model-%d-0", b.N%50)
				_ = registry.IsModelAvailable(ctx, model)
			case 3:
				_, _ = registry.GetStats(ctx)
			}
		}
	})
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
func createTestLogger() *logger.StyledLogger {
	loggerCfg := &logger.Config{Level: "error", Theme: "default"}
	log, _, _ := logger.New(loggerCfg)
	return logger.NewStyledLogger(log, theme.Default())
}
