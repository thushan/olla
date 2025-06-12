package profile

import (
	"testing"

	"github.com/thushan/olla/internal/core/domain"
)

func TestOllamaProfile(t *testing.T) {
	profile := NewOllamaProfile()

	if profile.GetName() != domain.ProfileOllama {
		t.Errorf("Expected name %q, got %q", domain.ProfileOllama, profile.GetName())
	}

	url := profile.GetModelDiscoveryURL("http://localhost:11434")
	expected := "http://localhost:11434/api/tags"
	if url != expected {
		t.Errorf("Expected %q, got %q", expected, url)
	}

	url = profile.GetModelDiscoveryURL("http://localhost:11434/")
	if url != expected {
		t.Errorf("Expected %q, got %q", expected, url)
	}

	if !profile.IsOpenAPICompatible() {
		t.Error("ProfileOllama should be OpenAPI compatible")
	}

	rules := profile.GetRequestParsingRules()
	if rules.ModelFieldName != "model" {
		t.Errorf("Expected model field 'model', got %q", rules.ModelFieldName)
	}
	if !rules.SupportsStreaming {
		t.Error("ProfileOllama should support streaming")
	}

	format := profile.GetModelResponseFormat()
	if format.ResponseType != "object" {
		t.Errorf("Expected response type 'object', got %q", format.ResponseType)
	}
	if format.ModelsFieldPath != "models" {
		t.Errorf("Expected models field path 'models', got %q", format.ModelsFieldPath)
	}

	// Test enhanced metadata fields
	if format.ModelsFieldPath != "models" {
		t.Errorf("Expected models field path 'models', got %q", format.ModelsFieldPath)
	}

	// Test Ollama model parsing
	ollamaModelData := map[string]interface{}{
		"name":        "devstral:latest",
		"size":        14333927918,
		"digest":      "c4b2fa0c33d75457e5f1c8507c906a79e73285768686db13b9cbac0c7ee3a854",
		"modified_at": "2025-05-30T14:24:44.5116551+10:00",
		"details": map[string]interface{}{
			"parameter_size":     "23.6B",
			"quantization_level": "Q4_K_M",
			"family":             "llama",
			"families":           []interface{}{"llama"},
			"format":             "gguf",
			"parent_model":       "",
		},
	}

	modelInfo, err := profile.ParseModel(ollamaModelData)
	if err != nil {
		t.Fatalf("Failed to parse Ollama model: %v", err)
	}

	if modelInfo.Name != "devstral:latest" {
		t.Errorf("Expected name 'devstral:latest', got %q", modelInfo.Name)
	}

	if modelInfo.Size != 14333927918 {
		t.Errorf("Expected size 14333927918, got %d", modelInfo.Size)
	}

	if modelInfo.Details == nil {
		t.Fatal("Expected details to be parsed")
	}

	if modelInfo.Details.ParameterSize == nil || *modelInfo.Details.ParameterSize != "23.6B" {
		t.Error("Expected parameter_size to be '23.6B'")
	}

	if modelInfo.Details.QuantizationLevel == nil || *modelInfo.Details.QuantizationLevel != "Q4_K_M" {
		t.Error("Expected quantization_level to be 'Q4_K_M'")
	}

	if modelInfo.Details.Digest == nil || *modelInfo.Details.Digest != "c4b2fa0c33d75457e5f1c8507c906a79e73285768686db13b9cbac0c7ee3a854" {
		t.Error("Expected digest to be parsed correctly")
	}
}

func TestLMStudioProfile(t *testing.T) {
	profile := NewLMStudioProfile()

	if profile.GetName() != domain.ProfileLmStudio {
		t.Errorf("Expected name %q, got %q", domain.ProfileLmStudio, profile.GetName())
	}

	baseUrl := "http://localhost:1234"
	url := profile.GetModelDiscoveryURL(baseUrl)
	expected := baseUrl + LMStudioProfileModelsPath
	if url != expected {
		t.Errorf("Expected %q, got %q", expected, url)
	}

	if !profile.IsOpenAPICompatible() {
		t.Error("LM Studio should be OpenAPI compatible")
	}

	rules := profile.GetRequestParsingRules()
	if rules.ModelFieldName != "model" {
		t.Errorf("Expected model field 'model', got %q", rules.ModelFieldName)
	}
	if rules.GeneratePath != "" {
		t.Error("LM Studio should not have generate path")
	}

	format := profile.GetModelResponseFormat()
	if format.ModelsFieldPath != "data" {
		t.Errorf("Expected models field path 'data', got %q", format.ModelsFieldPath)
	}

	// LM Studio should have minimal metadata support
	if format.ModelsFieldPath != "data" {
		t.Errorf("Expected models field path 'data', got %q", format.ModelsFieldPath)
	}

	// Test LM Studio model parsing
	lmStudioModelData := map[string]interface{}{
		"id":                 "meta-llama-3.1-8b-instruct",
		"object":             "model",
		"type":               "llm",
		"publisher":          "lmstudio-community",
		"arch":               "llama",
		"compatibility_type": "gguf",
		"quantization":       "Q4_K_M",
		"state":              "not-loaded",
		"max_context_length": 131072,
	}

	modelInfo, err := profile.ParseModel(lmStudioModelData)
	if err != nil {
		t.Fatalf("Failed to parse LM Studio model: %v", err)
	}

	if modelInfo.Name != "meta-llama-3.1-8b-instruct" {
		t.Errorf("Expected name 'meta-llama-3.1-8b-instruct', got %q", modelInfo.Name)
	}

	if modelInfo.Type != "model" {
		t.Errorf("Expected type 'model', got %q", modelInfo.Type)
	}

	if modelInfo.Details == nil {
		t.Fatal("Expected details to be parsed for LM Studio")
	}

	if modelInfo.Details.Family == nil || *modelInfo.Details.Family != "llama" {
		t.Error("Expected family to be 'llama'")
	}

	if modelInfo.Details.QuantizationLevel == nil || *modelInfo.Details.QuantizationLevel != "Q4_K_M" {
		t.Error("Expected quantization_level to be 'Q4_K_M'")
	}
}

func TestOpenAICompatibleProfile(t *testing.T) {
	profile := NewOpenAICompatibleProfile()

	if profile.GetName() != domain.ProfileOpenAICompatible {
		t.Errorf("Expected name %q, got %q", domain.ProfileOpenAICompatible, profile.GetName())
	}

	url := profile.GetModelDiscoveryURL("http://localhost:8080")
	expected := "http://localhost:8080/v1/models"
	if url != expected {
		t.Errorf("Expected %q, got %q", expected, url)
	}

	if !profile.IsOpenAPICompatible() {
		t.Error("OpenAI compatible profile should be OpenAPI compatible")
	}

	rules := profile.GetRequestParsingRules()
	if !rules.SupportsStreaming {
		t.Error("OpenAI compatible should support streaming")
	}

	format := profile.GetModelResponseFormat()
	if format.ResponseType != "object" {
		t.Errorf("Expected response type 'object', got %q", format.ResponseType)
	}

	// OpenAI compatible should have minimal metadata support
	if format.ModelsFieldPath != "data" {
		t.Errorf("Expected models field path 'data', got %q", format.ModelsFieldPath)
	}

	// Test OpenAI compatible model parsing
	openaiModelData := map[string]interface{}{
		"id":      "gpt-3.5-turbo",
		"object":  "model",
		"created": 1677610602,
	}

	modelInfo, err := profile.ParseModel(openaiModelData)
	if err != nil {
		t.Fatalf("Failed to parse OpenAI model: %v", err)
	}

	if modelInfo.Name != "gpt-3.5-turbo" {
		t.Errorf("Expected name 'gpt-3.5-turbo', got %q", modelInfo.Name)
	}

	if modelInfo.Type != "model" {
		t.Errorf("Expected type 'model', got %q", modelInfo.Type)
	}
}

func TestProfileVersioning(t *testing.T) {
	profiles := []domain.PlatformProfile{
		NewOllamaProfile(),
		NewLMStudioProfile(),
		NewOpenAICompatibleProfile(),
	}

	for _, profile := range profiles {
		version := profile.GetVersion()
		if version == "" {
			t.Errorf("Profile %s should have a version", profile.GetName())
		}
	}
}

func TestDetectionHints(t *testing.T) {
	profile := NewOllamaProfile()
	hints := profile.GetDetectionHints()

	if len(hints.PathIndicators) == 0 {
		t.Error("ProfileOllama should have path indicators for detection")
	}

	found := false
	for _, path := range hints.PathIndicators {
		if path == "/api/tags" {
			found = true
			break
		}
	}
	if !found {
		t.Error("ProfileOllama should have /api/tags as path indicator")
	}
}

func TestEnhancedMetadataConfiguration(t *testing.T) {
	tests := []struct {
		name                 string
		profile              domain.PlatformProfile
		expectDetailsSupport bool
		testModelData        map[string]interface{}
		expectedDetails      map[string]string
	}{
		{
			name:                 "Ollama supports rich metadata",
			profile:              NewOllamaProfile(),
			expectDetailsSupport: true,
			testModelData: map[string]interface{}{
				"name": "test-model",
				"details": map[string]interface{}{
					"parameter_size":     "7B",
					"quantization_level": "Q4_K_M",
					"family":             "llama",
				},
			},
			expectedDetails: map[string]string{
				"parameter_size":     "7B",
				"quantization_level": "Q4_K_M",
				"family":             "llama",
			},
		},
		{
			name:                 "LM Studio has rich metadata",
			profile:              NewLMStudioProfile(),
			expectDetailsSupport: true,
			testModelData: map[string]interface{}{
				"id":                 "test-model",
				"arch":               "llama",
				"quantization":       "Q4_K_M",
				"compatibility_type": "gguf",
			},
			expectedDetails: map[string]string{
				"family":             "llama",
				"quantization_level": "Q4_K_M",
				"format":             "gguf",
			},
		},
		{
			name:                 "OpenAI compatible has minimal metadata",
			profile:              NewOpenAICompatibleProfile(),
			expectDetailsSupport: false,
			testModelData: map[string]interface{}{
				"id":     "test-model",
				"object": "model",
			},
			expectedDetails: map[string]string{},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			modelInfo, err := test.profile.ParseModel(test.testModelData)
			if err != nil {
				t.Fatalf("Failed to parse model: %v", err)
			}

			hasDetailsSupport := modelInfo.Details != nil
			if hasDetailsSupport != test.expectDetailsSupport {
				t.Errorf("Expected details support %v, got %v", test.expectDetailsSupport, hasDetailsSupport)
			}

			if test.expectDetailsSupport && modelInfo.Details != nil {
				for field, expectedValue := range test.expectedDetails {
					switch field {
					case "parameter_size":
						if modelInfo.Details.ParameterSize == nil || *modelInfo.Details.ParameterSize != expectedValue {
							t.Errorf("Expected %s to be %q", field, expectedValue)
						}
					case "quantization_level":
						if modelInfo.Details.QuantizationLevel == nil || *modelInfo.Details.QuantizationLevel != expectedValue {
							t.Errorf("Expected %s to be %q", field, expectedValue)
						}
					case "family":
						if modelInfo.Details.Family == nil || *modelInfo.Details.Family != expectedValue {
							t.Errorf("Expected %s to be %q", field, expectedValue)
						}
					case "format":
						if modelInfo.Details.Format == nil || *modelInfo.Details.Format != expectedValue {
							t.Errorf("Expected %s to be %q", field, expectedValue)
						}
					}
				}
			}
		})
	}
}
