package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/thushan/olla/internal/adapter/inspector"
	"github.com/thushan/olla/internal/adapter/translator"
	"github.com/thushan/olla/internal/config"
	"github.com/thushan/olla/internal/core/constants"
	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/core/ports"
	"github.com/thushan/olla/internal/logger"
)

// A translator panic after bytes reached the client used to end with an error
// event and a clean EOF, which reads as a complete reply. It must now behave like
// a backend dying mid-stream: failure flagged, connection aborted.
func TestStreamingPanic_AfterWrite_AbortsConnection(t *testing.T) {
	t.Parallel()

	mockLogger := &mockStyledLogger{}
	panicTrans := &mockTranslator{
		name:                  "late-panic-translator",
		implementsErrorWriter: true,
		transformRequestFunc: func(ctx context.Context, r *http.Request) (*translator.TransformedRequest, error) {
			return &translator.TransformedRequest{
				OpenAIRequest: map[string]any{"model": "test-model", "stream": true},
				ModelName:     "test-model",
				IsStreaming:   true,
			}, nil
		},
		transformStreamingFunc: func(ctx context.Context, openaiStream io.Reader, w http.ResponseWriter, original *http.Request) error {
			_, _ = io.WriteString(w, "event: message_start\ndata: {}\n\n")
			_ = http.NewResponseController(w).Flush()
			panic("panic after first write")
		},
	}
	proxyService := &mockProxyService{
		proxyFunc: func(ctx context.Context, w http.ResponseWriter, r *http.Request, endpoints []*domain.Endpoint, stats *ports.RequestStats, rlog logger.StyledLogger) error {
			w.Header().Set(constants.HeaderContentType, "text/event-stream")
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, "data: {}\n\n")
			// Keep the backend side alive: the panic, not the backend, ends this
			// stream. The handler closes the pipe on panic, which fails this write.
			for {
				if _, err := io.WriteString(w, "data: {}\n\n"); err != nil {
					return err
				}
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(10 * time.Millisecond):
				}
			}
		},
	}
	app := &Application{
		logger:           mockLogger,
		proxyService:     proxyService,
		statsCollector:   &mockStatsCollector{},
		repository:       &mockEndpointRepository{},
		inspectorChain:   inspector.NewChain(mockLogger),
		profileFactory:   &mockProfileFactory{},
		discoveryService: &mockDiscoveryServiceForTranslation{},
		Config:           &config.Config{},
	}
	handler := app.translationHandler(panicTrans)

	var aborted atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				if err, ok := rec.(error); ok && errors.Is(err, http.ErrAbortHandler) {
					aborted.Store(true)
				}
				panic(rec)
			}
		}()
		handler.ServeHTTP(w, r)
	}))
	defer srv.Close()

	body, _ := json.Marshal(map[string]any{"model": "test-model", "stream": true})
	req, err := http.NewRequest(http.MethodPost, srv.URL+"/test", strings.NewReader(string(body)))
	require.NoError(t, err)
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	got, readErr := io.ReadAll(resp.Body)

	assert.Contains(t, string(got), "event: message_start", "output written before the panic must reach the client")
	assert.Contains(t, string(got), "event: error", "client must still get the error event")
	assert.Contains(t, string(got), "internal error during stream transformation")
	require.Error(t, readErr, "client saw a clean EOF after a translator panic")
	assert.True(t, aborted.Load(), "handler must end with http.ErrAbortHandler")
}

func TestHandleStreamingPanic_FlagsFailureAndAbort(t *testing.T) {
	t.Parallel()

	run := func(committed bool) (*proxyRequest, *httptest.ResponseRecorder) {
		mockLogger := &mockStyledLogger{}
		app := &Application{logger: mockLogger}
		trans := &mockTranslator{name: "t", implementsErrorWriter: true,
			writeErrorFunc: func(w http.ResponseWriter, err error, statusCode int) { w.WriteHeader(statusCode) }}
		pipeReader, pipeWriter := io.Pipe()
		proxyErrChan := make(chan error, 1)
		proxyErrChan <- nil
		rec := httptest.NewRecorder()
		cw := newCommittedResponseWriter(rec)
		cw.committed.Store(committed)
		pr := &proxyRequest{requestLogger: mockLogger}
		func() {
			defer app.handleStreamingPanic(cw, pipeReader, pipeWriter, proxyErrChan, cw, pr, trans)
			panic("boom")
		}()
		return pr, rec
	}

	pr, _ := run(true)
	assert.True(t, pr.hadError)
	assert.True(t, pr.abortStream, "committed response must be aborted")

	pr, rec := run(false)
	assert.True(t, pr.hadError)
	assert.False(t, pr.abortStream, "uncommitted response gets a proper 502, not an abort")
	assert.Equal(t, http.StatusBadGateway, rec.Code)
}
