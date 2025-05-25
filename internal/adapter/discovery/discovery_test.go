package discovery

import (
	"context"
	"testing"
	"time"

	"github.com/thushan/olla/internal/config"
	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/logger"
)

// Simple mock health checker
type mockHealthChecker struct{}

func (m *mockHealthChecker) Check(ctx context.Context, endpoint *domain.Endpoint) (domain.HealthCheckResult, error) {
	return domain.HealthCheckResult{Status: domain.StatusHealthy}, nil
}

func (m *mockHealthChecker) StartChecking(ctx context.Context) error {
	return nil
}

func (m *mockHealthChecker) StopChecking(ctx context.Context) error {
	return nil
}

func createSimpleLogger() *logger.StyledLogger {
	loggerCfg := &logger.Config{Level: "error", Theme: "default"}
	_, styledLogger, _, _ := logger.NewWithTheme(loggerCfg)
	return styledLogger
}

func TestStaticDiscoveryService_BasicOperations(t *testing.T) {
	repo := NewStaticEndpointRepository()
	checker := &mockHealthChecker{}
	logger := createSimpleLogger()

	cfg := &config.Config{
		Discovery: config.DiscoveryConfig{
			Static: config.StaticDiscoveryConfig{
				Endpoints: []config.EndpointConfig{
					{
						Name:           "simple-test",
						URL:            "http://localhost:11434",
						Priority:       100,
						HealthCheckURL: "http://localhost:11434/health",
						ModelURL:       "http://localhost:11434/api/tags",
						CheckInterval:  5 * time.Second,
						CheckTimeout:   2 * time.Second,
					},
				},
			},
		},
	}

	service := NewStaticDiscoveryService(repo, checker, cfg, logger)
	ctx := context.Background()

	err := service.RefreshEndpoints(ctx)
	if err != nil {
		t.Fatalf("RefreshEndpoints failed: %v", err)
	}

	endpoints, err := service.GetEndpoints(ctx)
	if err != nil {
		t.Fatalf("GetEndpoints failed: %v", err)
	}

	if len(endpoints) != 1 {
		t.Errorf("Expected 1 endpoint, got %d", len(endpoints))
	}

	if endpoints[0].Name != "simple-test" {
		t.Errorf("Expected name 'simple-test', got '%s'", endpoints[0].Name)
	}

	if endpoints[0].Priority != 100 {
		t.Errorf("Expected priority 100, got %d", endpoints[0].Priority)
	}
}

func TestStaticDiscoveryService_GetHealthyEndpointsWithFallback(t *testing.T) {
	repo := NewStaticEndpointRepository()
	checker := &mockHealthChecker{}
	logger := createSimpleLogger()

	cfg := &config.Config{
		Discovery: config.DiscoveryConfig{
			Static: config.StaticDiscoveryConfig{
				Endpoints: []config.EndpointConfig{
					{
						Name:           "fallback-test",
						URL:            "http://localhost:11434",
						Priority:       100,
						HealthCheckURL: "http://localhost:11434/health",
						ModelURL:       "http://localhost:11434/api/tags",
						CheckInterval:  5 * time.Second,
						CheckTimeout:   2 * time.Second,
					},
				},
			},
		},
	}

	service := NewStaticDiscoveryService(repo, checker, cfg, logger)
	ctx := context.Background()

	service.RefreshEndpoints(ctx)

	// Test fallback when no routable endpoints
	endpoints, _ := service.GetEndpoints(ctx)
	for _, ep := range endpoints {
		ep.Status = domain.StatusOffline
		repo.UpdateEndpoint(ctx, ep)
	}

	fallback, err := service.GetHealthyEndpointsWithFallback(ctx)
	if err != nil {
		t.Fatalf("GetHealthyEndpointsWithFallback failed: %v", err)
	}

	if len(fallback) != 1 {
		t.Errorf("Expected fallback to return 1 endpoint, got %d", len(fallback))
	}

	// Test with healthy endpoints
	endpoints, _ = service.GetEndpoints(ctx)
	for _, ep := range endpoints {
		ep.Status = domain.StatusHealthy
		repo.UpdateEndpoint(ctx, ep)
	}

	healthy, err := service.GetHealthyEndpointsWithFallback(ctx)
	if err != nil {
		t.Fatalf("GetHealthyEndpointsWithFallback with healthy failed: %v", err)
	}

	if len(healthy) != 1 {
		t.Errorf("Expected 1 healthy endpoint, got %d", len(healthy))
	}
}

func TestStaticDiscoveryService_StartAndStop(t *testing.T) {
	repo := NewStaticEndpointRepository()
	checker := &mockHealthChecker{}
	logger := createSimpleLogger()

	cfg := &config.Config{
		Discovery: config.DiscoveryConfig{
			Static: config.StaticDiscoveryConfig{
				Endpoints: []config.EndpointConfig{
					{
						Name:           "start-stop-test",
						URL:            "http://localhost:11434",
						Priority:       100,
						HealthCheckURL: "http://localhost:11434/health",
						ModelURL:       "http://localhost:11434/api/tags",
						CheckInterval:  5 * time.Second,
						CheckTimeout:   2 * time.Second,
					},
				},
			},
		},
	}

	service := NewStaticDiscoveryService(repo, checker, cfg, logger)
	ctx := context.Background()

	err := service.Start(ctx)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Verify endpoint was loaded
	endpoints, err := service.GetEndpoints(ctx)
	if err != nil {
		t.Fatalf("GetEndpoints after start failed: %v", err)
	}

	if len(endpoints) != 1 {
		t.Errorf("Expected 1 endpoint after start, got %d", len(endpoints))
	}

	err = service.Stop(ctx)
	if err != nil {
		t.Fatalf("Stop failed: %v", err)
	}
}

func TestValidateEndpointConfig(t *testing.T) {
	tests := []struct {
		name      string
		config    config.EndpointConfig
		shouldErr bool
	}{
		{
			name: "valid config",
			config: config.EndpointConfig{
				URL:            "http://localhost:11434",
				HealthCheckURL: "http://localhost:11434/health",
				ModelURL:       "http://localhost:11434/api/tags",
				CheckInterval:  5 * time.Second,
				CheckTimeout:   2 * time.Second,
				Priority:       100,
			},
			shouldErr: false,
		},
		{
			name: "empty URL",
			config: config.EndpointConfig{
				URL:            "",
				HealthCheckURL: "http://localhost:11434/health",
				ModelURL:       "http://localhost:11434/api/tags",
				CheckInterval:  5 * time.Second,
				CheckTimeout:   2 * time.Second,
			},
			shouldErr: true,
		},
		{
			name: "timeout too long",
			config: config.EndpointConfig{
				URL:            "http://localhost:11434",
				HealthCheckURL: "http://localhost:11434/health",
				ModelURL:       "http://localhost:11434/api/tags",
				CheckInterval:  2 * time.Second,
				CheckTimeout:   5 * time.Second, // Greater than interv
			},
			shouldErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateEndpointConfig(tt.config)
			if tt.shouldErr && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.shouldErr && err != nil {
				t.Errorf("Expected no error but got: %v", err)
			}
		})
	}
}
