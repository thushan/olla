package profile

import (
	"fmt"
	"sort"
	"sync"

	"github.com/thushan/olla/internal/core/domain"
)

type ProfileFactory interface {
	GetProfile(profileType string) (domain.InferenceProfile, error)
	GetAvailableProfiles() []string
	ReloadProfiles() error
	ValidateProfileType(platformType string) bool
}

type Factory struct {
	loader       *ProfileLoader
	prefixLookup map[string]string // maps URL prefix to profile name
	mu           sync.RWMutex
}

// NewFactory expects a profiles directory path. This breaks the old API
// but makes configuration explicit and testable.
func NewFactory(profilesDir string) (*Factory, error) {
	loader := NewProfileLoader(profilesDir)

	if err := loader.LoadProfiles(); err != nil {
		return nil, fmt.Errorf("failed to load profiles: %w", err)
	}

	factory := &Factory{
		loader:       loader,
		prefixLookup: make(map[string]string),
	}

	// Build prefix lookup table
	factory.buildPrefixLookup()

	return factory, nil
}

func NewFactoryWithDefaults() (*Factory, error) {
	return NewFactory("./config/profiles")
}

func (f *Factory) GetProfile(platformType string) (domain.InferenceProfile, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	profile, exists := f.loader.GetProfile(platformType)
	if !exists {
		// openai profile works as a decent fallback for unknowns
		if openai, hasOpenAI := f.loader.GetProfile(domain.ProfileOpenAICompatible); hasOpenAI {
			return openai, nil
		}
		return nil, fmt.Errorf("profile not found for platform type: %s", platformType)
	}

	return profile, nil
}

// GetPlatformProfile provides backward compatibility for code expecting PlatformProfile
func (f *Factory) GetPlatformProfile(platformType string) (domain.PlatformProfile, error) {
	profile, err := f.GetProfile(platformType)
	if err != nil {
		return nil, err
	}
	// InferenceProfile embeds PlatformProfile, so this cast is safe
	return profile, nil
}

func (f *Factory) GetAvailableProfiles() []string {
	f.mu.RLock()
	defer f.mu.RUnlock()

	profiles := f.loader.GetAllProfiles()
	names := make([]string, 0, len(profiles))
	for name := range profiles {
		if name != domain.ProfileOpenAICompatible {
			names = append(names, name)
		}
	}

	sort.Strings(names)
	return names
}

// ReloadProfiles allows hot-reloading of profile configurations
func (f *Factory) ReloadProfiles() error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if err := f.loader.LoadProfiles(); err != nil {
		return err
	}

	// Rebuild prefix lookup table
	f.prefixLookup = make(map[string]string)
	f.buildPrefixLookup()

	return nil
}

func (f *Factory) ValidateProfileType(platformType string) bool {
	if platformType == domain.ProfileAuto {
		return true
	}

	f.mu.RLock()
	defer f.mu.RUnlock()

	// First check if it's a known prefix
	if profileName, ok := f.prefixLookup[platformType]; ok {
		_, exists := f.loader.GetProfile(profileName)
		return exists
	}

	// Then check if it's a direct profile name
	_, exists := f.loader.GetProfile(platformType)
	return exists
}

// buildPrefixLookup builds a map from URL prefixes to profile names
func (f *Factory) buildPrefixLookup() {
	profiles := f.loader.GetAllProfiles()

	for profileName, profile := range profiles {
		config := profile.GetConfig()
		if config == nil {
			continue
		}

		// Add each routing prefix to the lookup table
		for _, prefix := range config.Routing.Prefixes {
			f.prefixLookup[prefix] = profileName
		}

		// Also add the profile name itself as a valid prefix
		f.prefixLookup[profileName] = profileName
	}
}
