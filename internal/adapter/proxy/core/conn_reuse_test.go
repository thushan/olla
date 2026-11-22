package core

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConnReuseTrace_Classify(t *testing.T) {
	t.Parallel()

	wrapped := fmt.Errorf("Post: %w", io.EOF)
	tests := []struct {
		name      string
		reused    bool
		err       error
		wantStale bool
	}{
		{"EOF on reused connection", true, wrapped, true},
		{"EOF on fresh connection", false, wrapped, false},
		{"non-EOF on reused connection", true, io.ErrUnexpectedEOF, false},
		{"nil error", true, nil, false},
		{"server closed idle on reused connection", true, fmt.Errorf("Post: %w", errors.New("http: server closed idle connection")), true},
		{"server closed idle on fresh connection", false, errors.New("http: server closed idle connection"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			trace := &ConnReuseTrace{}
			trace.reused.Store(tt.reused)
			got := trace.Classify(tt.err)
			assert.Equal(t, tt.wantStale, IsStaleConnError(got))
			if tt.err != nil && tt.wantStale {
				assert.ErrorIs(t, got, tt.err, "classification must not hide the original error")
				assert.True(t, IsConnectionError(got))
			}
		})
	}
}

// The trace must observe the pool: a second request to the same server reuses
// the connection the first one left idle.
func TestTraceConnReuse_ObservesPoolReuse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()
	tr := &http.Transport{}
	defer tr.CloseIdleConnections()

	do := func() *ConnReuseTrace {
		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL, nil)
		require.NoError(t, err)
		req, trace := TraceConnReuse(req)
		resp, err := tr.RoundTrip(req)
		require.NoError(t, err)
		_, _ = io.Copy(io.Discard, resp.Body)
		require.NoError(t, resp.Body.Close())
		return trace
	}

	assert.False(t, do().reused.Load(), "first request dials fresh")
	assert.True(t, do().reused.Load(), "second request reuses the idle connection")
}

func TestPreserveRequestBody_SetsReplayableBody(t *testing.T) {
	t.Parallel()

	h := &RetryHandler{logger: createBaseTestLogger()}
	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("payload"))

	b, err := h.preserveRequestBody(r)
	require.NoError(t, err)
	require.NotNil(t, r.GetBody)

	for range 2 {
		rc, err := r.GetBody()
		require.NoError(t, err)
		got, _ := io.ReadAll(rc)
		assert.Equal(t, "payload", string(got))
	}
	assert.True(t, bytes.Equal(b, []byte("payload")))
}
