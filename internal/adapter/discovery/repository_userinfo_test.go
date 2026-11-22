package discovery

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/thushan/olla/internal/config"
)

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

// The loader must run the shared URL rules; a credential written before the
// scheme parses without a User, so only the validator catches it.
func TestLoadFromConfig_RejectsCredentialBeforeScheme(t *testing.T) {
	cfg := config.EndpointConfig{
		Name:          "leaky",
		URL:           "wombat:q7x9kz@http://host:11434",
		Type:          "ollama",
		CheckInterval: 5 * time.Second,
		CheckTimeout:  2 * time.Second,
	}
	err := NewStaticEndpointRepository().LoadFromConfig(context.Background(), []config.EndpointConfig{cfg})
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if strings.Contains(err.Error(), "q7x9kz") || strings.Contains(err.Error(), "wombat") {
		t.Errorf("error leaks the configured value: %v", err)
	}
}
