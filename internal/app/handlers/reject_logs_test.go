package handlers

import (
	"fmt"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/core/ports"
)

type warnCounter struct {
	*mockStyledLogger
	mu     sync.Mutex
	counts map[string]int
}

func newWarnCounter() *warnCounter {
	return &warnCounter{mockStyledLogger: &mockStyledLogger{}, counts: map[string]int{}}
}

func (w *warnCounter) Warn(msg string, _ ...any) {
	w.mu.Lock()
	w.counts[msg]++
	w.mu.Unlock()
}

func (w *warnCounter) count(msg string) int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.counts[msg]
}

func rejectedRequest(ip, model string) *proxyRequest {
	return &proxyRequest{
		requestLogger: nil, clientIP: ip, model: model, stats: &ports.RequestStats{StartTime: time.Now()},
	}
}

// A client repeating a refused request, or rotating the model name it asks for, must not
// write a line per attempt. A different client or status is not hidden by it.
func TestRejectLogs_RepeatedRefusalsAreSampled(t *testing.T) {
	t.Parallel()

	log := newWarnCounter()
	app := &Application{logger: log, rejectLog: newRejectLogs(log, nil)}
	reject := func(ip, model string, status int) {
		pr := rejectedRequest(ip, model)
		pr.requestLogger = log
		app.logRequestRejected(pr, status, "model not found")
	}

	for i := range 300 {
		reject("203.0.113.7", fmt.Sprintf("no-such-model-%d", i), 404)
	}
	assert.Equal(t, 1, log.count("Request rejected"), "rotating model names is one stream")

	reject("198.51.100.9", "x", 404)
	assert.Equal(t, 2, log.count("Request rejected"), "a different client is its own stream")
	reject("203.0.113.7", "x", 503)
	assert.Equal(t, 3, log.count("Request rejected"), "a different status is a different cause")
}

// Strict-routing and fail-closed provider rejections used to write their own WARN per
// request before the final rejection line; only the sampled final line may remain.
func TestFilterEndpointsByProfile_RejectionsWriteNoWarnPerRequest(t *testing.T) {
	t.Parallel()

	log := newWarnCounter()
	u, _ := url.Parse("http://localhost:11434")
	endpoints := []*domain.Endpoint{{Name: "a", URL: u, URLString: u.String(), Type: domain.ProfileOllama, Status: domain.StatusHealthy}}
	app := &Application{
		logger:        log,
		rejectLog:     newRejectLogs(log, nil),
		modelRegistry: &mockSimpleModelRegistry{strict: true, endpointsForModel: map[string][]string{}},
	}

	for range 300 {
		strict := &domain.RequestProfile{Path: "/v1/chat/completions", ModelName: "missing", SupportedBy: []string{domain.ProfileOllama}}
		require.Empty(t, app.filterEndpointsByProfile(endpoints, strict, log))

		closed := &domain.RequestProfile{Path: "/x", SupportedBy: []string{domain.ProfileVLLM}, FailClosedOnNoMatch: true}
		require.Empty(t, app.filterEndpointsByProfile(endpoints, closed, log))
	}
	assert.Zero(t, log.count("Model routing rejected request"))
	assert.Zero(t, log.count("No compatible endpoints found for provider-scoped path, failing closed"))
}

func TestRejectLogs_NilLogsEverything(t *testing.T) {
	t.Parallel()
	var r *rejectLogs
	assert.True(t, r.allowRequest("x", 404))
	assert.True(t, r.allowRequest("x", 404))
}

// Shutdown must report what the handler throttle is still holding, under the layer that wrote it.
func TestApplication_FlushRejectLogsReportsPendingSummaries(t *testing.T) {
	t.Parallel()

	log := newWarnCounter()
	set := newThrottleSet()
	app := &Application{logger: log, rejectLog: newRejectLogs(log, set), throttles: set}

	for range 5 {
		app.rejectLog.allowRequest("203.0.113.7", 404)
	}
	assert.Zero(t, log.count("Request rejected, repeats suppressed"))

	app.FlushRejectLogs()
	assert.Equal(t, 1, log.count("Request rejected, repeats suppressed"))

	app.FlushRejectLogs()
	assert.Equal(t, 1, log.count("Request rejected, repeats suppressed"), "a flushed count is not reported twice")
}
