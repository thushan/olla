package proxy

import (
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
