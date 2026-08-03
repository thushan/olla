package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thushan/olla/internal/app/middleware"
)

// TestStatusETag_GzipRequestReturnsBare304 is the critical B1xB2 interaction
// assertion: a gzip-negotiated request that hits the ETag path (its second
// sequential poll carries a matching If-None-Match) must return a BARE 304:
//
//   - status 304 Not Modified
//   - ETag header echoing the validator the client sent
//   - NO Content-Encoding header (a 304 has no body to compress)
//   - empty body
//
// This property is what keeps the dashboard's polling loop cheap: the poll
// round-trips a few hundred bytes for a 304 instead of the full gzipped
// payload, and no intermediary sees a Content-Encoding it would try to decode
// against a missing body. All three status surfaces are covered because they
// share the same writeJSONWithETag + middleware.Gzip composition.
func TestStatusETag_GzipRequestReturnsBare304(t *testing.T) {
	t.Parallel()

	app := seedWireShapeApp(t)
	mux := http.NewServeMux()
	mux.HandleFunc("/internal/status", middleware.GzipFunc(app.statusHandler))

	// First poll: negotiate gzip, capture the ETag.
	first := httptest.NewRequest(http.MethodGet, "/internal/status", nil)
	first.Header.Set("Accept-Encoding", "gzip")
	firstRec := httptest.NewRecorder()
	mux.ServeHTTP(firstRec, first)
	require.Equal(t, http.StatusOK, firstRec.Code)
	etag := firstRec.Header().Get("ETag")
	require.NotEmpty(t, etag, "first response must emit an ETag")
	require.Equal(t, "gzip", firstRec.Header().Get("Content-Encoding"),
		"first response must be gzipped when the client negotiates it")

	// Second poll: same ETag, still negotiating gzip. The server-side
	// If-None-Match match short-circuits to 304 BEFORE any body is written,
	// so the gzip writer is never engaged and no Content-Encoding leaks onto
	// the 304.
	second := httptest.NewRequest(http.MethodGet, "/internal/status", nil)
	second.Header.Set("Accept-Encoding", "gzip")
	second.Header.Set("If-None-Match", etag)
	secondRec := httptest.NewRecorder()
	mux.ServeHTTP(secondRec, second)

	assert.Equal(t, http.StatusNotModified, secondRec.Code)
	assert.Equal(t, etag, secondRec.Header().Get("ETag"))
	assert.Empty(t, secondRec.Header().Get("Content-Encoding"),
		"a 304 has no body and must never carry Content-Encoding")
	assert.Empty(t, secondRec.Body.Bytes(), "a 304 must carry no body")
}

// TestEndpointsStatusETag_GzipRequestReturnsBare304 is the same interaction
// for /internal/status/endpoints. Same composition, separate assertion so a
// regression in either handler's write path is independently visible.
func TestEndpointsStatusETag_GzipRequestReturnsBare304(t *testing.T) {
	t.Parallel()

	app := seedWireShapeApp(t)
	mux := http.NewServeMux()
	mux.HandleFunc("/internal/status/endpoints", middleware.GzipFunc(app.endpointsStatusHandler))

	first := httptest.NewRequest(http.MethodGet, "/internal/status/endpoints", nil)
	first.Header.Set("Accept-Encoding", "gzip")
	firstRec := httptest.NewRecorder()
	mux.ServeHTTP(firstRec, first)
	require.Equal(t, http.StatusOK, firstRec.Code)
	etag := firstRec.Header().Get("ETag")
	require.NotEmpty(t, etag)

	second := httptest.NewRequest(http.MethodGet, "/internal/status/endpoints", nil)
	second.Header.Set("Accept-Encoding", "gzip")
	second.Header.Set("If-None-Match", etag)
	secondRec := httptest.NewRecorder()
	mux.ServeHTTP(secondRec, second)

	assert.Equal(t, http.StatusNotModified, secondRec.Code)
	assert.Equal(t, etag, secondRec.Header().Get("ETag"))
	assert.Empty(t, secondRec.Header().Get("Content-Encoding"))
	assert.Empty(t, secondRec.Body.Bytes())
}

// TestModelsStatusETag_GzipRequestReturnsBare304 is the same interaction for
// /internal/status/models.
func TestModelsStatusETag_GzipRequestReturnsBare304(t *testing.T) {
	t.Parallel()

	app := seedWireShapeApp(t)
	mux := http.NewServeMux()
	mux.HandleFunc("/internal/status/models", middleware.GzipFunc(app.modelsStatusHandler))

	first := httptest.NewRequest(http.MethodGet, "/internal/status/models", nil)
	first.Header.Set("Accept-Encoding", "gzip")
	firstRec := httptest.NewRecorder()
	mux.ServeHTTP(firstRec, first)
	require.Equal(t, http.StatusOK, firstRec.Code)
	etag := firstRec.Header().Get("ETag")
	require.NotEmpty(t, etag)

	second := httptest.NewRequest(http.MethodGet, "/internal/status/models", nil)
	second.Header.Set("Accept-Encoding", "gzip")
	second.Header.Set("If-None-Match", etag)
	secondRec := httptest.NewRecorder()
	mux.ServeHTTP(secondRec, second)

	assert.Equal(t, http.StatusNotModified, secondRec.Code)
	assert.Equal(t, etag, secondRec.Header().Get("ETag"))
	assert.Empty(t, secondRec.Header().Get("Content-Encoding"))
	assert.Empty(t, secondRec.Body.Bytes())
}
