package handlers

import (
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/thushan/olla/internal/adapter/registry"
	"github.com/thushan/olla/internal/core/constants"
	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/core/ports"
	"github.com/thushan/olla/internal/logger"
)

// capturingLogger records the fields passed to each Info call so tests can
// assert that specific keys appear (or don't appear) in completed-request logs.
// Only Info is instrumented; other levels delegate to the no-op mockStyledLogger.
type capturingLogger struct {
	mockStyledLogger
	infoFields []any
	infoMsg    string
}

func (c *capturingLogger) Info(msg string, args ...any) {
	// Capture the last Info call. Most logRequestResult paths call Info exactly
	// once for "Request completed".
	c.infoMsg = msg
	c.infoFields = args
}

// WithRequestID must return the capturingLogger itself, not the embedded mockStyledLogger,
// so that pr.requestLogger.Info routes back through our capturing implementation.
func (c *capturingLogger) WithRequestID(_ string) logger.StyledLogger { return c }

// hasField returns true when the captured INFO args contain key=value as an
// adjacent pair.
func (c *capturingLogger) hasField(key string, value any) bool {
	for i := 0; i+1 < len(c.infoFields); i += 2 {
		if c.infoFields[i] == key && c.infoFields[i+1] == value {
			return true
		}
	}
	return false
}

// hasKey returns true when the captured INFO args contain the given key in any
// adjacent pair, regardless of value.
func (c *capturingLogger) hasKey(key string) bool {
	for i := 0; i+1 < len(c.infoFields); i += 2 {
		if c.infoFields[i] == key {
			return true
		}
	}
	return false
}

// makeTestPR returns a minimal proxyRequest wired to the capturing logger.
func makeTestPR(cl *capturingLogger) *proxyRequest {
	return &proxyRequest{
		requestLogger: cl,
		stats: &ports.RequestStats{
			RequestID: "test-req-001",
			StartTime: time.Now(),
		},
	}
}

// TestLogRequestResult_StickyOutcome_AppearsAtInfo verifies that a sticky hit
// appears in the INFO completed-request log.
func TestLogRequestResult_StickyOutcome_AppearsAtInfo(t *testing.T) {
	t.Parallel()

	cl := &capturingLogger{}
	pr := makeTestPR(cl)
	pr.stickyOutcome = "hit"

	app := &Application{logger: cl}
	app.logRequestResult(pr, nil)

	assert.True(t, cl.hasField("sticky_outcome", "hit"),
		"INFO log must contain sticky_outcome=hit; fields: %v", cl.infoFields)
}

// TestLogRequestResult_StickyOutcome_OmittedWhenDisabled verifies that the
// "disabled" outcome is not logged at INFO - it carries no actionable signal.
func TestLogRequestResult_StickyOutcome_OmittedWhenDisabled(t *testing.T) {
	t.Parallel()

	cl := &capturingLogger{}
	pr := makeTestPR(cl)
	pr.stickyOutcome = constants.StickyOutcomeResultDisabled

	app := &Application{logger: cl}
	app.logRequestResult(pr, nil)

	assert.False(t, cl.hasKey("sticky_outcome"),
		"sticky_outcome must be absent from INFO log when outcome is 'disabled'; fields: %v", cl.infoFields)
}

// TestLogRequestResult_RoutingDecision_AppearsAtInfo verifies that the three
// routing decision fields (strategy, action, reason) all appear at INFO.
func TestLogRequestResult_RoutingDecision_AppearsAtInfo(t *testing.T) {
	t.Parallel()

	cl := &capturingLogger{}
	pr := makeTestPR(cl)
	pr.stats.RoutingDecision = &domain.ModelRoutingDecision{
		Strategy: "priority",
		Action:   "routed",
		Reason:   "model found",
	}

	app := &Application{logger: cl}
	app.logRequestResult(pr, nil)

	assert.True(t, cl.hasField("routing_strategy", "priority"),
		"INFO log must contain routing_strategy; fields: %v", cl.infoFields)
	assert.True(t, cl.hasField("routing_action", "routed"),
		"INFO log must contain routing_action; fields: %v", cl.infoFields)
	assert.True(t, cl.hasField("routing_reason", "model found"),
		"INFO log must contain routing_reason; fields: %v", cl.infoFields)
}

// TestLogRequestResult_ProviderModel_AppearsAtInfo verifies that the model name
// reported by the backend is promoted to INFO so log aggregators can correlate
// it with the alias or model name the client requested.
func TestLogRequestResult_ProviderModel_AppearsAtInfo(t *testing.T) {
	t.Parallel()

	cl := &capturingLogger{}
	pr := makeTestPR(cl)
	pr.stats.ProviderMetrics = &domain.ProviderMetrics{
		Model: "llama3.1:8b",
	}

	app := &Application{logger: cl}
	app.logRequestResult(pr, nil)

	assert.True(t, cl.hasField("provider_model", "llama3.1:8b"),
		"INFO log must contain provider_model; fields: %v", cl.infoFields)
}

// TestLogRequestResult_FallbackReason_AppearsAtInfo verifies that the translation
// fallback reason appears in INFO logs when translation (not passthrough) is used.
func TestLogRequestResult_FallbackReason_AppearsAtInfo(t *testing.T) {
	t.Parallel()

	cl := &capturingLogger{}
	pr := makeTestPR(cl)
	pr.translatorFallbackReason = string(constants.FallbackReasonCannotPassthrough)

	app := &Application{logger: cl}
	app.logRequestResult(pr, nil)

	assert.True(t, cl.hasField("fallback_reason", string(constants.FallbackReasonCannotPassthrough)),
		"INFO log must contain fallback_reason; fields: %v", cl.infoFields)
}

// TestLogRequestResult_FallbackReason_OmittedWhenNone verifies that the empty
// fallback reason (FallbackReasonNone == "") is not logged - passthrough is the
// expected path and emitting the field would add noise on the majority of requests.
func TestLogRequestResult_FallbackReason_OmittedWhenNone(t *testing.T) {
	t.Parallel()

	cl := &capturingLogger{}
	pr := makeTestPR(cl)
	pr.translatorFallbackReason = string(constants.FallbackReasonNone) // == ""

	app := &Application{logger: cl}
	app.logRequestResult(pr, nil)

	assert.False(t, cl.hasKey("fallback_reason"),
		"fallback_reason must be absent from INFO log when reason is FallbackReasonNone; fields: %v", cl.infoFields)
}

// TestLogRequestResult_SessionID_AtInfo verifies the client-supplied session id
// is logged at INFO so affinity routing can be traced without enabling DEBUG.
func TestLogRequestResult_SessionID_AtInfo(t *testing.T) {
	t.Parallel()

	cl := &capturingLogger{}
	pr := makeTestPR(cl)
	pr.sessionID = "sess-abc-123"

	app := &Application{logger: cl}
	app.logRequestResult(pr, nil)

	assert.True(t, cl.hasField("session_id", "sess-abc-123"),
		"session_id must appear at INFO; fields: %v", cl.infoFields)
}

// TestResolveAliasEndpoints_LogsActualModels verifies that the "Model alias
// resolved" log line includes an actual_models field listing the real model
// names behind the alias.
func TestResolveAliasEndpoints_LogsActualModels(t *testing.T) {
	t.Parallel()

	cl := &capturingLogger{}

	epURL, _ := url.Parse("http://ollama:11434")
	candidates := []*domain.Endpoint{
		{Name: "ollama", URL: epURL, URLString: "http://ollama:11434", Type: domain.ProfileOllama},
	}

	modelRegistry := &mockSimpleModelRegistry{
		endpointsForModel: map[string][]string{
			"gpt-real:8b": {"http://ollama:11434"},
		},
	}

	aliases := map[string][]string{
		"gpt-alias": {"gpt-real:8b"},
	}
	aliasResolver := registry.NewAliasResolver(aliases, cl)

	app := &Application{
		modelRegistry: modelRegistry,
		aliasResolver: aliasResolver,
		logger:        cl,
	}

	profile := domain.NewRequestProfile("/v1/chat/completions")
	profile.ModelName = "gpt-alias"
	profile.SupportedBy = []string{domain.ProfileOllama}

	_ = app.resolveAliasEndpoints(t.Context(), profile, candidates, cl)

	assert.True(t, cl.hasKey("actual_models"),
		"INFO log must contain actual_models field; fields: %v", cl.infoFields)
}
