package discovery

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/thushan/olla/internal/adapter/registry/profile"
	"github.com/thushan/olla/internal/core/domain"
)

func TestParseModelsResponse(t *testing.T) {
	parser := NewResponseParser(createTestLogger())
	profileFactory := profile.NewFactory()

	tests := []struct {
		name           string
		responseBody   []byte
		platformType   string
		expectedModels int
		expectedError  bool
		expectedNames  []string
		validateModel  func(*testing.T, *domain.ModelInfo, string)
	}{
		{
			name: "Ollama response format with rich metadata",
			responseBody: []byte(`{
				"models": [
					{
						"name": "devstral:latest",
						"size": 14333927918,
						"digest": "c4b2fa0c33d75457e5f1c8507c906a79e73285768686db13b9cbac0c7ee3a854",
						"modified_at": "2025-05-30T14:24:44.5116551+10:00",
						"details": {
							"parameter_size": "23.6B",
							"quantization_level": "Q4_K_M",
							"family": "llama",
							"families": ["llama"],
							"format": "gguf",
							"parent_model": ""
						}
					},
					{
						"name": "codegemma:9b",
						"size": 5011852809,
						"description": "Code generation model"
					}
				]
			}`),
			platformType:   domain.ProfileOllama,
			expectedModels: 2,
			expectedError:  false,
			expectedNames:  []string{"devstral:latest", "codegemma:9b"},
			validateModel: func(t *testing.T, model *domain.ModelInfo, name string) {
				if name == "devstral:latest" {
					if model.Size != 14333927918 {
						t.Errorf("Expected size 14333927918, got %d", model.Size)
					}
					if model.Details == nil {
						t.Fatal("Expected details to be parsed")
					}
					if model.Details.ParameterSize == nil || *model.Details.ParameterSize != "23.6B" {
						t.Error("Expected parameter_size to be '23.6B'")
					}
					if model.Details.QuantizationLevel == nil || *model.Details.QuantizationLevel != "Q4_K_M" {
						t.Error("Expected quantization_level to be 'Q4_K_M'")
					}
					if model.Details.Family == nil || *model.Details.Family != "llama" {
						t.Error("Expected family to be 'llama'")
					}
					if model.Details.Digest == nil || *model.Details.Digest != "c4b2fa0c33d75457e5f1c8507c906a79e73285768686db13b9cbac0c7ee3a854" {
						t.Error("Expected digest to be parsed correctly")
					}
					if len(model.Details.Families) != 1 || model.Details.Families[0] != "llama" {
						t.Error("Expected families array to contain 'llama'")
					}
				}
			},
		},
		{
			name: "LM Studio response format with rich metadata",
			responseBody: []byte(`{
				"object": "list",
				"data": [
					{
						"id": "meta-llama-3.1-8b-instruct",
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
						"id": "microsoft/DialoGPT-medium",
						"object": "model",
						"created": 1686935002,
						"owned_by": "microsoft"
					}
				]
			}`),
			platformType:   domain.ProfileLmStudio,
			expectedModels: 2,
			expectedError:  false,
			expectedNames:  []string{"meta-llama-3.1-8b-instruct", "microsoft/DialoGPT-medium"},
			validateModel: func(t *testing.T, model *domain.ModelInfo, name string) {
				if name == "meta-llama-3.1-8b-instruct" {
					if model.Type != "model" {
						t.Errorf("Expected type 'model', got %s", model.Type)
					}
					if model.Details == nil {
						t.Fatal("Expected details to be parsed for LM Studio")
					}
					if model.Details.Family == nil || *model.Details.Family != "llama" {
						t.Error("Expected family to be 'llama'")
					}
					if model.Details.QuantizationLevel == nil || *model.Details.QuantizationLevel != "Q4_K_M" {
						t.Error("Expected quantization_level to be 'Q4_K_M'")
					}
					if model.Details.Format == nil || *model.Details.Format != "gguf" {
						t.Error("Expected format to be 'gguf'")
					}
					if *model.Details.MaxContextLength != 131072 {
						t.Errorf("Expected MaxContextLength to be 131072 but got %d", *model.Details.MaxContextLength)
					}
				}
			},
		},
		{
			name: "OpenAI compatible response format",
			responseBody: []byte(`{
				"object": "list",
				"data": [
					{
						"id": "gpt-3.5-turbo",
						"object": "model",
						"created": 1677610602,
						"owned_by": "openai"
					},
					{
						"id": "text-davinci-003",
						"object": "model",
						"created": 1669599635,
						"owned_by": "openai-internal"
					}
				]
			}`),
			platformType:   domain.ProfileOpenAICompatible,
			expectedModels: 2,
			expectedError:  false,
			expectedNames:  []string{"gpt-3.5-turbo", "text-davinci-003"},
			validateModel: func(t *testing.T, model *domain.ModelInfo, name string) {
				if model.Type != "model" {
					t.Errorf("Expected type 'model', got %s", model.Type)
				}
				// OpenAI compatible should have minimal metadata
				if name == "gpt-3.5-turbo" && model.Details != nil && model.Details.ModifiedAt != nil {
					// Should have parsed created timestamp
					expectedTime := time.Unix(1677610602, 0)
					if !model.Details.ModifiedAt.Equal(expectedTime) {
						t.Errorf("Expected modified_at to be %v, got %v", expectedTime, *model.Details.ModifiedAt)
					}
				}
			},
		},
		{
			name:           "Empty response",
			responseBody:   []byte(`{"models": []}`),
			platformType:   domain.ProfileOllama,
			expectedModels: 0,
			expectedError:  false,
			expectedNames:  []string{},
		},
		{
			name:           "Empty body",
			responseBody:   []byte{},
			platformType:   domain.ProfileOllama,
			expectedModels: 0,
			expectedError:  false,
			expectedNames:  []string{},
		},
		{
			name:          "Invalid JSON",
			responseBody:  []byte(`{"models": [`),
			platformType:  domain.ProfileOllama,
			expectedError: true,
		},
		{
			name:           "Missing models field",
			responseBody:   []byte(`{"other": []}`),
			platformType:   domain.ProfileOllama,
			expectedModels: 0,
			expectedError:  false,
		},
		{
			name:          "Models field is not array",
			responseBody:  []byte(`{"models": "not-an-array"}`),
			platformType:  domain.ProfileOllama,
			expectedError: true,
		},
		{
			name: "Models with missing names are skipped",
			responseBody: []byte(`{
				"models": [
					{"name": "valid-model", "size": 123},
					{"size": 456},
					{"name": "", "size": 789},
					{"name": "another-valid-model"}
				]
			}`),
			platformType:   domain.ProfileOllama,
			expectedModels: 2,
			expectedError:  false,
			expectedNames:  []string{"valid-model", "another-valid-model"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			platformProfile, err := profileFactory.GetProfile(tt.platformType)
			if err != nil {
				t.Fatalf("Failed to get profile: %v", err)
			}

			format := platformProfile.GetModelResponseFormat()
			models, err := parser.ParseModelsResponse(tt.responseBody, format, platformProfile)

			if tt.expectedError {
				if err == nil {
					t.Errorf("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if len(models) != tt.expectedModels {
				t.Errorf("Expected %d models, got %d", tt.expectedModels, len(models))
				return
			}

			for i, expectedName := range tt.expectedNames {
				if i >= len(models) {
					t.Errorf("Expected model %d to exist", i)
					continue
				}
				if models[i].Name != expectedName {
					t.Errorf("Expected model %d name to be %s, got %s", i, expectedName, models[i].Name)
				}

				// Check that LastSeen is recent
				if time.Since(models[i].LastSeen) > time.Second {
					t.Errorf("Expected LastSeen to be recent, got %v", models[i].LastSeen)
				}

				// Run custom validation if provided
				if tt.validateModel != nil {
					tt.validateModel(t, models[i], expectedName)
				}
			}
		})
	}
}

func TestParseModelErrorHandling(t *testing.T) {
	parser := NewResponseParser(createTestLogger())
	profileFactory := profile.NewFactory()

	tests := []struct {
		name         string
		responseBody []byte
		platformType string
		expectError  bool
		expectSkip   bool
	}{
		{
			name: "Ollama with malformed model data",
			responseBody: []byte(`{
				"models": [
					{"name": "valid-model"},
					{"invalid": "data"},
					{"name": "another-valid"}
				]
			}`),
			platformType: domain.ProfileOllama,
			expectSkip:   true,
		},
		{
			name: "LM Studio with missing required fields",
			responseBody: []byte(`{
				"data": [
					{"id": "valid-model"},
					{"object": "model"},
					{"id": "another-valid", "object": "model"}
				]
			}`),
			platformType: domain.ProfileLmStudio,
			expectSkip:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			platformProfile, err := profileFactory.GetProfile(tt.platformType)
			if err != nil {
				t.Fatalf("Failed to get profile: %v", err)
			}

			format := platformProfile.GetModelResponseFormat()
			models, err := parser.ParseModelsResponse(tt.responseBody, format, platformProfile)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if tt.expectSkip {
				// Should have skipped invalid models but processed valid ones
				validCount := 0
				for _, model := range models {
					if model.Name != "" {
						validCount++
					}
				}
				if validCount == 0 {
					t.Error("Expected to parse at least some valid models")
				}
				t.Logf("Successfully parsed %d valid models, skipped invalid ones", validCount)
			}
		})
	}
}

func TestPlatformSpecificMetadata(t *testing.T) {
	parser := NewResponseParser(createTestLogger())
	profileFactory := profile.NewFactory()

	// Test Ollama nested details parsing
	t.Run("Ollama nested details", func(t *testing.T) {
		responseBody := []byte(`{
			"models": [{
				"name": "test-model",
				"digest": "abc123",
				"modified_at": "2025-05-30T14:24:44Z",
				"details": {
					"parameter_size": "7B",
					"quantization_level": "Q4_K_M",
					"family": "llama",
					"families": ["llama", "code"],
					"format": "gguf"
				}
			}]
		}`)

		profile, _ := profileFactory.GetProfile(domain.ProfileOllama)
		format := profile.GetModelResponseFormat()
		models, err := parser.ParseModelsResponse(responseBody, format, profile)

		if err != nil || len(models) != 1 {
			t.Fatalf("Failed to parse: %v", err)
		}

		model := models[0]
		if model.Details == nil {
			t.Fatal("Expected details to be parsed")
		}

		tests := map[string]string{
			"parameter_size":     "7B",
			"quantization_level": "Q4_K_M",
			"family":             "llama",
			"format":             "gguf",
		}

		for field, expected := range tests {
			var actual *string
			switch field {
			case "parameter_size":
				actual = model.Details.ParameterSize
			case "quantization_level":
				actual = model.Details.QuantizationLevel
			case "family":
				actual = model.Details.Family
			case "format":
				actual = model.Details.Format
			}

			if actual == nil || *actual != expected {
				t.Errorf("Expected %s to be %q, got %v", field, expected, actual)
			}
		}

		if len(model.Details.Families) != 2 {
			t.Errorf("Expected 2 families, got %d", len(model.Details.Families))
		}
	})

	// Test LM Studio flat structure parsing
	t.Run("LM Studio flat structure", func(t *testing.T) {
		responseBody := []byte(`{
			"data": [{
				"id": "test-model",
				"arch": "qwen2_vl",
				"quantization": "4bit",
				"compatibility_type": "mlx",
				"max_context_length": 32768,
				"state": "loaded"
			}]
		}`)

		profile, _ := profileFactory.GetProfile(domain.ProfileLmStudio)
		format := profile.GetModelResponseFormat()
		models, err := parser.ParseModelsResponse(responseBody, format, profile)

		if err != nil || len(models) != 1 {
			t.Fatalf("Failed to parse: %v", err)
		}

		model := models[0]
		if model.Details == nil {
			t.Fatal("Expected details to be parsed")
		}

		if model.Details.Family == nil || *model.Details.Family != "qwen2_vl" {
			t.Error("Expected family to be mapped from arch")
		}

		if model.Details.QuantizationLevel == nil || *model.Details.QuantizationLevel != "4bit" {
			t.Error("Expected quantization_level to be mapped from quantization")
		}

		if model.Details.Format == nil || *model.Details.Format != "mlx" {
			t.Error("Expected format to be mapped from compatibility_type")
		}

		if *model.Details.MaxContextLength != 32768 {
			t.Error("Expected MaxConnectionLength to contain context length")
		}

		if !strings.Contains(*model.Details.State, "loaded") {
			t.Error("Expected State to be 'loaded'")
		}
	})
}

func TestResponseParserConcurrency(t *testing.T) {
	parser := NewResponseParser(createTestLogger())
	profileFactory := profile.NewFactory()

	responseBody := []byte(`{
		"models": [
			{"name": "model1", "size": 1000},
			{"name": "model2", "size": 2000}
		]
	}`)

	profile, _ := profileFactory.GetProfile(domain.ProfileOllama)
	format := profile.GetModelResponseFormat()

	// Test concurrent parsing
	const numGoroutines = 10
	results := make(chan error, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			models, err := parser.ParseModelsResponse(responseBody, format, profile)
			if err != nil {
				results <- err
				return
			}
			if len(models) != 2 {
				results <- fmt.Errorf("expected 2 models, got %d", len(models))
				return
			}
			results <- nil
		}()
	}

	for i := 0; i < numGoroutines; i++ {
		if err := <-results; err != nil {
			t.Errorf("Concurrent parsing failed: %v", err)
		}
	}
}
