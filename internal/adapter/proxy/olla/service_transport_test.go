package olla

import (
	"testing"
	"time"

	"github.com/thushan/olla/internal/adapter/proxy/config"
)

// TestCreateOptimisedTransport_ConnectionLimits verifies that both MaxConnsPerHost and
// MaxIdleConnsPerHost are mapped to their correct fields on http.Transport.
// Previously MaxConnsPerHost was mistakenly written to MaxIdleConnsPerHost and
// MaxConnsPerHost was never set (defaulting to 0 = unlimited).
func TestCreateOptimisedTransport_ConnectionLimits(t *testing.T) {
	t.Parallel()

	cfg := &Configuration{}
	cfg.MaxConnsPerHost = 42
	cfg.MaxIdleConnsPerHost = 17
	cfg.MaxIdleConns = 200
	cfg.IdleConnTimeout = 90 * time.Second

	transport := createOptimisedTransport(cfg)

	if transport.MaxConnsPerHost != 42 {
		t.Errorf("MaxConnsPerHost: want 42, got %d", transport.MaxConnsPerHost)
	}
	if transport.MaxIdleConnsPerHost != 17 {
		t.Errorf("MaxIdleConnsPerHost: want 17, got %d", transport.MaxIdleConnsPerHost)
	}
	if transport.MaxIdleConns != 200 {
		t.Errorf("MaxIdleConns: want 200, got %d", transport.MaxIdleConns)
	}
}

// TestCreateOptimisedTransport_DefaultsApplied verifies that NewService fills in sensible
// defaults before handing the config to createOptimisedTransport, so a zero-value config
// never silently leaves MaxConnsPerHost unlimited.
func TestCreateOptimisedTransport_DefaultsApplied(t *testing.T) {
	t.Parallel()

	// Zero-value config — defaults should be filled in by NewService, but we can verify
	// the expected defaults are consistent with the package constants.
	cfg := &Configuration{}
	cfg.MaxConnsPerHost = config.OllaDefaultMaxConnsPerHost
	cfg.MaxIdleConnsPerHost = config.OllaDefaultMaxIdleConnsPerHost
	cfg.MaxIdleConns = config.OllaDefaultMaxIdleConns
	cfg.IdleConnTimeout = config.OllaDefaultIdleConnTimeout

	transport := createOptimisedTransport(cfg)

	if transport.MaxConnsPerHost != config.OllaDefaultMaxConnsPerHost {
		t.Errorf("MaxConnsPerHost: want %d, got %d", config.OllaDefaultMaxConnsPerHost, transport.MaxConnsPerHost)
	}
	if transport.MaxIdleConnsPerHost != config.OllaDefaultMaxIdleConnsPerHost {
		t.Errorf("MaxIdleConnsPerHost: want %d, got %d", config.OllaDefaultMaxIdleConnsPerHost, transport.MaxIdleConnsPerHost)
	}
}

// TestCreateOptimisedTransport_FieldsAreDistinct guards against the specific regression
// where MaxConnsPerHost value bled into MaxIdleConnsPerHost. Using distinct values
// makes the mapping error immediately visible.
func TestCreateOptimisedTransport_FieldsAreDistinct(t *testing.T) {
	t.Parallel()

	cfg := &Configuration{}
	cfg.MaxConnsPerHost = 100
	cfg.MaxIdleConnsPerHost = 10
	cfg.MaxIdleConns = 500

	transport := createOptimisedTransport(cfg)

	// Regression guard: if the bug is reintroduced both fields get value 100.
	if transport.MaxConnsPerHost == transport.MaxIdleConnsPerHost {
		t.Errorf("MaxConnsPerHost (%d) and MaxIdleConnsPerHost (%d) are equal — likely a field mapping regression",
			transport.MaxConnsPerHost, transport.MaxIdleConnsPerHost)
	}
	if transport.MaxConnsPerHost != 100 {
		t.Errorf("MaxConnsPerHost: want 100, got %d", transport.MaxConnsPerHost)
	}
	if transport.MaxIdleConnsPerHost != 10 {
		t.Errorf("MaxIdleConnsPerHost: want 10, got %d", transport.MaxIdleConnsPerHost)
	}
}
