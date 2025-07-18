package inspector

import (
	"context"
	"net/http"
	"testing"

	"github.com/thushan/olla/internal/adapter/registry/profile"
	"github.com/thushan/olla/internal/core/domain"
)

func TestNewFactory(t *testing.T) {
	profileFactory := profile.NewFactoryLegacy()
	logger := createTestLogger()

	factory := NewFactory(profileFactory, logger)

	if factory == nil {
		t.Fatal("NewFactory() should not return nil")
	}

	if factory.profileFactory != profileFactory {
		t.Error("NewFactory() should store profile factory reference")
	}

	if factory.logger != logger {
		t.Error("NewFactory() should store logger reference")
	}
}

func TestFactory_CreateDefaultChain(t *testing.T) {
	logger := createTestLogger()
	profileFactory := profile.NewFactoryLegacy()
	factory := NewFactory(profileFactory, logger)

	chain := factory.CreateDefaultChain()

	if chain == nil {
		t.Fatal("CreateDefaultChain() should not return nil")
	}

	// chain is functional by testing with a known path
	ctx := context.Background()
	req, _ := http.NewRequest("POST", "/v1/chat/completions", nil)
	targetPath := "/v1/chat/completions"

	profile, err := chain.Inspect(ctx, req, targetPath)

	if err != nil {
		t.Errorf("CreateDefaultChain() chain should work, got error: %v", err)
	}

	if profile == nil {
		t.Fatal("CreateDefaultChain() chain should return profile")
	}

	if len(profile.SupportedBy) == 0 {
		t.Error("CreateDefaultChain() should create functional chain that identifies supported profiles")
	}

	// should include all major profiles for chat completions
	expectedProfiles := []string{domain.ProfileOllama, domain.ProfileLmStudio, domain.ProfileOpenAICompatible}
	for _, expected := range expectedProfiles {
		if !contains(profile.SupportedBy, expected) {
			t.Errorf("CreateDefaultChain() should support %v for chat completions, got %v", expected, profile.SupportedBy)
		}
	}
}

func TestFactory_CreateDefaultChain_PathInspectorIncluded(t *testing.T) {
	logger := createTestLogger()
	profileFactory := profile.NewFactoryLegacy()
	factory := NewFactory(profileFactory, logger)

	chain := factory.CreateDefaultChain()

	// test with Ollama-specific path
	ctx := context.Background()
	req, _ := http.NewRequest("POST", "/api/generate", nil)
	targetPath := "/api/generate"

	profile, err := chain.Inspect(ctx, req, targetPath)

	if err != nil {
		t.Errorf("CreateDefaultChain() error = %v, want nil", err)
	}

	if !contains(profile.SupportedBy, domain.ProfileOllama) {
		t.Errorf("CreateDefaultChain() should include PathInspector that supports Ollama for /api/generate")
	}

	if contains(profile.SupportedBy, domain.ProfileLmStudio) {
		t.Errorf("CreateDefaultChain() PathInspector should not support LMStudio for Ollama-specific path")
	}
}

func TestFactory_CreateDefaultChain_MultipleCallsReturnDifferentInstances(t *testing.T) {
	logger := createTestLogger()
	profileFactory := profile.NewFactoryLegacy()
	factory := NewFactory(profileFactory, logger)

	// 2Chains!
	chain1 := factory.CreateDefaultChain()
	chain2 := factory.CreateDefaultChain()

	if chain1 == chain2 {
		t.Error("CreateDefaultChain() should return different instances on each call")
	}

	ctx := context.Background()
	req, _ := http.NewRequest("POST", "/v1/chat/completions", nil)
	targetPath := "/v1/chat/completions"

	profile1, err1 := chain1.Inspect(ctx, req, targetPath)
	profile2, err2 := chain2.Inspect(ctx, req, targetPath)

	if err1 != nil || err2 != nil {
		t.Errorf("Both chains should be functional, got errors: %v, %v", err1, err2)
	}

	if len(profile1.SupportedBy) == 0 || len(profile2.SupportedBy) == 0 {
		t.Error("Both chains should produce results")
	}
}

func TestFactory_CreatePathInspector(t *testing.T) {
	logger := createTestLogger()
	profileFactory := profile.NewFactoryLegacy()
	factory := NewFactory(profileFactory, logger)

	inspector := factory.CreatePathInspector()

	if inspector == nil {
		t.Fatal("CreatePathInspector() should not return nil")
	}

	if inspector.Name() != "path" {
		t.Errorf("CreatePathInspector() name = %v, want 'path'", inspector.Name())
	}

	ctx := context.Background()
	req, _ := http.NewRequest("POST", "/v1/chat/completions", nil)
	profile := domain.NewRequestProfile("/v1/chat/completions")

	err := inspector.Inspect(ctx, req, profile)

	if err != nil {
		t.Errorf("CreatePathInspector() inspector should work, got error: %v", err)
	}

	if len(profile.SupportedBy) == 0 {
		t.Error("CreatePathInspector() should create functional path inspector")
	}
}

func TestFactory_CreatePathInspector_MultipleCalls(t *testing.T) {
	logger := createTestLogger()
	profileFactory := profile.NewFactoryLegacy()
	factory := NewFactory(profileFactory, logger)

	inspector1 := factory.CreatePathInspector()
	inspector2 := factory.CreatePathInspector()

	if inspector1 == inspector2 {
		t.Error("CreatePathInspector() should return different instances")
	}

	if inspector1.Name() != inspector2.Name() {
		t.Error("CreatePathInspector() instances should have same name")
	}
}

func TestFactory_CreateChain(t *testing.T) {
	logger := createTestLogger()
	profileFactory := profile.NewFactoryLegacy()
	factory := NewFactory(profileFactory, logger)

	chain := factory.CreateChain()

	if chain == nil {
		t.Fatal("CreateChain() should not return nil")
	}

	ctx := context.Background()
	req, _ := http.NewRequest("POST", "/v1/chat/completions", nil)
	targetPath := "/v1/chat/completions"

	profile, err := chain.Inspect(ctx, req, targetPath)

	if err != nil {
		t.Errorf("CreateChain() empty chain should work, got error: %v", err)
	}

	if len(profile.SupportedBy) != 0 {
		t.Errorf("CreateChain() should create empty chain, got %v supported profiles", len(profile.SupportedBy))
	}
}

func TestFactory_CreateChain_CanAddInspectors(t *testing.T) {
	logger := createTestLogger()
	profileFactory := profile.NewFactoryLegacy()
	factory := NewFactory(profileFactory, logger)

	chain := factory.CreateChain()
	pathInspector := factory.CreatePathInspector()

	chain.AddInspector(pathInspector)

	ctx := context.Background()
	req, _ := http.NewRequest("POST", "/v1/chat/completions", nil)
	targetPath := "/v1/chat/completions"

	profile, err := chain.Inspect(ctx, req, targetPath)

	if err != nil {
		t.Errorf("CreateChain() with added inspector should work, got error: %v", err)
	}

	if len(profile.SupportedBy) == 0 {
		t.Error("CreateChain() with added inspector should produce results")
	}
}

func TestFactory_IntegrationWithRealProfiles(t *testing.T) {
	//make sure that factory works with real profile factory and all available profiles
	logger := createTestLogger()
	profileFactory := profile.NewFactoryLegacy()
	factory := NewFactory(profileFactory, logger)

	chain := factory.CreateDefaultChain()

	tests := []struct {
		name             string
		path             string
		expectedProfiles []string
	}{
		{
			name: "chat completions supports all",
			path: "/v1/chat/completions",
			expectedProfiles: []string{
				domain.ProfileOllama,
				domain.ProfileLmStudio,
				domain.ProfileOpenAICompatible,
			},
		},
		{
			name: "completions supports all",
			path: "/v1/completions",
			expectedProfiles: []string{
				domain.ProfileOllama,
				domain.ProfileLmStudio,
				domain.ProfileOpenAICompatible,
			},
		},
		{
			name: "ollama generate supports only ollama",
			path: "/api/generate",
			expectedProfiles: []string{
				domain.ProfileOllama,
			},
		},
	}

	ctx := context.Background()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest("POST", tt.path, nil)
			profile, err := chain.Inspect(ctx, req, tt.path)

			if err != nil {
				t.Errorf("Integration test error = %v, want nil", err)
			}

			for _, expectedProfile := range tt.expectedProfiles {
				if !contains(profile.SupportedBy, expectedProfile) {
					t.Errorf("Integration test should support %v for %v, got %v",
						expectedProfile, tt.path, profile.SupportedBy)
				}
			}

			// check that we don't have unexpected profiles for specific paths
			if tt.path == "/api/generate" {
				if contains(profile.SupportedBy, domain.ProfileLmStudio) {
					t.Errorf("Integration test should not support LMStudio for Ollama-specific path")
				}
			}
		})
	}
}

func TestFactory_NilProfileFactory(t *testing.T) {
	logger := createTestLogger()

	// shuld not panic during factory creation
	factory := NewFactory(nil, logger)

	if factory == nil {
		t.Fatal("NewFactory() should handle nil profile factory")
	}

	// creating inspectors with nil profile factory should not panic
	// but may not be functional
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("Factory methods should not panic with nil profile factory: %v", r)
		}
	}()

	chain := factory.CreateDefaultChain()
	if chain == nil {
		t.Error("CreateDefaultChain() should not return nil even with nil profile factory")
	}

	inspector := factory.CreatePathInspector()
	if inspector == nil {
		t.Error("CreatePathInspector() should not return nil even with nil profile factory")
	}
}

func TestFactory_Phase2Readiness(t *testing.T) {
	// this test ensures the factory is ready for Phase 2 extension
	profileFactory := profile.NewFactoryLegacy()
	logger := createTestLogger()
	factory := NewFactory(profileFactory, logger)

	// current default chain should work
	chain := factory.CreateDefaultChain()

	// should be able to create empty chain and add inspectors manually
	customChain := factory.CreateChain()
	pathInspector := factory.CreatePathInspector()

	customChain.AddInspector(pathInspector)
	// phase 2, we would add: customChain.AddInspector(factory.CreateModelInspector())

	ctx := context.Background()
	req, _ := http.NewRequest("POST", "/v1/chat/completions", nil)

	profile1, err1 := chain.Inspect(ctx, req, "/v1/chat/completions")
	profile2, err2 := customChain.Inspect(ctx, req, "/v1/chat/completions")

	if err1 != nil || err2 != nil {
		t.Errorf("Phase 2 readiness test failed: %v, %v", err1, err2)
	}

	// both should produce similar results for path inspection
	if len(profile1.SupportedBy) != len(profile2.SupportedBy) {
		t.Error("Default and custom chains should produce similar results")
	}
}
