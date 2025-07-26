package discovery

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/thushan/olla/internal/core/domain"
)

// TestEnhancedModelDiscoveryIntegration tests the detection of various model data
func TestEnhancedModelDiscoveryIntegration(t *testing.T) {
	tests := []struct {
		name               string
		platformType       string
		response           string
		expectedModelCount int
		validateMetadata   func(*testing.T, []*domain.ModelInfo)
	}{
		{
			name:               "Ollama Platform Rich Metadata",
			platformType:       domain.ProfileOllama,
			expectedModelCount: 2,
			response: `{
				"models": [
					{
						"name": "devstral:latest",
						"size": 14333927918,
						"digest": "c4b2fa0c33d75457e5f1c8507c906a79e73285768686db13b9cbac0c7ee3a854",
						"modified_at": "2025-05-30T14:24:44.5116551+10:00",
						"description": "Code-focused model for development tasks",
						"details": {
							"parameter_size": "23.6B",
							"quantization_level": "Q4_K_M",
							"family": "llama",
							"families": ["llama"],
							"format": "gguf",
							"parent_model": "codestral"
						}
					},
					{
						"name": "codegemma:9b",
						"size": 5011852809,
						"description": "Code generation model",
						"digest": "sha256:0c96700aaada572ce9bb6999d1fda9b53e9e6cef5d74fda1e066a1ba811b93f3",
						"details": {
							"format": "gguf",
							"family": "gemma",
							"families": ["gemma"],
							"parameter_size": "9B",
							"quantization_level": "Q4_0"
						}
					}
				]
			}`,
			validateMetadata: func(t *testing.T, models []*domain.ModelInfo) {
				var devstral *domain.ModelInfo
				for _, model := range models {
					if model.Name == "devstral:latest" {
						devstral = model
						break
					}
				}

				if devstral == nil {
					t.Fatal("Expected to find devstral:latest model")
				}

				if devstral.Size != 14333927918 {
					t.Errorf("Expected size 14333927918, got %d", devstral.Size)
				}

				if devstral.Description != "Code-focused model for development tasks" {
					t.Errorf("Expected description to be preserved, got %q", devstral.Description)
				}

				if devstral.Details == nil {
					t.Fatal("Expected devstral model to have details")
				}

				if devstral.Details.ParameterSize == nil || *devstral.Details.ParameterSize != "23.6B" {
					t.Error("Expected parameter_size to be '23.6B'")
				}

				if devstral.Details.QuantizationLevel == nil || *devstral.Details.QuantizationLevel != "Q4_K_M" {
					t.Error("Expected quantization_level to be 'Q4_K_M'")
				}

				if devstral.Details.Family == nil || *devstral.Details.Family != "llama" {
					t.Error("Expected family to be 'llama'")
				}

				if devstral.Details.Format == nil || *devstral.Details.Format != "gguf" {
					t.Error("Expected format to be 'gguf'")
				}

				if devstral.Details.ParentModel == nil || *devstral.Details.ParentModel != "codestral" {
					t.Error("Expected parent_model to be 'codestral'")
				}

				if devstral.Details.Digest == nil || *devstral.Details.Digest != "c4b2fa0c33d75457e5f1c8507c906a79e73285768686db13b9cbac0c7ee3a854" {
					t.Error("Expected digest to be parsed correctly")
				}

				if devstral.Details.ModifiedAt == nil {
					t.Error("Expected modified_at to be parsed")
				}

				if len(devstral.Details.Families) != 1 || devstral.Details.Families[0] != "llama" {
					t.Error("Expected families array to contain 'llama'")
				}
			},
		},
		{
			name:               "LM Studio Platform Rich Metadata",
			platformType:       domain.ProfileLmStudio,
			expectedModelCount: 3,
			response: `{
				"object": "list",
				"data": [
					{
						"id": "meta-llama-3.3-8b-instruct",
						"object": "model",
						"type": "llm",
						"publisher": "lmstudio-community",
						"arch": "llama",
						"compatibility_type": "gguf",
						"quantization": "Q4_K_M",
						"state": "not-loaded",
						"max_context_length": 131072
					},
					{
						"id": "qwen3-vl-7b-instruct",
						"object": "model",
						"type": "vlm",
						"publisher": "mlx-community",
						"arch": "qwen3_vl",
						"compatibility_type": "mlx",
						"quantization": "4bit",
						"state": "loaded",
						"max_context_length": 32768
					},
					{
						"id": "text-embedding-nomic-embed-text-v1.5",
						"object": "model",
						"type": "embeddings",
						"publisher": "nomic-ai",
						"arch": "nomic-bert",
						"compatibility_type": "gguf",
						"quantization": "fp16",
						"state": "not-loaded",
						"max_context_length": 8192
					}
				]
			}`,
			validateMetadata: func(t *testing.T, models []*domain.ModelInfo) {
				var llama *domain.ModelInfo
				for _, model := range models {
					if model.Name == "meta-llama-3.3-8b-instruct" {
						llama = model
						break
					}
				}

				if llama == nil {
					t.Fatal("Expected to find meta-llama-3.3-8b-instruct model")
				}

				if llama.Details == nil {
					t.Fatal("Expected llama model to have details")
				}

				if llama.Details.Family == nil || *llama.Details.Family != "llama" {
					t.Error("Expected family to be mapped from arch field")
				}

				if llama.Details.QuantizationLevel == nil || *llama.Details.QuantizationLevel != "Q4_K_M" {
					t.Error("Expected quantization_level to be mapped from quantization field")
				}

				if llama.Details.Format == nil || *llama.Details.Format != "gguf" {
					t.Error("Expected format to be mapped from compatibility_type field")
				}

				if llama.Details.ParentModel == nil || *llama.Details.ParentModel != "lmstudio-community" {
					t.Error("Expected parent_model to be mapped from publisher field")
				}

				if llama.Details.State == nil || *llama.Details.State != "not-loaded" {
					t.Error("Expected state to be 'not-loaded'")
				}

				if llama.Details.MaxContextLength == nil || *llama.Details.MaxContextLength != 131072 {
					t.Error("Expected max_context_length to be 131072")
				}

				if llama.Details.Type == nil || *llama.Details.Type != "llm" {
					t.Error("Expected type to be 'llm'")
				}

				// Test VLM model
				var qwen *domain.ModelInfo
				for _, model := range models {
					if model.Name == "qwen3-vl-7b-instruct" {
						qwen = model
						break
					}
				}

				if qwen == nil {
					t.Fatal("Expected to find qwen3-vl-7b-instruct model")
				}

				if qwen.Details == nil {
					t.Fatal("Expected qwen model to have details")
				}

				if qwen.Details.Family == nil || *qwen.Details.Family != "qwen3_vl" {
					t.Error("Expected qwen family to be 'qwen3_vl'")
				}

				if qwen.Details.State == nil || *qwen.Details.State != "loaded" {
					t.Error("Expected qwen state to be 'loaded'")
				}

				if qwen.Details.MaxContextLength == nil || *qwen.Details.MaxContextLength != 32768 {
					t.Error("Expected qwen max_context_length to be 32768")
				}

				if qwen.Details.Type == nil || *qwen.Details.Type != "vlm" {
					t.Error("Expected qwen type to be 'vlm'")
				}
			},
		},
		{
			name:               "OpenAI Compatible Platform Minimal Metadata",
			platformType:       domain.ProfileOpenAICompatible,
			expectedModelCount: 2,
			response: `{
				"object": "list",
				"data": [
					{
						"id": "gpt-3.5-turbo",
						"object": "model",
						"created": 1677610602,
						"owned_by": "openai"
					},
					{
						"id": "gpt-4",
						"object": "model",
						"created": 1687882411,
						"owned_by": "openai"
					}
				]
			}`,
			validateMetadata: func(t *testing.T, models []*domain.ModelInfo) {
				var gpt35 *domain.ModelInfo
				for _, model := range models {
					if model.Name == "gpt-3.5-turbo" {
						gpt35 = model
						break
					}
				}

				if gpt35 == nil {
					t.Fatal("Expected to find gpt-3.5-turbo model")
				}

				if gpt35.Type != "model" {
					t.Error("Expected type to be 'model'")
				}

				if gpt35.Details != nil && gpt35.Details.ModifiedAt != nil {
					expectedTime := time.Unix(1677610602, 0)
					if !gpt35.Details.ModifiedAt.Equal(expectedTime) {
						t.Errorf("Expected created time to be parsed correctly")
					}
				}

				for i, model := range models {
					if time.Since(model.LastSeen) > time.Second {
						t.Errorf("Model %d (%s) LastSeen is not recent", i, model.Name)
					}
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(200)
				w.Write([]byte(tt.response))
			}))
			defer server.Close()

			endpoint := createTestEndpoint(server.URL, tt.platformType)
			client := NewHTTPModelDiscoveryClientWithDefaults(createTestProfileFactory(t), createTestLogger())

			ctx := context.Background()
			models, err := client.DiscoverModels(ctx, endpoint)

			if err != nil {
				t.Fatalf("Discovery failed: %v", err)
			}

			if len(models) != tt.expectedModelCount {
				t.Fatalf("Expected %d models, got %d", tt.expectedModelCount, len(models))
			}

			tt.validateMetadata(t, models)
			t.Logf("Successfully validated %d models for %s", len(models), tt.platformType)
		})
	}
}

// TestAutoDetectionWithRichMetadata tests auto-detection preserves rich metadata
func TestAutoDetectionWithRichMetadata(t *testing.T) {
	requestLog := make([]string, 0)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestLog = append(requestLog, r.URL.Path)

		switch r.URL.Path {
		case "/api/tags":
			w.WriteHeader(200)
			w.Write([]byte(`{
				"models": [
					{
						"name": "llama3:8b",
						"size": 4661224676,
						"digest": "sha256:abc123",
						"modified_at": "2025-06-11T10:00:00Z",
						"description": "Llama 3 8B parameter model",
						"details": {
							"parameter_size": "8B",
							"quantization_level": "Q4_0",
							"family": "llama",
							"families": ["llama"],
							"format": "gguf",
							"parent_model": "meta-llama/Meta-Llama-3-8B"
						}
					}
				]
			}`))
		case "/api/v0/models":
			w.WriteHeader(404)
			w.Write([]byte(`{"error": "should not reach here"}`))
		default:
			w.WriteHeader(404)
			w.Write([]byte(`{"error": "not found"}`))
		}
	}))
	defer server.Close()

	endpoint := createTestEndpoint(server.URL, domain.ProfileAuto)
	client := NewHTTPModelDiscoveryClientWithDefaults(createTestProfileFactory(t), createTestLogger())

	ctx := context.Background()
	models, err := client.DiscoverModels(ctx, endpoint)

	if err != nil {
		t.Fatalf("Auto-detection failed: %v", err)
	}

	if len(models) != 1 {
		t.Fatalf("Expected 1 model, got %d", len(models))
	}

	model := models[0]

	if model.Name != "llama3:8b" {
		t.Errorf("Expected model name 'llama3:8b', got %s", model.Name)
	}

	if model.Size != 4661224676 {
		t.Errorf("Expected model size 4661224676, got %d", model.Size)
	}

	if model.Description != "Llama 3 8B parameter model" {
		t.Errorf("Expected description to be preserved, got %q", model.Description)
	}

	if model.Details == nil {
		t.Fatal("Expected auto-detected model to preserve rich metadata")
	}

	metadataChecks := map[string]func() bool{
		"parameter_size": func() bool { return model.Details.ParameterSize != nil && *model.Details.ParameterSize == "8B" },
		"quantization_level": func() bool {
			return model.Details.QuantizationLevel != nil && *model.Details.QuantizationLevel == "Q4_0"
		},
		"family":      func() bool { return model.Details.Family != nil && *model.Details.Family == "llama" },
		"format":      func() bool { return model.Details.Format != nil && *model.Details.Format == "gguf" },
		"digest":      func() bool { return model.Details.Digest != nil && *model.Details.Digest == "sha256:abc123" },
		"modified_at": func() bool { return model.Details.ModifiedAt != nil },
		"families":    func() bool { return len(model.Details.Families) == 1 && model.Details.Families[0] == "llama" },
		"parent_model": func() bool {
			return model.Details.ParentModel != nil && *model.Details.ParentModel == "meta-llama/Meta-Llama-3-8B"
		},
	}

	for field, check := range metadataChecks {
		if !check() {
			t.Errorf("Auto-detection failed to preserve %s metadata", field)
		}
	}

	expectedRequests := []string{"/api/tags"}
	if len(requestLog) != len(expectedRequests) {
		t.Errorf("Expected %d requests, got %d: %v", len(expectedRequests), len(requestLog), requestLog)
	}

	t.Log("Auto-detection successfully preserved rich metadata")
}

// TestLMStudioEnhancedMetadata specifically tests LM Studio's rich API response
func TestLMStudioEnhancedMetadata(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/api/v0/models") {
			w.WriteHeader(404)
			return
		}

		w.WriteHeader(200)
		w.Write([]byte(`{
			"object": "list",
			"data": [
				{
					"id": "qwen3-vl-7b-instruct",
					"object": "model",
					"type": "vlm",
					"publisher": "mlx-community",
					"arch": "qwen3_vl",
					"compatibility_type": "mlx",
					"quantization": "4bit",
					"state": "not-loaded",
					"max_context_length": 32768
				},
				{
					"id": "meta-llama-3.3-8b-instruct",
					"object": "model",
					"type": "llm",
					"publisher": "lmstudio-community",
					"arch": "llama",
					"compatibility_type": "gguf",
					"quantization": "Q4_K_M",
					"state": "loaded",
					"max_context_length": 131072
				}
			]
		}`))
	}))
	defer server.Close()

	endpoint := createTestEndpoint(server.URL, domain.ProfileLmStudio)
	client := NewHTTPModelDiscoveryClientWithDefaults(createTestProfileFactory(t), createTestLogger())

	models, err := client.DiscoverModels(context.Background(), endpoint)
	if err != nil {
		t.Fatalf("LM Studio discovery failed: %v", err)
	}

	if len(models) != 2 {
		t.Fatalf("Expected 2 models, got %d", len(models))
	}

	var vlmModel *domain.ModelInfo
	for _, model := range models {
		if model.Name == "qwen3-vl-7b-instruct" {
			vlmModel = model
			break
		}
	}

	if vlmModel == nil {
		t.Fatal("Expected to find VLM model")
	}

	if vlmModel.Details == nil {
		t.Fatal("Expected VLM model to have details")
	}

	lmStudioFields := map[string]func() bool{
		"family (arch)": func() bool { return vlmModel.Details.Family != nil && *vlmModel.Details.Family == "qwen3_vl" },
		"quantization_level": func() bool {
			return vlmModel.Details.QuantizationLevel != nil && *vlmModel.Details.QuantizationLevel == "4bit"
		},
		"format (compatibility)": func() bool { return vlmModel.Details.Format != nil && *vlmModel.Details.Format == "mlx" },
		"parent_model (publisher)": func() bool {
			return vlmModel.Details.ParentModel != nil && *vlmModel.Details.ParentModel == "mlx-community"
		},
		"state": func() bool { return vlmModel.Details.State != nil && *vlmModel.Details.State == "not-loaded" },
		"max_context_length": func() bool {
			return vlmModel.Details.MaxContextLength != nil && *vlmModel.Details.MaxContextLength == 32768
		},
		"type": func() bool { return vlmModel.Details.Type != nil && *vlmModel.Details.Type == "vlm" },
	}

	for fieldName, check := range lmStudioFields {
		if !check() {
			t.Errorf("LM Studio VLM model failed validation for %s", fieldName)
		}
	}

	t.Log("LM Studio enhanced metadata validation successful")
}

// TestResilientModelParsing tests that models with partial or malformed data still get processed
func TestResilientModelParsing(t *testing.T) {
	tests := []struct {
		name          string
		platformType  string
		response      string
		modelName     string
		expectDetails bool
		expectError   bool
		description   string
	}{
		{
			name:         "Ollama model with malformed details field",
			platformType: domain.ProfileOllama,
			response: `{
				"models": [{
					"name": "test-model",
					"size": 1000000,
					"details": "this-should-be-object-not-string"
				}]
			}`,
			modelName:     "test-model",
			expectDetails: false,
			expectError:   true,
			description:   "Should reject malformed JSON with strict typing",
		},
		{
			name:         "Ollama model with missing details",
			platformType: domain.ProfileOllama,
			response: `{
				"models": [{
					"name": "test-model",
					"size": 1000000,
					"description": "A test model"
				}]
			}`,
			modelName:     "test-model",
			expectDetails: false,
			description:   "Should parse model without details when details field is missing",
		},
		{
			name:         "LM Studio model with minimal data",
			platformType: domain.ProfileLmStudio,
			response: `{
				"data": [{
					"id": "minimal-model",
					"object": "model"
				}]
			}`,
			modelName:     "minimal-model",
			expectDetails: false,
			description:   "Should parse LM Studio model with minimal data",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(200)
				w.Write([]byte(tt.response))
			}))
			defer server.Close()

			endpoint := createTestEndpoint(server.URL, tt.platformType)
			client := NewHTTPModelDiscoveryClientWithDefaults(createTestProfileFactory(t), createTestLogger())

			models, err := client.DiscoverModels(context.Background(), endpoint)
			if err != nil {
				if tt.expectError {
					// other tests will fail, so continue
					return
				} else {
					t.Fatalf("Discovery failed: %v", err)
				}
			}

			if len(models) != 1 {
				t.Fatalf("Expected 1 model, got %d", len(models))
			}

			model := models[0]
			if model.Name != tt.modelName {
				t.Errorf("Expected model name %s, got %s", tt.modelName, model.Name)
			}

			if model.LastSeen.IsZero() {
				t.Error("Expected LastSeen to be set")
			}

			hasDetails := model.Details != nil
			if hasDetails != tt.expectDetails {
				t.Errorf("Expected details=%v, got details=%v. %s", tt.expectDetails, hasDetails, tt.description)
			}

			t.Logf("%s", tt.description)
		})
	}
}
