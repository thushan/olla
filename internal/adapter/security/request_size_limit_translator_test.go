package security

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/thushan/olla/internal/config"
)

// countingBody reports how many bytes the middleware pulled from a chunked body, so the tests
// assert the read is bounded rather than inferring it from memory use.
type countingBody struct {
	r io.Reader
	n atomic.Int64
}

func (c *countingBody) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n.Add(int64(n))
	return n, err
}

func (c *countingBody) Close() error { return nil }

func chunkedRequest(path string, size int) (*http.Request, *countingBody) {
	body := &countingBody{r: strings.NewReader(strings.Repeat("x", size))}
	req := httptest.NewRequest(http.MethodPost, path, body)
	req.ContentLength = -1
	return req, body
}

const (
	translatorServerCap = 1 << 20  // stands in for max_body_size
	translatorCap       = 16 << 10 // stands in for the translator's max_message_size
)

func newTranslatorChain(t *testing.T, reject RejectFunc, translatorLimit int64, handler http.Handler) http.Handler {
	t.Helper()
	sv := createTestSizeLimitValidator(config.ServerRequestLimits{MaxBodySize: translatorServerCap})
	adapters := &Adapters{SizeValidation: sv}
	return adapters.CreateTranslatorChainMiddleware(reject, translatorLimit)(handler)
}

func TestTranslatorChain_ChunkedBodyCappedAtTranslatorLimit(t *testing.T) {
	t.Parallel()

	reached := false
	next := http.HandlerFunc(func(http.ResponseWriter, *http.Request) { reached = true })
	var rejected int
	reject := func(w http.ResponseWriter, status int, _ string) {
		rejected = status
		w.WriteHeader(status)
	}
	handler := newTranslatorChain(t, reject, translatorCap, next)

	req, body := chunkedRequest("/olla/anthropic/v1/messages", translatorCap+1)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusRequestEntityTooLarge || rejected != http.StatusRequestEntityTooLarge {
		t.Fatalf("want 413 through reject, got %d (reject saw %d)", rr.Code, rejected)
	}
	if reached {
		t.Error("handler must not see a chunked body over the translator cap")
	}

	// A body far larger than the server cap must still stop near the translator cap. The
	// slack covers io.ReadAll's final chunk, not a second buffer's worth.
	req, body = chunkedRequest("/olla/anthropic/v1/messages", translatorServerCap-1)
	handler.ServeHTTP(httptest.NewRecorder(), req)
	if got := body.n.Load(); got > translatorCap+4096 {
		t.Errorf("read %d bytes of a chunked body; want it bounded near the translator cap %d", got, translatorCap)
	}
}

func TestTranslatorChain_ChunkedBodyUnderCapPasses(t *testing.T) {
	t.Parallel()

	var got int
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		got = len(b)
		w.WriteHeader(http.StatusOK)
	})
	handler := newTranslatorChain(t, nil, translatorCap, next)

	// Exactly at the cap is allowed, matching the handler's own maxBodySize+1 read.
	req, _ := chunkedRequest("/olla/anthropic/v1/messages", translatorCap)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK || got != translatorCap {
		t.Errorf("want 200 with %d bytes delivered, got %d with %d", translatorCap, rr.Code, got)
	}
}

// A translator cap above the server cap must not loosen the server cap.
func TestTranslatorChain_ServerCapStillWinsWhenLower(t *testing.T) {
	t.Parallel()

	next := http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Error("handler reached") })
	handler := newTranslatorChain(t, nil, 4*translatorServerCap, next)

	req, _ := chunkedRequest("/olla/anthropic/v1/messages", translatorServerCap+1)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("want 413, got %d", rr.Code)
	}
}

// Content-Length requests are judged against max_body_size only: the translator cap bounds
// the chunked drain and nothing else, so the handler still owns that rejection.
func TestTranslatorChain_ContentLengthUnchanged(t *testing.T) {
	t.Parallel()

	reached := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached = true
		n, _ := io.Copy(io.Discard, r.Body)
		if n != translatorCap+1 {
			t.Errorf("handler got %d bytes, want %d", n, translatorCap+1)
		}
	})
	handler := newTranslatorChain(t, nil, translatorCap, next)

	req := httptest.NewRequest(http.MethodPost, "/olla/anthropic/v1/messages", strings.NewReader(strings.Repeat("x", translatorCap+1)))
	handler.ServeHTTP(httptest.NewRecorder(), req)
	if !reached {
		t.Error("a Content-Length body between the translator and server caps must reach the handler")
	}

	next = http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Error("handler reached") })
	handler = newTranslatorChain(t, nil, translatorCap, next)
	req = httptest.NewRequest(http.MethodPost, "/olla/anthropic/v1/messages", strings.NewReader(strings.Repeat("x", translatorServerCap+1)))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("want 413 over the server cap, got %d", rr.Code)
	}
}

// Proxy routes stream: the chain must not buffer, and the translator cap does not apply.
func TestProxyChain_ChunkedBodyStillStreamed(t *testing.T) {
	t.Parallel()

	sv := createTestSizeLimitValidator(config.ServerRequestLimits{MaxBodySize: translatorServerCap})
	adapters := &Adapters{SizeValidation: sv}

	var seen int
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		seen = len(b)
	})
	req, body := chunkedRequest("/olla/proxy/", translatorCap*4)

	rr := httptest.NewRecorder()
	adapters.CreateChainMiddleware(nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if body.n.Load() != 0 {
			t.Errorf("proxy chain read %d bytes before the handler", body.n.Load())
		}
		next.ServeHTTP(w, r)
	})).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK || seen != translatorCap*4 {
		t.Errorf("proxy route above the translator cap: code %d, handler saw %d bytes", rr.Code, seen)
	}
}
