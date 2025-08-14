package health

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/thushan/olla/internal/version"

	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/util"
)

const (
	DefaultMaxRetries = 2
	DefaultBaseDelay  = 100 * time.Millisecond
	MaxBackoffDelay   = 2 * time.Second
)

type HealthClient struct {
	client         HTTPClient
	circuitBreaker *CircuitBreaker
}

func NewHealthClient(client HTTPClient, circuitBreaker *CircuitBreaker) *HealthClient {
	return &HealthClient{
		client:         client,
		circuitBreaker: circuitBreaker,
	}
}

// Check performs a single health check against an endpoint with retry logic and panic recovery
func (hc *HealthClient) Check(ctx context.Context, endpoint *domain.Endpoint) (result domain.HealthCheckResult, err error) {
	// Panic recovery for critical path
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("health check panic recovered: %v", r)
			result = domain.HealthCheckResult{
				Status:     domain.StatusOffline,
				Error:      err,
				ErrorType:  domain.ErrorTypeHTTPError,
				Latency:    0,
				StatusCode: 0, // No HTTP response during panic
			}
		}
	}()

	healthCheckURL := endpoint.GetHealthCheckURLString()

	// Check circuit breaker first
	if hc.circuitBreaker.IsOpen(healthCheckURL) {
		result = domain.HealthCheckResult{
			Status:     domain.StatusOffline,
			Error:      ErrCircuitBreakerOpen,
			ErrorType:  domain.ErrorTypeCircuitOpen,
			Latency:    0,
			StatusCode: 0, // No HTTP response when circuit breaker is open
		}
		return result, domain.NewHealthCheckError(endpoint, "circuit_breaker", 0, 0, ErrCircuitBreakerOpen)
	}

	// Perform check with retry logic
	var lastErr error
	overallStart := time.Now()
	maxRetries := DefaultMaxRetries

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			// Calculate exponential backoff delay
			delay := calculateBackoffDelay(attempt)

			// Use a separate context for the delay to avoid cancelling the main operation
			delayCtx, delayCancel := context.WithTimeout(context.Background(), delay)
			select {
			case <-delayCtx.Done():
				// Delay completed normally
			case <-ctx.Done():
				delayCancel()
				// Main context cancelled, stop retrying
				result.Latency = time.Since(overallStart)
				return result, domain.NewHealthCheckError(endpoint, "retry_cancelled", 0, result.Latency, ctx.Err())
			}
			delayCancel()
		}

		result, lastErr = hc.performSingleCheck(ctx, endpoint, healthCheckURL)

		// Check if we should retry
		if lastErr == nil || !shouldRetry(lastErr, result.ErrorType) {
			break
		}
	}

	// Record overall latency including retries
	result.Latency = time.Since(overallStart)

	// Record result in circuit breaker
	if lastErr != nil || result.Status != domain.StatusHealthy {
		hc.circuitBreaker.RecordFailure(healthCheckURL)
	} else {
		hc.circuitBreaker.RecordSuccess(healthCheckURL)
	}

	if lastErr != nil {
		return result, domain.NewHealthCheckError(endpoint, "health_check", result.StatusCode, result.Latency, lastErr)
	}

	return result, nil
}

func (hc *HealthClient) performSingleCheck(ctx context.Context, endpoint *domain.Endpoint, healthCheckURL string) (domain.HealthCheckResult, error) {
	start := time.Now()
	result := domain.HealthCheckResult{
		Status: domain.StatusUnknown,
	}

	checkCtx, cancel := context.WithTimeout(ctx, endpoint.CheckTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(checkCtx, http.MethodGet, healthCheckURL, http.NoBody)
	if err != nil {
		result.Latency = time.Since(start)
		result.Error = err
		result.ErrorType = classifyError(err)
		result.Status = determineStatus(0, result.Latency, err, result.ErrorType)
		result.StatusCode = 0 // No HTTP response received
		return result, err
	}

	req = injectDefaultHeaders(req)
	resp, err := hc.client.Do(req)
	result.Latency = time.Since(start)

	if err != nil {
		result.Error = err
		result.ErrorType = classifyError(err)
		result.Status = determineStatus(0, result.Latency, err, result.ErrorType)
		result.StatusCode = 0 // No HTTP response received
		return result, err
	}

	// SHERPA-64: Resource leak post HTTP request fails after extended query
	//  14-Aug-2024 [ML] this was causing connection reuse issues across multiple health checks
	// 		 			 but interestingly, repro was mostly seen with LMStudio endpoints
	defer func() {
		// Ensure response body is fully read and closed to enable connection reuse
		if resp.Body != nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
		}
	}()

	// Capture HTTP status code and determine endpoint status
	result.StatusCode = resp.StatusCode
	result.Status = determineStatus(resp.StatusCode, result.Latency, nil, domain.ErrorTypeNone)

	return result, nil
}

func injectDefaultHeaders(req *http.Request) *http.Request {
	req.Header.Set("User-Agent", fmt.Sprintf("%s-HealthChecker/%s", version.ShortName, version.Version))
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Cache-Control", "no-cache")
	return req
}

func calculateBackoffDelay(attempt int) time.Duration {
	// Use centralized backoff calculation with 25% jitter
	// 	SHERPA-198: Jitterbug - calculation was invalid, 0 jitter was being applied
	// 	28-Oct-2024 [TF]: Fixed jitter calculation to use a pseudo-random value
	return util.CalculateExponentialBackoff(attempt, DefaultBaseDelay, MaxBackoffDelay, 0.25)
}

func shouldRetry(err error, errorType domain.HealthCheckErrorType) bool {
	// Don't retry circuit breaker errors
	if errors.Is(err, ErrCircuitBreakerOpen) {
		return false
	}

	// Don't retry context cancellation
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}

	// Retry network errors and timeouts
	switch errorType {
	case domain.ErrorTypeNetwork, domain.ErrorTypeTimeout:
		return true
	case domain.ErrorTypeHTTPError:
		// Only retry certain HTTP errors
		var netErr net.Error
		if errors.As(err, &netErr) {
			return netErr.Temporary()
		}
		return false
	default:
		return false
	}
}

// classifyError determines the type of error that occurred during health checking
func classifyError(err error) domain.HealthCheckErrorType {
	if errors.Is(err, ErrCircuitBreakerOpen) {
		return domain.ErrorTypeCircuitOpen
	}

	var netErr net.Error
	if errors.As(err, &netErr) {
		if netErr.Timeout() {
			return domain.ErrorTypeTimeout
		}
		return domain.ErrorTypeNetwork
	}

	// Check for context errors
	if errors.Is(err, context.DeadlineExceeded) {
		return domain.ErrorTypeTimeout
	}
	if errors.Is(err, context.Canceled) {
		return domain.ErrorTypeNetwork
	}

	return domain.ErrorTypeHTTPError
}

// determineStatus converts HTTP response info into endpoint status
// Status logic: offline for network errors, busy for slow responses, healthy otherwise
func determineStatus(statusCode int, latency time.Duration, err error, errorType domain.HealthCheckErrorType) domain.EndpointStatus {
	if err != nil {
		switch errorType {
		case domain.ErrorTypeNetwork, domain.ErrorTypeTimeout, domain.ErrorTypeCircuitOpen:
			return domain.StatusOffline
		default:
			return domain.StatusUnhealthy
		}
	}

	if statusCode >= HealthyEndpointStatusRangeStart && statusCode < HealthyEndpointStatusRangeEnd {
		if latency > SlowResponseThreshold {
			return domain.StatusBusy
		}
		return domain.StatusHealthy
	}

	if latency > SlowResponseThreshold {
		return domain.StatusBusy
	}
	return domain.StatusUnhealthy
}

// calculateBackoff determines the next check interval and backoff multiplier
func calculateBackoff(endpoint *domain.Endpoint, success bool) (time.Duration, int) {
	if success {
		return endpoint.CheckInterval, 1
	}

	// For first failure (BackoffMultiplier is 1), keep normal interval
	// Only apply backoff on subsequent failures
	if endpoint.BackoffMultiplier <= 1 {
		// First failure - use normal interval but set multiplier to 2 for next time
		return endpoint.CheckInterval, 2
	}

	// Calculate the multiplier for subsequent failures (exponential: 2, 4, 8...)
	multiplier := endpoint.BackoffMultiplier * 2
	if multiplier > MaxBackoffMultiplier {
		multiplier = MaxBackoffMultiplier
	}

	// Use the current BackoffMultiplier for interval (not the new one)
	backoffInterval := endpoint.CheckInterval * time.Duration(endpoint.BackoffMultiplier)
	if backoffInterval > MaxBackoffSeconds {
		backoffInterval = MaxBackoffSeconds
	}

	return backoffInterval, multiplier
}
