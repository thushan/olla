package core

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/thushan/olla/internal/app/middleware"
	"github.com/thushan/olla/internal/core/constants"
	"github.com/thushan/olla/pkg/eventbus"
)

// Characterisation tests for the context keys the typed-key rework retypes
// (docs/specs/go-1.27-modernise.md, section 6 key inventory). They pin the
// observable behaviour each key gates, not its spelling, so the retype lands
// with them green.

// The "stream" key has no writer in production: nothing on the request path
// ever installs it, so streaming decisions always come from the configured
// profile or the response content type. These rows pin that no-writer
// behaviour next to the branch a test-side writer gates, through the function
// both engines call per response.
func TestAutoDetectStreamingMode_StreamKeyNoProductionWriter(t *testing.T) {
	t.Parallel()

	t.Run("no writer streams known streaming formats", func(t *testing.T) {
		t.Parallel()
		resp := &http.Response{Header: http.Header{}}
		resp.Header.Set(constants.HeaderContentType, constants.ContentTypeEventStream)
		if !AutoDetectStreamingMode(t.Context(), resp, constants.ConfigurationProxyProfileAuto) {
			t.Error("SSE response must stream with no stream key in the context")
		}
	})

	t.Run("no writer buffers binary content", func(t *testing.T) {
		t.Parallel()
		resp := &http.Response{Header: http.Header{}}
		resp.Header.Set(constants.HeaderContentType, constants.ContentTypeImagePNG)
		if AutoDetectStreamingMode(t.Context(), resp, constants.ConfigurationProxyProfileAuto) {
			t.Error("binary response must buffer with no stream key in the context")
		}
	})

	t.Run("stream true forces streaming over binary content", func(t *testing.T) {
		t.Parallel()
		ctx := context.WithValue(t.Context(), constants.ContextKeyStream, true)
		resp := &http.Response{Header: http.Header{}}
		resp.Header.Set(constants.HeaderContentType, constants.ContentTypeImagePNG)
		if !AutoDetectStreamingMode(ctx, resp, constants.ConfigurationProxyProfileAuto) {
			t.Error("stream=true in the context must stream a binary response")
		}
	})

	t.Run("stream false leaves binary content buffered", func(t *testing.T) {
		t.Parallel()
		ctx := context.WithValue(t.Context(), constants.ContextKeyStream, false)
		resp := &http.Response{Header: http.Header{}}
		resp.Header.Set(constants.HeaderContentType, constants.ContentTypeImagePNG)
		if AutoDetectStreamingMode(ctx, resp, constants.ConfigurationProxyProfileAuto) {
			t.Error("stream=false must not override the binary-content buffering decision")
		}
	})
}

// The proxy event carries the request ID read from the context slot the proxy
// handler writes (handler_proxy.go setupRequestContext). The middleware
// package keeps a second, already-typed "request_id" key; the two are distinct
// context slots and are deliberately not merged, so only the proxy handler's
// slot feeds the event.
func TestRecordFailure_EventRequestIDComesFromProxyContextSlot(t *testing.T) {
	t.Parallel()

	publishAndReceive := func(t *testing.T, ctx context.Context) ProxyEvent {
		t.Helper()
		b := &BaseProxyComponents{EventBus: eventbus.New[ProxyEvent]()}
		events, cleanup := b.EventBus.Subscribe(t.Context())
		defer cleanup()

		b.RecordFailure(ctx, nil, "", time.Millisecond, errors.New("backend gone"))

		select {
		case ev := <-events:
			return ev
		case <-time.After(5 * time.Second):
			t.Fatal("no proxy event was published")
			return ProxyEvent{}
		}
	}

	t.Run("proxy request id feeds the event", func(t *testing.T) {
		t.Parallel()
		ctx := context.WithValue(t.Context(), constants.ContextRequestIdKey, "proxy-ctx-id")
		ev := publishAndReceive(t, ctx)
		if ev.RequestID != "proxy-ctx-id" {
			t.Errorf("event RequestID = %q, want the id from the proxy handler's context slot", ev.RequestID)
		}
	})

	t.Run("middleware request id does not feed the event", func(t *testing.T) {
		t.Parallel()
		ctx := context.WithValue(t.Context(), middleware.RequestIDKey, "middleware-id")
		ev := publishAndReceive(t, ctx)
		if ev.RequestID != "" {
			t.Errorf("event RequestID = %q, want empty: the middleware's request_id key is a separate slot and must not satisfy the proxy event read", ev.RequestID)
		}
	})
}
