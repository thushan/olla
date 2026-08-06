package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestStatusHandler_ETag304OnMatch confirms the dashboard's polling contract:
// a first request emits an ETag with a 200, the second request carrying that
// ETag as If-None-Match gets a bare 304 with no body and the same ETag echoed.
func TestStatusHandler_ETag304OnMatch(t *testing.T) {
	t.Parallel()

	app := seedWireShapeApp(t)

	req1 := httptest.NewRequest(http.MethodGet, "/internal/status", nil)
	w1 := httptest.NewRecorder()
	app.statusHandler(w1, req1)

	require.Equal(t, http.StatusOK, w1.Code)
	etag := w1.Header().Get("ETag")
	require.NotEmpty(t, etag, "first response must carry an ETag")
	assert.Contains(t, w1.Header().Get("Content-Type"), "application/json")
	assert.Equal(t, "private, no-cache", w1.Header().Get("Cache-Control"), "200 must not be shared-cacheable")

	req2 := httptest.NewRequest(http.MethodGet, "/internal/status", nil)
	req2.Header.Set("If-None-Match", etag)
	w2 := httptest.NewRecorder()
	app.statusHandler(w2, req2)

	assert.Equal(t, http.StatusNotModified, w2.Code)
	assert.Equal(t, etag, w2.Header().Get("ETag"))
	assert.Empty(t, w2.Body.Bytes(), "304 must carry no body")
	assert.Equal(t, "private, no-cache", w2.Header().Get("Cache-Control"), "304 must also carry Cache-Control")
}

// TestEndpointsStatusHandler_ETag304OnMatch is the same contract for the
// /internal/status/endpoints surface.
func TestEndpointsStatusHandler_ETag304OnMatch(t *testing.T) {
	t.Parallel()

	app := seedWireShapeApp(t)

	req1 := httptest.NewRequest(http.MethodGet, "/internal/status/endpoints", nil)
	w1 := httptest.NewRecorder()
	app.endpointsStatusHandler(w1, req1)

	require.Equal(t, http.StatusOK, w1.Code)
	etag := w1.Header().Get("ETag")
	require.NotEmpty(t, etag)
	assert.Equal(t, "private, no-cache", w1.Header().Get("Cache-Control"))

	req2 := httptest.NewRequest(http.MethodGet, "/internal/status/endpoints", nil)
	req2.Header.Set("If-None-Match", etag)
	w2 := httptest.NewRecorder()
	app.endpointsStatusHandler(w2, req2)

	assert.Equal(t, http.StatusNotModified, w2.Code)
	assert.Equal(t, etag, w2.Header().Get("ETag"))
	assert.Empty(t, w2.Body.Bytes())
	assert.Equal(t, "private, no-cache", w2.Header().Get("Cache-Control"))
}

// TestModelsStatusHandler_ETag304OnMatch is the same contract for the
// /internal/status/models surface.
func TestModelsStatusHandler_ETag304OnMatch(t *testing.T) {
	t.Parallel()

	app := seedWireShapeApp(t)

	req1 := httptest.NewRequest(http.MethodGet, "/internal/status/models", nil)
	w1 := httptest.NewRecorder()
	app.modelsStatusHandler(w1, req1)

	require.Equal(t, http.StatusOK, w1.Code)
	etag := w1.Header().Get("ETag")
	require.NotEmpty(t, etag)
	assert.Equal(t, "private, no-cache", w1.Header().Get("Cache-Control"))

	req2 := httptest.NewRequest(http.MethodGet, "/internal/status/models", nil)
	req2.Header.Set("If-None-Match", etag)
	w2 := httptest.NewRecorder()
	app.modelsStatusHandler(w2, req2)

	assert.Equal(t, http.StatusNotModified, w2.Code)
	assert.Equal(t, etag, w2.Header().Get("ETag"))
	assert.Empty(t, w2.Body.Bytes())
	assert.Equal(t, "private, no-cache", w2.Header().Get("Cache-Control"))
}

// TestStatusHandler_ETagMismatchReturns200 guards the inverse: a non-matching
// If-None-Match must always fall through to a full 200 response with a body.
func TestStatusHandler_ETagMismatchReturns200(t *testing.T) {
	t.Parallel()

	app := seedWireShapeApp(t)

	req := httptest.NewRequest(http.MethodGet, "/internal/status", nil)
	req.Header.Set("If-None-Match", `"stale-validator"`)
	w := httptest.NewRecorder()
	app.statusHandler(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.NotEmpty(t, w.Body.Bytes())
}

// TestStatusHandler_ETagIsWeakAndRoundTrips confirms the ETag the server emits
// is a weak validator (W/"..." prefix) and that a client echoing it verbatim
// still gets a 304. This is the dashboard polling contract under the new
// weak-validator wire format.
func TestStatusHandler_ETagIsWeakAndRoundTrips(t *testing.T) {
	t.Parallel()

	app := seedWireShapeApp(t)

	req1 := httptest.NewRequest(http.MethodGet, "/internal/status", nil)
	w1 := httptest.NewRecorder()
	app.statusHandler(w1, req1)
	require.Equal(t, http.StatusOK, w1.Code)

	etag := w1.Header().Get("ETag")
	require.NotEmpty(t, etag)
	assert.True(t, strings.HasPrefix(etag, `W/"`),
		"status ETag must be a weak validator (W/\"...\"), got: %q", etag)

	// Echo the W/"..." value back exactly as a client would.
	req2 := httptest.NewRequest(http.MethodGet, "/internal/status", nil)
	req2.Header.Set("If-None-Match", etag)
	w2 := httptest.NewRecorder()
	app.statusHandler(w2, req2)
	assert.Equal(t, http.StatusNotModified, w2.Code)

	// And confirm a hypothetical strong-form client validator with the same
	// opaque-tag also matches under RFC 7232 weak comparison.
	strong := strings.TrimPrefix(etag, "W/")
	req3 := httptest.NewRequest(http.MethodGet, "/internal/status", nil)
	req3.Header.Set("If-None-Match", strong)
	w3 := httptest.NewRecorder()
	app.statusHandler(w3, req3)
	assert.Equal(t, http.StatusNotModified, w3.Code,
		"strong If-None-Match with same opaque-tag must match weak current ETag")
}

// TestStatusResponse_ETagStableAcrossRelativeTimeChurn is the load-bearing
// property of the bespoke hash: only the stable fields feed it, so the ETag
// stays identical across two builds whose Timestamp and relative time-ago
// renderings (uptime, last_check, next_check) have moved. This is exactly the
// shape of two polls taken a second apart against unchanged endpoint state.
func TestStatusResponse_ETagStableAcrossRelativeTimeChurn(t *testing.T) {
	t.Parallel()

	now := time.Now()
	startTime := now.Add(-time.Minute)

	r1 := StatusResponse{
		Timestamp: now,
		Proxy:     ProxySummary{Engine: "olla", Profile: "ollama", Balancer: "priority"},
		System: SystemSummary{
			StartTime:    startTime,
			Status:       statusHealthy,
			EndpointsUp:  "1/1",
			SuccessRate:  "100.0%",
			AvgLatency:   "12ms",
			TotalTraffic: "1.5 MB",
			UptimeHuman:  "1m", // excluded relative string
			Version:      "0.0.29",
			Commit:       "abc",
		},
		Endpoints: []EndpointResponse{{
			ID:          "abc",
			Name:        "ep1",
			Status:      statusHealthy,
			URL:         "http://localhost:11434",
			SuccessRate: "100.0%",
			LastCheck:   "5s ago", // excluded
			NextCheck:   "in 25s", // excluded
		}},
	}

	// r2 shares no slice backing array with r1: a shallow copy would alias
	// Endpoints and let the relative-string mutation below leak into r1.
	r2 := r1
	r2.Endpoints = append([]EndpointResponse(nil), r1.Endpoints...)

	r2.Timestamp = now.Add(2 * time.Second)
	r2.System.UptimeHuman = "1m 2s"
	r2.Endpoints[0].LastCheck = "7s ago"
	r2.Endpoints[0].NextCheck = "in 23s"

	assert.Equal(t, hashStatusResponse(&r1), hashStatusResponse(&r2),
		"ETag must be stable when only the wall-clock-rendered strings move")
}

// TestEndpointStatusResponse_ETagStableAcrossRelativeTimeChurn is the same
// property for the endpoints status surface, exercising the LastModelSync and
// HealthCheck relative strings.
func TestEndpointStatusResponse_ETagStableAcrossRelativeTimeChurn(t *testing.T) {
	t.Parallel()

	checkAt1 := time.Now()
	nextAt1 := checkAt1.Add(30 * time.Second)
	syncAt1 := checkAt1.Add(-time.Minute)

	r1 := EndpointStatusResponse{
		Timestamp:     time.Now(),
		TotalCount:    1,
		HealthyCount:  1,
		RoutableCount: 1,
		Endpoints: []EndpointSummary{{
			ID:              "abc",
			Name:            "ep1",
			Type:            "ollama",
			Status:          healthyStatus,
			URL:             "http://localhost:11434",
			HealthCheck:     "3s ago",  // excluded
			LastModelSync:   "10s ago", // excluded
			HealthCheckAt:   &checkAt1,
			NextCheckAt:     &nextAt1,
			LastModelSyncAt: &syncAt1,
		}},
	}

	// A later health-check tick with no real change: HealthCheckAt/NextCheckAt
	// advance every 30s (see internal/adapter/health) and LastModelSyncAt
	// advances on its own periodic sync, none of which the dashboard needs to
	// invalidate its cache over - this is exactly the campaign-found ETag
	// churn-under-zero-traffic bug.
	checkAt2 := checkAt1.Add(30 * time.Second)
	nextAt2 := checkAt2.Add(30 * time.Second)
	syncAt2 := syncAt1.Add(time.Minute)

	r2 := r1
	r2.Endpoints = append([]EndpointSummary(nil), r1.Endpoints...)
	r2.Timestamp = r1.Timestamp.Add(time.Second)
	r2.Endpoints[0].HealthCheck = "4s ago"
	r2.Endpoints[0].LastModelSync = "11s ago"
	r2.Endpoints[0].HealthCheckAt = &checkAt2
	r2.Endpoints[0].NextCheckAt = &nextAt2
	r2.Endpoints[0].LastModelSyncAt = &syncAt2

	assert.Equal(t, hashEndpointStatusResponse(&r1), hashEndpointStatusResponse(&r2))
}

// TestEndpointStatusResponse_ETagChangesOnStatusFlip guards the inverse of
// the churn-stability test above: a genuine status change must still produce
// a different ETag so the dashboard doesn't cache a stale payload.
func TestEndpointStatusResponse_ETagChangesOnStatusFlip(t *testing.T) {
	t.Parallel()

	r1 := EndpointStatusResponse{
		TotalCount:   1,
		HealthyCount: 1,
		Endpoints: []EndpointSummary{{
			ID:     "abc",
			Name:   "ep1",
			Status: healthyStatus,
		}},
	}
	r2 := r1
	r2.Endpoints = append([]EndpointSummary(nil), r1.Endpoints...)
	r2.Endpoints[0].Status = "unhealthy"
	r2.HealthyCount = 0

	assert.NotEqual(t, hashEndpointStatusResponse(&r1), hashEndpointStatusResponse(&r2),
		"ETag must change when an endpoint's status actually flips")
}

// TestModelStatusResponse_ETagStableAcrossRelativeTimeChurn covers the models
// surface: the LastSeen relative string is excluded, LastSeenAt absolute is
// kept.
func TestModelStatusResponse_ETagStableAcrossRelativeTimeChurn(t *testing.T) {
	t.Parallel()

	lastSeenAt := time.Now().Add(-time.Minute)
	r1 := ModelStatusResponse{
		Timestamp:      time.Now(),
		TotalModels:    1,
		TotalFamilies:  1,
		TotalEndpoints: 1,
		ModelsByFamily: map[string][]string{"llama": {"llama3"}},
		RecentModels: []ModelSummary{{
			Name:       "llama3",
			Family:     "llama",
			LastSeen:   "1m ago", // excluded
			LastSeenAt: &lastSeenAt,
			Endpoints:  []string{"ep1"},
		}},
	}

	r2 := r1
	r2.RecentModels = append([]ModelSummary(nil), r1.RecentModels...)
	// ModelsByFamily is small; rebuild from scratch to avoid aliasing.
	r2.ModelsByFamily = map[string][]string{"llama": {"llama3"}}
	r2.Timestamp = r1.Timestamp.Add(time.Second)
	r2.RecentModels[0].LastSeen = "1m 1s ago"

	assert.Equal(t, hashModelStatusResponse(&r1), hashModelStatusResponse(&r2))
}

// TestStatusResponse_ETagChangesOnRealDataChange guards the inverse of
// stability: when a stable field actually moves (e.g. an endpoint goes
// unhealthy), the ETag must change so the dashboard sees a fresh payload.
func TestStatusResponse_ETagChangesOnRealDataChange(t *testing.T) {
	t.Parallel()

	r1 := StatusResponse{
		Endpoints: []EndpointResponse{{ID: "abc", Name: "ep1", Status: statusHealthy}},
	}
	r2 := r1
	r2.Endpoints = append([]EndpointResponse(nil), r1.Endpoints...)
	r2.Endpoints[0].Status = statusDegraded

	assert.NotEqual(t, hashStatusResponse(&r1), hashStatusResponse(&r2),
		"ETag must change when a stable field changes")
}

// TestEtagMatches_AcceptsStar confirms the universal "*" If-None-Match form.
func TestEtagMatches_AcceptsStar(t *testing.T) {
	t.Parallel()
	assert.True(t, etagMatches("*", `"anything"`))
	assert.False(t, etagMatches("", `"anything"`))
	assert.True(t, etagMatches(`"x", "y"`, `"y"`))
	assert.False(t, etagMatches(`"x"`, `"y"`))
}

// TestEtagMatches_WeakComparison locks in RFC 7232 weak comparison now that
// the status ETag is a weak validator. The opaque-tag is compared after
// stripping the W/ prefix from BOTH sides, so:
//   - client echoes our W/"abc" verbatim -> match (the dashboard polling case),
//   - client sends a strong "abc" against our weak W/"abc" -> match,
//   - client sends a different opaque-tag -> no match regardless of weak/strong.
//
// Strong-to-strong and weak-to-weak fall out of the same strip-then-compare.
func TestEtagMatches_WeakComparison(t *testing.T) {
	t.Parallel()

	const cur = `W/"abc"`
	assert.True(t, etagMatches(`W/"abc"`, cur), "weak-to-weak same tag must match")
	assert.True(t, etagMatches(`"abc"`, cur), "strong client against weak current must match under weak comparison")
	assert.True(t, etagMatches(`W/"abc", W/"zzz"`, cur), "weak match in a multi-validator list")
	assert.True(t, etagMatches(`"zzz", "abc"`, cur), "strong match in a multi-validator list")
	assert.False(t, etagMatches(`W/"zzz"`, cur), "different opaque-tag must not match")
	assert.False(t, etagMatches(`"zzz"`, cur), "different opaque-tag must not match even in strong form")
}

// TestEtagMatches_OversizedHeaderShortCircuits confirms an oversized
// If-None-Match is treated as no-match rather than parsed, so a multi-MB
// header cannot drive an unbounded split. The handler falls through to a full
// 200 instead. This complements the server-level MaxHeaderBytes cap with a
// handler-level bound that's independent of http.Server configuration.
func TestEtagMatches_OversizedHeaderShortCircuits(t *testing.T) {
	t.Parallel()

	cur := `W/"abc"`
	huge := `W/"` + strings.Repeat("a", 1<<16) + `"`
	assert.False(t, etagMatches(huge, cur), "oversized If-None-Match must not match")
	assert.False(t, etagMatches("", cur), "empty If-None-Match must not match")
}
