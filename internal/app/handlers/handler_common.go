package handlers

import (
	"context"
	"net/http"
	"strings"

	"github.com/thushan/olla/internal/core/constants"
)

// NormaliseProviderType converts various provider name formats to canonical form
// Exported for use in other handler files
func NormaliseProviderType(provider string) string {
	switch strings.ToLower(strings.ReplaceAll(provider, "_", "-")) {
	case "lmstudio", "lm-studio":
		return "lm-studio"
	default:
		return strings.ToLower(provider)
	}
}

// extractProviderFromPath extracts provider type from paths like /olla/{provider}/*
// returns provider type and remaining path after provider
func extractProviderFromPath(path string) (provider string, remainingPath string, ok bool) {
	// expected format: /olla/{provider}/...
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

// getOriginalPath retrieves the original request path before modification
func (a *Application) getOriginalPath(ctx context.Context) string {
	if originalPath, ok := ctx.Value(constants.OriginalPathKey).(string); ok {
		return originalPath
	}
	return ""
}

// supportedProviders defines the valid provider types
// TODO: This should be loaded from available profiles at startup
var supportedProviders = map[string]bool{
	"ollama":    true,
	"lm-studio": true,
	"openai":    true,
	"vllm":      true,
}

// isProviderSupported checks if a provider type is valid
func isProviderSupported(provider string) bool {
	// normalize first to handle variations
	normalized := NormaliseProviderType(provider)
	return supportedProviders[normalized]
}

// getProviderPrefix returns the URL prefix for a provider
func getProviderPrefix(provider string) string {
	// use the original provider name in the URL to maintain compatibility
	// (e.g., if user accessed /olla/lmstudio/, keep that in the prefix)
	return constants.DefaultOllaProxyPathPrefix + provider
}
