package services

import (
	"context"
	"testing"

	"github.com/thushan/olla/internal/adapter/proxy"
	"github.com/thushan/olla/internal/adapter/proxy/olla"
	"github.com/thushan/olla/internal/adapter/proxy/sherpa"
	"github.com/thushan/olla/internal/adapter/stats"
	appconfig "github.com/thushan/olla/internal/config"
	"github.com/thushan/olla/internal/core/domain"
)

// noopDiscoveryService and noopEndpointSelector are the minimal stand-ins
// needed to construct a proxy.Factory service - these tests only assert on
// the effective configuration a constructed engine ends up with, never
// route a request.
type noopDiscoveryService struct{}

func (noopDiscoveryService) GetEndpoints(context.Context) ([]*domain.Endpoint, error) {
	return nil, nil
}
func (noopDiscoveryService) GetHealthyEndpoints(context.Context) ([]*domain.Endpoint, error) {
	return nil, nil
}
func (noopDiscoveryService) RefreshEndpoints(context.Context) error { return nil }
func (noopDiscoveryService) UpdateEndpointStatus(context.Context, *domain.Endpoint) error {
	return nil
}

type noopEndpointSelector struct{}

func (noopEndpointSelector) Select(context.Context, []*domain.Endpoint) (*domain.Endpoint, error) {
	return nil, nil
}
func (noopEndpointSelector) Name() string                          { return "noop" }
func (noopEndpointSelector) IncrementConnections(*domain.Endpoint) {}
func (noopEndpointSelector) DecrementConnections(*domain.Endpoint) {}

// TestProxyConfiguration_RealChain_OllaGetsEngineBufferDefault is the
// regression test for the real F1 bug: it wasn't enough for the factory to
// copy the raw (possibly zero) StreamBufferSize - config.DefaultConfig()
// hardcoded it to 8KiB, and both shipped config.yaml files also set it to
// 8192 explicitly. Either alone permanently defeats Olla's 64KiB zero-check
// no matter how correct the factory's copy logic is. This goes through the
// actual production chain - config.DefaultConfig / config.Load ->
// ProxyServiceWrapper.createProxyConfiguration -> proxy.Factory - not a bare
// struct, which is the only way a regression at either of those layers would
// be caught.
func TestProxyConfiguration_RealChain_OllaGetsEngineBufferDefault(t *testing.T) {
	logger := newTestLogger()
	collector := stats.NewCollector(logger)
	discovery := noopDiscoveryService{}
	selector := noopEndpointSelector{}

	buildOllaService := func(t *testing.T, cfg *appconfig.Config) *olla.Service {
		t.Helper()
		wrapper := NewProxyServiceWrapper(&cfg.Proxy, logger)
		proxyConfig := wrapper.createProxyConfiguration()

		factory := proxy.NewFactory(collector, nil, logger)
		svc, err := factory.Create(proxy.DefaultProxyOlla, discovery, selector, proxyConfig)
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		ollaSvc, ok := svc.(*olla.Service)
		if !ok {
			t.Fatalf("expected *olla.Service, got %T", svc)
		}
		return ollaSvc
	}

	t.Run("DefaultConfig", func(t *testing.T) {
		cfg := appconfig.DefaultConfig()
		if got := buildOllaService(t, cfg).Configuration().StreamBufferSize; got != 65536 {
			t.Errorf("StreamBufferSize = %d, want 65536 under DefaultConfig()", got)
		}
	})

	t.Run("shipped config/config.yaml", func(t *testing.T) {
		// The other half of the real bug: config/config.yaml (and the root
		// copy) used to set stream_buffer_size: 8192 explicitly, which alone
		// would defeat the zero-check regardless of DefaultConfig(). Load the
		// actual shipped file, not a fixture, so a regression there is caught
		// too.
		cfg, err := appconfig.Load("../../../config/config.yaml")
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if got := buildOllaService(t, cfg).Configuration().StreamBufferSize; got != 65536 {
			t.Errorf("StreamBufferSize = %d, want 65536 loading the shipped config/config.yaml", got)
		}
	})

	t.Run("no config file found", func(t *testing.T) {
		// Load()'s search paths are relative to cwd - chdir to an empty temp
		// dir so none of them can possibly exist, forcing the genuine
		// no-config-file fallback path rather than relying on the test
		// runner's ambient working directory not containing one.
		t.Chdir(t.TempDir())

		cfg, err := appconfig.Load()
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if got := buildOllaService(t, cfg).Configuration().StreamBufferSize; got != 65536 {
			t.Errorf("StreamBufferSize = %d, want 65536 with no config file present", got)
		}
	})
}

// TestProxyConfiguration_RealChain_SherpaGetsGenericBufferDefault verifies
// Sherpa still resolves its own (different) generic 8KiB default through the
// same real production chain, unaffected by the Olla-side fix.
func TestProxyConfiguration_RealChain_SherpaGetsGenericBufferDefault(t *testing.T) {
	logger := newTestLogger()
	collector := stats.NewCollector(logger)
	discovery := noopDiscoveryService{}
	selector := noopEndpointSelector{}

	cfg := appconfig.DefaultConfig()
	wrapper := NewProxyServiceWrapper(&cfg.Proxy, logger)
	proxyConfig := wrapper.createProxyConfiguration()

	factory := proxy.NewFactory(collector, nil, logger)
	svc, err := factory.Create(proxy.DefaultProxySherpa, discovery, selector, proxyConfig)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	sherpaSvc, ok := svc.(*sherpa.Service)
	if !ok {
		t.Fatalf("expected *sherpa.Service, got %T", svc)
	}
	if got := sherpaSvc.Configuration().GetStreamBufferSize(); got != 8192 {
		t.Errorf("StreamBufferSize = %d, want 8192 (Sherpa's generic default) under DefaultConfig", got)
	}
}
