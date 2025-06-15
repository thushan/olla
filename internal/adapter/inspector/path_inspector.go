package inspector

import (
	"context"
	"net/http"
	"strings"

	"github.com/thushan/olla/internal/adapter/registry/profile"
	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/logger"
)

type PathInspector struct {
	profileFactory *profile.Factory
	logger         logger.StyledLogger
	pathToProfiles map[string][]string
}

const (
	PathInspectorName = "path"
)

func NewPathInspector(profileFactory *profile.Factory, logger logger.StyledLogger) *PathInspector {
	inspector := &PathInspector{
		profileFactory: profileFactory,
		logger:         logger,
		pathToProfiles: make(map[string][]string),
	}

	inspector.buildPathMap()
	return inspector
}

// buildPathMap builds a map of paths to profiles for fast lookups.
func (pi *PathInspector) buildPathMap() {
	if pi.profileFactory == nil {
		pi.logger.Debug("Profile factory is nil, skipping path map building")
		return
	}

	availableProfiles := pi.profileFactory.GetAvailableProfiles()

	for _, profileName := range availableProfiles {
		platformProfile, err := pi.profileFactory.GetProfile(profileName)
		if err != nil {
			pi.logger.Debug("Failed to get profile", "profile", profileName, "error", err)
			continue
		}

		paths := platformProfile.GetPaths()
		for _, path := range paths {
			if path == "" {
				continue
			}

			if _, exists := pi.pathToProfiles[path]; !exists {
				pi.pathToProfiles[path] = make([]string, 0, 3) // Most paths will be supported by 1-3 profiles
			}
			pi.pathToProfiles[path] = append(pi.pathToProfiles[path], profileName)
		}
	}

	// Also add OpenAI compatible profile paths
	openaiProfile, err := pi.profileFactory.GetProfile(domain.ProfileOpenAICompatible)
	if err == nil {
		paths := openaiProfile.GetPaths()
		for _, path := range paths {
			if path == "" {
				continue
			}

			if _, exists := pi.pathToProfiles[path]; !exists {
				pi.pathToProfiles[path] = make([]string, 0, 1)
			}
			pi.pathToProfiles[path] = append(pi.pathToProfiles[path], domain.ProfileOpenAICompatible)
		}
	}

	pi.logger.Info("Built path map for fast lookups", 
		"path_count", len(pi.pathToProfiles),
		"profile_count", len(availableProfiles))
}

func (pi *PathInspector) Name() string {
	return PathInspectorName
}

// pathMatchesRules checks if a path matches any of the rules in RequestParsingRules.
// This method is kept for testing purposes only and is not used in the actual implementation.
func (pi *PathInspector) pathMatchesRules(path string, rules domain.RequestParsingRules) bool {
	if rules.ChatCompletionsPath != "" && strings.HasSuffix(path, rules.ChatCompletionsPath) {
		return true
	}
	if rules.CompletionsPath != "" && strings.HasSuffix(path, rules.CompletionsPath) {
		return true
	}
	if rules.GeneratePath != "" && strings.HasSuffix(path, rules.GeneratePath) {
		return true
	}
	return false
}

func (pi *PathInspector) Inspect(_ context.Context, _ *http.Request, profile *domain.RequestProfile) error {
	path := profile.Path
	if path == "" {
		return nil
	}

	// First try exact match
	if supportedProfiles, exists := pi.pathToProfiles[path]; exists {
		for _, supportedProfile := range supportedProfiles {
			profile.AddSupportedProfile(supportedProfile)
		}
		profile.SetInspectionMeta(domain.InspectionMetaPathSupport, supportedProfiles)

		pi.logger.Debug("Path inspection completed (exact match)",
			"path", path,
			"supported_profiles", supportedProfiles)

		return nil
	}

	// If no exact match, try suffix match (for paths with prefixes)
	var supportedProfiles []string
	for mapPath, profiles := range pi.pathToProfiles {
		if strings.HasSuffix(path, mapPath) {
			for _, profileName := range profiles {
				// Check if this profile is already in the supported list
				alreadySupported := false
				for _, existing := range profile.SupportedBy {
					if existing == profileName {
						alreadySupported = true
						break
					}
				}

				if !alreadySupported {
					profile.AddSupportedProfile(profileName)
					supportedProfiles = append(supportedProfiles, profileName)
				}
			}
		}
	}

	if len(supportedProfiles) > 0 {
		profile.SetInspectionMeta(domain.InspectionMetaPathSupport, supportedProfiles)
	}

	pi.logger.Debug("Path inspection completed (suffix match)",
		"path", path,
		"supported_profiles", supportedProfiles)

	return nil
}
