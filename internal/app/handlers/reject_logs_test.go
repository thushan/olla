package handlers

import (
	"net/url"
	"strings"
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

// A client repeating a refused request must not write a line per attempt, in either of the
// two places a refusal is logged. A different model or client is not hidden by it.
func TestRejectLogs_RepeatedRefusalsAreSampled(t *testing.T) {
	t.Parallel()

	log := newWarnCounter()
	u, _ := url.Parse("http://localhost:11434")
	endpoints := []*domain.Endpoint{{Name: "a", URL: u, URLString: u.String(), Type: domain.ProfileOllama, Status: domain.StatusHealthy}}
	app := &Application{
		logger:        log,
		rejectLog:     newRejectLogs(log, nil),
		modelRegistry: &mockSimpleModelRegistry{strict: true, endpointsForModel: map[string][]string{}},
	}

	route := func(model string) {
		profile := &domain.RequestProfile{Path: "/v1/chat/completions", ModelName: model, SupportedBy: []string{domain.ProfileOllama}}
		require.Empty(t, app.filterEndpointsByProfile(endpoints, profile, log))
	}
	for range 300 {
		route("no-such-model")
	}
	assert.Equal(t, 1, log.count("Model routing rejected request"))
	route("another-missing-model")
	assert.Equal(t, 2, log.count("Model routing rejected request"), "a different model is its own stream")

	for range 300 {
		app.logRequestRejected(&proxyRequest{
			requestLogger: log, clientIP: "203.0.113.7", stats: &ports.RequestStats{StartTime: time.Now()},
		}, 404)
	}
	assert.Equal(t, 1, log.count("Request rejected"))
	app.logRequestRejected(&proxyRequest{
		requestLogger: log, clientIP: "198.51.100.9", stats: &ports.RequestStats{StartTime: time.Now()},
	}, 404)
	assert.Equal(t, 2, log.count("Request rejected"), "a different client is its own stream")
}

func TestRejectLogs_NilLogsEverything(t *testing.T) {
	t.Parallel()
	var r *rejectLogs
	assert.True(t, r.allowRequest("x"))
	assert.True(t, r.allowRequest("x"))
}

// The throttle truncates keys, so a long model name must not push the status out of the key:
// the same model rejected with 404 and 503 are different causes and each gets its own line.
func TestRejectLogs_LongModelNameKeepsStatusInKey(t *testing.T) {
	t.Parallel()

	log := newWarnCounter()
	r := newRejectLogs(log, nil)
	model := strings.Repeat("m", 100)

	assert.True(t, r.allowRouting(model, &domain.ModelRoutingDecision{StatusCode: 404}))
	assert.True(t, r.allowRouting(model, &domain.ModelRoutingDecision{StatusCode: 503}),
		"a different status for the same long model is a different stream")
	assert.False(t, r.allowRouting(model, &domain.ModelRoutingDecision{StatusCode: 404}))
}

// Shutdown must report what the handler throttles are still holding, under the layer that wrote it.
func TestApplication_FlushRejectLogsReportsPendingSummaries(t *testing.T) {
	t.Parallel()

	log := newWarnCounter()
	set := newThrottleSet()
	app := &Application{logger: log, rejectLog: newRejectLogs(log, set), throttles: set}

	for range 5 {
		app.rejectLog.allowRequest("203.0.113.7")
		app.rejectLog.allowRouting("m", &domain.ModelRoutingDecision{StatusCode: 404})
	}
	assert.Zero(t, log.count("Request rejected, repeats suppressed"))

	app.FlushRejectLogs()
	assert.Equal(t, 1, log.count("Request rejected, repeats suppressed"))
	assert.Equal(t, 1, log.count("Model routing rejected request, repeats suppressed"))

	app.FlushRejectLogs()
	assert.Equal(t, 1, log.count("Request rejected, repeats suppressed"), "a flushed count is not reported twice")
}
