package handlers

import (
	"strconv"

	"github.com/thushan/olla/internal/logger"
)

// rejectLogs samples the "Request rejected" WARN for requests Olla refuses to route (an
// unknown model under strict routing, a model or provider with no healthy host). A client
// can repeat these at wire speed, and each repeat used to write full detail lines.
//
// The key is the response status plus the client IP, status first because the throttle truncates
// long keys and a long forwarded identity must not push the status off the end. The status is a finite cause, so one
// client's 404s do not hide its 503s, while a client rotating model names still costs one
// line per window rather than one per name. The throttle caps its tracked keys.
type rejectLogs struct {
	request *logger.Throttle
	// psFailure samples the per-endpoint /api/ps failure WARN, keyed by endpoint name
	// (a finite, operator-configured set).
	psFailure *logger.Throttle
}

// newRejectLogs registers the throttle in set (nil is allowed) so Application can flush it at shutdown.
func newRejectLogs(log logger.StyledLogger, set *logger.ThrottleSet) *rejectLogs {
	return &rejectLogs{
		request:   set.Track(logger.NewRejectThrottle(log, "Request rejected", "proxy", "client_status", nil)),
		psFailure: set.Track(logger.NewRejectThrottle(log, "Ollama /api/ps failed", "ollama_ps", "endpoint", nil)),
	}
}

// allowRequest is true when this client's rejection should be logged in full. A nil receiver
// (an Application built without NewApplication, as in tests) logs everything.
func (r *rejectLogs) allowRequest(clientIP string, status int) bool {
	return r == nil || r.request.Allow(strconv.Itoa(status)+"|"+clientIP)
}

// allowPSFailure is true when this endpoint's /api/ps failure should be logged at WARN. A
// dashboard polling an instance that hangs would otherwise write a WARN per poll. A nil
// receiver logs everything.
func (r *rejectLogs) allowPSFailure(endpoint string) bool {
	return r == nil || r.psFailure.Allow(endpoint)
}

// newThrottleSet exists because NewApplication's logger parameter shadows the package.
func newThrottleSet() *logger.ThrottleSet { return &logger.ThrottleSet{} }
