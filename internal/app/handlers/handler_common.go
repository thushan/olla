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
	ctx := context.WithValue(r.Context(), constants.ContextOriginalPathKey, r.URL.Path)

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
		constants.ProviderTypeLemonade: true,
		constants.ProviderTypeLMDeploy: true,
		constants.ProviderTypeLMStudio: true,
		constants.ProviderTypeOllama:   true,
		constants.ProviderTypeOpenAI:   true,
		constants.ProviderTypeSGLang:   true,
		constants.ProviderTypeVLLM:     true,
	}
	return staticProviders[normalised]
}

// getProviderPrefix returns the canonical /olla/<provider> prefix (no trailing slash) for strip-and-forward routing.
func getProviderPrefix(provider string) string {
	return constants.DefaultOllaProxyPathPrefix + provider
}

// getRawProviderPrefix extracts the /olla/<provider> prefix (no trailing slash) from the incoming request path.
// Unlike getProviderPrefix, this preserves the original spelling used by the caller
// (e.g., /olla/lmstudio rather than /olla/lm-studio) so that path stripping works
// even when the caller uses an alias spelling.
func getRawProviderPrefix(path string) string {
	if !strings.HasPrefix(path, constants.DefaultOllaProxyPathPrefix) {
		return constants.DefaultOllaProxyPathPrefix
	}
	withoutBase := strings.TrimPrefix(path, constants.DefaultOllaProxyPathPrefix)
	slashIdx := strings.Index(withoutBase, constants.DefaultPathPrefix)
	if slashIdx == -1 {
		return constants.DefaultOllaProxyPathPrefix + withoutBase
	}
	return constants.DefaultOllaProxyPathPrefix + withoutBase[:slashIdx]
}
