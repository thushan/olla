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

	baseUrl := "http://localhost:11434"
	url := profile.GetModelDiscoveryURL(baseUrl)
	expected := baseUrl + OllamaProfileModelModelsPath
	if url != expected {
		t.Errorf("Expected %q, got %q", expected, url)
	}

	url = profile.GetModelDiscoveryURL(baseUrl)
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
}

func TestLMStudioProfile(t *testing.T) {
	profile := NewLMStudioProfile()

	if profile.GetName() != domain.ProfileLmStudio {
		t.Errorf("Expected name %q, got %q", domain.ProfileLmStudio, profile.GetName())
	}

	baseURL := "http://localhost:11434"
	url := profile.GetModelDiscoveryURL(baseURL)
	expected := baseURL + LMStudioProfileModelsPath
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
