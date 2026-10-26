package handlers

import (
	"strconv"

	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/logger"
)

// rejectLogs samples the per-request WARNs for requests Olla refuses to route. A client can
// repeat a refused request at wire speed (an unknown model under strict routing, a model
// with no healthy host), and each repeat used to write full detail lines.
//
// Request rejections are keyed by client IP: one noisy client is one stream of lines, and a
// different client is never hidden by it. Routing rejections are keyed by model and status
// instead, because the same cause (strict model_not_found, model_unavailable) is what an
// operator wants to see once, whichever client sent it. The throttle truncates keys and
// caps their number, so a client inventing model names costs at most 256 lines a window.
// The status leads the key so truncating a very long model name cannot merge two statuses.
type rejectLogs struct {
	request *logger.Throttle
	routing *logger.Throttle
}

// newRejectLogs registers both throttles in set (nil is allowed) so Application can flush them at shutdown.
func newRejectLogs(log logger.StyledLogger, set *logger.ThrottleSet) *rejectLogs {
	return &rejectLogs{
		request: set.Track(logger.NewRejectThrottle(log, "Request rejected", "proxy", "client_ip", nil)),
		routing: set.Track(logger.NewRejectThrottle(log, "Model routing rejected request", "routing", "model_status", nil)),
	}
}

// allowRequest is true when this client's rejection should be logged in full. A nil receiver
// (an Application built without NewApplication, as in tests) logs everything.
func (r *rejectLogs) allowRequest(clientIP string) bool {
	return r == nil || r.request.Allow(clientIP)
}

func (r *rejectLogs) allowRouting(model string, decision *domain.ModelRoutingDecision) bool {
	if r == nil {
		return true
	}
	return r.routing.Allow(strconv.Itoa(decision.StatusCode) + "|" + model)
}

// newThrottleSet exists because NewApplication's logger parameter shadows the package.
func newThrottleSet() *logger.ThrottleSet { return &logger.ThrottleSet{} }
