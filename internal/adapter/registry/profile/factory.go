package profile

import (
	"fmt"
	"github.com/thushan/olla/internal/core/domain"
	"sort"
	"sync"
)

type Factory struct {
	profiles map[string]domain.PlatformProfile
	mu       sync.RWMutex
}

func NewFactory() *Factory {
	factory := &Factory{
		profiles: make(map[string]domain.PlatformProfile),
	}

	factory.RegisterProfile(NewOllamaProfile())
	factory.RegisterProfile(NewLMStudioProfile())
	factory.RegisterProfile(NewOpenAICompatibleProfile())

	return factory
}

func (f *Factory) RegisterProfile(profile domain.PlatformProfile) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.profiles[profile.GetName()] = profile
}

func (f *Factory) GetProfile(platformType string) (domain.PlatformProfile, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	profile, exists := f.profiles[platformType]
	if !exists {
		// Return OpenAI compatible profile as fallback
		if openai, hasOpenAI := f.profiles[domain.ProfileOpenAICompatible]; hasOpenAI {
			return openai, nil
		}
		return nil, fmt.Errorf("profile not found for platform type: %s", platformType)
	}

	return profile, nil
}

func (f *Factory) GetAvailableProfiles() []string {
	f.mu.RLock()
	defer f.mu.RUnlock()

	profiles := make([]string, 0, len(f.profiles))
	for name := range f.profiles {
		if name != domain.ProfileOpenAICompatible {
			profiles = append(profiles, name)
		}
	}

	sort.Strings(profiles)
	return profiles
}

func (f *Factory) ValidateProfileType(platformType string) bool {
	if platformType == domain.ProfileAuto {
		return true
	}

	f.mu.RLock()
	defer f.mu.RUnlock()
	_, exists := f.profiles[platformType]
	return exists
}
