package core

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	proxyconfig "github.com/thushan/olla/internal/adapter/proxy/config"
	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/core/ports"
	"github.com/thushan/olla/internal/logger"
)

func newPolicyRetryHandler(t *testing.T, ds *testDiscoveryService, p proxyconfig.RetryPolicy) *RetryHandler {
	t.Helper()
	log, _, _ := logger.New(&logger.Config{Level: "error"})
	return NewRetryHandler(ds, logger.NewPlainStyledLogger(log), WithRetryPolicy(p))
}

// TestExecuteWithRetry_Policy drives the handler with a backend set where every
// endpoint but the last refuses the connection, so the attempt count shows
// exactly how much failover the policy allowed.
func TestExecuteWithRetry_Policy(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		policy       proxyconfig.RetryPolicy
		method       string
		endpoints    int
		wantAttempts int
		wantErr      bool
		wantDemoted  bool
	}{
		{name: "defaults fail over to every endpoint", method: http.MethodPost, endpoints: 3, wantAttempts: 3},
		{name: "disabled is one attempt", policy: proxyconfig.RetryPolicy{Disabled: true}, method: http.MethodGet, endpoints: 3, wantAttempts: 1, wantErr: true, wantDemoted: true},
		{name: "max_attempts caps", policy: proxyconfig.RetryPolicy{MaxAttempts: 2}, method: http.MethodGet, endpoints: 4, wantAttempts: 2, wantErr: true, wantDemoted: true},
		{name: "max_attempts above endpoint count is harmless", policy: proxyconfig.RetryPolicy{MaxAttempts: 10}, method: http.MethodGet, endpoints: 3, wantAttempts: 3},
		{name: "non_idempotent off stops POST", policy: proxyconfig.RetryPolicy{NoNonIdempotent: true}, method: http.MethodPost, endpoints: 3, wantAttempts: 1, wantErr: true, wantDemoted: true},
		{name: "non_idempotent off stops DELETE", policy: proxyconfig.RetryPolicy{NoNonIdempotent: true}, method: http.MethodDelete, endpoints: 3, wantAttempts: 1, wantErr: true, wantDemoted: true},
		{name: "non_idempotent off still fails over GET", policy: proxyconfig.RetryPolicy{NoNonIdempotent: true}, method: http.MethodGet, endpoints: 3, wantAttempts: 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ds := &testDiscoveryService{}
			h := newPolicyRetryHandler(t, ds, tt.policy)

			eps := make([]*domain.Endpoint, tt.endpoints)
			for i := range eps {
				eps[i] = namedEndpoint(string(rune('a' + i)))
			}

			attempts := 0
			proxyFunc := func(_ context.Context, w http.ResponseWriter, _ *http.Request, _ *domain.Endpoint, _ *ports.RequestStats) error {
				attempts++
				if attempts < tt.endpoints {
					return &connectionResetError{}
				}
				w.WriteHeader(http.StatusOK)
				return nil
			}

			req := httptest.NewRequest(tt.method, "/v1/chat", nil)
			err := h.ExecuteWithRetry(context.Background(), httptest.NewRecorder(), req, eps,
				&roundRobinSelector{}, &ports.RequestStats{}, proxyFunc)

			assert.Equal(t, tt.wantAttempts, attempts)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			// The connection genuinely failed, so health demotion happens whether
			// or not the request is allowed to fail over.
			if tt.wantDemoted {
				require.NotNil(t, ds.updatedEndpoint, "failed endpoint must still be demoted")
				assert.Equal(t, domain.StatusOffline, ds.updatedEndpoint.Status)
			}
		})
	}
}

// A circuit-breaker rejection never contacted the endpoint, so it must not spend
// the attempt budget: with retry disabled or a small max_attempts, an open
// breaker on one endpoint must not fail the request while a healthy peer exists.
func TestExecuteWithRetry_CBOpenDoesNotSpendBudget(t *testing.T) {
	t.Parallel()

	for name, policy := range map[string]proxyconfig.RetryPolicy{
		"disabled":       {Disabled: true},
		"max_attempts 1": {MaxAttempts: 1},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			ds := &testDiscoveryService{}
			h := newPolicyRetryHandler(t, ds, policy)

			var served []string
			proxyFunc := func(_ context.Context, w http.ResponseWriter, _ *http.Request, ep *domain.Endpoint, _ *ports.RequestStats) error {
				if ep.Name == "a" {
					return cbOpenErr(ep.Name)
				}
				served = append(served, ep.Name)
				w.WriteHeader(http.StatusOK)
				return nil
			}

			req := httptest.NewRequest(http.MethodPost, "/v1/chat", nil)
			err := h.ExecuteWithRetry(context.Background(), httptest.NewRecorder(), req,
				[]*domain.Endpoint{namedEndpoint("a"), namedEndpoint("b")}, &roundRobinSelector{}, &ports.RequestStats{}, proxyFunc)

			require.NoError(t, err)
			assert.Equal(t, []string{"b"}, served)
			assert.Nil(t, ds.updatedEndpoint, "CB-open must not demote health")
		})
	}
}

// When every endpoint is breaker-open the loop must still end, with the
// circuit-breaker exhaustion error and no demotion.
func TestExecuteWithRetry_AllCBOpen_TerminatesWithPolicyDisabled(t *testing.T) {
	t.Parallel()

	ds := &testDiscoveryService{}
	h := newPolicyRetryHandler(t, ds, proxyconfig.RetryPolicy{Disabled: true})

	attempts := 0
	proxyFunc := func(_ context.Context, _ http.ResponseWriter, _ *http.Request, ep *domain.Endpoint, _ *ports.RequestStats) error {
		attempts++
		return cbOpenErr(ep.Name)
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	err := h.ExecuteWithRetry(context.Background(), httptest.NewRecorder(), req,
		[]*domain.Endpoint{namedEndpoint("a"), namedEndpoint("b")}, &roundRobinSelector{}, &ports.RequestStats{}, proxyFunc)

	require.ErrorIs(t, err, ErrCircuitBreakerOpen)
	assert.Equal(t, 2, attempts)
	assert.Nil(t, ds.updatedEndpoint)
}

// A CB-open rejection never reached the backend, so non_idempotent=false must
// not stop a POST failing over past it.
func TestExecuteWithRetry_NonIdempotentOff_CBOpenStillFailsOver(t *testing.T) {
	t.Parallel()

	h := newPolicyRetryHandler(t, &testDiscoveryService{}, proxyconfig.RetryPolicy{NoNonIdempotent: true})

	attempts := 0
	proxyFunc := func(_ context.Context, w http.ResponseWriter, _ *http.Request, ep *domain.Endpoint, _ *ports.RequestStats) error {
		attempts++
		if attempts == 1 {
			return cbOpenErr(ep.Name)
		}
		w.WriteHeader(http.StatusOK)
		return nil
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/chat", nil)
	err := h.ExecuteWithRetry(context.Background(), httptest.NewRecorder(), req,
		[]*domain.Endpoint{namedEndpoint("a"), namedEndpoint("b")}, &roundRobinSelector{}, &ports.RequestStats{}, proxyFunc)

	require.NoError(t, err)
	assert.Equal(t, 2, attempts)
}
