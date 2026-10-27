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

// pathCapability sets the capability a request path implies when its body
// cannot: TEI's native /rerank, /predict and /embed bodies carry no model field,
// so without this a model-less request lands on whichever instance the balancer
// picks, and TEI answers 424 when that instance serves the wrong kind.
type pathCapability struct {
	apply  func(*domain.ModelCapabilities)
	suffix string
}

// Matching rules (see pathMatches): the single-segment names TEI serves
// (/rerank, /predict, /embed...) only mean those things as the whole path, so
// they must not claim /api/rerank or Ollama's /api/embed. The versioned
// vLLM-style variants are listed in full so they keep matching behind a prefix.
var pathCapabilities = []pathCapability{
	{suffix: "/rerank", apply: func(c *domain.ModelCapabilities) { c.Rerank = true }},
	{suffix: "/v1/rerank", apply: func(c *domain.ModelCapabilities) { c.Rerank = true }},
	{suffix: "/v2/rerank", apply: func(c *domain.ModelCapabilities) { c.Rerank = true }},
	// OpenAI-style embeddings. The body's `input` field is the primary signal, but it is only
	// readable when the body is; the path keeps embeddings requests off chat-only hosts either way.
	{suffix: "/v1/embeddings", apply: func(c *domain.ModelCapabilities) { c.Embeddings = true }},
	{suffix: "/predict", apply: func(c *domain.ModelCapabilities) { c.Classification = true }},
	{suffix: "/embed", apply: func(c *domain.ModelCapabilities) { c.Embeddings = true }},
	{suffix: "/embed_all", apply: func(c *domain.ModelCapabilities) { c.Embeddings = true }},
	{suffix: "/embed_sparse", apply: func(c *domain.ModelCapabilities) { c.Embeddings = true }},
	{suffix: "/similarity", apply: func(c *domain.ModelCapabilities) { c.Embeddings = true }},
}

// pathMatches reports whether a request path belongs to a profile path. They
// match when equal, or when the request path carries extra leading segments (a
// mount prefix the route did not strip, such as the translators' /olla/anthropic)
// and the profile path has two or more segments. A single segment such as
// /embed, /health or /version is too common a name to claim every deeper path
// that happens to end with it. A suffix always starts at a '/', so it can only
// begin on a segment boundary.
func pathMatches(requestPath, profilePath string) bool {
	if requestPath == profilePath {
		return true
	}
	return strings.Count(profilePath, "/") >= 2 && strings.HasPrefix(profilePath, "/") && strings.HasSuffix(requestPath, profilePath)
}

// capabilitiesForPath returns the capabilities a path implies, or nil.
func capabilitiesForPath(path string) *domain.ModelCapabilities {
	var caps *domain.ModelCapabilities
	for _, pc := range pathCapabilities {
		if pathMatches(path, pc.suffix) {
			if caps == nil {
				caps = &domain.ModelCapabilities{}
			}
			pc.apply(caps)
		}
	}
	return caps
}

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

	if caps := capabilitiesForPath(path); caps != nil {
		profile.ModelCapabilities = caps
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

	// No exact match: try profile paths behind a mount prefix (see pathMatches)
	var supportedProfiles []string
	for mapPath, profiles := range pi.pathToProfiles {
		if pathMatches(path, mapPath) {
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
