package discovery

import (
	"fmt"

	"github.com/thushan/olla/internal/config"
	"github.com/thushan/olla/internal/core/constants"
)

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

	// unreachable — IsValidAuthType guards the switch above
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
