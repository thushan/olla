package proxy

import (
	"testing"
	"time"

	"github.com/thushan/olla/internal/adapter/proxy/config"
	"github.com/thushan/olla/internal/adapter/proxy/olla"
	"github.com/thushan/olla/internal/adapter/proxy/sherpa"
)

// TestFactory_UnsetConfig_AppliesEngineDefaults is the regression test for the
// zero-check-defeated bug: the factory used to copy already-defaulted getter
// values (generic 8KiB / 60s) into the engine config before the engine's own
// NewService zero-checks ran, so Olla's 64KiB buffer default and its own
// read-timeout default never fired. Constructing through the real factory
// path with a bare, unset Configuration is the only way to catch this - unit
// tests against the engines directly bypass the factory copy entirely.
func TestFactory_UnsetConfig_AppliesEngineDefaults(t *testing.T) {
	logger := createTestLogger()
	collector := createTestStatsCollector()
	factory := NewFactory(collector, nil, logger)
	discovery := &mockDiscoveryService{}
	selector := newMockEndpointSelector(collector)

	t.Run("olla", func(t *testing.T) {
		svc, err := factory.Create(DefaultProxyOlla, discovery, selector, &Configuration{})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		ollaSvc, ok := svc.(*olla.Service)
		if !ok {
			t.Fatalf("expected *olla.Service, got %T", svc)
		}
		t.Cleanup(ollaSvc.Cleanup)
		effective := ollaSvc.Configuration()

		if effective.StreamBufferSize != config.OllaDefaultStreamBufferSize {
			t.Errorf("StreamBufferSize = %d, want OllaDefaultStreamBufferSize (%d)", effective.StreamBufferSize, config.OllaDefaultStreamBufferSize)
		}
		if effective.ReadTimeout != config.OllaDefaultReadTimeout {
			t.Errorf("ReadTimeout = %v, want OllaDefaultReadTimeout (%v)", effective.ReadTimeout, config.OllaDefaultReadTimeout)
		}
		if effective.ReadTimeout != 60*time.Second {
			t.Errorf("ReadTimeout = %v, want 60s (decided default)", effective.ReadTimeout)
		}
	})

	t.Run("sherpa", func(t *testing.T) {
		svc, err := factory.Create(DefaultProxySherpa, discovery, selector, &Configuration{})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		sherpaSvc, ok := svc.(*sherpa.Service)
		if !ok {
			t.Fatalf("expected *sherpa.Service, got %T", svc)
		}
		effective := sherpaSvc.Configuration()

		if effective.GetStreamBufferSize() != config.DefaultStreamBufferSize {
			t.Errorf("StreamBufferSize = %d, want DefaultStreamBufferSize (%d)", effective.GetStreamBufferSize(), config.DefaultStreamBufferSize)
		}
		if effective.GetReadTimeout() != config.DefaultReadTimeout {
			t.Errorf("ReadTimeout = %v, want DefaultReadTimeout (%v)", effective.GetReadTimeout(), config.DefaultReadTimeout)
		}
	})
}
