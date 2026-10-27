package dashboard

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/thushan/olla/internal/logger"
	"github.com/thushan/olla/internal/router"
)

// warnRecorder embeds stubLogger and records Warn lines.
type warnRecorder struct {
	stubLogger
	mu    sync.Mutex
	lines []warnLine
}

type warnLine struct {
	attrs map[string]any
	msg   string
}

func (w *warnRecorder) Warn(msg string, args ...any) {
	l := warnLine{msg: msg, attrs: map[string]any{}}
	for i := 0; i+1 < len(args); i += 2 {
		l.attrs[args[i].(string)] = args[i+1]
	}
	w.mu.Lock()
	w.lines = append(w.lines, l)
	w.mu.Unlock()
}

func (w *warnRecorder) snapshot() []warnLine {
	w.mu.Lock()
	defer w.mu.Unlock()
	return append([]warnLine(nil), w.lines...)
}

// A blocked client can retry at wire speed: every request still gets the
// self-diagnosing 403, but the Warn is sampled to one line per client per window
// plus an exact summary of the rest.
func TestAccessMiddleware_RejectWarnIsThrottled(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		remoteAddr string
		host       string
	}{
		{"ip outside allowed range", "203.0.113.9:5555", "localhost"},
		{"host not accepted", "127.0.0.1:5555", "evil.example.com"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			now := time.Unix(1_700_000_000, 0)
			var mu sync.Mutex
			clock := func() time.Time { mu.Lock(); defer mu.Unlock(); return now }

			rec := &warnRecorder{}
			reached := false
			h := accessMiddleware(loopbackConfig(), rec, reachedHandler(&reached), newAccessWarnThrottle(rec, clock, nil))

			const total = 500
			for range total {
				req := httptest.NewRequest(http.MethodGet, "/internal/ui/", nil)
				req.RemoteAddr = tt.remoteAddr
				req.Host = tt.host
				w := httptest.NewRecorder()
				h.ServeHTTP(w, req)
				require.Equal(t, http.StatusForbidden, w.Code)
				require.Contains(t, w.Body.String(), "403 forbidden", "the response body is never sampled")
			}
			require.False(t, reached)
			require.Len(t, rec.snapshot(), 1, "one full Warn for the whole flood")
			require.Equal(t, "dashboard access denied", rec.snapshot()[0].msg)

			mu.Lock()
			now = now.Add(logger.DefaultThrottleWindow)
			mu.Unlock()
			req := httptest.NewRequest(http.MethodGet, "/internal/ui/", nil)
			req.RemoteAddr = tt.remoteAddr
			req.Host = tt.host
			h.ServeHTTP(httptest.NewRecorder(), req)

			lines := rec.snapshot()
			require.Len(t, lines, 3, "summary for window one, then a fresh full line")
			require.Equal(t, "dashboard access denied, repeats suppressed", lines[1].msg)
			require.Equal(t, "dashboard", lines[1].attrs["layer"])
			require.Equal(t, total-1, lines[1].attrs["suppressed"])
			require.Equal(t, "dashboard access denied", lines[2].msg)
		})
	}
}

// A flood that stops just before shutdown must still be reported: the set the
// routes register their shared throttle in flushes the tail.
func TestRegisterRoutes_ShutdownFlushReportsPendingSummary(t *testing.T) {
	t.Parallel()

	rec := &warnRecorder{}
	set := &logger.ThrottleSet{}
	reg := router.NewRouteRegistry(rec)
	require.True(t, RegisterRoutes(reg, loopbackConfig(), rec, http.NotFoundHandler(), set))

	mux := http.NewServeMux()
	reg.WireUp(mux)
	const total = 20
	// both routes share one throttle, so the redirect route's rejections join the same window
	for _, path := range []string{DashboardRoute, SlashlessDashboardRoute} {
		for range total / 2 {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			req.RemoteAddr = "203.0.113.9:5555"
			mux.ServeHTTP(httptest.NewRecorder(), req)
		}
	}
	require.Len(t, rec.snapshot(), 1, "one full Warn across both routes")

	set.Flush()
	lines := rec.snapshot()
	require.Len(t, lines, 2)
	require.Equal(t, "dashboard access denied, repeats suppressed", lines[1].msg)
	require.Equal(t, total-1, lines[1].attrs["suppressed"])
}
