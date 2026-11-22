package core

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptrace"
	"strings"
	"sync/atomic"
)

// StaleConnError marks an io.EOF that arrived on a pooled (reused) connection.
//
// A healthy keep-alive backend (uvicorn closes idle connections after 5s) can
// close the socket in the same instant Olla writes a POST on it. Go's transport
// never retries a written non-replayable request, so the caller sees io.EOF.
// That is a race with the backend's idle timer, not evidence the backend went
// away, so the request still fails over but the endpoint must not be demoted
// or charged to its circuit breaker. An EOF on a fresh connection is not
// wrapped: there the backend really did drop us.
type StaleConnError struct {
	Err error
}

func (e *StaleConnError) Error() string { return e.Err.Error() }
func (e *StaleConnError) Unwrap() error { return e.Err }

// IsStaleConnError reports whether err is an EOF on a reused connection.
func IsStaleConnError(err error) bool {
	var sce *StaleConnError
	return errors.As(err, &sce)
}

// ConnReuseTrace records whether the connection that carried the latest
// attempt of an outbound request came from the idle pool.
type ConnReuseTrace struct {
	reused atomic.Bool
}

// TraceConnReuse attaches one ClientTrace to req. GotConnInfo fires once per
// transport-internal attempt, so after a transparent Go retry the flag reflects
// the connection that actually produced the final error.
func TraceConnReuse(req *http.Request) (*http.Request, *ConnReuseTrace) {
	t := &ConnReuseTrace{}
	trace := &httptrace.ClientTrace{
		GotConn: func(info httptrace.GotConnInfo) { t.reused.Store(info.Reused) },
	}
	return req.WithContext(httptrace.WithClientTrace(req.Context(), trace)), t
}

// serverClosedIdleText is net/http's errServerClosedIdle. It is unexported and
// carries no sentinel, so the text is the only handle. Go raises it when the
// backend's idle close lands after the pooled connection was taken but before
// the request was written: the same keep-alive race as a reused-connection EOF.
const serverClosedIdleText = "http: server closed idle connection"

// isStaleReuseSignal reports whether err is one of the two shapes the idle
// race takes on a reused connection.
func isStaleReuseSignal(err error) bool {
	return errors.Is(err, io.EOF) || strings.Contains(err.Error(), serverClosedIdleText)
}

// Classify tags a round-trip error as a StaleConnError when it is an EOF (or
// net/http's "server closed idle connection") on a reused connection. Any
// other error is returned untouched.
func (t *ConnReuseTrace) Classify(err error) error {
	if err == nil || !t.reused.Load() || IsStaleConnError(err) || !isStaleReuseSignal(err) {
		return err
	}
	return &StaleConnError{Err: err}
}
