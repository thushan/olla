package dashboard

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestInternalAPIMiddleware_PassesThroughWhenFlagOff pins the default: with no
// gate, the middleware is the identity function, so a client the policy would
// reject still reaches the handler. That is what keeps existing scrapes from
// outside allowed_cidrs working after this change.
func TestInternalAPIMiddleware_PassesThroughWhenFlagOff(t *testing.T) {
	t.Parallel()

	cfg := loopbackConfig()
	if cfg.GateInternalAPI {
		t.Fatal("loopbackConfig must leave the gate off")
	}

	reached := false
	h := InternalAPIMiddleware(cfg, stubLogger{}, nil)(reachedHandler(&reached))

	req := httptest.NewRequest(http.MethodGet, "/internal/status", nil)
	req.RemoteAddr = "203.0.113.9:54321"
	req.Host = "evil.example"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK || !reached {
		t.Fatalf("gate off: status=%d reached=%v, want 200 and the handler run", rec.Code, reached)
	}
}

// TestInternalAPIMiddleware_GatesWithoutDashboard proves the policy does not
// depend on dashboard.enabled: the gate alone is enough to refuse a
// non-allowed client and to let a loopback one through.
func TestInternalAPIMiddleware_GatesWithoutDashboard(t *testing.T) {
	t.Parallel()

	cfg := loopbackConfig()
	cfg.Enabled = false
	cfg.GateInternalAPI = true
	if err := cfg.Validate(); err != nil {
		t.Fatalf("validate gate cfg: %v", err)
	}

	mw := InternalAPIMiddleware(cfg, stubLogger{}, nil)

	reached := false
	denied := doRequest(t, mw(reachedHandler(&reached)), "203.0.113.9:54321", "evil.example", "", "")
	if denied.Code != http.StatusForbidden {
		t.Fatalf("non-allowed client: status=%d, want 403", denied.Code)
	}
	if reached {
		t.Fatal("non-allowed client reached the wrapped handler")
	}

	allowed := doRequest(t, mw(reachedHandler(&reached)), "127.0.0.1:54321", "localhost", "", "")
	if allowed.Code != http.StatusOK || !reached {
		t.Fatalf("loopback client: status=%d reached=%v, want 200 and the handler run", allowed.Code, reached)
	}

	// An allowed source is not enough: the Host check still applies on the
	// gated internal API, same as on /internal/ui/.
	reached = false
	badHost := doRequest(t, mw(reachedHandler(&reached)), "127.0.0.1:54321", "evil.example", "", "")
	if badHost.Code != http.StatusForbidden || reached {
		t.Fatalf("allowed ip, disallowed host: status=%d reached=%v, want 403 and no handler", badHost.Code, reached)
	}
}
