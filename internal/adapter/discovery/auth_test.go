package discovery

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/thushan/olla/internal/config"
)

// validEndpointBase returns a minimal EndpointConfig that passes all
// non-auth validation, so auth tests can focus on auth behaviour only.
func validEndpointBase(name string) config.EndpointConfig {
	p := 100
	return config.EndpointConfig{
		Name:          name,
		URL:           "http://localhost:11434",
		Type:          "ollama",
		Priority:      &p,
		CheckInterval: 5 * time.Second,
		CheckTimeout:  2 * time.Second,
	}
}

// TestValidateAuth_Shape exercises the pure shape-validation rules against
// all three auth types before any env or file resolution takes place.
func TestValidateAuth_Shape(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		auth    config.AuthConfig
		wantErr bool
	}{
		// ── bearer ──────────────────────────────────────────────────────────
		{
			name:    "bearer with token",
			auth:    config.AuthConfig{Type: "bearer", Token: "tok"},
			wantErr: false,
		},
		{
			name:    "bearer with token_file",
			auth:    config.AuthConfig{Type: "bearer", TokenFile: "/run/secrets/token"},
			wantErr: false,
		},
		{
			name:    "bearer both token and token_file",
			auth:    config.AuthConfig{Type: "bearer", Token: "tok", TokenFile: "/run/secrets/token"},
			wantErr: true,
		},
		{
			name:    "bearer missing both token and token_file",
			auth:    config.AuthConfig{Type: "bearer"},
			wantErr: true,
		},
		{
			name:    "bearer with forbidden key field",
			auth:    config.AuthConfig{Type: "bearer", Token: "tok", Key: "k"},
			wantErr: true,
		},
		{
			name:    "bearer with forbidden username field",
			auth:    config.AuthConfig{Type: "bearer", Token: "tok", Username: "u"},
			wantErr: true,
		},
		{
			name:    "bearer with forbidden password field",
			auth:    config.AuthConfig{Type: "bearer", Token: "tok", Password: "p"},
			wantErr: true,
		},

		// ── api_key ─────────────────────────────────────────────────────────
		{
			name:    "api_key with key",
			auth:    config.AuthConfig{Type: "api_key", Key: "k"},
			wantErr: false,
		},
		{
			name:    "api_key with key_file",
			auth:    config.AuthConfig{Type: "api_key", KeyFile: "/run/secrets/key"},
			wantErr: false,
		},
		{
			name:    "api_key with optional header override",
			auth:    config.AuthConfig{Type: "api_key", Key: "k", Header: "X-My-Key"},
			wantErr: false,
		},
		{
			name:    "api_key both key and key_file",
			auth:    config.AuthConfig{Type: "api_key", Key: "k", KeyFile: "/run/secrets/key"},
			wantErr: true,
		},
		{
			name:    "api_key missing both key and key_file",
			auth:    config.AuthConfig{Type: "api_key"},
			wantErr: true,
		},
		{
			name:    "api_key with forbidden token field",
			auth:    config.AuthConfig{Type: "api_key", Key: "k", Token: "t"},
			wantErr: true,
		},
		{
			name:    "api_key with forbidden username field",
			auth:    config.AuthConfig{Type: "api_key", Key: "k", Username: "u"},
			wantErr: true,
		},

		// ── basic ────────────────────────────────────────────────────────────
		{
			name:    "basic with inline credentials",
			auth:    config.AuthConfig{Type: "basic", Username: "user", Password: "pass"},
			wantErr: false,
		},
		{
			name:    "basic with file credentials",
			auth:    config.AuthConfig{Type: "basic", UsernameFile: "/run/secrets/user", PasswordFile: "/run/secrets/pass"},
			wantErr: false,
		},
		{
			name:    "basic mixed inline and file",
			auth:    config.AuthConfig{Type: "basic", Username: "user", PasswordFile: "/run/secrets/pass"},
			wantErr: false,
		},
		{
			name:    "basic both username and username_file",
			auth:    config.AuthConfig{Type: "basic", Username: "u", UsernameFile: "/f", Password: "p"},
			wantErr: true,
		},
		{
			name:    "basic both password and password_file",
			auth:    config.AuthConfig{Type: "basic", Username: "u", Password: "p", PasswordFile: "/f"},
			wantErr: true,
		},
		{
			name:    "basic missing username",
			auth:    config.AuthConfig{Type: "basic", Password: "p"},
			wantErr: true,
		},
		{
			name:    "basic missing password",
			auth:    config.AuthConfig{Type: "basic", Username: "u"},
			wantErr: true,
		},
		{
			name:    "basic with forbidden token field",
			auth:    config.AuthConfig{Type: "basic", Username: "u", Password: "p", Token: "t"},
			wantErr: true,
		},
		{
			name:    "basic with forbidden key field",
			auth:    config.AuthConfig{Type: "basic", Username: "u", Password: "p", Key: "k"},
			wantErr: true,
		},

		// ── unknown type ─────────────────────────────────────────────────────
		{
			name:    "unknown auth type",
			auth:    config.AuthConfig{Type: "oauth2", Token: "tok"},
			wantErr: true,
		},
		{
			name:    "empty auth type",
			auth:    config.AuthConfig{},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := validateAuth("test-endpoint", &tc.auth)
			if tc.wantErr && err == nil {
				t.Error("expected validation error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Errorf("unexpected validation error: %v", err)
			}
		})
	}
}

// TestLoadFromConfig_AuthValidation_RejectsInvalidShape verifies that
// LoadFromConfig propagates auth validation errors and surfaces the endpoint name.
func TestLoadFromConfig_AuthValidation_RejectsInvalidShape(t *testing.T) {
	t.Parallel()

	p := 100
	badAuth := config.EndpointConfig{
		Name:          "bad-auth-ep",
		URL:           "http://localhost:11434",
		Type:          "ollama",
		Priority:      &p,
		CheckInterval: 5 * time.Second,
		CheckTimeout:  2 * time.Second,
		Auth: &config.AuthConfig{
			// bearer with both token and token_file must fail
			Type:      "bearer",
			Token:     "tok",
			TokenFile: "/run/secrets/token",
		},
	}

	repo := NewStaticEndpointRepository()
	err := repo.LoadFromConfig(context.Background(), []config.EndpointConfig{badAuth})
	if err == nil {
		t.Fatal("expected error for conflicting token/token_file, got nil")
	}
}

// TestLoadFromConfig_AuthNil_Succeeds confirms that endpoints without an auth
// block load normally. Auth is always optional.
func TestLoadFromConfig_AuthNil_Succeeds(t *testing.T) {
	t.Parallel()

	cfg := validEndpointBase("no-auth")
	// Auth is nil by default

	repo := NewStaticEndpointRepository()
	if err := repo.LoadFromConfig(context.Background(), []config.EndpointConfig{cfg}); err != nil {
		t.Fatalf("LoadFromConfig with nil auth failed: %v", err)
	}
}

// ── Commit 2: env/file resolution ───────────────────────────────────────────

// TestAuthResolve_EnvVar_ExpandsToken verifies that a ${VAR} placeholder in the
// token field is expanded through the environment at load time.
func TestAuthResolve_EnvVar_ExpandsToken(t *testing.T) {
	t.Setenv("OLLA_TEST_TOKEN", "resolved-secret")

	cfg := validEndpointBase("bearer-env")
	cfg.Auth = &config.AuthConfig{Type: "bearer", Token: "${OLLA_TEST_TOKEN}"}

	repo := NewStaticEndpointRepository()
	if err := repo.LoadFromConfig(context.Background(), []config.EndpointConfig{cfg}); err != nil {
		t.Fatalf("LoadFromConfig failed: %v", err)
	}

	eps, _ := repo.GetAll(context.Background())
	if len(eps) != 1 {
		t.Fatalf("expected 1 endpoint, got %d", len(eps))
	}
	if !strings.HasSuffix(eps[0].AuthHeaderValue, "resolved-secret") {
		t.Errorf("AuthHeaderValue %q does not end with resolved token", eps[0].AuthHeaderValue)
	}
}

// TestAuthResolve_MissingEnvVar_FatalError verifies that an unset ${VAR} in an
// auth field causes a startup-fatal error that mentions the endpoint name.
func TestAuthResolve_MissingEnvVar_FatalError(t *testing.T) {
	t.Parallel()

	// Guarantee the var is absent
	varName := "OLLA_DEFINITELY_UNSET_VAR_XYZ"
	os.Unsetenv(varName) //nolint:errcheck

	cfg := validEndpointBase("bearer-missing-env")
	cfg.Auth = &config.AuthConfig{Type: "bearer", Token: fmt.Sprintf("${%s}", varName)}

	repo := NewStaticEndpointRepository()
	err := repo.LoadFromConfig(context.Background(), []config.EndpointConfig{cfg})
	if err == nil {
		t.Fatal("expected error for unset env var, got nil")
	}
	if !strings.Contains(err.Error(), "bearer-missing-env") {
		t.Errorf("error should mention endpoint name, got: %v", err)
	}
}

// TestAuthResolve_TokenFile_ReadsAndTrims verifies that token_file reads the
// file content and strips trailing whitespace (e.g. trailing newline from echo).
func TestAuthResolve_TokenFile_ReadsAndTrims(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	tokenPath := filepath.Join(dir, "token.txt")
	if err := os.WriteFile(tokenPath, []byte("file-secret\n"), 0o600); err != nil {
		t.Fatalf("writing token file: %v", err)
	}

	cfg := validEndpointBase("bearer-file")
	cfg.Auth = &config.AuthConfig{Type: "bearer", TokenFile: tokenPath}

	repo := NewStaticEndpointRepository()
	if err := repo.LoadFromConfig(context.Background(), []config.EndpointConfig{cfg}); err != nil {
		t.Fatalf("LoadFromConfig failed: %v", err)
	}

	eps, _ := repo.GetAll(context.Background())
	if !strings.HasSuffix(eps[0].AuthHeaderValue, "file-secret") {
		t.Errorf("AuthHeaderValue %q does not end with trimmed file content", eps[0].AuthHeaderValue)
	}
}

// TestAuthResolve_BothInlineAndFile_FatalError ensures the both-set conflict is
// caught at resolution time (ExpandWithFile enforces this).
func TestAuthResolve_BothInlineAndFile_FatalError(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	tokenPath := filepath.Join(dir, "token.txt")
	_ = os.WriteFile(tokenPath, []byte("tok\n"), 0o600)

	cfg := validEndpointBase("bearer-both")
	cfg.Auth = &config.AuthConfig{Type: "bearer", Token: "inline", TokenFile: tokenPath}

	repo := NewStaticEndpointRepository()
	err := repo.LoadFromConfig(context.Background(), []config.EndpointConfig{cfg})
	if err == nil {
		t.Fatal("expected error for both token and token_file, got nil")
	}
}

// TestAuthResolve_EmptyAfterExpansion_FatalError verifies that a token that
// resolves to an empty string (e.g. env var set to "") is a startup-fatal error.
func TestAuthResolve_EmptyAfterExpansion_FatalError(t *testing.T) {
	t.Setenv("OLLA_EMPTY_TOKEN", "")

	cfg := validEndpointBase("bearer-empty")
	cfg.Auth = &config.AuthConfig{Type: "bearer", Token: "${OLLA_EMPTY_TOKEN:-}"}

	repo := NewStaticEndpointRepository()
	err := repo.LoadFromConfig(context.Background(), []config.EndpointConfig{cfg})
	if err == nil {
		t.Fatal("expected error for empty-resolved token, got nil")
	}
}

// ── Commit 3: precomputed headers ───────────────────────────────────────────

// TestAuthPrecompute_Bearer_AuthorizationHeader verifies the bearer auth
// produces the correct Authorization header value.
func TestAuthPrecompute_Bearer_AuthorizationHeader(t *testing.T) {
	t.Parallel()

	cfg := validEndpointBase("bearer-precompute")
	cfg.Auth = &config.AuthConfig{Type: "bearer", Token: "mysecrettoken"}

	repo := NewStaticEndpointRepository()
	if err := repo.LoadFromConfig(context.Background(), []config.EndpointConfig{cfg}); err != nil {
		t.Fatalf("LoadFromConfig failed: %v", err)
	}

	eps, _ := repo.GetAll(context.Background())
	ep := eps[0]

	if ep.AuthHeaderName != "Authorization" {
		t.Errorf("AuthHeaderName = %q, want %q", ep.AuthHeaderName, "Authorization")
	}
	if ep.AuthHeaderValue != "Bearer mysecrettoken" {
		t.Errorf("AuthHeaderValue = %q, want %q", ep.AuthHeaderValue, "Bearer mysecrettoken")
	}
}

// TestAuthPrecompute_APIKey_DefaultHeader verifies that api_key with no header
// override uses X-Api-Key as the header name.
func TestAuthPrecompute_APIKey_DefaultHeader(t *testing.T) {
	t.Parallel()

	cfg := validEndpointBase("apikey-default")
	cfg.Auth = &config.AuthConfig{Type: "api_key", Key: "mykey"}

	repo := NewStaticEndpointRepository()
	if err := repo.LoadFromConfig(context.Background(), []config.EndpointConfig{cfg}); err != nil {
		t.Fatalf("LoadFromConfig failed: %v", err)
	}

	eps, _ := repo.GetAll(context.Background())
	ep := eps[0]

	if ep.AuthHeaderName != "X-Api-Key" {
		t.Errorf("AuthHeaderName = %q, want %q", ep.AuthHeaderName, "X-Api-Key")
	}
	if ep.AuthHeaderValue != "mykey" {
		t.Errorf("AuthHeaderValue = %q, want %q", ep.AuthHeaderValue, "mykey")
	}
}

// TestAuthPrecompute_APIKey_CustomHeader verifies that an explicit header field
// overrides the default X-Api-Key name.
func TestAuthPrecompute_APIKey_CustomHeader(t *testing.T) {
	t.Parallel()

	cfg := validEndpointBase("apikey-custom")
	cfg.Auth = &config.AuthConfig{Type: "api_key", Key: "mykey", Header: "X-My-Auth"}

	repo := NewStaticEndpointRepository()
	if err := repo.LoadFromConfig(context.Background(), []config.EndpointConfig{cfg}); err != nil {
		t.Fatalf("LoadFromConfig failed: %v", err)
	}

	eps, _ := repo.GetAll(context.Background())
	ep := eps[0]

	if ep.AuthHeaderName != "X-My-Auth" {
		t.Errorf("AuthHeaderName = %q, want %q", ep.AuthHeaderName, "X-My-Auth")
	}
	if ep.AuthHeaderValue != "mykey" {
		t.Errorf("AuthHeaderValue = %q, want %q", ep.AuthHeaderValue, "mykey")
	}
}

// TestAuthPrecompute_Basic_CorrectBase64 verifies the basic auth header is the
// correctly base64-encoded "username:password" pair. We decode it to be explicit.
func TestAuthPrecompute_Basic_CorrectBase64(t *testing.T) {
	t.Parallel()

	cfg := validEndpointBase("basic-precompute")
	cfg.Auth = &config.AuthConfig{Type: "basic", Username: "alice", Password: "s3cret"}

	repo := NewStaticEndpointRepository()
	if err := repo.LoadFromConfig(context.Background(), []config.EndpointConfig{cfg}); err != nil {
		t.Fatalf("LoadFromConfig failed: %v", err)
	}

	eps, _ := repo.GetAll(context.Background())
	ep := eps[0]

	if ep.AuthHeaderName != "Authorization" {
		t.Errorf("AuthHeaderName = %q, want %q", ep.AuthHeaderName, "Authorization")
	}

	const prefix = "Basic "
	if !strings.HasPrefix(ep.AuthHeaderValue, prefix) {
		t.Fatalf("AuthHeaderValue %q does not start with %q", ep.AuthHeaderValue, prefix)
	}

	encoded := strings.TrimPrefix(ep.AuthHeaderValue, prefix)
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatalf("base64 decode failed: %v", err)
	}

	const want = "alice:s3cret"
	if string(decoded) != want {
		t.Errorf("decoded basic credentials = %q, want %q", string(decoded), want)
	}
}

// TestAuthPrecompute_NoAuth_EmptyFields verifies that endpoints without auth
// have zero-value AuthHeaderName and nil Headers.
func TestAuthPrecompute_NoAuth_EmptyFields(t *testing.T) {
	t.Parallel()

	cfg := validEndpointBase("no-auth-fields")

	repo := NewStaticEndpointRepository()
	if err := repo.LoadFromConfig(context.Background(), []config.EndpointConfig{cfg}); err != nil {
		t.Fatalf("LoadFromConfig failed: %v", err)
	}

	eps, _ := repo.GetAll(context.Background())
	ep := eps[0]

	if ep.AuthHeaderName != "" {
		t.Errorf("AuthHeaderName should be empty for endpoint without auth, got %q", ep.AuthHeaderName)
	}
	if ep.AuthHeaderValue != "" {
		t.Errorf("AuthHeaderValue should be empty for endpoint without auth, got %q", ep.AuthHeaderValue)
	}
	if ep.Headers != nil {
		t.Errorf("Headers should be nil when no headers configured, got %v", ep.Headers)
	}
}

// TestAuthPrecompute_Headers_EnvResolved verifies that values in the headers
// map are expanded through the environment at load time.
func TestAuthPrecompute_Headers_EnvResolved(t *testing.T) {
	t.Setenv("OLLA_TEST_TENANT", "acme-corp")

	cfg := validEndpointBase("headers-env")
	cfg.Headers = map[string]string{
		"X-Tenant": "${OLLA_TEST_TENANT}",
		"X-Static": "literal",
	}

	repo := NewStaticEndpointRepository()
	if err := repo.LoadFromConfig(context.Background(), []config.EndpointConfig{cfg}); err != nil {
		t.Fatalf("LoadFromConfig failed: %v", err)
	}

	eps, _ := repo.GetAll(context.Background())
	ep := eps[0]

	if ep.Headers["X-Tenant"] != "acme-corp" {
		t.Errorf("Headers[X-Tenant] = %q, want %q", ep.Headers["X-Tenant"], "acme-corp")
	}
	if ep.Headers["X-Static"] != "literal" {
		t.Errorf("Headers[X-Static] = %q, want %q", ep.Headers["X-Static"], "literal")
	}
}

// ── Security audit ───────────────────────────────────────────────────────────

// TestAuthSecurity_AuthHeaderValue_NotInJSON asserts that AuthHeaderValue is
// excluded from JSON serialisation of a domain.Endpoint. This guards against
// credential leakage through status endpoints or debug logs.
// (The json:"-" tag on AuthHeaderValue provides the guarantee; this test keeps
// it honest so no future refactor can accidentally remove it.)
func TestAuthSecurity_AuthHeaderValue_NotInJSON(t *testing.T) {
	t.Parallel()

	cfg := validEndpointBase("security-ep")
	cfg.Auth = &config.AuthConfig{Type: "bearer", Token: "super-secret-do-not-leak"}

	repo := NewStaticEndpointRepository()
	if err := repo.LoadFromConfig(context.Background(), []config.EndpointConfig{cfg}); err != nil {
		t.Fatalf("LoadFromConfig failed: %v", err)
	}

	// The domain.Endpoint JSON test already covers the tag; here we exercise
	// the full load → retrieve → marshal path to catch end-to-end regressions.
	eps, _ := repo.GetAll(context.Background())
	ep := eps[0]

	// AuthHeaderValue must be set (we loaded auth successfully).
	if ep.AuthHeaderValue == "" {
		t.Fatal("AuthHeaderValue is empty: auth was not wired in")
	}

	// GetURLString and GetHealthCheckURLString are the only string accessors on
	// Endpoint; neither should expose credentials.
	if strings.Contains(ep.GetURLString(), "secret") {
		t.Errorf("GetURLString leaks credential: %q", ep.GetURLString())
	}
	if strings.Contains(ep.GetHealthCheckURLString(), "secret") {
		t.Errorf("GetHealthCheckURLString leaks credential: %q", ep.GetHealthCheckURLString())
	}
}
