package core

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thushan/olla/internal/logger"
)

type errorCounter struct{ errors atomic.Int32 }

func (h *errorCounter) Enabled(context.Context, slog.Level) bool { return true }
func (h *errorCounter) WithAttrs([]slog.Attr) slog.Handler       { return h }
func (h *errorCounter) WithGroup(string) slog.Handler            { return h }
func (h *errorCounter) Handle(_ context.Context, r slog.Record) error {
	if r.Level >= slog.LevelError {
		h.errors.Add(1)
	}
	return nil
}

// An oversized chunked body is answered 413 by the handler, which samples it. The retry
// layer must return the error without its own ERROR per request, while a genuine read
// failure still logs.
func TestPreserveRequestBody_MaxBytesErrorIsNotLogged(t *testing.T) {
	t.Parallel()

	counter := &errorCounter{}
	h := NewRetryHandler(nil, logger.NewPlainStyledLogger(slog.New(counter)))

	for range 100 {
		req := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(strings.Repeat("x", 64)))
		req.Body = http.MaxBytesReader(httptest.NewRecorder(), req.Body, 16)
		_, err := h.preserveRequestBody(req)
		var maxErr *http.MaxBytesError
		require.ErrorAs(t, err, &maxErr)
	}
	assert.Zero(t, counter.errors.Load())

	req := httptest.NewRequest(http.MethodPost, "/x", nil)
	req.Body = io.NopCloser(iotestErrReader{})
	_, err := h.preserveRequestBody(req)
	require.Error(t, err)
	assert.EqualValues(t, 1, counter.errors.Load(), "other read failures still log")
}

type iotestErrReader struct{}

func (iotestErrReader) Read([]byte) (int, error) { return 0, io.ErrClosedPipe }
