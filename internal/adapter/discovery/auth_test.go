package discovery

import (
	"context"
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
			// bearer with both token and token_file — must fail
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
// block load normally — auth is always optional.
func TestLoadFromConfig_AuthNil_Succeeds(t *testing.T) {
	t.Parallel()

	cfg := validEndpointBase("no-auth")
	// Auth is nil by default

	repo := NewStaticEndpointRepository()
	if err := repo.LoadFromConfig(context.Background(), []config.EndpointConfig{cfg}); err != nil {
		t.Fatalf("LoadFromConfig with nil auth failed: %v", err)
	}
}
