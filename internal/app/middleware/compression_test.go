package middleware

import (
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// largeBody is comfortably above gzipMinSize so the threshold check does not
// short-circuit the compression decision in these tests.
const largeBody = `{"endpoints":[{"name":"alpha","status":"healthy","url":"http://localhost:11434"},{"name":"beta","status":"healthy","url":"http://localhost:11435"},{"name":"gamma","status":"healthy","url":"http://localhost:11436"}],"total_count":3,"healthy_count":3,"routable_count":3}`

func writeJSONOK(w http.ResponseWriter, body string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, body)
}

// TestGzip_CompressesJSONWhenNegotiated confirms the happy path: a client that
// advertises Accept-Encoding: gzip receives a gzipped body and the
// Content-Encoding header that lets it know to decode.
func TestGzip_CompressesJSONWhenNegotiated(t *testing.T) {
	t.Parallel()

	h := GzipFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSONOK(w, largeBody)
	})

	req := httptest.NewRequest(http.MethodGet, "/internal/status", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	rec := httptest.NewRecorder()
	h(rec, req)

	assert.Equal(t, "gzip", rec.Header().Get("Content-Encoding"))
	assert.Contains(t, rec.Header().Get("Vary"), "Accept-Encoding")

	gr, err := gzip.NewReader(rec.Body)
	require.NoError(t, err)
	body, err := io.ReadAll(gr)
	require.NoError(t, err)
	assert.Equal(t, largeBody, string(body))
}

// TestGzip_PassesThroughWithoutAcceptEncoding confirms the middleware is a
// no-op when the client did not negotiate gzip - no Content-Encoding header
// and the body is delivered verbatim.
func TestGzip_PassesThroughWithoutAcceptEncoding(t *testing.T) {
	t.Parallel()

	h := GzipFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSONOK(w, largeBody)
	})

	req := httptest.NewRequest(http.MethodGet, "/internal/status", nil)
	rec := httptest.NewRecorder()
	h(rec, req)

	assert.Empty(t, rec.Header().Get("Content-Encoding"))
	assert.Equal(t, largeBody, rec.Body.String())
}

// TestGzip_QZeroDeclines confirms RFC 7231 semantics: an explicit
// "gzip;q=0" forbids gzip and must not be compressed, even though gzip is
// named in the header.
func TestGzip_QZeroDeclines(t *testing.T) {
	t.Parallel()

	h := GzipFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSONOK(w, largeBody)
	})

	req := httptest.NewRequest(http.MethodGet, "/internal/status", nil)
	req.Header.Set("Accept-Encoding", "gzip;q=0")
	rec := httptest.NewRecorder()
	h(rec, req)

	assert.Empty(t, rec.Header().Get("Content-Encoding"))
	assert.Equal(t, largeBody, rec.Body.String())
}

// TestGzip_304CarriesNoContentEncoding is the critical correctness property:
// a 304 Not Modified has no body and so must never carry Content-Encoding.
// This is the interaction B1 (ETag/304) relies on at the boundary.
func TestGzip_304CarriesNoContentEncoding(t *testing.T) {
	t.Parallel()

	h := GzipFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("ETag", `"abc"`)
		w.WriteHeader(http.StatusNotModified)
	})

	req := httptest.NewRequest(http.MethodGet, "/internal/status", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	req.Header.Set("If-None-Match", `"abc"`)
	rec := httptest.NewRecorder()
	h(rec, req)

	assert.Equal(t, http.StatusNotModified, rec.Code)
	assert.Empty(t, rec.Header().Get("Content-Encoding"))
	assert.Empty(t, rec.Body.Bytes())
}

// TestGzip_SkipsEventStream confirms SSE responses pass through uncompressed
// even when gzip was negotiated. Buffering SSE would break streaming chat.
func TestGzip_SkipsEventStream(t *testing.T) {
	t.Parallel()

	h := GzipFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "data: hello\n\n")
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/chat", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	rec := httptest.NewRecorder()
	h(rec, req)

	assert.Empty(t, rec.Header().Get("Content-Encoding"))
	assert.Equal(t, "data: hello\n\n", rec.Body.String())
}

// TestGzip_SkipsSmallBody confirms bodies below gzipMinSize pass through
// uncompressed, since the gzip framing overhead would exceed the savings.
func TestGzip_SkipsSmallBody(t *testing.T) {
	t.Parallel()

	const tiny = `{"ok":true}`
	h := GzipFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSONOK(w, tiny)
	})

	req := httptest.NewRequest(http.MethodGet, "/internal/status", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	rec := httptest.NewRecorder()
	h(rec, req)

	assert.Empty(t, rec.Header().Get("Content-Encoding"))
	assert.Equal(t, tiny, rec.Body.String())
}

// TestGzip_PreservesFlusher confirms the wrapped writer still satisfies
// http.Flusher so a handler that calls Flush() during streaming is not
// silently broken by being wrapped.
func TestGzip_PreservesFlusher(t *testing.T) {
	t.Parallel()

	flushed := make(chan struct{}, 1)
	h := GzipFunc(func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		require.True(t, ok, "wrapped writer must implement http.Flusher")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, strings.Repeat("x", 1024))
		flusher.Flush()
		flushed <- struct{}{}
	})

	req := httptest.NewRequest(http.MethodGet, "/internal/status", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	rec := httptest.NewRecorder()
	h(rec, req)

	<-flushed
	assert.Equal(t, "gzip", rec.Header().Get("Content-Encoding"))
}

// TestAcceptsGzip_Parsing covers the Accept-Encoding parser table-wise so the
// q-value semantics stay explicit and locked.
func TestAcceptsGzip_Parsing(t *testing.T) {
	t.Parallel()

	cases := map[string]bool{
		"":                  false,
		"identity":          false,
		"gzip":              true,
		"gzip, deflate":     true,
		"deflate, gzip":     true,
		"gzip;q=1.0":        true,
		"gzip;q=0":          false,
		"gzip;q=0.0":        false,
		"*":                 true,
		"*;q=0":             false,
		"identity, *;q=0":   false,
		"br":                false,
		"  gzip  ":          true,
		"gzip;q=0.001":      true,
	}
	for header, want := range cases {
		assert.Equal(t, want, acceptsGzip(header), "acceptsGzip(%q)", header)
	}
}
