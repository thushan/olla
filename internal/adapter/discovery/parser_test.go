package discovery

import (
	"testing"
	"time"

	"github.com/thushan/olla/internal/core/domain"
)

func TestParseModelsResponse(t *testing.T) {
	parser := NewResponseParser(createTestLogger())

	tests := []struct {
		name           string
		responseBody   []byte
		format         domain.ModelResponseFormat
		expectedModels int
		expectedError  bool
		expectedNames  []string
	}{
		{
			name: "Ollama response format",
			responseBody: []byte(`{
				"models": [
					{
						"name": "codegemma:9b",
						"size": 5011852809,
						"digest": "sha256:0c96700aaada572ce9bb6999d1fda9b53e9e6cef5d74fda1e066a1ba811b93f3"
					},
					{
						"name": "devstral:23.6b",
						"size": 14333927918
					}
				]
			}`),
			format: domain.ModelResponseFormat{
				ResponseType:    "object",
				ModelsFieldPath: "models",
			},
			expectedModels: 2,
			expectedError:  false,
			expectedNames:  []string{"codegemma:9b", "devstral:23.6b"},
		},
		{
			name: "LM Studio response format",
			responseBody: []byte(`{
				"object": "list",
				"data": [
					{
						"id": "microsoft/DialoGPT-medium",
						"object": "model",
						"created": 1686935002,
						"owned_by": "microsoft"
					},
					{
						"id": "gpt-3.5-turbo",
						"object": "model",
						"created": 1686935002,
						"owned_by": "openai"
					}
				]
			}`),
			format: domain.ModelResponseFormat{
				ResponseType:    "object",
				ModelsFieldPath: "data",
			},
			expectedModels: 2,
			expectedError:  false,
			expectedNames:  []string{"microsoft/DialoGPT-medium", "gpt-3.5-turbo"},
		},
		{
			name: "OpenAI compatible response format",
			responseBody: []byte(`{
				"object": "list",
				"data": [
					{
						"id": "text-davinci-003",
						"object": "model",
						"created": 1669599635,
						"owned_by": "openai-internal"
					}
				]
			}`),
			format: domain.ModelResponseFormat{
				ResponseType:    "object",
				ModelsFieldPath: "data",
			},
			expectedModels: 1,
			expectedError:  false,
			expectedNames:  []string{"text-davinci-003"},
		},
		{
			name:           "Empty response",
			responseBody:   []byte(`{"models": []}`),
			format:         getOllamaFormat(),
			expectedModels: 0,
			expectedError:  false,
			expectedNames:  []string{},
		},
		{
			name:           "Empty body",
			responseBody:   []byte{},
			format:         getOllamaFormat(),
			expectedModels: 0,
			expectedError:  false,
			expectedNames:  []string{},
		},
		{
			name:          "Invalid JSON",
			responseBody:  []byte(`{"models": [`),
			format:        getOllamaFormat(),
			expectedError: true,
		},
		{
			name:           "Missing models field",
			responseBody:   []byte(`{"other": []}`),
			format:         getOllamaFormat(),
			expectedModels: 0,
			expectedError:  false,
		},
		{
			name:          "Available field is not array",
			responseBody:  []byte(`{"models": "not-an-array"}`),
			format:        getOllamaFormat(),
			expectedError: true,
		},
		{
			name: "Available with missing names are skipped",
			responseBody: []byte(`{
				"models": [
					{"name": "valid-model", "size": 123},
					{"size": 456},
					{"name": "", "size": 789},
					{"name": "another-valid-model"}
				]
			}`),
			format:         getOllamaFormat(),
			expectedModels: 2,
			expectedError:  false,
			expectedNames:  []string{"valid-model", "another-valid-model"},
		},
		{
			name: "Response with descriptions",
			responseBody: []byte(`{
				"models": [
					{
						"name": "codegemma:9b",
						"size": 3825819519,
						"description": "Code Gemma 9B model"
					}
				]
			}`),
			format:         getOllamaFormat(),
			expectedModels: 1,
			expectedError:  false,
			expectedNames:  []string{"codegemma:9b"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			models, err := parser.ParseModelsResponse(tt.responseBody, tt.format)

			if tt.expectedError {
				if err == nil {
					t.Errorf("Expected error but got nuffin muffin!")
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
			}

			// Validate specific fields for first test case
			if tt.name == "Ollama response format" && len(models) > 0 {
				if models[0].Size != 5011852809 {
					t.Errorf("Expected first model size to be 5011852809, got %d", models[0].Size)
				}
			}

			if tt.name == "LM Studio response format" && len(models) > 0 {
				if models[0].Type != "model" {
					t.Errorf("Expected first model type to be 'model', got %s", models[0].Type)
				}
			}

			if tt.name == "Response with descriptions" && len(models) > 0 {
				if models[0].Description != "Code Gemma 9B model" {
					t.Errorf("Expected description to be 'Code Gemma 9B model', got %s", models[0].Description)
				}
			}
		})
	}
}

func TestParseOllamaResponse(t *testing.T) {
	parser := NewResponseParser(createTestLogger())

	responseBody := []byte(`{
		"models": [
			{
				"name": "codegemma:9b",
				"size": 5011852809,
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
	}`)

	models, err := parser.parseOllamaResponse(responseBody)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if len(models) != 1 {
		t.Fatalf("Expected 1 model, got %d", len(models))
	}

	model := models[0]
	if model.Name != "codegemma:9b" {
		t.Errorf("Expected name 'codegemma:9b', got %s", model.Name)
	}
	if model.Size != 5011852809 {
		t.Errorf("Expected size 5011852809, got %d", model.Size)
	}
}

func TestParseLMStudioResponse(t *testing.T) {
	parser := NewResponseParser(createTestLogger())

	responseBody := []byte(`{
		"object": "list",
		"data": [
			{
				"id": "microsoft/DialoGPT-medium",
				"object": "model",
				"created": 1686935002,
				"owned_by": "microsoft"
			}
		]
	}`)

	models, err := parser.parseLMStudioResponse(responseBody)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if len(models) != 1 {
		t.Fatalf("Expected 1 model, got %d", len(models))
	}

	model := models[0]
	if model.Name != "microsoft/DialoGPT-medium" {
		t.Errorf("Expected name 'microsoft/DialoGPT-medium', got %s", model.Name)
	}
	if model.Type != "model" {
		t.Errorf("Expected type 'model', got %s", model.Type)
	}
}

func TestParseOpenAICompatibleResponse(t *testing.T) {
	parser := NewResponseParser(createTestLogger())

	responseBody := []byte(`{
		"object": "list",
		"data": [
			{
				"id": "gpt-3.5-turbo",
				"object": "model",
				"created": 1677610602,
				"owned_by": "openai"
			}
		]
	}`)

	models, err := parser.parseOpenAICompatibleResponse(responseBody)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if len(models) != 1 {
		t.Fatalf("Expected 1 model, got %d", len(models))
	}

	model := models[0]
	if model.Name != "gpt-3.5-turbo" {
		t.Errorf("Expected name 'gpt-3.5-turbo', got %s", model.Name)
	}
	if model.Type != "model" {
		t.Errorf("Expected type 'model', got %s", model.Type)
	}
}

func getOllamaFormat() domain.ModelResponseFormat {
	return domain.ModelResponseFormat{
		ResponseType:    "object",
		ModelsFieldPath: "models",
	}
}

func (p *ResponseParser) parseOllamaResponse(data []byte) ([]*domain.ModelInfo, error) {
	format := domain.ModelResponseFormat{
		ResponseType:    "object",
		ModelsFieldPath: "models",
	}
	return p.parseObjectResponse(data, format)
}

func (p *ResponseParser) parseLMStudioResponse(data []byte) ([]*domain.ModelInfo, error) {
	format := domain.ModelResponseFormat{
		ResponseType:    "object",
		ModelsFieldPath: "data",
	}
	return p.parseObjectResponse(data, format)
}

func (p *ResponseParser) parseOpenAICompatibleResponse(data []byte) ([]*domain.ModelInfo, error) {
	format := domain.ModelResponseFormat{
		ResponseType:    "object",
		ModelsFieldPath: "data",
	}
	return p.parseObjectResponse(data, format)
}
