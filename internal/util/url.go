package util

import (
	"net/url"
	"path"
)

// ResolveURLPath resolves a path or absolute URL against a base URL.
// If pathOrURL is already an absolute URL (has a scheme like http://), it is returned as-is.
// Otherwise, pathOrURL is treated as a relative path and joined with the base URL's path,
// preserving any path prefix in the base URL.
//
// This function uses url.Parse() and path.Join() from the standard library to handle
// URL resolution correctly, avoiding the pitfalls of url.ResolveReference() which treats
// paths starting with "/" as absolute references per RFC 3986 (replacing the entire path).
//
// Examples:
//   - ResolveURLPath("http://localhost:12434/api/", "/v1/models") -> "http://localhost:12434/api/v1/models"
//   - ResolveURLPath("http://localhost:12434/api/", "http://other:9000/models") -> "http://other:9000/models"
func ResolveURLPath(baseURL, pathOrURL string) string {
	if baseURL == "" {
		return pathOrURL
	}
	if pathOrURL == "" {
		return baseURL
	}

	// Check if pathOrURL is already an absolute URL
	if parsed, err := url.Parse(pathOrURL); err == nil && parsed.IsAbs() {
		return pathOrURL
	}

	// Parse base URL
	base, err := url.Parse(baseURL)
	if err != nil {
		// If base URL is invalid, return pathOrURL as fallback
		return pathOrURL
	}

	// Use path.Join to preserve the base path prefix when joining with relative paths
	// and to normalise redundant slashes
	base.Path = path.Join(base.Path, pathOrURL)
	return base.String()
}
