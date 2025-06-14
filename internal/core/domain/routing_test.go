package domain

import (
	"fmt"
	"testing"
)

func TestNewRequestProfile(t *testing.T) {
	path := "/v1/chat/completions"
	profile := NewRequestProfile(path)

	if profile == nil {
		t.Fatal("NewRequestProfile() should not return nil")
	}

	if profile.Path != path {
		t.Errorf("NewRequestProfile() path = %v, want %v", profile.Path, path)
	}

	if profile.SupportedBy == nil {
		t.Error("NewRequestProfile() should initialize SupportedBy slice")
	}

	if len(profile.SupportedBy) != 0 {
		t.Errorf("NewRequestProfile() should start with empty SupportedBy, got %v", profile.SupportedBy)
	}

	if profile.ModelName != "" {
		t.Errorf("NewRequestProfile() should start with empty ModelName, got %v", profile.ModelName)
	}

	if profile.InspectionMeta == nil {
		t.Error("NewRequestProfile() should initialize InspectionMeta map")
	}

	// test that the xsync map is functional
	profile.InspectionMeta.Store("test", "value")
	value, exists := profile.InspectionMeta.Load("test")
	if !exists || value != "value" {
		t.Error("NewRequestProfile() InspectionMeta should be functional")
	}
}

func TestRequestProfile_AddSupportedProfile(t *testing.T) {
	profile := NewRequestProfile("/test")

	// fist profile
	profile.AddSupportedProfile(ProfileOllama)
	if len(profile.SupportedBy) != 1 {
		t.Errorf("AddSupportedProfile() should add profile, got length %d", len(profile.SupportedBy))
	}
	if profile.SupportedBy[0] != ProfileOllama {
		t.Errorf("AddSupportedProfile() should add correct profile, got %v", profile.SupportedBy[0])
	}

	// another
	profile.AddSupportedProfile(ProfileLmStudio)
	if len(profile.SupportedBy) != 2 {
		t.Errorf("AddSupportedProfile() should add second profile, got length %d", len(profile.SupportedBy))
	}
	if !contains(profile.SupportedBy, ProfileLmStudio) {
		t.Errorf("AddSupportedProfile() should include LmStudio, got %v", profile.SupportedBy)
	}

	// whats better than one Ollama? Two!
	profile.AddSupportedProfile(ProfileOllama)
	if len(profile.SupportedBy) != 2 {
		t.Errorf("AddSupportedProfile() should not duplicate profiles, got length %d", len(profile.SupportedBy))
	}

	// empty one that should be ignored
	originalLength := len(profile.SupportedBy)
	profile.AddSupportedProfile("")
	if len(profile.SupportedBy) != originalLength {
		t.Error("AddSupportedProfile() should ignore empty profile names")
	}
}

func TestRequestProfile_IsCompatibleWith_AutoType(t *testing.T) {
	profile := NewRequestProfile("/test")

	// auto should be compatible
	if !profile.IsCompatibleWith(ProfileAuto) {
		t.Error("IsCompatibleWith() should always accept auto type")
	}

	// and with existing profiles
	profile.AddSupportedProfile(ProfileOllama)
	if !profile.IsCompatibleWith(ProfileAuto) {
		t.Error("IsCompatibleWith() should always accept auto type even with specific profiles")
	}
}

func TestRequestProfile_IsCompatibleWith_EmptySupported(t *testing.T) {
	profile := NewRequestProfile("/test")

	testTypes := []string{ProfileOllama, ProfileLmStudio, ProfileOpenAICompatible, "custom-type"}

	for _, profileType := range testTypes {
		if !profile.IsCompatibleWith(profileType) {
			t.Errorf("IsCompatibleWith() with empty supported list should accept %v", profileType)
		}
	}
}

func TestRequestProfile_IsCompatibleWith_ExactMatch(t *testing.T) {
	profile := NewRequestProfile("/test")
	profile.AddSupportedProfile(ProfileOllama)
	profile.AddSupportedProfile(ProfileLmStudio)

	if !profile.IsCompatibleWith(ProfileOllama) {
		t.Error("IsCompatibleWith() should match exact profile Ollama")
	}

	if !profile.IsCompatibleWith(ProfileLmStudio) {
		t.Error("IsCompatibleWith() should match exact profile LmStudio")
	}

	if profile.IsCompatibleWith("unsupported-profile") {
		t.Error("IsCompatibleWith() should not match unsupported profile")
	}
}

func TestRequestProfile_IsCompatibleWith_OpenAICompatibility(t *testing.T) {
	profile := NewRequestProfile("/test")
	profile.AddSupportedProfile(ProfileOpenAICompatible)

	// openai should be compatible with Ollama and LMStudio
	if !profile.IsCompatibleWith(ProfileOllama) {
		t.Error("IsCompatibleWith() OpenAI compatible should work with Ollama")
	}

	if !profile.IsCompatibleWith(ProfileLmStudio) {
		t.Error("IsCompatibleWith() OpenAI compatible should work with LMStudio")
	}

	if !profile.IsCompatibleWith(ProfileOpenAICompatible) {
		t.Error("IsCompatibleWith() should match OpenAI compatible exactly")
	}

	// but not with unsupported types
	if profile.IsCompatibleWith("unknown-type") {
		t.Error("IsCompatibleWith() OpenAI compatible should not work with unknown types")
	}
}

func TestRequestProfile_IsCompatibleWith_SpecificPlatforms(t *testing.T) {
	ollamaProfile := NewRequestProfile("/api/generate")
	ollamaProfile.AddSupportedProfile(ProfileOllama)

	if !ollamaProfile.IsCompatibleWith(ProfileOllama) {
		t.Error("IsCompatibleWith() Ollama-specific should work with Ollama")
	}

	if ollamaProfile.IsCompatibleWith(ProfileLmStudio) {
		t.Error("IsCompatibleWith() Ollama-specific should not work with LMStudio")
	}

	if ollamaProfile.IsCompatibleWith(ProfileOpenAICompatible) {
		t.Error("IsCompatibleWith() Ollama-specific should not work with OpenAI compatible")
	}
}

func TestRequestProfile_IsCompatibleWith_MultipleProfiles(t *testing.T) {
	profile := NewRequestProfile("/v1/chat/completions")
	profile.AddSupportedProfile(ProfileOllama)
	profile.AddSupportedProfile(ProfileLmStudio)
	profile.AddSupportedProfile(ProfileOpenAICompatible)

	supportedTypes := []string{ProfileOllama, ProfileLmStudio, ProfileOpenAICompatible}
	for _, profileType := range supportedTypes {
		if !profile.IsCompatibleWith(profileType) {
			t.Errorf("IsCompatibleWith() should work with supported profile %v", profileType)
		}
	}

	if profile.IsCompatibleWith("custom-platform") {
		t.Error("IsCompatibleWith() should not work with unsupported custom platform")
	}
}

func TestRequestProfile_SetInspectionMeta(t *testing.T) {
	profile := NewRequestProfile("/test")

	profile.SetInspectionMeta("key1", "value1")
	profile.SetInspectionMeta("key2", 42)
	profile.SetInspectionMeta("key3", []string{"a", "b", "c"})

	value1, exists1 := profile.InspectionMeta.Load("key1")
	if !exists1 || value1 != "value1" {
		t.Errorf("SetInspectionMeta() key1 = %v, want 'value1'", value1)
	}

	value2, exists2 := profile.InspectionMeta.Load("key2")
	if !exists2 || value2 != 42 {
		t.Errorf("SetInspectionMeta() key2 = %v, want 42", value2)
	}

	value3, exists3 := profile.InspectionMeta.Load("key3")
	if !exists3 {
		t.Error("SetInspectionMeta() key3 should exist")
	}
	if slice, ok := value3.([]string); !ok || len(slice) != 3 {
		t.Errorf("SetInspectionMeta() key3 should be string slice of length 3, got %v", value3)
	}

	profile.SetInspectionMeta("key1", "new-value")
	newValue1, _ := profile.InspectionMeta.Load("key1")
	if newValue1 != "new-value" {
		t.Errorf("SetInspectionMeta() should overwrite existing key, got %v", newValue1)
	}
}

func TestRequestProfile_ThreadSafety(t *testing.T) {
	profile := NewRequestProfile("/test")

	// testing concurrent access to metadata (xsync.Map should handle this)
	done := make(chan bool, 10)

	for i := 0; i < 5; i++ {
		go func(id int) {
			for j := 0; j < 100; j++ {
				key := fmt.Sprintf("key_%d_%d", id, j)
				value := fmt.Sprintf("value_%d_%d", id, j)
				profile.SetInspectionMeta(key, value)
			}
			done <- true
		}(i)
	}

	for i := 0; i < 5; i++ {
		go func(id int) {
			for j := 0; j < 100; j++ {
				key := fmt.Sprintf("key_%d_%d", id%5, j)
				profile.InspectionMeta.Load(key)
			}
			done <- true
		}(i)
	}

	for i := 0; i < 10; i++ {
		<-done
	}

	value, exists := profile.InspectionMeta.Load("key_0_0")
	if !exists {
		t.Error("ThreadSafety test should have stored concurrent values")
	}
	if value != "value_0_0" {
		t.Errorf("ThreadSafety test value should be correct, got %v", value)
	}
}

func TestRequestProfile_RealWorldScenarios(t *testing.T) {
	scenarios := []struct {
		name          string
		path          string
		supportedBy   []string
		testEndpoints []string
		expectedMatch map[string]bool
	}{
		{
			name:          "chat completions - universal support",
			path:          "/v1/chat/completions",
			supportedBy:   []string{ProfileOllama, ProfileLmStudio, ProfileOpenAICompatible},
			testEndpoints: []string{ProfileOllama, ProfileLmStudio, ProfileOpenAICompatible, "custom"},
			expectedMatch: map[string]bool{
				ProfileOllama:           true,
				ProfileLmStudio:         true,
				ProfileOpenAICompatible: true,
				"custom":                false,
			},
		},
		{
			name:          "ollama generate - platform specific",
			path:          "/api/generate",
			supportedBy:   []string{ProfileOllama},
			testEndpoints: []string{ProfileOllama, ProfileLmStudio, ProfileOpenAICompatible},
			expectedMatch: map[string]bool{
				ProfileOllama:           true,
				ProfileLmStudio:         false,
				ProfileOpenAICompatible: false,
			},
		},
		{
			name:          "openai only - compatibility layer",
			path:          "/v1/custom/endpoint",
			supportedBy:   []string{ProfileOpenAICompatible},
			testEndpoints: []string{ProfileOllama, ProfileLmStudio, ProfileOpenAICompatible},
			expectedMatch: map[string]bool{
				ProfileOllama:           true, // OpenAI = Ollama
				ProfileLmStudio:         true, // OpenAI = LMStudio
				ProfileOpenAICompatible: true,
			},
		},
		{
			name:          "unknown path - no filtering",
			path:          "/unknown/api",
			supportedBy:   []string{}, // Empty - should accept all
			testEndpoints: []string{ProfileOllama, ProfileLmStudio, ProfileOpenAICompatible, "custom"},
			expectedMatch: map[string]bool{
				ProfileOllama:           true,
				ProfileLmStudio:         true,
				ProfileOpenAICompatible: true,
				"custom":                true,
			},
		},
	}

	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			profile := NewRequestProfile(scenario.path)

			for _, supported := range scenario.supportedBy {
				profile.AddSupportedProfile(supported)
			}

			for _, endpoint := range scenario.testEndpoints {
				expected := scenario.expectedMatch[endpoint]
				actual := profile.IsCompatibleWith(endpoint)

				if actual != expected {
					t.Errorf("Scenario %s: endpoint %s compatibility = %v, want %v",
						scenario.name, endpoint, actual, expected)
				}
			}

			if !profile.IsCompatibleWith(ProfileAuto) {
				t.Errorf("Scenario %s: auto type should always be compatible", scenario.name)
			}
		})
	}
}

func TestRequestProfile_EdgeCases(t *testing.T) {
	t.Run("nil profile operations", func(t *testing.T) {
		// testing panics
		var profile *RequestProfile

		// should not panic
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("Operations on nil profile should not panic: %v", r)
			}
		}()

		if profile != nil {
			profile.IsCompatibleWith(ProfileOllama)
		}
	})

	t.Run("empty path", func(t *testing.T) {
		profile := NewRequestProfile("")
		profile.AddSupportedProfile(ProfileOllama)

		if !profile.IsCompatibleWith(ProfileOllama) {
			t.Error("Empty path should not affect compatibility logic")
		}
	})

	t.Run("special characters in path", func(t *testing.T) {
		specialPath := "/api/café/models?param=value&other=test"
		profile := NewRequestProfile(specialPath)

		if profile.Path != specialPath {
			t.Error("Special characters in path should be preserved")
		}
	})
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
