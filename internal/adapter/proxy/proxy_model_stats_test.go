package proxy

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/thushan/olla/internal/core/constants"
	"github.com/thushan/olla/internal/core/domain"
)

// TestAllProxies_ModelStatsPopulateAfterTraffic is the integration-style
// regression test for RecordModelRequest's dead-since-ed93e9a callers:
// /internal/stats/models was permanently empty because nothing ever called
// StatsCollector.RecordModelRequest in production. Both engines must now
// populate model-level stats for a request whose resolved model name
// (stats.Model, as set by the handler layer before dispatch) is non-empty.
func TestAllProxies_ModelStatsPopulateAfterTraffic(t *testing.T) {
	suites := []ProxyTestSuite{
		SherpaTestSuite{},
		OllaTestSuite{},
	}

	for _, suite := range suites {
		t.Run(suite.Name(), func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set(constants.HeaderContentType, constants.ContentTypeJSON)
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(`{"status": "ok"}`))
			}))
			defer upstream.Close()

			endpoint := createTestEndpoint("test", upstream.URL, domain.StatusHealthy)
			proxy, selector, collector := createTestProxyComponents(suite, []*domain.Endpoint{endpoint})
			selector.endpoint = endpoint

			req, stats, rlog := createTestRequestWithBody("POST", "/api/chat", `{"model":"llama3.2"}`)
			stats.Model = "llama3.2"

			w, err := executeProxyRequest(proxy, req, stats, rlog)
			assertProxySuccess(t, w, err, stats, http.StatusOK, `{"status": "ok"}`)

			modelStats := collector.GetModelStats()
			ms, ok := modelStats["llama3.2"]
			if !ok {
				t.Fatalf("expected /internal/stats/models data for llama3.2 after proxy traffic, got %v", modelStats)
			}
			if ms.TotalRequests != 1 || ms.SuccessfulRequests != 1 {
				t.Errorf("expected 1 total/successful model request, got %+v", ms)
			}
		})
	}
}

// TestAllProxies_ModelStatsUseResolvedNameNotAlias is the regression test
// for review finding #2: stats.Model is set from the client-facing alias
// (handler_proxy.go), and RewriteModelForAlias only rewrites the outbound
// request body, not stats.Model - so model-level stats used to fragment
// under whatever alias name each client sent, rather than aggregating under
// the single backend model actually serving the traffic. Decision: record
// ONLY the resolved (backend) name, not both alias and resolved - a single
// source of truth for /internal/stats/models, consistent with how the
// unified model registry already treats aliases as different names for the
// same served model.
func TestAllProxies_ModelStatsUseResolvedNameNotAlias(t *testing.T) {
	suites := []ProxyTestSuite{
		SherpaTestSuite{},
		OllaTestSuite{},
	}

	const (
		clientAlias    = "my-alias"
		resolvedModel  = "llama3.2-actual"
		requestPayload = `{"model":"my-alias"}`
	)

	for _, suite := range suites {
		t.Run(suite.Name(), func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set(constants.HeaderContentType, constants.ContentTypeJSON)
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(`{"status": "ok"}`))
			}))
			defer upstream.Close()

			endpoint := createTestEndpoint("test", upstream.URL, domain.StatusHealthy)
			proxy, selector, collector := createTestProxyComponents(suite, []*domain.Endpoint{endpoint})
			selector.endpoint = endpoint

			req, stats, rlog := createTestRequestWithBody("POST", "/api/chat", requestPayload)
			// stats.Model is what the handler layer sets from the raw client
			// request before dispatch - the alias, not the resolved name.
			stats.Model = clientAlias

			aliasMap := map[string]string{endpoint.URLString: resolvedModel}
			ctx := context.WithValue(req.Context(), constants.ContextModelAliasMapKey, aliasMap)
			req = req.WithContext(ctx)

			w, err := executeProxyRequest(proxy, req, stats, rlog)
			assertProxySuccess(t, w, err, stats, http.StatusOK, `{"status": "ok"}`)

			modelStats := collector.GetModelStats()
			if _, ok := modelStats[clientAlias]; ok {
				t.Errorf("model stats must not be recorded under the client alias %q, got %v", clientAlias, modelStats)
			}
			ms, ok := modelStats[resolvedModel]
			if !ok {
				t.Fatalf("expected model stats under the resolved backend model %q, got %v", resolvedModel, modelStats)
			}
			if ms.TotalRequests != 1 || ms.SuccessfulRequests != 1 {
				t.Errorf("expected 1 total/successful request under the resolved model, got %+v", ms)
			}
		})
	}
}

// TestAllProxies_NoModelNameSkipsModelStats guards the modelName != "" gate
// end-to-end: a request with no resolved model must not create a spurious ""
// entry in model-level stats.
func TestAllProxies_NoModelNameSkipsModelStats(t *testing.T) {
	suites := []ProxyTestSuite{
		SherpaTestSuite{},
		OllaTestSuite{},
	}

	for _, suite := range suites {
		t.Run(suite.Name(), func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(`{"status": "ok"}`))
			}))
			defer upstream.Close()

			endpoint := createTestEndpoint("test", upstream.URL, domain.StatusHealthy)
			proxy, selector, collector := createTestProxyComponents(suite, []*domain.Endpoint{endpoint})
			selector.endpoint = endpoint

			req, stats, rlog := createTestRequestWithBody("GET", "/api/test", "")
			// stats.Model deliberately left empty.

			w, err := executeProxyRequest(proxy, req, stats, rlog)
			assertProxySuccess(t, w, err, stats, http.StatusOK, "")

			if _, ok := collector.GetModelStats()[""]; ok {
				t.Error("empty model name must not create a model-stats entry")
			}
		})
	}
}
