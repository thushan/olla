package discovery

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/thushan/olla/internal/config"
)

// FR-14 config-validation half: an endpoint URL containing user:pass@host
// must be rejected at load time, with an error that points the operator at
// the auth config block. The display-side sanitisation (stripping userinfo
// from the url field surfaced in JSON) lives in the handlers; this is the
// hard gate that prevents credentials ever entering the runtime URLString.
func TestEndpointConfigValidation_RejectsUserinfo(t *testing.T) {
	cases := []struct {
		name string
		url  string
	}{
		{"user and password", "https://alice:s3cr3t@ollama.local:11434"},
		{"user only", "https://token@ollama.local:11434"},
		{"userinfo with path and query", "http://u:p@host:8080/v1?x=1"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := NewStaticEndpointRepository()
			cfg := config.EndpointConfig{
				Name:           "credentialed",
				URL:            tc.url,
				Type:           "ollama",
				HealthCheckURL: "/health",
				ModelURL:       "/api/tags",
				CheckInterval:  5 * time.Second,
				CheckTimeout:   2 * time.Second,
			}
			err := repo.LoadFromConfig(context.Background(), []config.EndpointConfig{cfg})
			if err == nil {
				t.Fatalf("expected error for userinfo URL %q, got nil", tc.url)
			}
			if !strings.Contains(err.Error(), "auth config block") {
				t.Errorf("error should direct operator to the auth config block, got: %v", err)
			}
			if !strings.Contains(err.Error(), "credentials") {
				t.Errorf("error should mention credentials, got: %v", err)
			}
		})
	}
}

// A plain URL without userinfo must continue to load cleanly (regression guard).
func TestEndpointConfigValidation_AcceptsPlainURL(t *testing.T) {
	repo := NewStaticEndpointRepository()
	cfg := config.EndpointConfig{
		Name:           "plain",
		URL:            "http://localhost:11434",
		Type:           "ollama",
		HealthCheckURL: "/health",
		ModelURL:       "/api/tags",
		CheckInterval:  5 * time.Second,
		CheckTimeout:   2 * time.Second,
	}
	if err := repo.LoadFromConfig(context.Background(), []config.EndpointConfig{cfg}); err != nil {
		t.Fatalf("plain URL should load, got: %v", err)
	}
}
