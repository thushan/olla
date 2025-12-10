// Package util provides common utilities for the Olla application.
package util

// JoinURLPath concatenates a base URL with a path, handling trailing/leading slashes.
// This uses string concatenation rather than url.ResolveReference() because
// ResolveReference treats paths starting with "/" as absolute references per RFC 3986,
// which replaces the entire path of the base URL instead of appending to it.
// For example: "http://localhost/api/".ResolveReference("/v1/models") = "http://localhost/v1/models"
// But we want: "http://localhost/api/" + "/v1/models" = "http://localhost/api/v1/models"
func JoinURLPath(baseURL, path string) string {
	if baseURL == "" {
		return path
	}
	if path == "" {
		return baseURL
	}

	// Normalise: strip trailing slash from base, strip leading slash from path
	baseHasSlash := baseURL[len(baseURL)-1] == '/'
	pathHasSlash := path[0] == '/'

	if baseHasSlash && pathHasSlash {
		return baseURL + path[1:]
	}
	if !baseHasSlash && !pathHasSlash {
		return baseURL + "/" + path
	}
	return baseURL + path
}
