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
	NormalizeProviderName(providerName string) string
}

type Factory struct {
	loader       *ProfileLoader
	prefixLookup map[string]string // maps URL prefix to profile name
	mu           sync.RWMutex
}

// NewFactory loads provider profiles from the specified directory.
// The explicit path requirement enables better testing and deployment flexibility.
func NewFactory(profilesDir string) (*Factory, error) {
	loader := NewProfileLoader(profilesDir)

	if err := loader.LoadProfiles(); err != nil {
		return nil, fmt.Errorf("failed to load profiles: %w", err)
	}

	factory := &Factory{
		loader:       loader,
		prefixLookup: make(map[string]string),
	}

	// Pre-compute prefix mappings for O(1) provider resolution
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
		// Unknown providers might still speak OpenAI's protocol
		if openai, hasOpenAI := f.loader.GetProfile(domain.ProfileOpenAICompatible); hasOpenAI {
			return openai, nil
		}
		return nil, fmt.Errorf("profile not found for platform type: %s", platformType)
	}

	return profile, nil
}

// GetPlatformProfile maintains API compatibility during the profile system migration
func (f *Factory) GetPlatformProfile(platformType string) (domain.PlatformProfile, error) {
	profile, err := f.GetProfile(platformType)
	if err != nil {
		return nil, err
	}
	// Safe downcast - InferenceProfile extends PlatformProfile
	return profile, nil
}

func (f *Factory) GetAvailableProfiles() []string {
	f.mu.RLock()
	defer f.mu.RUnlock()

	profiles := f.loader.GetAllProfiles()
	names := make([]string, 0, len(profiles))
	for name := range profiles {
		// openai-compatible isn't a real provider - it's a protocol specification
		if name != domain.ProfileOpenAICompatible {
			names = append(names, name)
		}
	}

	sort.Strings(names)
	return names
}

// ReloadProfiles refreshes provider configs without restarting.
// Useful for testing and dynamic provider updates.
func (f *Factory) ReloadProfiles() error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if err := f.loader.LoadProfiles(); err != nil {
		return err
	}

	// Invalidate and rebuild the prefix cache
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

	// Try prefix lookup first - handles lmstudio/lm-studio/lm_studio variations
	if profileName, ok := f.prefixLookup[platformType]; ok {
		_, exists := f.loader.GetProfile(profileName)
		return exists
	}

	// Fall back to exact profile name match
	_, exists := f.loader.GetProfile(platformType)
	return exists
}

// NormalizeProviderName resolves provider aliases to canonical names.
// Essential for handling user-provided variations like lmstudio vs lm-studio.
func (f *Factory) NormalizeProviderName(providerName string) string {
	f.mu.RLock()
	defer f.mu.RUnlock()

	// Prefix lookup handles all the variant spellings
	if profileName, ok := f.prefixLookup[providerName]; ok {
		return profileName
	}

	// Unknown names pass through - might be valid profile names
	return providerName
}

// buildPrefixLookup creates the routing table from YAML configurations.
// Called on factory creation and after profile reloads.
func (f *Factory) buildPrefixLookup() {
	profiles := f.loader.GetAllProfiles()

	for profileName, profile := range profiles {
		config := profile.GetConfig()
		if config == nil {
			continue
		}

		// Each prefix in the YAML becomes a valid route
		for _, prefix := range config.Routing.Prefixes {
			f.prefixLookup[prefix] = profileName
		}

		// Profile names are implicit prefixes for convenience
		f.prefixLookup[profileName] = profileName
	}
}

// GetLoader returns the profile loader for testing purposes
func (f *Factory) GetLoader() *ProfileLoader {
	return f.loader
}
