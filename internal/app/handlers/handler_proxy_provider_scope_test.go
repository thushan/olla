package handlers

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/thushan/olla/internal/core/domain"
)

// TestFilterEndpointsByProfile_FailClosedOnNoMatch is the unit-level guard
// for the provider-scoped cross-type leak: filterEndpointsByProfile's
// zero-compatible-endpoints fallback must not widen to all endpoints when the
// profile's SupportedBy represents a hard provider-type boundary
// (FailClosedOnNoMatch, set by createProviderProfile for a non-OpenAI
// provider-scoped route). A vllm-scoped request with only an
// openai-compatible endpoint healthy must get nothing back, not the
// wrong-type endpoint.
func TestFilterEndpointsByProfile_FailClosedOnNoMatch(t *testing.T) {
	styledLog := &mockStyledLogger{}
	app := &Application{logger: styledLog}

	openaiURL, _ := url.Parse("http://localhost:8000")
	endpoints := []*domain.Endpoint{{
		Name:      "openai-compat-1",
		URL:       openaiURL,
		URLString: openaiURL.String(),
		Type:      domain.ProfileOpenAICompatible,
		Status:    domain.StatusHealthy,
	}}

	profile := &domain.RequestProfile{
		SupportedBy:         []string{"vllm"},
		FailClosedOnNoMatch: true,
	}

	filtered := app.filterEndpointsByProfile(endpoints, profile, styledLog)
	if len(filtered) != 0 {
		t.Fatalf("expected zero endpoints when a provider-scoped profile has no type match, got %d: %v", len(filtered), filtered)
	}
}

// TestFilterEndpointsByProfile_WidensWhenNotFailClosed pins the inverse: a
// profile that does NOT set FailClosedOnNoMatch (the OpenAI-inclusive
// profile, and the unscoped /olla/proxy/ route's inspector-derived profile)
// keeps today's documented behaviour of widening to all endpoints when
// nothing matches, rather than failing the request.
func TestFilterEndpointsByProfile_WidensWhenNotFailClosed(t *testing.T) {
	styledLog := &mockStyledLogger{}
	app := &Application{logger: styledLog}

	vllmURL, _ := url.Parse("http://localhost:8001")
	endpoints := []*domain.Endpoint{{
		Name:      "vllm-1",
		URL:       vllmURL,
		URLString: vllmURL.String(),
		Type:      "vllm",
		Status:    domain.StatusHealthy,
	}}

	profile := &domain.RequestProfile{
		SupportedBy: []string{domain.ProfileOpenAICompatible},
		// FailClosedOnNoMatch deliberately left false.
	}

	filtered := app.filterEndpointsByProfile(endpoints, profile, styledLog)
	if len(filtered) != 1 {
		t.Fatalf("expected the inclusive profile to widen to the one available endpoint, got %d: %v", len(filtered), filtered)
	}
}

// TestProviderProxyHandler_VllmScopedFailsClosedOnCrossTypeOutage is the
// handler-level regression test for the campaign-found cross-type leak: a
// /olla/vllm/ request during a vllm outage, with only an openai-compatible
// endpoint healthy, must 404 as "No vllm endpoints available" - never be
// silently served by the wrong-type backend.
func TestProviderProxyHandler_VllmScopedFailsClosedOnCrossTypeOutage(t *testing.T) {
	app := createTestApplication(t)

	openaiURL, _ := url.Parse("http://localhost:8000")
	app.discoveryService = &mockDiscoveryServiceWithHealthy{
		endpoints: []*domain.Endpoint{{
			Name:      "openai-compat-1",
			URL:       openaiURL,
			URLString: openaiURL.String(),
			Type:      domain.ProfileOpenAICompatible,
			Status:    domain.StatusHealthy,
		}},
	}

	capture := &captureProxyService{}
	app.proxyService = capture

	req := httptest.NewRequest(http.MethodPost, "/olla/vllm/v1/chat/completions", strings.NewReader(`{"model":"llama3"}`))
	w := httptest.NewRecorder()

	app.providerProxyHandler(w, req)

	if capture.capturedCtx != nil {
		t.Fatal("proxy must never be invoked - the openai-compatible endpoint must not serve a vllm-scoped request")
	}
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d (body=%q)", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "No vllm endpoints available") {
		t.Errorf("expected the provider-specific rejection message, got %q", w.Body.String())
	}
}

// TestProviderProxyHandler_OpenAIRouteStaysInclusive confirms the OpenAI
// route's documented inclusive behaviour is untouched by the fail-closed
// fix: when no endpoint's type is in IsCompatibleWith's OpenAI-compatible
// allowlist, the request still widens to whatever is healthy rather than
// 404ing, because createProviderProfile never sets FailClosedOnNoMatch for
// the OpenAI branch.
func TestProviderProxyHandler_OpenAIRouteStaysInclusive(t *testing.T) {
	app := createTestApplication(t)

	vllmURL, _ := url.Parse("http://localhost:8001")
	app.discoveryService = &mockDiscoveryServiceWithHealthy{
		endpoints: []*domain.Endpoint{{
			Name:      "vllm-1",
			URL:       vllmURL,
			URLString: vllmURL.String(),
			Type:      "vllm",
			Status:    domain.StatusHealthy,
		}},
	}

	capture := &captureProxyService{}
	app.proxyService = capture

	req := httptest.NewRequest(http.MethodPost, "/olla/openai/v1/chat/completions", strings.NewReader(`{"model":"llama3"}`))
	w := httptest.NewRecorder()

	app.providerProxyHandler(w, req)

	if capture.capturedCtx == nil {
		t.Fatalf("expected the openai route to widen and proxy through the available endpoint, got status=%d body=%q", w.Code, w.Body.String())
	}
}
