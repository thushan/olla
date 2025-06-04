package profile

import (
	"github.com/thushan/olla/internal/core/domain"
	"testing"
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
}

func TestLMStudioProfile(t *testing.T) {
	profile := NewLMStudioProfile()

	if profile.GetName() != domain.ProfileLmStudio {
		t.Errorf("Expected name %q, got %q", domain.ProfileLmStudio, profile.GetName())
	}

	url := profile.GetModelDiscoveryURL("http://localhost:1234")
	expected := "http://localhost:1234/v1/models"
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
	if format.ModelNameField != "id" {
		t.Errorf("Expected model name field 'id', got %q", format.ModelNameField)
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
