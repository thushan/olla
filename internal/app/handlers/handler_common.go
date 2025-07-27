package handlers

import (
	"context"
	"net/http"
	"strings"

	"github.com/thushan/olla/internal/util"
)

// stripPathPrefix removes the route prefix from request path.
// uses context-aware stripping if a route prefix is stored in context,
// otherwise falls back to simple prefix removal
func (a *Application) stripPathPrefix(ctx context.Context, r *http.Request, defaultPrefix string) string {
	// check if we have a route prefix in context (set by router)
	if routePrefix, ok := ctx.Value(defaultPrefix).(string); ok {
		return util.StripPrefix(r.URL.Path, routePrefix)
	}

	// fallback to simple prefix stripping
	return util.StripPrefix(r.URL.Path, defaultPrefix)
}

// extractProviderFromPath extracts provider type from paths like /olla/{provider}/*
// returns provider type and remaining path after provider
func extractProviderFromPath(path string) (provider string, remainingPath string, ok bool) {
	// expected format: /olla/{provider}/...
	if !strings.HasPrefix(path, "/olla/") {
		return "", "", false
	}

	// remove /olla/ prefix
	withoutPrefix := strings.TrimPrefix(path, "/olla/")

	// find next slash to extract provider
	slashIdx := strings.Index(withoutPrefix, "/")
	if slashIdx == -1 {
		// path like /olla/ollama with no trailing part
		return withoutPrefix, "/", true
	}

	provider = withoutPrefix[:slashIdx]
	remainingPath = withoutPrefix[slashIdx:]

	return provider, remainingPath, true
}

// modifyRequestPath updates the request path for backend routing.
// preserves the original path in context for logging/debugging
func (a *Application) modifyRequestPath(r *http.Request, newPath string) *http.Request {
	// store original path for reference
	ctx := context.WithValue(r.Context(), "original_path", r.URL.Path)

	// update path
	r.URL.Path = newPath
	if r.URL.RawPath != "" {
		r.URL.RawPath = newPath
	}

	return r.WithContext(ctx)
}

// getOriginalPath retrieves the original request path before modification
func (a *Application) getOriginalPath(ctx context.Context) string {
	if originalPath, ok := ctx.Value("original_path").(string); ok {
		return originalPath
	}
	return ""
}

// isProviderSupported checks if a provider type is valid
func isProviderSupported(provider string) bool {
	switch provider {
	case "ollama", "lmstudio", "openai", "vllm":
		return true
	default:
		return false
	}
}

// getProviderPrefix returns the URL prefix for a provider
func getProviderPrefix(provider string) string {
	return "/olla/" + provider
}

// these common functions reduce duplication across proxy handlers
// and ensure consistent path handling throughout the application
