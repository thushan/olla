package discovery

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/thushan/olla/internal/config"
)

// Config-validation half: an endpoint URL containing user:pass@host
// must be rejected at load time, with an error that points the operator at
// the auth config block AND spells out the exact rewrite so a failed boot is
// a copy-paste fix. The display-side sanitisation (stripping userinfo from
// the url field surfaced in JSON) lives in the handlers; this is the hard
// gate that prevents credentials ever entering the runtime URLString.
func TestEndpointConfigValidation_RejectsUserinfo(t *testing.T) {
	cases := []struct {
		name string
		url  string
		// wantContains are substrings the rewritten error must carry. The
		// user/pass pair is echoed from the URL so the operator can paste the
		// fix without transcribing them.
		wantContains []string
	}{
		{
			name:         "user and password",
			url:          "https://alice:s3cr3t@ollama.local:11434",
			wantContains: []string{"alice", "s3cr3t", "type: basic"},
		},
		{
			name:         "user only",
			url:          "https://token@ollama.local:11434",
			wantContains: []string{"token", "type: basic"},
		},
		{
			name:         "userinfo with path and query",
			url:          "http://u:p@host:8080/v1?x=1",
			wantContains: []string{"host:8080", "type: basic"},
		},
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
			// The fix should be spelled out: the username/password_file
			// variants so secrets can be kept out of config too.
			if !strings.Contains(err.Error(), "username_file") {
				t.Errorf("error should mention username_file for secret-file resolution, got: %v", err)
			}
			for _, want := range tc.wantContains {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error should contain %q, got: %v", want, err)
				}
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
