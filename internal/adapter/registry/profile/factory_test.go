package profile

import (
	"testing"

	"github.com/thushan/olla/internal/core/domain"
)

// testFactory creates a factory for tests, using built-in profiles
func testFactory(t *testing.T) *Factory {
	t.Helper()
	factory, err := NewFactory("") // Empty dir uses built-in profiles
	if err != nil {
		t.Fatalf("Failed to create test factory: %v", err)
	}
	return factory
}

func TestNewFactory(t *testing.T) {
	factory := testFactory(t)

	profiles := factory.GetAvailableProfiles()
	expectedProfiles := []string{domain.ProfileLmStudio, domain.ProfileOllama}

	if len(profiles) != len(expectedProfiles) {
		t.Errorf("Expected %d profiles, got %d", len(expectedProfiles), len(profiles))
	}

	for _, expected := range expectedProfiles {
		found := false
		for _, actual := range profiles {
			if actual == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected profile %q not found", expected)
		}
	}
}

func TestGetProfile(t *testing.T) {
	factory := testFactory(t)

	profile, err := factory.GetProfile(domain.ProfileOllama)
	if err != nil {
		t.Fatalf("Failed to get ollama profile: %v", err)
	}
	if profile.GetName() != domain.ProfileOllama {
		t.Errorf("Expected ollama profile, got %q", profile.GetName())
	}

	profile, err = factory.GetProfile("nonexistent")
	if err != nil {
		t.Fatalf("Should return OpenAI compatible profile for nonexistent type: %v", err)
	}
	if profile.GetName() != domain.ProfileOpenAICompatible {
		t.Errorf("Expected OpenAI compatible profile for nonexistent type, got %q", profile.GetName())
	}
}

func TestValidateProfileType(t *testing.T) {
	factory := testFactory(t)

	if !factory.ValidateProfileType(domain.ProfileOllama) {
		t.Error("ollama should be valid profile type")
	}
	if !factory.ValidateProfileType(domain.ProfileAuto) {
		t.Error("auto should be valid profile type")
	}
	if factory.ValidateProfileType("invalid") {
		t.Error("invalid should not be valid profile type")
	}
}

func TestValidateProfileType_WithRoutingPrefixes(t *testing.T) {
	// Create factory with default profiles
	factory := testFactory(t)

	tests := []struct {
		name     string
		provider string
		expected bool
	}{
		// Direct profile names
		{"ollama profile name", "ollama", true},
		{"lm-studio profile name", "lm-studio", true},
		{"openai-compatible profile name", "openai-compatible", true},

		// Routing prefixes for lm-studio
		{"lmstudio prefix", "lmstudio", true},
		{"lm_studio prefix", "lm_studio", true},

		// Routing prefix for openai
		{"openai prefix", "openai", true},

		// Auto profile
		{"auto profile", "auto", true},

		// Unknown providers
		{"unknown provider", "unknown", false},
		{"empty provider", "", false},

		// vLLM if profile exists - not in built-in profiles
		{"vllm provider", "vllm", false}, // vllm is not a built-in profile
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := factory.ValidateProfileType(tt.provider)
			if result != tt.expected {
				t.Errorf("ValidateProfileType(%q) = %v, want %v", tt.provider, result, tt.expected)
			}
		})
	}
}

/*
	func TestProfileFactoryRegistration(t *testing.T) {
		factory := &Factory{
			profiles: make(map[string]domain.PlatformProfile),
		}

		customProfile := &mockProfile{name: "custom"}
		factory.RegisterProfile(customProfile)

		profile, err := factory.GetProfile("custom")
		if err != nil {
			t.Fatalf("Failed to get custom profile: %v", err)
		}
		if profile.GetName() != "custom" {
			t.Errorf("Expected custom profile, got %q", profile.GetName())
		}
	}

	func TestProfileFactoryConcurrency(t *testing.T) {
		factory := testFactory(t)

		var wg sync.WaitGroup
		errors := make(chan error, 100)

		for i := 0; i < 50; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, err := factory.GetProfile(domain.ProfileOllama)
				if err != nil {
					errors <- err
				}
			}()
		}

		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				custom := &mockProfile{name: fmt.Sprintf("custom-%d", id)}
				factory.RegisterProfile(custom)
			}(i)
		}

		wg.Wait()
		close(errors)

		for err := range errors {
			t.Errorf("Concurrent access error: %v", err)
		}
	}
*/
func TestGetAvailableProfiles(t *testing.T) {
	factory := testFactory(t)

	profiles := factory.GetAvailableProfiles()

	for _, profile := range profiles {
		if profile == domain.ProfileOpenAICompatible {
			t.Error("Available profiles should not include fallback profile")
		}
	}

	if len(profiles) == 0 {
		t.Error("Should have at least some available profiles")
	}
}

type mockProfile struct {
	name string
}

func (m *mockProfile) GetName() string                            { return m.name }
func (m *mockProfile) GetVersion() string                         { return "1.0" }
func (m *mockProfile) GetModelDiscoveryURL(baseURL string) string { return baseURL + "/mock" }
func (m *mockProfile) GetHealthCheckPath() string                 { return "/health" }
func (m *mockProfile) IsOpenAPICompatible() bool                  { return true }
func (m *mockProfile) GetRequestParsingRules() domain.RequestParsingRules {
	return domain.RequestParsingRules{}
}
func (m *mockProfile) GetModelResponseFormat() domain.ModelResponseFormat {
	return domain.ModelResponseFormat{}
}
func (m *mockProfile) GetDetectionHints() domain.DetectionHints { return domain.DetectionHints{} }
func (m *mockProfile) ParseModelsResponse(data []byte) ([]*domain.ModelInfo, error) {
	return nil, nil
}
