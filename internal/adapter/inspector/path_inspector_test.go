package inspector

import (
	"context"
	"net/http"
	"testing"

	"github.com/thushan/olla/internal/adapter/registry/profile"
	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/logger"
)

func TestPathInspector_Name(t *testing.T) {
	inspector := createTestPathInspector(t)

	if got := inspector.Name(); got != "path" {
		t.Errorf("PathInspector.Name() = %v, want %v", got, "path")
	}
}

func TestPathInspector_Inspect_EmptyPath(t *testing.T) {
	inspector := createTestPathInspector(t)
	ctx := context.Background()
	req, _ := http.NewRequest("POST", "/", nil)
	profile := domain.NewRequestProfile("")

	err := inspector.Inspect(ctx, req, profile)

	if err != nil {
		t.Errorf("PathInspector.Inspect() with empty path should not error, got %v", err)
	}

	if len(profile.SupportedBy) != 0 {
		t.Errorf("PathInspector.Inspect() with empty path should not add supported profiles, got %v", profile.SupportedBy)
	}
}

func TestPathInspector_Inspect_ChatCompletions(t *testing.T) {
	inspector := createTestPathInspector(t)
	ctx := context.Background()
	req, _ := http.NewRequest("POST", "/v1/chat/completions", nil)
	profile := domain.NewRequestProfile("/v1/chat/completions")

	err := inspector.Inspect(ctx, req, profile)

	if err != nil {
		t.Errorf("PathInspector.Inspect() error = %v, want nil", err)
	}

	expectedProfiles := []string{domain.ProfileOllama, domain.ProfileLmStudio, domain.ProfileOpenAICompatible}
	if !containsAllProfiles(profile.SupportedBy, expectedProfiles) {
		t.Errorf("PathInspector.Inspect() supported profiles = %v, want all of %v", profile.SupportedBy, expectedProfiles)
	}

	meta, exists := profile.InspectionMeta.Load(domain.InspectionMetaPathSupport)
	if !exists {
		t.Error("PathInspector.Inspect() should set path support metadata")
	}

	if metaSlice, ok := meta.([]string); !ok || len(metaSlice) == 0 {
		t.Errorf("PathInspector.Inspect() metadata should be non-empty string slice, got %v", meta)
	}
}

func TestPathInspector_Inspect_Completions(t *testing.T) {
	inspector := createTestPathInspector(t)
	ctx := context.Background()
	req, _ := http.NewRequest("POST", "/v1/completions", nil)
	profile := domain.NewRequestProfile("/v1/completions")

	err := inspector.Inspect(ctx, req, profile)

	if err != nil {
		t.Errorf("PathInspector.Inspect() error = %v, want nil", err)
	}

	expectedProfiles := []string{domain.ProfileOllama, domain.ProfileLmStudio, domain.ProfileOpenAICompatible}
	if !containsAllProfiles(profile.SupportedBy, expectedProfiles) {
		t.Errorf("PathInspector.Inspect() supported profiles = %v, want all of %v", profile.SupportedBy, expectedProfiles)
	}
}

func TestPathInspector_Inspect_OllamaGenerate(t *testing.T) {
	inspector := createTestPathInspector(t)
	ctx := context.Background()
	req, _ := http.NewRequest("POST", "/api/generate", nil)
	profile := domain.NewRequestProfile("/api/generate")

	err := inspector.Inspect(ctx, req, profile)

	if err != nil {
		t.Errorf("PathInspector.Inspect() error = %v, want nil", err)
	}

	if !contains(profile.SupportedBy, domain.ProfileOllama) {
		t.Errorf("PathInspector.Inspect() should support Ollama for /api/generate path, got %v", profile.SupportedBy)
	}

	if contains(profile.SupportedBy, domain.ProfileLmStudio) {
		t.Errorf("PathInspector.Inspect() should not support LMStudio for /api/generate path, got %v", profile.SupportedBy)
	}
}

/*
	func TestPathInspector_Inspect_LMStudioModels(t *testing.T) {
		inspector := createTestPathInspector(t)
		ctx := context.Background()
		req, _ := http.NewRequest("GET", "/api/v0/models", nil)
		profile := domain.NewRequestProfile("/api/v0/models")

		err := inspector.Inspect(ctx, req, profile)

		if err != nil {
			t.Errorf("PathInspector.Inspect() error = %v, want nil", err)
		}

		if contains(profile.SupportedBy, domain.ProfileLmStudio) {
			t.Errorf("PathInspector.Inspect() LMStudio models path should not match parsing rules (models path != request path), got %v", profile.SupportedBy)
		}
	}
*/
func TestPathInspector_Inspect_UnknownPath(t *testing.T) {
	inspector := createTestPathInspector(t)
	ctx := context.Background()
	req, _ := http.NewRequest("POST", "/unknown/path", nil)
	profile := domain.NewRequestProfile("/unknown/path")

	err := inspector.Inspect(ctx, req, profile)

	if err != nil {
		t.Errorf("PathInspector.Inspect() error = %v, want nil", err)
	}

	if len(profile.SupportedBy) != 0 {
		t.Errorf("PathInspector.Inspect() unknown path should not match any profiles, got %v", profile.SupportedBy)
	}

	_, exists := profile.InspectionMeta.Load(domain.InspectionMetaPathSupport)
	if exists {
		t.Error("PathInspector.Inspect() should not set metadata for unknown paths")
	}
}

func TestPathInspector_Inspect_PrefixedPaths(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected []string
	}{
		{
			name:     "prefixed chat completions",
			path:     "/api/v1/chat/completions",
			expected: []string{domain.ProfileOllama, domain.ProfileLmStudio, domain.ProfileOpenAICompatible},
		},
		{
			name:     "deeply prefixed completions",
			path:     "/some/deep/prefix/v1/completions",
			expected: []string{domain.ProfileOllama, domain.ProfileLmStudio, domain.ProfileOpenAICompatible},
		},
		{
			name:     "prefixed ollama generate",
			path:     "/prefix/api/generate",
			expected: []string{domain.ProfileOllama},
		},
	}

	inspector := createTestPathInspector(t)
	ctx := context.Background()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest("POST", tt.path, nil)
			profile := domain.NewRequestProfile(tt.path)

			err := inspector.Inspect(ctx, req, profile)

			if err != nil {
				t.Errorf("PathInspector.Inspect() error = %v, want nil", err)
			}

			for _, expectedProfile := range tt.expected {
				if !contains(profile.SupportedBy, expectedProfile) {
					t.Errorf("PathInspector.Inspect() should support %v for path %v, got %v", expectedProfile, tt.path, profile.SupportedBy)
				}
			}
		})
	}
}

func TestPathInspector_pathMatchesRules(t *testing.T) {
	inspector := createTestPathInspector(t)

	tests := []struct {
		name     string
		path     string
		rules    domain.RequestParsingRules
		expected bool
	}{
		{
			name: "exact chat completions match",
			path: "/v1/chat/completions",
			rules: domain.RequestParsingRules{
				ChatCompletionsPath: "/v1/chat/completions",
				CompletionsPath:     "/v1/completions",
			},
			expected: true,
		},
		{
			name: "suffix chat completions match",
			path: "/api/v1/chat/completions",
			rules: domain.RequestParsingRules{
				ChatCompletionsPath: "/v1/chat/completions",
			},
			expected: true,
		},
		{
			name: "completions path match",
			path: "/v1/completions",
			rules: domain.RequestParsingRules{
				CompletionsPath: "/v1/completions",
			},
			expected: true,
		},
		{
			name: "generate path match",
			path: "/api/generate",
			rules: domain.RequestParsingRules{
				GeneratePath: "/api/generate",
			},
			expected: true,
		},
		{
			name: "no match",
			path: "/unknown/path",
			rules: domain.RequestParsingRules{
				ChatCompletionsPath: "/v1/chat/completions",
				CompletionsPath:     "/v1/completions",
			},
			expected: false,
		},
		{
			name:     "empty rules",
			path:     "/v1/chat/completions",
			rules:    domain.RequestParsingRules{},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := inspector.pathMatchesRules(tt.path, tt.rules)
			if result != tt.expected {
				t.Errorf("pathMatchesRules() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func createTestPathInspector(t *testing.T) *PathInspector {
	t.Helper()

	profileFactory := profile.NewFactoryLegacy()
	logger := createTestLogger()

	return NewPathInspector(profileFactory, logger)
}

func createTestLogger() logger.StyledLogger {
	cfg := &logger.Config{
		Level:      "error",
		PrettyLogs: false,
		Theme:      "default",
	}

	baseLogger, styledLogger, _, err := logger.NewWithTheme(cfg)
	if err != nil {
		panic(err)
	}

	_ = baseLogger
	return styledLogger
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func containsAllProfiles(got, expected []string) bool {
	for _, exp := range expected {
		if !contains(got, exp) {
			return false
		}
	}
	return true
}
