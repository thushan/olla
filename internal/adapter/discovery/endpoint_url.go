package discovery

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/thushan/olla/internal/config"
)

// ValidateEndpointURLs applies the endpoint URL rules shared by config loading
// and --validate-config. Errors name the endpoint and field but never the
// value, nor the parser's error (it quotes its input), so a credential written
// in the wrong place cannot reach a log, the startup table or /internal/status.
func ValidateEndpointURLs(cfg config.EndpointConfig) error {
	if cfg.URL == "" {
		return fmt.Errorf("endpoint %q: url is required", cfg.Name)
	}
	if err := validateURLField(cfg.Name, "url", cfg.URL, false); err != nil {
		return err
	}
	if err := validateURLField(cfg.Name, "health_check_url", cfg.HealthCheckURL, true); err != nil {
		return err
	}
	return validateURLField(cfg.Name, "model_url", cfg.ModelURL, true)
}

// validateURLField checks one field. Overrides (health_check_url, model_url) are
// optional and may also be a bare path, which is used as-is without parsing.
func validateURLField(name, field, v string, isOverride bool) error {
	if v == "" && isOverride {
		return nil
	}

	// One blunt rule instead of parsing userinfo: '@' (or its escaped form) has
	// no place in these URLs, and this also catches credentials written before
	// the scheme or parsed as the host, which url.Parse does not report as User.
	if strings.Contains(v, "@") || strings.Contains(v, "%40") {
		return fmt.Errorf("endpoint %q: %s must not contain '@' or credentials; put them in the endpoint's auth block", name, field)
	}

	if isOverride && strings.HasPrefix(v, "/") && !strings.HasPrefix(v, "//") {
		return nil
	}

	want := "an absolute http:// or https:// URL, e.g. http://localhost:11434"
	if isOverride {
		want = "a path starting with exactly one '/' or an absolute http:// or https:// URL"
	}
	if !strings.Contains(v, "://") {
		return fmt.Errorf("endpoint %q: %s must be %s", name, field, want)
	}

	u, err := url.Parse(v)
	if err != nil {
		return fmt.Errorf("endpoint %q: %s is not a valid URL (check the host and port; IPv6 needs brackets, e.g. http://[::1]:11434)", name, field)
	}
	if u.Scheme != "http" && u.Scheme != "https" || u.Opaque != "" || u.Hostname() == "" {
		return fmt.Errorf("endpoint %q: %s must be %s with a host", name, field, want)
	}
	return nil
}
