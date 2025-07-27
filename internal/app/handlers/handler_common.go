package handlers

import (
	"context"
	"net/http"
	"strings"

	"github.com/thushan/olla/internal/core/constants"
)

// NormaliseProviderType handles the various ways users might specify a provider
// (e.g., lmstudio, lm-studio, lm_studio all map to "lm-studio").
// This ensures consistent internal representation regardless of user input.
func NormaliseProviderType(provider string) string {
	normalised := strings.ToLower(strings.ReplaceAll(provider, "_", "-"))

	// LM Studio is special - it has multiple common variations that all need
	// to map to the canonical form for profile lookups to work correctly
	switch normalised {
	case constants.ProviderPrefixLMStudio1, constants.ProviderPrefixLMStudio2:
		return constants.ProviderTypeLMStudio
	default:
		return normalised
	}
}

// extractProviderFromPath parses URLs like /olla/ollama/api/chat to extract
// the provider type and remaining path for backend routing.
func extractProviderFromPath(path string) (provider string, remainingPath string, ok bool) {
	if !strings.HasPrefix(path, constants.DefaultOllaProxyPathPrefix) {
		return "", "", false
	}

	// remove /olla/ prefix
	withoutPrefix := strings.TrimPrefix(path, constants.DefaultOllaProxyPathPrefix)

	// find next slash to extract provider
	slashIdx := strings.Index(withoutPrefix, constants.DefaultPathPrefix)
	if slashIdx == -1 {
		// path like /olla/ollama with no trailing part
		return withoutPrefix, constants.DefaultPathPrefix, true
	}

	provider = withoutPrefix[:slashIdx]
	remainingPath = withoutPrefix[slashIdx:]

	// normalise the provider name to canonical form
	provider = NormaliseProviderType(provider)

	return provider, remainingPath, true
}

// modifyRequestPath updates the request path for backend routing.
// preserves the original path in context for logging/debugging
func (a *Application) modifyRequestPath(r *http.Request, newPath string) *http.Request {
	// store original path for reference
	ctx := context.WithValue(r.Context(), constants.OriginalPathKey, r.URL.Path)

	// update path
	r.URL.Path = newPath
	if r.URL.RawPath != "" {
		r.URL.RawPath = newPath
	}

	return r.WithContext(ctx)
}

// isProviderSupported checks if a provider type is valid using the profile factory
func (a *Application) isProviderSupported(provider string) bool {
	// normalise first to handle variations
	normalised := NormaliseProviderType(provider)

	// use profile factory to validate if this is a known provider type
	if a.profileFactory != nil {
		return a.profileFactory.ValidateProfileType(normalised)
	}

	// fallback for tests when profile factory is not available
	// check against the static provider list used in registerStaticProviderRoutes
	// this ensures consistency between validation and route registration
	staticProviders := map[string]bool{
		constants.ProviderTypeOllama:   true,
		constants.ProviderTypeLMStudio: true,
		constants.ProviderTypeOpenAI:   true,
		constants.ProviderTypeVLLM:     true,
	}
	return staticProviders[normalised]
}

// getProviderPrefix returns the URL prefix for a provider
func getProviderPrefix(provider string) string {
	// use the original provider name in the URL to maintain compatibility
	// (e.g., if user accessed /olla/lmstudio/, keep that in the prefix)
	return constants.DefaultOllaProxyPathPrefix + provider
}
