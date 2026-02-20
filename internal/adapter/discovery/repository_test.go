package discovery

import (
	"context"
	"testing"
	"time"

	"github.com/thushan/olla/internal/config"
)

// TestApplyEndpointDefaults_SetsZeroValues verifies that zero-value timing and priority
// fields are filled with the package defaults, ensuring a minimal YAML config can
// pass validation without requiring explicit timing fields.
func TestApplyEndpointDefaults_SetsZeroValues(t *testing.T) {
	t.Parallel()

	cfg := &config.EndpointConfig{
		Name: "test",
		URL:  "http://localhost:11434",
		// CheckInterval, CheckTimeout and Priority are intentionally left at zero
	}

	applyEndpointDefaults(cfg)

	if cfg.CheckInterval != DefaultCheckInterval {
		t.Errorf("CheckInterval = %v, want %v", cfg.CheckInterval, DefaultCheckInterval)
	}
	if cfg.CheckTimeout != DefaultCheckTimeout {
		t.Errorf("CheckTimeout = %v, want %v", cfg.CheckTimeout, DefaultCheckTimeout)
	}
	if cfg.Priority != DefaultPriority {
		t.Errorf("Priority = %d, want %d", cfg.Priority, DefaultPriority)
	}
}

// TestApplyEndpointDefaults_PreservesExplicitValues confirms that explicitly configured
// values are never overwritten by the defaults — the function must only fill gaps.
func TestApplyEndpointDefaults_PreservesExplicitValues(t *testing.T) {
	t.Parallel()

	cfg := &config.EndpointConfig{
		Name:          "test",
		URL:           "http://localhost:11434",
		CheckInterval: 10 * time.Second,
		CheckTimeout:  3 * time.Second,
		Priority:      50,
	}

	applyEndpointDefaults(cfg)

	if cfg.CheckInterval != 10*time.Second {
		t.Errorf("CheckInterval = %v, want %v (should not be overwritten)", cfg.CheckInterval, 10*time.Second)
	}
	if cfg.CheckTimeout != 3*time.Second {
		t.Errorf("CheckTimeout = %v, want %v (should not be overwritten)", cfg.CheckTimeout, 3*time.Second)
	}
	if cfg.Priority != 50 {
		t.Errorf("Priority = %d, want %d (should not be overwritten)", cfg.Priority, 50)
	}
}

// TestApplyEndpointDefaults_PartialDefaults verifies that only the zero-value fields
// receive defaults when the config is partially specified.
func TestApplyEndpointDefaults_PartialDefaults(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		cfg          config.EndpointConfig
		wantInterval time.Duration
		wantTimeout  time.Duration
		wantPriority int
	}{
		{
			name: "only CheckInterval set",
			cfg: config.EndpointConfig{
				CheckInterval: 15 * time.Second,
				// CheckTimeout and Priority are zero
			},
			wantInterval: 15 * time.Second,
			wantTimeout:  DefaultCheckTimeout,
			wantPriority: DefaultPriority,
		},
		{
			name: "only CheckTimeout set",
			cfg: config.EndpointConfig{
				CheckTimeout: 1 * time.Second,
				// CheckInterval and Priority are zero
			},
			wantInterval: DefaultCheckInterval,
			wantTimeout:  1 * time.Second,
			wantPriority: DefaultPriority,
		},
		{
			name: "only Priority set",
			cfg: config.EndpointConfig{
				Priority: 200,
				// CheckInterval and CheckTimeout are zero
			},
			wantInterval: DefaultCheckInterval,
			wantTimeout:  DefaultCheckTimeout,
			wantPriority: 200,
		},
		{
			name: "CheckInterval and Priority set, CheckTimeout zero",
			cfg: config.EndpointConfig{
				CheckInterval: 20 * time.Second,
				Priority:      75,
			},
			wantInterval: 20 * time.Second,
			wantTimeout:  DefaultCheckTimeout,
			wantPriority: 75,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cfg := tc.cfg // copy to avoid mutation across subtests
			applyEndpointDefaults(&cfg)

			if cfg.CheckInterval != tc.wantInterval {
				t.Errorf("CheckInterval = %v, want %v", cfg.CheckInterval, tc.wantInterval)
			}
			if cfg.CheckTimeout != tc.wantTimeout {
				t.Errorf("CheckTimeout = %v, want %v", cfg.CheckTimeout, tc.wantTimeout)
			}
			if cfg.Priority != tc.wantPriority {
				t.Errorf("Priority = %d, want %d", cfg.Priority, tc.wantPriority)
			}
		})
	}
}

// TestLoadFromConfig_WithMinimalEndpoints verifies that endpoints omitting timing fields
// in YAML (which arrive as zero values) successfully load and pass validation.
// This exercises the applyEndpointDefaults → validateEndpointConfig path end-to-end.
func TestLoadFromConfig_WithMinimalEndpoints(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		cfg       config.EndpointConfig
		expectErr bool
	}{
		{
			name: "omitted check_interval and check_timeout receive defaults",
			cfg: config.EndpointConfig{
				Name: "minimal-ollama",
				URL:  "http://localhost:11434",
				Type: "ollama",
				// CheckInterval and CheckTimeout left at zero — applyEndpointDefaults must fill them
			},
			expectErr: false,
		},
		{
			name: "omitted priority receives default",
			cfg: config.EndpointConfig{
				Name:          "no-priority",
				URL:           "http://localhost:11434",
				Type:          "ollama",
				CheckInterval: 5 * time.Second,
				CheckTimeout:  2 * time.Second,
				// Priority left at zero
			},
			expectErr: false,
		},
		{
			name: "all timing fields omitted, auto type",
			cfg: config.EndpointConfig{
				Name: "bare-auto",
				URL:  "http://localhost:8080",
				Type: "auto",
				// All timing/priority fields at zero
			},
			expectErr: false,
		},
		{
			name: "all timing fields omitted, no type",
			cfg: config.EndpointConfig{
				Name: "bare-no-type",
				URL:  "http://localhost:8080",
				// All timing/priority fields and type at zero/empty
			},
			expectErr: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			repo := NewStaticEndpointRepository()
			err := repo.LoadFromConfig(context.Background(), []config.EndpointConfig{tc.cfg})

			if tc.expectErr && err == nil {
				t.Error("expected error but got none")
			}
			if !tc.expectErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			if !tc.expectErr && err == nil {
				// Confirm the endpoint was actually registered with the default timing values
				endpoints, getErr := repo.GetAll(context.Background())
				if getErr != nil {
					t.Fatalf("GetAll failed: %v", getErr)
				}
				if len(endpoints) != 1 {
					t.Fatalf("expected 1 endpoint, got %d", len(endpoints))
				}
				ep := endpoints[0]
				if ep.CheckInterval != DefaultCheckInterval {
					t.Errorf("CheckInterval = %v, want default %v", ep.CheckInterval, DefaultCheckInterval)
				}
				if ep.CheckTimeout != DefaultCheckTimeout {
					t.Errorf("CheckTimeout = %v, want default %v", ep.CheckTimeout, DefaultCheckTimeout)
				}
				if ep.Priority != DefaultPriority {
					t.Errorf("Priority = %d, want default %d", ep.Priority, DefaultPriority)
				}
			}
		})
	}
}

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

func TestEndpointConfigValidation_EmptyURLs(t *testing.T) {
	// Test that validation accepts empty health_check_url and model_url when they can get defaults
	testCases := []struct {
		name      string
		config    config.EndpointConfig
		expectErr bool
	}{
		{
			name: "empty health_check_url with known type is valid",
			config: config.EndpointConfig{
				Name:           "test-ollama",
				URL:            "http://localhost:11434",
				Type:           "ollama",
				HealthCheckURL: "",
				ModelURL:       "/api/tags",
				CheckInterval:  5 * time.Second,
				CheckTimeout:   2 * time.Second,
			},
			expectErr: false,
		},
		{
			name: "empty model_url with known type is valid",
			config: config.EndpointConfig{
				Name:           "test-ollama",
				URL:            "http://localhost:11434",
				Type:           "ollama",
				HealthCheckURL: "/",
				ModelURL:       "",
				CheckInterval:  5 * time.Second,
				CheckTimeout:   2 * time.Second,
			},
			expectErr: false,
		},
		{
			name: "both URLs empty with known type is valid",
			config: config.EndpointConfig{
				Name:           "test-ollama",
				URL:            "http://localhost:11434",
				Type:           "ollama",
				HealthCheckURL: "",
				ModelURL:       "",
				CheckInterval:  5 * time.Second,
				CheckTimeout:   2 * time.Second,
			},
			expectErr: false,
		},
		{
			name: "both URLs empty with auto type is valid",
			config: config.EndpointConfig{
				Name:           "test-auto",
				URL:            "http://localhost:11434",
				Type:           "auto",
				HealthCheckURL: "",
				ModelURL:       "",
				CheckInterval:  5 * time.Second,
				CheckTimeout:   2 * time.Second,
			},
			expectErr: false,
		},
		{
			name: "both URLs empty with empty type is valid",
			config: config.EndpointConfig{
				Name:           "test-no-type",
				URL:            "http://localhost:11434",
				Type:           "",
				HealthCheckURL: "",
				ModelURL:       "",
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

func TestStaticEndpointRepository_ProfileFallback_HealthCheckURL(t *testing.T) {
	// Test that empty HealthCheckURL gets populated from profile defaults
	repo := NewStaticEndpointRepository()
	ctx := context.Background()

	configs := []config.EndpointConfig{
		{
			Name:           "ollama-no-healthcheck",
			URL:            "http://localhost:11434",
			Type:           "ollama",
			HealthCheckURL: "", // Empty - should fall back to profile default "/"
			ModelURL:       "/api/tags",
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

	// Ollama profile default health check path is "/"
	expectedHealthURL := "http://localhost:11434/"
	if endpoint.HealthCheckURLString != expectedHealthURL {
		t.Errorf("HealthCheckURLString = %q, expected %q (profile default for ollama)", endpoint.HealthCheckURLString, expectedHealthURL)
	}
}

func TestStaticEndpointRepository_ProfileFallback_ModelURL(t *testing.T) {
	// Test that empty ModelURL gets populated from profile defaults
	repo := NewStaticEndpointRepository()
	ctx := context.Background()

	configs := []config.EndpointConfig{
		{
			Name:           "ollama-no-modelurl",
			URL:            "http://localhost:11434",
			Type:           "ollama",
			HealthCheckURL: "/",
			ModelURL:       "", // Empty - should fall back to profile default "/api/tags"
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

	// Ollama profile default model discovery path is "/api/tags"
	expectedModelURL := "http://localhost:11434/api/tags"
	if endpoint.ModelURLString != expectedModelURL {
		t.Errorf("ModelURLString = %q, expected %q (profile default for ollama)", endpoint.ModelURLString, expectedModelURL)
	}
}

func TestStaticEndpointRepository_ProfileFallback_BothURLsEmpty(t *testing.T) {
	// Test that both empty URLs get populated from profile defaults
	repo := NewStaticEndpointRepository()
	ctx := context.Background()

	configs := []config.EndpointConfig{
		{
			Name:           "ollama-no-urls",
			URL:            "http://localhost:11434",
			Type:           "ollama",
			HealthCheckURL: "", // Should fall back to "/"
			ModelURL:       "", // Should fall back to "/api/tags"
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

	// Verify both URLs got profile defaults
	expectedHealthURL := "http://localhost:11434/"
	if endpoint.HealthCheckURLString != expectedHealthURL {
		t.Errorf("HealthCheckURLString = %q, expected %q", endpoint.HealthCheckURLString, expectedHealthURL)
	}

	expectedModelURL := "http://localhost:11434/api/tags"
	if endpoint.ModelURLString != expectedModelURL {
		t.Errorf("ModelURLString = %q, expected %q", endpoint.ModelURLString, expectedModelURL)
	}
}

func TestStaticEndpointRepository_AutoType_EmptyURLs(t *testing.T) {
	// Test that "auto" type with empty URLs gets sensible defaults
	repo := NewStaticEndpointRepository()
	ctx := context.Background()

	configs := []config.EndpointConfig{
		{
			Name:           "auto-endpoint",
			URL:            "http://localhost:8080",
			Type:           "auto",
			HealthCheckURL: "", // Should fall back to "/" (default)
			ModelURL:       "", // Should fall back to "/v1/models" (default)
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

	// For "auto" type, should get sensible defaults since no specific profile
	// Default health check path is "/"
	expectedHealthURL := "http://localhost:8080/"
	if endpoint.HealthCheckURLString != expectedHealthURL {
		t.Errorf("HealthCheckURLString = %q, expected %q (default fallback)", endpoint.HealthCheckURLString, expectedHealthURL)
	}

	// Default model URL is "/v1/models"
	expectedModelURL := "http://localhost:8080/v1/models"
	if endpoint.ModelURLString != expectedModelURL {
		t.Errorf("ModelURLString = %q, expected %q (default fallback)", endpoint.ModelURLString, expectedModelURL)
	}
}

func TestStaticEndpointRepository_LMStudio_ProfileFallback(t *testing.T) {
	// Test that lm-studio profile defaults work correctly
	repo := NewStaticEndpointRepository()
	ctx := context.Background()

	configs := []config.EndpointConfig{
		{
			Name:           "lmstudio-no-urls",
			URL:            "http://localhost:1234",
			Type:           "lm-studio",
			HealthCheckURL: "", // Should fall back to profile default "/v1/models"
			ModelURL:       "", // Should fall back to profile default "/api/v0/models"
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

	// LM Studio profile uses /api/v0/models for model discovery (from lmstudio.yaml)
	expectedModelURL := "http://localhost:1234/api/v0/models"
	if endpoint.ModelURLString != expectedModelURL {
		t.Errorf("ModelURLString = %q, expected %q (lm-studio profile default)", endpoint.ModelURLString, expectedModelURL)
	}

	// LM Studio profile uses /v1/models for health check
	expectedHealthURL := "http://localhost:1234/v1/models"
	if endpoint.HealthCheckURLString != expectedHealthURL {
		t.Errorf("HealthCheckURLString = %q, expected %q (lm-studio profile default)", endpoint.HealthCheckURLString, expectedHealthURL)
	}
}

func TestStaticEndpointRepository_EmptyURLs_WithNestedPath(t *testing.T) {
	// Test that empty URLs with nested base paths work correctly
	repo := NewStaticEndpointRepository()
	ctx := context.Background()

	configs := []config.EndpointConfig{
		{
			Name:           "nested-ollama",
			URL:            "http://localhost:12434/engines/ollama/",
			Type:           "ollama",
			HealthCheckURL: "", // Should fall back to "/" (ollama default)
			ModelURL:       "", // Should fall back to "/api/tags" (ollama default)
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

	// Verify URLs preserve the nested path prefix from base URL
	// Note: path.Join normalises paths and removes trailing slashes
	// so "/" gets joined with "/engines/ollama/" to become "/engines/ollama"
	expectedHealthURL := "http://localhost:12434/engines/ollama"
	if endpoint.HealthCheckURLString != expectedHealthURL {
		t.Errorf("HealthCheckURLString = %q, expected %q", endpoint.HealthCheckURLString, expectedHealthURL)
	}

	expectedModelURL := "http://localhost:12434/engines/ollama/api/tags"
	if endpoint.ModelURLString != expectedModelURL {
		t.Errorf("ModelURLString = %q, expected %q", endpoint.ModelURLString, expectedModelURL)
	}
}
