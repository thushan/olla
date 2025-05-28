package discovery

import (
	"context"
	"testing"
	"time"

	"github.com/thushan/olla/internal/config"
	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/logger"
)

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

	endpoints := []config.EndpointConfig{
		{
			Name:           "simple-test",
			URL:            "http://localhost:11434",
			Priority:       100,
			HealthCheckURL: "/health",
			ModelURL:       "/api/tags",
			CheckInterval:  5 * time.Second,
			CheckTimeout:   2 * time.Second,
		},
	}

	service := NewStaticDiscoveryService(repo, checker, endpoints, logger)
	ctx := context.Background()

	err := service.RefreshEndpoints(ctx)
	if err != nil {
		t.Fatalf("RefreshEndpoints failed: %v", err)
	}

	endpoints_result, err := service.GetEndpoints(ctx)
	if err != nil {
		t.Fatalf("GetEndpoints failed: %v", err)
	}

	if len(endpoints_result) != 1 {
		t.Errorf("Expected 1 endpoint, got %d", len(endpoints_result))
	}

	if endpoints_result[0].Name != "simple-test" {
		t.Errorf("Expected name 'simple-test', got '%s'", endpoints_result[0].Name)
	}

	if endpoints_result[0].Priority != 100 {
		t.Errorf("Expected priority 100, got %d", endpoints_result[0].Priority)
	}
}

func TestStaticDiscoveryService_GetHealthyEndpointsWithFallback(t *testing.T) {
	repo := NewStaticEndpointRepository()
	checker := &mockHealthChecker{}
	logger := createSimpleLogger()

	endpoints := []config.EndpointConfig{
		{
			Name:           "fallback-test",
			URL:            "http://localhost:11434",
			Priority:       100,
			HealthCheckURL: "/health",
			ModelURL:       "/api/tags",
			CheckInterval:  5 * time.Second,
			CheckTimeout:   2 * time.Second,
		},
	}

	service := NewStaticDiscoveryService(repo, checker, endpoints, logger)
	ctx := context.Background()

	service.RefreshEndpoints(ctx)

	endpoints_result, _ := service.GetEndpoints(ctx)
	for _, ep := range endpoints_result {
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

	endpoints_result, _ = service.GetEndpoints(ctx)
	for _, ep := range endpoints_result {
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

	endpoints := []config.EndpointConfig{
		{
			Name:           "start-stop-test",
			URL:            "http://localhost:11434",
			Priority:       100,
			HealthCheckURL: "/health",
			ModelURL:       "/api/tags",
			CheckInterval:  5 * time.Second,
			CheckTimeout:   2 * time.Second,
		},
	}

	service := NewStaticDiscoveryService(repo, checker, endpoints, logger)
	ctx := context.Background()

	err := service.Start(ctx)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	endpoints_result, err := service.GetEndpoints(ctx)
	if err != nil {
		t.Fatalf("GetEndpoints after start failed: %v", err)
	}

	if len(endpoints_result) != 1 {
		t.Errorf("Expected 1 endpoint after start, got %d", len(endpoints_result))
	}

	err = service.Stop(ctx)
	if err != nil {
		t.Fatalf("Stop failed: %v", err)
	}
}

func TestStaticDiscoveryService_EmptyEndpointHandling(t *testing.T) {
	repo := NewStaticEndpointRepository()
	checker := &mockHealthChecker{}
	logger := createSimpleLogger()

	endpoints := []config.EndpointConfig{}

	service := NewStaticDiscoveryService(repo, checker, endpoints, logger)
	ctx := context.Background()

	err := service.RefreshEndpoints(ctx)
	if err != nil {
		t.Fatalf("RefreshEndpoints with empty config failed: %v", err)
	}

	endpoints_result, err := service.GetEndpoints(ctx)
	if err != nil {
		t.Fatalf("GetEndpoints failed: %v", err)
	}

	if len(endpoints_result) != 0 {
		t.Errorf("Expected 0 endpoints, got %d", len(endpoints_result))
	}

	fallback, err := service.GetHealthyEndpointsWithFallback(ctx)
	if err == nil {
		t.Error("Expected error for no endpoints configured")
	}
	if fallback != nil {
		t.Error("Expected nil fallback for no endpoints")
	}
}

func TestStaticDiscoveryService_RapidStatusChanges(t *testing.T) {
	repo := NewStaticEndpointRepository()
	checker := &mockHealthChecker{}
	logger := createSimpleLogger()

	endpoints := []config.EndpointConfig{
		{
			Name:           "rapid-test",
			URL:            "http://localhost:11434",
			Priority:       100,
			HealthCheckURL: "/health",
			ModelURL:       "/api/tags",
			CheckInterval:  5 * time.Second,
			CheckTimeout:   2 * time.Second,
		},
	}

	service := NewStaticDiscoveryService(repo, checker, endpoints, logger)
	ctx := context.Background()

	service.RefreshEndpoints(ctx)
	endpoints_result, _ := service.GetEndpoints(ctx)
	endpoint := endpoints_result[0]

	statuses := []domain.EndpointStatus{
		domain.StatusHealthy,
		domain.StatusBusy,
		domain.StatusOffline,
		domain.StatusWarming,
		domain.StatusUnhealthy,
		domain.StatusHealthy,
	}

	for i, status := range statuses {
		endpoint.Status = status
		err := repo.UpdateEndpoint(ctx, endpoint)
		if err != nil {
			t.Fatalf("Status update %d failed: %v", i, err)
		}

		healthy, _ := service.GetHealthyEndpoints(ctx)
		routable, _ := service.GetRoutableEndpoints(ctx)

		expectedHealthy := 0
		if status == domain.StatusHealthy {
			expectedHealthy = 1
		}

		expectedRoutable := 0
		if status.IsRoutable() {
			expectedRoutable = 1
		}

		if len(healthy) != expectedHealthy {
			t.Errorf("Status %s: expected %d healthy, got %d", status, expectedHealthy, len(healthy))
		}

		if len(routable) != expectedRoutable {
			t.Errorf("Status %s: expected %d routable, got %d", status, expectedRoutable, len(routable))
		}
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
				Name:           "test",
				URL:            "http://localhost:11434",
				HealthCheckURL: "/health",
				ModelURL:       "/api/tags",
				CheckInterval:  5 * time.Second,
				CheckTimeout:   2 * time.Second,
				Priority:       100,
			},
			shouldErr: false,
		},
		{
			name: "empty URL",
			config: config.EndpointConfig{
				Name:           "test",
				URL:            "",
				HealthCheckURL: "/health",
				ModelURL:       "/api/tags",
				CheckInterval:  5 * time.Second,
				CheckTimeout:   2 * time.Second,
			},
			shouldErr: true,
		},
		{
			name: "empty health check URL",
			config: config.EndpointConfig{
				Name:           "test",
				URL:            "http://localhost:11434",
				HealthCheckURL: "",
				ModelURL:       "/api/tags",
				CheckInterval:  5 * time.Second,
				CheckTimeout:   2 * time.Second,
			},
			shouldErr: true,
		},
		{
			name: "timeout too long",
			config: config.EndpointConfig{
				Name:           "test",
				URL:            "http://localhost:11434",
				HealthCheckURL: "/health",
				ModelURL:       "/api/tags",
				CheckInterval:  2 * time.Second,
				CheckTimeout:   5 * time.Second,
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