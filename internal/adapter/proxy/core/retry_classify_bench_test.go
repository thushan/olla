package core

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"syscall"
	"testing"
)

var benchClassifySink bool

// benchClassifyCases is one error per branch of IsConnectionError, in the
// wrapping the HTTP client actually produces. The classifier sits on the retry
// path of every failed attempt and carries the errorsastype findings in
// retry.go, so a rewrite of it is compared against these rows.
func benchClassifyCases() []struct {
	name string
	err  error
} {
	const target = "http://localhost:11434/v1/chat/completions"

	dialRefused := &url.Error{Op: "Post", URL: target, Err: &net.OpError{
		Op: "dial", Net: "tcp",
		Err: &os.SyscallError{Syscall: "connect", Err: syscall.ECONNREFUSED},
	}}
	readReset := &url.Error{Op: "Post", URL: target, Err: &net.OpError{
		Op: "read", Net: "tcp", Err: syscall.ECONNRESET,
	}}

	return []struct {
		name string
		err  error
	}{
		{"nil", nil},
		{"wrapped-deadline", &url.Error{Op: "Post", URL: target, Err: context.DeadlineExceeded}},
		{"wrapped-cancel", &url.Error{Op: "Post", URL: target, Err: context.Canceled}},
		{"stale-conn-eof", &url.Error{Op: "Post", URL: target, Err: &StaleConnError{Err: io.EOF}}},
		{"wrapped-eof", &url.Error{Op: "Post", URL: target, Err: io.EOF}},
		{"dial-refused-net-error", dialRefused},
		{"read-reset-net-error", readReset},
		// An errno-shaped error outside a net.OpError. syscall.Errno implements
		// net.Error (Error, Timeout, Temporary), so the errors.As net.Error
		// branch catches it first; the syscall.Errno branch of the classifier
		// is unreachable for every input. The row still times the classifier
		// on this shape.
		{"errno-refused-unwrapped", fmt.Errorf("write: %w", syscall.ECONNREFUSED)},
		{"string-fallback-nohost", errors.New("dial tcp: lookup backend.local: no such host")},
		{"string-fallback-timeout", errors.New("read tcp: i/o timeout")},
		{"non-connection-error", errors.New("unexpected status 400")},
	}
}

func BenchmarkIsConnectionError(b *testing.B) {
	for _, tc := range benchClassifyCases() {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			for range b.N {
				benchClassifySink = IsConnectionError(tc.err)
			}
		})
	}
}

func BenchmarkIsStaleConnError(b *testing.B) {
	err := &url.Error{Op: "Post", URL: "http://localhost:11434/v1/chat/completions", Err: &StaleConnError{Err: io.EOF}}

	b.ReportAllocs()
	for range b.N {
		benchClassifySink = IsStaleConnError(err)
	}
}
