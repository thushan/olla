package proxy

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
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

// TestAllProxies_ModelledRequestCountedOnce is the regression test for the
// v0.0.29 double count: RecordSuccess fed both RecordRequest and
// RecordModelRequest, and the collector's RecordModelRequest called
// RecordRequest again, so every request naming a model doubled the proxy
// and per-endpoint totals and bytes.
func TestAllProxies_ModelledRequestCountedOnce(t *testing.T) {
	const body = `{"status": "ok"}`

	for _, suite := range []ProxyTestSuite{SherpaTestSuite{}, OllaTestSuite{}} {
		t.Run(suite.Name(), func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set(constants.HeaderContentType, constants.ContentTypeJSON)
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(body))
			}))
			defer upstream.Close()

			endpoint := createTestEndpoint("test", upstream.URL, domain.StatusHealthy)
			proxy, selector, collector := createTestProxyComponents(suite, []*domain.Endpoint{endpoint})
			selector.endpoint = endpoint

			req, stats, rlog := createTestRequestWithBody("POST", "/api/chat", `{"model":"llama3.2"}`)
			stats.Model = "llama3.2"

			w, err := executeProxyRequest(proxy, req, stats, rlog)
			assertProxySuccess(t, w, err, stats, http.StatusOK, body)

			ps := collector.GetProxyStats()
			if ps.TotalRequests != 1 || ps.SuccessfulRequests != 1 || ps.FailedRequests != 0 {
				t.Errorf("proxy stats: want 1 total/1 success/0 failed, got %+v", ps)
			}
			es, ok := collector.GetEndpointStats()[endpoint.URLString]
			if !ok {
				t.Fatalf("no endpoint stats for %s", endpoint.URLString)
			}
			if es.TotalRequests != 1 || es.SuccessfulRequests != 1 {
				t.Errorf("endpoint stats: want 1 total/1 success, got %+v", es)
			}
			if es.TotalBytes != int64(len(body)) {
				t.Errorf("endpoint bytes: want %d (bytes sent), got %d", len(body), es.TotalBytes)
			}
			if ms := collector.GetModelStats()["llama3.2"]; ms.TotalRequests != 1 || ms.TotalBytes != int64(len(body)) {
				t.Errorf("model stats: want 1 request/%d bytes, got %+v", len(body), ms)
			}
		})
	}
}

// TestAllProxies_Upstream5xxCountedAsFailure is the regression test for 5xx
// responses being counted as successes because the transport round trip
// worked. The client must still get the backend's status untouched and Olla
// must not fail over: only the counters change. 4xx (429 included) is the
// caller's problem and stays a success, matching the WARN rule on the
// "Request completed" line.
func TestAllProxies_Upstream5xxCountedAsFailure(t *testing.T) {
	cases := []struct {
		name        string
		status      int
		stream      bool
		wantFailure bool
	}{
		{"503 non-streaming", http.StatusServiceUnavailable, false, true},
		{"503 streaming", http.StatusServiceUnavailable, true, true},
		{"500 non-streaming", http.StatusInternalServerError, false, true},
		{"429 stays a success", http.StatusTooManyRequests, false, false},
		{"404 stays a success", http.StatusNotFound, false, false},
		{"200", http.StatusOK, false, false},
	}

	for _, suite := range []ProxyTestSuite{SherpaTestSuite{}, OllaTestSuite{}} {
		for _, tc := range cases {
			t.Run(suite.Name()+"/"+tc.name, func(t *testing.T) {
				var badHits, goodHits atomic.Int64
				bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					badHits.Add(1)
					if tc.stream {
						w.Header().Set(constants.HeaderContentType, "text/event-stream")
						w.WriteHeader(tc.status)
						for _, chunk := range []string{"data: {\"error\":\"overloaded\"}\n\n", "data: [DONE]\n\n"} {
							_, _ = w.Write([]byte(chunk))
							w.(http.Flusher).Flush()
						}
						return
					}
					w.Header().Set(constants.HeaderContentType, constants.ContentTypeJSON)
					w.WriteHeader(tc.status)
					_, _ = w.Write([]byte(`{"error":"backend says no"}`))
				}))
				defer bad.Close()
				good := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					goodHits.Add(1)
					w.WriteHeader(http.StatusOK)
				}))
				defer good.Close()

				badEp := createTestEndpoint("bad", bad.URL, domain.StatusHealthy)
				goodEp := createTestEndpoint("good", good.URL, domain.StatusHealthy)
				endpoints := []*domain.Endpoint{badEp, goodEp}
				proxy, _, collector := createTestProxyComponents(suite, endpoints)

				req, stats, rlog := createTestRequestWithBody("POST", "/v1/chat/completions", `{"model":"llama3.2"}`)
				stats.Model = "llama3.2"
				w := httptest.NewRecorder()
				err := proxy.ProxyRequestToEndpoints(req.Context(), w, req, endpoints, stats, rlog)

				if err != nil {
					t.Fatalf("a relayed backend status is not a proxy error, got %v", err)
				}
				if w.Code != tc.status {
					t.Errorf("client must get the backend status %d unchanged, got %d", tc.status, w.Code)
				}
				if badHits.Load() != 1 || goodHits.Load() != 0 {
					t.Errorf("no failover on a backend status: want bad=1 good=0, got bad=%d good=%d", badHits.Load(), goodHits.Load())
				}

				wantFailed, wantOK := int64(0), int64(1)
				if tc.wantFailure {
					wantFailed, wantOK = 1, 0
				}
				ps := collector.GetProxyStats()
				if ps.TotalRequests != 1 || ps.FailedRequests != wantFailed || ps.SuccessfulRequests != wantOK {
					t.Errorf("proxy stats: want 1 total/%d success/%d failed, got %+v", wantOK, wantFailed, ps)
				}
				es := collector.GetEndpointStats()[badEp.URLString]
				if es.TotalRequests != 1 || es.FailedRequests != wantFailed || es.SuccessfulRequests != wantOK {
					t.Errorf("endpoint stats: want 1 total/%d success/%d failed, got %+v", wantOK, wantFailed, es)
				}
				if es.TotalBytes != int64(w.Body.Len()) {
					t.Errorf("endpoint bytes: want %d (bytes relayed), got %d", w.Body.Len(), es.TotalBytes)
				}
				ms := collector.GetModelStats()["llama3.2"]
				if ms.TotalRequests != 1 || ms.FailedRequests != wantFailed {
					t.Errorf("model stats: want 1 total/%d failed, got %+v", wantFailed, ms)
				}
			})
		}
	}
}
