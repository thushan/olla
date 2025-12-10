package discovery

import (
	"context"
	"testing"
	"time"

	"github.com/thushan/olla/internal/config"
)

func TestEndpointConfigValidation_WithType(t *testing.T) {
	testCases := []struct {
		name      string
		config    config.EndpointConfig
		expectErr bool
	}{
		{
			name: "valid ollama type",
			config: config.EndpointConfig{
				Name:           "test",
				URL:            "http://localhost:11434",
				Type:           "ollama",
				HealthCheckURL: "/health",
				ModelURL:       "/api/tags",
				CheckInterval:  5 * time.Second,
				CheckTimeout:   2 * time.Second,
			},
			expectErr: false,
		},
		{
			name: "valid lm-studio type",
			config: config.EndpointConfig{
				Name:           "test",
				URL:            "http://localhost:1234",
				Type:           "lm-studio",
				HealthCheckURL: "/health",
				ModelURL:       "/v1/models",
				CheckInterval:  5 * time.Second,
				CheckTimeout:   2 * time.Second,
			},
			expectErr: false,
		},
		{
			name: "valid auto type",
			config: config.EndpointConfig{
				Name:           "test",
				URL:            "http://localhost:11434",
				Type:           "auto",
				HealthCheckURL: "/health",
				ModelURL:       "/api/tags",
				CheckInterval:  5 * time.Second,
				CheckTimeout:   2 * time.Second,
			},
			expectErr: false,
		},
		{
			name: "invalid type",
			config: config.EndpointConfig{
				Name:           "test",
				URL:            "http://localhost:11434",
				Type:           "invalid-type",
				HealthCheckURL: "/health",
				ModelURL:       "/api/tags",
				CheckInterval:  5 * time.Second,
				CheckTimeout:   2 * time.Second,
			},
			expectErr: true,
		},
		{
			name: "empty type (should be valid)",
			config: config.EndpointConfig{
				Name:           "test",
				URL:            "http://localhost:11434",
				Type:           "",
				HealthCheckURL: "/health",
				ModelURL:       "/api/tags",
				CheckInterval:  5 * time.Second,
				CheckTimeout:   2 * time.Second,
			},
			expectErr: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			repo := NewStaticEndpointRepository()
			err := repo.LoadFromConfig(context.Background(), []config.EndpointConfig{tc.config})

			if tc.expectErr && err == nil {
				t.Error("Expected error but got none")
			}
			if !tc.expectErr && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

func TestStaticEndpointRepository_LoadFromConfig(t *testing.T) {
	repo := NewStaticEndpointRepository()
	ctx := context.Background()

	configs := []config.EndpointConfig{
		{
			Name:           "ollama",
			URL:            "http://localhost:11434",
			Type:           "ollama",
			HealthCheckURL: "/health",
			ModelURL:       "/api/tags",
			Priority:       100,
			CheckInterval:  5 * time.Second,
			CheckTimeout:   2 * time.Second,
		},
		{
			Name:           "lmstudio",
			URL:            "http://localhost:1234",
			Type:           "lm-studio",
			HealthCheckURL: "/v1/models",
			ModelURL:       "/v1/models",
			Priority:       90,
			CheckInterval:  10 * time.Second,
			CheckTimeout:   3 * time.Second,
		},
	}

	err := repo.LoadFromConfig(ctx, configs)
	if err != nil {
		t.Fatalf("LoadFromConfig failed: %v", err)
	}

	endpoints, err := repo.GetAll(ctx)
	if err != nil {
		t.Fatalf("GetAll failed: %v", err)
	}

	if len(endpoints) != 2 {
		t.Errorf("Expected 2 endpoints, got %d", len(endpoints))
	}

	for _, endpoint := range endpoints {
		if endpoint.Type == "" {
			t.Error("Endpoint should have type set")
		}
		if endpoint.Name == "ollama" && endpoint.Type != "ollama" {
			t.Errorf("Expected ollama endpoint to have type 'ollama', got %q", endpoint.Type)
		}
		if endpoint.Name == "lmstudio" && endpoint.Type != "lm-studio" {
			t.Errorf("Expected lmstudio endpoint to have type 'lm-studio', got %q", endpoint.Type)
		}
	}
}

func TestStaticEndpointRepository_EmptyConfig(t *testing.T) {
	repo := NewStaticEndpointRepository()
	ctx := context.Background()

	err := repo.LoadFromConfig(ctx, []config.EndpointConfig{})
	if err != nil {
		t.Fatalf("LoadFromConfig with empty config failed: %v", err)
	}

	endpoints, err := repo.GetAll(ctx)
	if err != nil {
		t.Fatalf("GetAll failed: %v", err)
	}

	if len(endpoints) != 0 {
		t.Errorf("Expected 0 endpoints for empty config, got %d", len(endpoints))
	}
}

func TestJoinURLPath(t *testing.T) {
	tests := []struct {
		name     string
		baseURL  string
		path     string
		expected string
	}{
		{
			name:     "base with trailing slash, path with leading slash",
			baseURL:  "http://localhost:12434/engines/llama.cpp/",
			path:     "/v1/models",
			expected: "http://localhost:12434/engines/llama.cpp/v1/models",
		},
		{
			name:     "base without trailing slash, path with leading slash",
			baseURL:  "http://localhost:11434",
			path:     "/api/tags",
			expected: "http://localhost:11434/api/tags",
		},
		{
			name:     "base with trailing slash, path without leading slash",
			baseURL:  "http://localhost:12434/api/",
			path:     "v1/models",
			expected: "http://localhost:12434/api/v1/models",
		},
		{
			name:     "base without trailing slash, path without leading slash",
			baseURL:  "http://localhost:11434",
			path:     "api/tags",
			expected: "http://localhost:11434/api/tags",
		},
		{
			name:     "empty base",
			baseURL:  "",
			path:     "/v1/models",
			expected: "/v1/models",
		},
		{
			name:     "empty path",
			baseURL:  "http://localhost:11434",
			path:     "",
			expected: "http://localhost:11434",
		},
		{
			name:     "Docker nested path case",
			baseURL:  "http://localhost:12434/engines/llama.cpp/",
			path:     "/v1/models",
			expected: "http://localhost:12434/engines/llama.cpp/v1/models",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := joinURLPath(tc.baseURL, tc.path)
			if result != tc.expected {
				t.Errorf("joinURLPath(%q, %q) = %q, expected %q",
					tc.baseURL, tc.path, result, tc.expected)
			}
		})
	}
}

func TestStaticEndpointRepository_NestedPathURLs(t *testing.T) {
	// Test that endpoints with nested paths (like Docker) correctly preserve
	// the path prefix when building model discovery URLs
	repo := NewStaticEndpointRepository()
	ctx := context.Background()

	configs := []config.EndpointConfig{
		{
			Name:           "docker-llamacpp",
			URL:            "http://localhost:12434/engines/llama.cpp/",
			Type:           "openai-compatible",
			HealthCheckURL: "/health",
			ModelURL:       "/v1/models",
			Priority:       100,
			CheckInterval:  5 * time.Second,
			CheckTimeout:   2 * time.Second,
		},
	}

	err := repo.LoadFromConfig(ctx, configs)
	if err != nil {
		t.Fatalf("LoadFromConfig failed: %v", err)
	}

	endpoints, err := repo.GetAll(ctx)
	if err != nil {
		t.Fatalf("GetAll failed: %v", err)
	}

	if len(endpoints) != 1 {
		t.Fatalf("Expected 1 endpoint, got %d", len(endpoints))
	}

	endpoint := endpoints[0]

	// Verify the model URL string preserves the path prefix
	expectedModelURL := "http://localhost:12434/engines/llama.cpp/v1/models"
	if endpoint.ModelURLString != expectedModelURL {
		t.Errorf("ModelURLString = %q, expected %q", endpoint.ModelURLString, expectedModelURL)
	}

	// Verify the health check URL string preserves the path prefix
	expectedHealthURL := "http://localhost:12434/engines/llama.cpp/health"
	if endpoint.HealthCheckURLString != expectedHealthURL {
		t.Errorf("HealthCheckURLString = %q, expected %q", endpoint.HealthCheckURLString, expectedHealthURL)
	}
}
