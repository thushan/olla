package common

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"syscall"
	"time"
)

var (
	// ErrNoHealthyEndpoints is returned when no healthy endpoints are available
	ErrNoHealthyEndpoints = errors.New("no healthy endpoints available")
)

// MakeUserFriendlyError converts technical errors into user-friendly messages with actionable guidance
// all implementations should use this function to ensure consistent error handling with detailed context
// for TUI output and logging.
//
//nolint:gocognit // intentionally complex; we could break it down further, but this is already quite readable
func MakeUserFriendlyError(err error, duration time.Duration, errorContext string, responseTimeout time.Duration) error {
	if err == nil {
		return nil
	}

	switch {
	case errors.Is(err, context.Canceled):
		// Common client timeout pattern (curl default, browser timeouts, etc.)
		if duration >= 25*time.Second && duration <= 35*time.Second {
			return fmt.Errorf("request cancelled after %.1fs - likely client timeout (increase client timeout to 60-90s for LLM responses)", duration.Seconds())
		}
		// Very quick cancellations suggest network issues
		if duration < 2*time.Second {
			return fmt.Errorf("request cancelled after %.1fs - client disconnected immediately (check network connectivity)", duration.Seconds())
		}
		// Early cancellations might be impatient users
		if duration < 10*time.Second {
			return fmt.Errorf("request cancelled after %.1fs - client disconnected early (LLM responses may take 30-60+ seconds)", duration.Seconds())
		}
		return fmt.Errorf("request cancelled after %.1fs - client disconnected during processing", duration.Seconds())

	case errors.Is(err, context.DeadlineExceeded):
		if responseTimeout > 0 {
			return fmt.Errorf("request timeout after %.1fs - server timeout of %.1fs exceeded (LLM model taking longer than expected)",
				duration.Seconds(), responseTimeout.Seconds())
		}
		return fmt.Errorf("request timeout after %.1fs - server timeout exceeded (LLM response took too long)", duration.Seconds())

	case errors.Is(err, io.EOF):
		if errorContext == "streaming" {
			if duration < 5*time.Second {
				return fmt.Errorf("AI backend closed connection after %.1fs - response ended prematurely", duration.Seconds())
			}
			return fmt.Errorf("AI backend closed connection after %.1fs - response stream ended unexpectedly", duration.Seconds())
		}
		return fmt.Errorf("connection closed after %.1fs - AI backend ended communication unexpectedly", duration.Seconds())
	}

	// Network connection errors
	var netErr net.Error
	if errors.As(err, &netErr) {
		if netErr.Timeout() {
			return fmt.Errorf("network timeout after %.1fs - unable to connect to LLM backend (check backend availability)", duration.Seconds())
		}
		return fmt.Errorf("network error after %.1fs - %w (check network connectivity to LLM backend)", duration.Seconds(), netErr)
	}

	// TCP/connection errors
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		if opErr.Op == "dial" {
			return fmt.Errorf("connection failed after %.1fs - cannot reach LLM backend at %s (check backend is running)",
				duration.Seconds(), opErr.Addr)
		}
		if opErr.Op == "read" {
			return fmt.Errorf("connection lost after %.1fs while reading response - LLM backend disconnected unexpectedly", duration.Seconds())
		}
		if opErr.Op == "write" {
			return fmt.Errorf("connection lost after %.1fs while sending request - LLM backend unavailable", duration.Seconds())
		}
	}

	// Connection refused (backend down)
	var syscallErr *syscall.Errno
	if errors.As(err, &syscallErr) {
		if errors.Is(*syscallErr, syscall.ECONNREFUSED) {
			return fmt.Errorf("connection refused after %.1fs - LLM backend is not running or not accepting connections", duration.Seconds())
		}
		if errors.Is(*syscallErr, syscall.ECONNRESET) {
			return fmt.Errorf("connection reset after %.1fs - LLM backend forcibly closed connection (possible overloaded)", duration.Seconds())
		}
	}

	// HTTP transport errors
	errStr := err.Error()
	switch {
	case strings.Contains(errStr, "connection refused"):
		return fmt.Errorf("connection refused after %.1fs - LLM backend is not running or not accepting connections", duration.Seconds())
	case strings.Contains(errStr, "connection reset"):
		return fmt.Errorf("connection reset after %.1fs - LLM backend closed connection unexpectedly (possible overload)", duration.Seconds())
	case strings.Contains(errStr, "no such host"):
		return fmt.Errorf("DNS lookup failed after %.1fs - cannot resolve LLM backend hostname (check configuration)", duration.Seconds())
	case strings.Contains(errStr, "TLS handshake timeout"):
		return fmt.Errorf("TLS handshake timeout after %.1fs - SSL/TLS connection to LLM backend failed", duration.Seconds())
	case strings.Contains(errStr, "certificate"):
		return fmt.Errorf("TLS certificate error after %.1fs - invalid SSL certificate on LLM backend", duration.Seconds())
	}

	// Generic error with duration context
	return fmt.Errorf("request failed after %.1fs: %w", duration.Seconds(), err)
}
