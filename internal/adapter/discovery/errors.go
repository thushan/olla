package discovery

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// DiscoveryError wraps discovery operation errors with context
type DiscoveryError struct {
	Err         error
	EndpointURL string
	ProfileType string
	Operation   string
	StatusCode  int
	Latency     time.Duration
}

func (e *DiscoveryError) Error() string {
	if e.StatusCode > 0 {
		return fmt.Sprintf("discovery %s failed for %s (profile: %s, status: %d, latency: %v): %v",
			e.Operation, e.EndpointURL, e.ProfileType, e.StatusCode, e.Latency, e.Err)
	}
	return fmt.Sprintf("discovery %s failed for %s (profile: %s, latency: %v): %v",
		e.Operation, e.EndpointURL, e.ProfileType, e.Latency, e.Err)
}

func (e *DiscoveryError) Unwrap() error {
	return e.Err
}

func NewDiscoveryError(endpointURL, profileType, operation string, statusCode int, latency time.Duration, err error) *DiscoveryError {
	return &DiscoveryError{
		EndpointURL: endpointURL,
		ProfileType: profileType,
		Operation:   operation,
		StatusCode:  statusCode,
		Latency:     latency,
		Err:         err,
	}
}

// ProfileNotFoundError indicates no suitable profile was found for an endpoint
type ProfileNotFoundError struct {
	ProfileType string
}

func (e *ProfileNotFoundError) Error() string {
	return fmt.Sprintf("profile not found: %s", e.ProfileType)
}

// ParseError indicates response parsing failed
type ParseError struct {
	Err    error
	Format string
	Data   []byte
}

func (e *ParseError) Error() string {
	return fmt.Sprintf("failed to parse %s response: %v", e.Format, e.Err)
}

func (e *ParseError) Unwrap() error {
	return e.Err
}

// NetworkError indicates a network-level failure
type NetworkError struct {
	Err error
	URL string
}

func (e *NetworkError) Error() string {
	return fmt.Sprintf("network error for %s: %v", e.URL, e.Err)
}

func (e *NetworkError) Unwrap() error {
	return e.Err
}

// GetUserFriendlyMessage gives a concise, user-friendly error message from discovery errors
func GetUserFriendlyMessage(err error) string {
	var discErr *DiscoveryError
	if errors.As(err, &discErr) {
		if discErr.StatusCode > 0 {
			switch {
			case discErr.StatusCode >= 400 && discErr.StatusCode < 500:
				return fmt.Sprintf("endpoint configuration issue (HTTP %d)", discErr.StatusCode)
			case discErr.StatusCode >= 500:
				return fmt.Sprintf("endpoint server error (HTTP %d)", discErr.StatusCode)
			default:
				return fmt.Sprintf("HTTP error (%d)", discErr.StatusCode)
			}
		}

		// common network issues
		if discErr.Err != nil {
			errStr := discErr.Err.Error()
			switch {
			case strings.Contains(errStr, "connectex:") || strings.Contains(errStr, "connection refused") || strings.Contains(errStr, "no route to host"):
				return "endpoint unreachable"
			case strings.Contains(errStr, "timeout") || strings.Contains(errStr, "context deadline exceeded"):
				return "connection timeout"
			case strings.Contains(errStr, "network is unreachable"):
				return "network unreachable"
			default:
				return "network error"
			}
		}
	}

	var networkErr *NetworkError
	if errors.As(err, &networkErr) {
		return "network connection failed"
	}

	var parseErr *ParseError
	if errors.As(err, &parseErr) {
		return "invalid response format"
	}

	return "discovery failed"
}

// IsRecoverable determines if an error should trigger retry logic, instead of failing immediately.
func IsRecoverable(err error) bool {
	// we may want to improve this as we discover more recoverable errors
	// but abstracting it here means we can change it in one place

	// can't really recover from wrong format etc
	var parseError *ParseError
	if errors.As(err, &parseError) {
		return false
	}

	// hopefully recoverable?
	var networkError *NetworkError
	if errors.As(err, &networkError) {
		return true
	}

	var discErr *DiscoveryError
	if errors.As(err, &discErr) {
		// HTTP 4xx errors are non-recoverable (wrong endpoint, auth, etc.)
		if discErr.StatusCode >= 400 && discErr.StatusCode < 500 {
			return false
		}

		// Check underlying error
		if discErr.Err != nil {
			return IsRecoverable(discErr.Err)
		}

		// DiscoveryError with no underlying error and no 4xx status
		// this might be a transient issue, at least in testing it was
		return true
	}

	// [TF] let's see if this is workable, we're not too sure yet
	return true
}
