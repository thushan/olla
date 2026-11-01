package core

import "errors"

// ErrCircuitBreakerOpen signals that a per-endpoint circuit breaker rejected
// the attempt before any network I/O. ExecuteWithRetry treats it as
// failover-eligible but must NOT route it through the connection-error path,
// which would 1-strike-demote the endpoint's persisted health: a breaker-open
// rejection reflects the breaker's accumulated state, not a fresh connectivity
// failure, so demoting here would double-count and corrupt health tracking.
var ErrCircuitBreakerOpen = errors.New("circuit breaker open")

// ErrUpstreamServerError marks a request the backend answered with a 5xx. It is
// only carried on the proxy error event for stats; the response itself is
// relayed to the client as-is.
var ErrUpstreamServerError = errors.New("upstream returned a server error")
