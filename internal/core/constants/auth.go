// Package constants defines auth scheme and header constants used across
// the outbound request pipeline.
package constants

// Auth type identifiers. These are the valid values for endpoint auth.type in config.
const (
	AuthTypeBearer = "bearer"
	AuthTypeAPIKey = "api_key"
	AuthTypeBasic  = "basic"
)

// HTTP header names used when injecting auth onto outbound requests.
const (
	// AuthHeaderAuthorization is the standard header for bearer and basic auth.
	AuthHeaderAuthorization = "Authorization"

	// AuthDefaultAPIKeyHeader is the fallback header name when an api_key auth
	// block omits the optional header field.
	AuthDefaultAPIKeyHeader = "X-Api-Key" //nolint:gosec // false positive: this is a header name, not a credential
)

// Auth scheme prefixes. Note the trailing space; these are prepended to the
// credential value when building the final Authorization header.
const (
	AuthSchemeBearer = "Bearer "
	AuthSchemeBasic  = "Basic "
)

// IsValidAuthType reports whether s is a recognised auth.type value.
func IsValidAuthType(s string) bool {
	switch s {
	case AuthTypeBearer, AuthTypeAPIKey, AuthTypeBasic:
		return true
	default:
		return false
	}
}
