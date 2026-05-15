package discovery

import (
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/thushan/olla/internal/config"
	"github.com/thushan/olla/internal/core/constants"
	"github.com/thushan/olla/pkg/envresolver"
)

// resolvedAuth holds the credential values after all env/file references have
// been expanded. It is an intermediate type that does not cross package boundaries.
type resolvedAuth struct {
	// username and password are only populated for basic auth.
	username string
	password string
	// credential holds the resolved token (bearer) or key (api_key).
	credential string
	// header overrides the default header name for api_key auth.
	header   string
	authType string
}

// resolveAuth expands env placeholders and reads _file siblings for all
// credential fields in cfg. It must be called after validateAuth succeeds.
func resolveAuth(name string, cfg *config.AuthConfig) (resolvedAuth, error) {
	switch cfg.Type {
	case constants.AuthTypeBearer:
		return resolveBearerAuth(name, cfg)
	case constants.AuthTypeAPIKey:
		return resolveAPIKeyAuth(name, cfg)
	case constants.AuthTypeBasic:
		return resolveBasicAuth(name, cfg)
	default:
		// unreachable; validateAuth guards this
		return resolvedAuth{}, fmt.Errorf("endpoint %q: unknown auth type %q", name, cfg.Type)
	}
}

func resolveBearerAuth(name string, cfg *config.AuthConfig) (resolvedAuth, error) {
	token, err := resolveValueOrFile(cfg.Token, cfg.TokenFile)
	if err != nil {
		return resolvedAuth{}, fmt.Errorf("endpoint %q: bearer token: %w", name, err)
	}
	if token == "" {
		return resolvedAuth{}, fmt.Errorf("endpoint %q: bearer token resolved to empty string", name)
	}
	return resolvedAuth{authType: constants.AuthTypeBearer, credential: token}, nil
}

func resolveAPIKeyAuth(name string, cfg *config.AuthConfig) (resolvedAuth, error) {
	key, err := resolveValueOrFile(cfg.Key, cfg.KeyFile)
	if err != nil {
		return resolvedAuth{}, fmt.Errorf("endpoint %q: api_key: %w", name, err)
	}
	if key == "" {
		return resolvedAuth{}, fmt.Errorf("endpoint %q: api_key resolved to empty string", name)
	}

	header := cfg.Header
	if header == "" {
		header = constants.AuthDefaultAPIKeyHeader
	}
	return resolvedAuth{authType: constants.AuthTypeAPIKey, credential: key, header: header}, nil
}

func resolveBasicAuth(name string, cfg *config.AuthConfig) (resolvedAuth, error) {
	username, err := resolveValueOrFile(cfg.Username, cfg.UsernameFile)
	if err != nil {
		return resolvedAuth{}, fmt.Errorf("endpoint %q: basic username: %w", name, err)
	}
	if username == "" {
		return resolvedAuth{}, fmt.Errorf("endpoint %q: basic username resolved to empty string", name)
	}

	password, err := resolveValueOrFile(cfg.Password, cfg.PasswordFile)
	if err != nil {
		return resolvedAuth{}, fmt.Errorf("endpoint %q: basic password: %w", name, err)
	}
	if password == "" {
		return resolvedAuth{}, fmt.Errorf("endpoint %q: basic password resolved to empty string", name)
	}

	return resolvedAuth{authType: constants.AuthTypeBasic, username: username, password: password}, nil
}

// resolveValueOrFile handles the inline-value / _file-sibling pattern.
// If fileValue is non-empty the file is read; otherwise the inline value is
// expanded through envresolver. ExpandWithFile already rejects the both-set case.
func resolveValueOrFile(value, fileValue string) (string, error) {
	// ExpandWithFile covers: both set → error, file-only → read+trim, neither → "".
	// For the inline path it calls Expand (non-strict). We re-enter with ExpandStrict
	// when we actually have an inline placeholder to catch missing vars.
	if fileValue != "" {
		return envresolver.ExpandWithFile("", fileValue)
	}
	if value == "" {
		return "", nil
	}
	return envresolver.ExpandStrict(value)
}

// precomputeAuthHeaders builds the final AuthHeaderName and AuthHeaderValue
// from a resolved credential. Using strings.Builder keeps this allocation-free
// when the hot path copies these pre-built strings into request headers.
func precomputeAuthHeaders(r resolvedAuth) (headerName, headerValue string) {
	switch r.authType {
	case constants.AuthTypeBearer:
		var b strings.Builder
		b.Grow(len(constants.AuthSchemeBearer) + len(r.credential))
		b.WriteString(constants.AuthSchemeBearer)
		b.WriteString(r.credential)
		return constants.AuthHeaderAuthorization, b.String()

	case constants.AuthTypeAPIKey:
		return r.header, r.credential

	case constants.AuthTypeBasic:
		raw := r.username + ":" + r.password
		encoded := base64.StdEncoding.EncodeToString([]byte(raw))
		var b strings.Builder
		b.Grow(len(constants.AuthSchemeBasic) + len(encoded))
		b.WriteString(constants.AuthSchemeBasic)
		b.WriteString(encoded)
		return constants.AuthHeaderAuthorization, b.String()
	}

	return "", ""
}

// validateAuth checks the shape of an auth block before any env/file resolution
// happens. All conflicts and missing required fields are caught here so the
// process fails fast with a clear message that names the offending endpoint.
func validateAuth(name string, cfg *config.AuthConfig) error {
	if !constants.IsValidAuthType(cfg.Type) {
		return fmt.Errorf("endpoint %q: auth.type %q is not valid (must be bearer, api_key, or basic)", name, cfg.Type)
	}

	switch cfg.Type {
	case constants.AuthTypeBearer:
		return validateBearerAuth(name, cfg)
	case constants.AuthTypeAPIKey:
		return validateAPIKeyAuth(name, cfg)
	case constants.AuthTypeBasic:
		return validateBasicAuth(name, cfg)
	}

	// unreachable; IsValidAuthType guards the switch above
	return nil
}

func validateBearerAuth(name string, cfg *config.AuthConfig) error {
	hasToken := cfg.Token != ""
	hasTokenFile := cfg.TokenFile != ""

	if hasToken && hasTokenFile {
		return fmt.Errorf("endpoint %q: bearer auth has both token and token_file set; use exactly one", name)
	}
	if !hasToken && !hasTokenFile {
		return fmt.Errorf("endpoint %q: bearer auth requires token or token_file", name)
	}

	// Fields that must not appear for this auth type
	if cfg.Key != "" || cfg.KeyFile != "" {
		return fmt.Errorf("endpoint %q: bearer auth does not accept key/key_file fields", name)
	}
	if cfg.Username != "" || cfg.UsernameFile != "" || cfg.Password != "" || cfg.PasswordFile != "" {
		return fmt.Errorf("endpoint %q: bearer auth does not accept username/password fields", name)
	}

	return nil
}

func validateAPIKeyAuth(name string, cfg *config.AuthConfig) error {
	hasKey := cfg.Key != ""
	hasKeyFile := cfg.KeyFile != ""

	if hasKey && hasKeyFile {
		return fmt.Errorf("endpoint %q: api_key auth has both key and key_file set; use exactly one", name)
	}
	if !hasKey && !hasKeyFile {
		return fmt.Errorf("endpoint %q: api_key auth requires key or key_file", name)
	}

	// Fields that must not appear for this auth type
	if cfg.Token != "" || cfg.TokenFile != "" {
		return fmt.Errorf("endpoint %q: api_key auth does not accept token/token_file fields", name)
	}
	if cfg.Username != "" || cfg.UsernameFile != "" || cfg.Password != "" || cfg.PasswordFile != "" {
		return fmt.Errorf("endpoint %q: api_key auth does not accept username/password fields", name)
	}

	return nil
}

func validateBasicAuth(name string, cfg *config.AuthConfig) error {
	hasUsername := cfg.Username != ""
	hasUsernameFile := cfg.UsernameFile != ""
	hasPassword := cfg.Password != ""
	hasPasswordFile := cfg.PasswordFile != ""

	if hasUsername && hasUsernameFile {
		return fmt.Errorf("endpoint %q: basic auth has both username and username_file set; use exactly one", name)
	}
	if !hasUsername && !hasUsernameFile {
		return fmt.Errorf("endpoint %q: basic auth requires username or username_file", name)
	}

	if hasPassword && hasPasswordFile {
		return fmt.Errorf("endpoint %q: basic auth has both password and password_file set; use exactly one", name)
	}
	if !hasPassword && !hasPasswordFile {
		return fmt.Errorf("endpoint %q: basic auth requires password or password_file", name)
	}

	// Fields that must not appear for this auth type
	if cfg.Token != "" || cfg.TokenFile != "" {
		return fmt.Errorf("endpoint %q: basic auth does not accept token/token_file fields", name)
	}
	if cfg.Key != "" || cfg.KeyFile != "" {
		return fmt.Errorf("endpoint %q: basic auth does not accept key/key_file fields", name)
	}

	return nil
}
