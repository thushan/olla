package profile

import (
	"testing"

	"github.com/thushan/olla/internal/core/domain"
)

func TestNewFactory(t *testing.T) {
	factory := NewFactoryLegacy()

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
	factory := NewFactoryLegacy()

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
	factory := NewFactoryLegacy()

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
		factory := NewFactoryLegacy()

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
	factory := NewFactoryLegacy()

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
