package security

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/thushan/olla/internal/adapter/stats"
	"github.com/thushan/olla/internal/config"
	"github.com/thushan/olla/internal/core/constants"
	"github.com/thushan/olla/internal/core/ports"
	"github.com/thushan/olla/internal/logger"
)

// captureHandler records every log record so tests can count lines and read
// their attributes.
type captureHandler struct {
	mu      *sync.Mutex
	records *[]capturedRecord
}

type capturedRecord struct {
	attrs map[string]any
	msg   string
}

func newCaptureLogger() (logger.StyledLogger, func() []capturedRecord) {
	h := &captureHandler{mu: &sync.Mutex{}, records: &[]capturedRecord{}}
	snapshot := func() []capturedRecord {
		h.mu.Lock()
		defer h.mu.Unlock()
		return append([]capturedRecord(nil), *h.records...)
	}
	return logger.NewPlainStyledLogger(slog.New(h)), snapshot
}

func (h *captureHandler) Enabled(context.Context, slog.Level) bool { return true }
func (h *captureHandler) WithAttrs([]slog.Attr) slog.Handler       { return h }
func (h *captureHandler) WithGroup(string) slog.Handler            { return h }
func (h *captureHandler) Handle(_ context.Context, r slog.Record) error {
	rec := capturedRecord{msg: r.Message, attrs: map[string]any{}}
	r.Attrs(func(a slog.Attr) bool {
		rec.attrs[a.Key] = a.Value.Any()
		return true
	})
	h.mu.Lock()
	*h.records = append(*h.records, rec)
	h.mu.Unlock()
	return nil
}

func countExact(recs []capturedRecord, msg string) (n int) {
	for _, r := range recs {
		if r.msg == msg {
			n++
		}
	}
	return n
}

func countMsg(recs []capturedRecord, prefix string) (n int) {
	for _, r := range recs {
		if strings.HasPrefix(r.msg, prefix) {
			n++
		}
	}
	return n
}

type fakeClock struct {
	t  time.Time
	mu sync.Mutex
}

func (c *fakeClock) Now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

type countingMetrics struct {
	violations map[string]int
	mu         sync.Mutex
}

func (m *countingMetrics) RecordViolation(_ context.Context, v ports.SecurityViolation) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.violations == nil {
		m.violations = map[string]int{}
	}
	m.violations[v.ViolationType]++
	return nil
}
func (m *countingMetrics) GetMetrics(context.Context) (ports.SecurityMetrics, error) {
	return ports.SecurityMetrics{}, nil
}

func TestRateLimitMiddleware_FloodLogsBoundedAndSummarisesExactly(t *testing.T) {
	t.Parallel()

	log, records := newCaptureLogger()
	metrics := &countingMetrics{}
	v := NewRateLimitValidator(config.ServerRateLimits{
		PerIPRequestsPerMinute: 60,
		BurstSize:              5,
	}, metrics, log)
	clock := &fakeClock{t: time.Unix(1_700_000_000, 0)}
	v.rejectLog = logger.NewRejectThrottle(log, "Rate limit exceeded", "rate_limit", "client_ip", clock.Now)

	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	h := v.CreateMiddleware(nil)(next)

	const total = 10_000
	var allowed, limited int
	for range total {
		req := httptest.NewRequest(http.MethodPost, "/olla/proxy/v1/chat/completions", nil)
		req.RemoteAddr = "203.0.113.7:4321"
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		switch w.Code {
		case http.StatusOK:
			allowed++
		case http.StatusTooManyRequests:
			limited++
			require.NotEmpty(t, w.Header().Get("Retry-After"), "every 429 keeps its Retry-After")
		default:
			t.Fatalf("unexpected status %d", w.Code)
		}
	}
	require.Positive(t, limited)
	require.Equal(t, total, allowed+limited)

	// counters are the source of truth: every rejection is counted
	require.Equal(t, limited, metrics.violations[constants.ViolationRateLimit])

	require.Equal(t, 1, countExact(records(), "Rate limit exceeded"), "one full line for the whole flood")

	v.Stop() // flushes the tail of the window
	var suppressed int
	for _, r := range records() {
		if r.msg == "Rate limit exceeded, repeats suppressed" {
			require.Equal(t, "203.0.113.7", r.attrs["client_ip"])
			require.Equal(t, "rate_limit", r.attrs["layer"], "the summary names the layer that wrote it")
			suppressed += int(r.attrs["suppressed"].(int64))
		}
	}
	require.Equal(t, limited-1, suppressed, "suppressed count is exact")
}

func TestRateLimitMiddleware_WindowRolloverLogsNextFirstEventInFull(t *testing.T) {
	t.Parallel()

	log, records := newCaptureLogger()
	v := NewRateLimitValidator(config.ServerRateLimits{PerIPRequestsPerMinute: 60, BurstSize: 1}, nil, log)
	defer v.Stop()
	clock := &fakeClock{t: time.Unix(1_700_000_000, 0)}
	v.rejectLog = logger.NewRejectThrottle(log, "Rate limit exceeded", "rate_limit", "client_ip", clock.Now)
	h := v.CreateMiddleware(nil)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))

	hit := func() {
		req := httptest.NewRequest(http.MethodPost, "/x", nil)
		req.RemoteAddr = "198.51.100.9:1"
		h.ServeHTTP(httptest.NewRecorder(), req)
	}
	for range 50 {
		hit()
	}
	require.Equal(t, 1, countExact(records(), "Rate limit exceeded"))

	clock.Advance(logger.DefaultThrottleWindow)
	hit()
	recs := records()
	require.Equal(t, 2, countExact(recs, "Rate limit exceeded"), "a fresh full line opens the new window")
	require.Equal(t, 1, countExact(recs, "Rate limit exceeded, repeats suppressed"), "window one is summarised exactly once")
}

func TestRateLimitMiddleware_DistinctIPStormKeepsThrottleBounded(t *testing.T) {
	t.Parallel()

	log, records := newCaptureLogger()
	v := NewRateLimitValidator(config.ServerRateLimits{PerIPRequestsPerMinute: 1, BurstSize: 1}, nil, log)
	defer v.Stop()
	// frozen clock: a slow race runner must not be able to open a second window mid-storm
	clock := &fakeClock{t: time.Unix(1_700_000_000, 0)}
	v.rejectLog = logger.NewRejectThrottle(log, "Rate limit exceeded", "rate_limit", "client_ip", clock.Now)
	h := v.CreateMiddleware(nil)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))

	const total = 100_000
	const workers = 8
	var wg sync.WaitGroup
	for w := range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := w; i < total; i += workers {
				req := httptest.NewRequest(http.MethodPost, "/x", nil)
				req.RemoteAddr = fmt.Sprintf("10.%d.%d.%d:5000", i>>16&255, i>>8&255, i&255)
				// two hits per IP: the second is always limited
				h.ServeHTTP(httptest.NewRecorder(), req)
				h.ServeHTTP(httptest.NewRecorder(), req)
			}
		}()
	}
	wg.Wait()

	require.LessOrEqual(t, v.rejectLog.Len(), logger.DefaultThrottleMaxKeys)
	require.Equal(t, logger.DefaultThrottleMaxKeys+1, countExact(records(), "Rate limit exceeded"),
		"a storm of distinct IPs logs a bounded number of full lines")
}

func TestRateLimitValidator_StopFlushesAndLeaksNoGoroutines(t *testing.T) {
	log, _ := newCaptureLogger()
	before := runtime.NumGoroutine()
	for range 20 {
		v := NewRateLimitValidator(config.ServerRateLimits{
			PerIPRequestsPerMinute: 60, BurstSize: 1, CleanupInterval: time.Minute,
		}, nil, log)
		v.Stop()
	}
	// poll by hand: testify's Eventually runs its own goroutines, which would
	// inflate the count being measured
	deadline := time.Now().Add(2 * time.Second)
	for runtime.NumGoroutine() > before && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	require.LessOrEqual(t, runtime.NumGoroutine(), before, "Stop must end the cleanup goroutine; the throttle adds none")
}

func TestSizeValidator_RejectionLogIsThrottledPerClient(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		middleware func(*SizeValidator) func(http.Handler) http.Handler
	}{
		{"proxy chain", func(sv *SizeValidator) func(http.Handler) http.Handler { return sv.CreateMiddleware(nil) }},
		{"non-proxy chain", func(sv *SizeValidator) func(http.Handler) http.Handler { return sv.CreateNonProxyMiddleware(nil) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			log, records := newCaptureLogger()
			metrics := &countingMetrics{}
			sv := NewSizeValidator(config.ServerRequestLimits{MaxBodySize: 16, MaxHeaderSize: 1 << 20}, metrics, log)
			clock := &fakeClock{t: time.Unix(1_700_000_000, 0)}
			sv.rejectLog = logger.NewRejectThrottle(log, "Request rejected", "size_limit", "client_kind", clock.Now)
			h := tt.middleware(sv)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))

			const total = 2_000
			for range total {
				req := httptest.NewRequest(http.MethodPost, "/olla/proxy/v1/chat/completions", strings.NewReader(strings.Repeat("x", 64)))
				req.RemoteAddr = "203.0.113.50:9999"
				w := httptest.NewRecorder()
				h.ServeHTTP(w, req)
				require.Equal(t, http.StatusRequestEntityTooLarge, w.Code)
			}

			require.Equal(t, total, metrics.violations[constants.ViolationSizeLimit], "counters see every rejection")
			require.Equal(t, 1, countExact(records(), "Request rejected"))

			clock.Advance(logger.DefaultThrottleWindow)
			req := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(strings.Repeat("x", 64)))
			req.RemoteAddr = "203.0.113.50:9999"
			h.ServeHTTP(httptest.NewRecorder(), req)

			var suppressed int
			for _, r := range records() {
				if r.msg == "Request rejected, repeats suppressed" {
					require.Equal(t, "size_limit", r.attrs["layer"], "the proxy handler writes the same message from its own throttle")
					suppressed += int(r.attrs["suppressed"].(int64))
				}
			}
			require.Equal(t, total-1, suppressed)
		})
	}
}

// Shutdown stops the rate limiter and the size validator through Adapters.Stop; a
// size flood that ended just before it must still be summarised.
func TestAdaptersStop_FlushesSizeValidatorSummary(t *testing.T) {
	t.Parallel()

	log, records := newCaptureLogger()
	adapters := &Adapters{SizeValidation: NewSizeValidator(config.ServerRequestLimits{MaxBodySize: 16, MaxHeaderSize: 1 << 20}, &countingMetrics{}, log)}
	h := adapters.SizeValidation.CreateMiddleware(nil)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))

	const total = 10
	for range total {
		req := httptest.NewRequest(http.MethodPost, "/olla/proxy/v1/chat/completions", strings.NewReader(strings.Repeat("x", 64)))
		req.RemoteAddr = "203.0.113.50:9999"
		h.ServeHTTP(httptest.NewRecorder(), req)
	}
	require.Equal(t, 0, countExact(records(), "Request rejected, repeats suppressed"))

	adapters.Stop()
	require.Equal(t, 1, countExact(records(), "Request rejected, repeats suppressed"))
}

// The production metrics adapter used to WARN "Large request blocked" for every violation
// over 50 MiB, ahead of the size limiter's own sampling. That line is gone; the counters
// still see every rejection.
func TestSizeValidator_OversizedFloodWithProductionMetricsLogsOnce(t *testing.T) {
	t.Parallel()

	log, records := newCaptureLogger()
	collector := stats.NewCollector(log)
	sv := NewSizeValidator(config.ServerRequestLimits{MaxBodySize: 1 << 20, MaxHeaderSize: 1 << 20},
		NewSecurityMetricsAdapter(collector, log), log)
	clock := &fakeClock{t: time.Unix(1_700_000_000, 0)}
	sv.rejectLog = logger.NewRejectThrottle(log, "Request rejected", "size_limit", "client_kind", clock.Now)
	h := sv.CreateMiddleware(nil)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))

	const total = 200
	for range total {
		req := httptest.NewRequest(http.MethodPost, "/olla/proxy/v1/chat/completions", nil)
		req.ContentLength = 100*1024*1024 + 1
		req.RemoteAddr = "203.0.113.50:9999"
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		require.Equal(t, http.StatusRequestEntityTooLarge, w.Code)
	}

	require.Zero(t, countExact(records(), "Large request blocked"))
	require.Equal(t, 1, countExact(records(), "Request rejected"))
	require.EqualValues(t, total, collector.GetSecurityStats().SizeLimitViolations)
}

// A client that trips the body limit and then the header limit inside one window must
// still get a diagnostic for each: they are different causes.
func TestSizeValidator_BodyAndHeaderRejectionsSampleSeparately(t *testing.T) {
	t.Parallel()

	log, records := newCaptureLogger()
	sv := NewSizeValidator(config.ServerRequestLimits{MaxBodySize: 16, MaxHeaderSize: 4096}, &countingMetrics{}, log)
	clock := &fakeClock{t: time.Unix(1_700_000_000, 0)}
	sv.rejectLog = logger.NewRejectThrottle(log, "Request rejected", "size_limit", "client_kind", clock.Now)
	h := sv.CreateMiddleware(nil)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))

	send := func(mutate func(*http.Request)) {
		req := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(""))
		req.RemoteAddr = "203.0.113.50:9999"
		mutate(req)
		h.ServeHTTP(httptest.NewRecorder(), req)
	}
	for range 5 {
		send(func(r *http.Request) { r.ContentLength = 64 })
	}
	for range 5 {
		send(func(r *http.Request) { r.Header.Set("X-Pad", strings.Repeat("p", 8192)) })
	}

	var reasons []string
	for _, r := range records() {
		if r.msg == "Request rejected" {
			reasons = append(reasons, r.attrs["reason"].(string))
		}
	}
	require.Len(t, reasons, 2)
	require.Contains(t, reasons[0], "body too large")
	require.Contains(t, strings.ToLower(reasons[1]), "headers too large")
}
