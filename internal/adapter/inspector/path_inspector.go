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
}

func NewPathInspector(profileFactory *profile.Factory, logger logger.StyledLogger) *PathInspector {
	return &PathInspector{
		profileFactory: profileFactory,
		logger:         logger,
	}
}

func (pi *PathInspector) Name() string {
	return "path"
}

func (pi *PathInspector) Inspect(_ context.Context, _ *http.Request, profile *domain.RequestProfile) error {
	path := profile.Path
	if path == "" {
		return nil
	}

	availableProfiles := pi.profileFactory.GetAvailableProfiles()
	supportedProfiles := make([]string, 0, len(availableProfiles)+1)

	for _, profileName := range availableProfiles {
		platformProfile, err := pi.profileFactory.GetProfile(profileName)
		if err != nil {
			pi.logger.Debug("Failed to get profile", "profile", profileName, "error", err)
			continue
		}

		rules := platformProfile.GetRequestParsingRules()
		if pi.pathMatchesRules(path, rules) {
			supportedProfiles = append(supportedProfiles, profileName)
		}
	}

	openaiProfile, err := pi.profileFactory.GetProfile(domain.ProfileOpenAICompatible)
	if err == nil {
		rules := openaiProfile.GetRequestParsingRules()
		if pi.pathMatchesRules(path, rules) {
			supportedProfiles = append(supportedProfiles, domain.ProfileOpenAICompatible)
		}
	}

	for _, supportedProfile := range supportedProfiles {
		profile.AddSupportedProfile(supportedProfile)
	}

	if len(supportedProfiles) > 0 {
		profile.SetInspectionMeta(domain.InspectionMetaPathSupport, supportedProfiles)
	}

	pi.logger.Debug("Path inspection completed",
		"path", path,
		"supported_profiles", supportedProfiles)

	return nil
}

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
