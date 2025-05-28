package health

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/thushan/olla/internal/core/domain"
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

// Check performs a single health check against an endpoint
func (hc *HealthClient) Check(ctx context.Context, endpoint *domain.Endpoint) (domain.HealthCheckResult, error) {
	start := time.Now()

	healthCheckUrl := endpoint.GetHealthCheckURLString()

	result := domain.HealthCheckResult{
		Status: domain.StatusUnknown,
	}

	if hc.circuitBreaker.IsOpen(healthCheckUrl) {
		result.Status = domain.StatusOffline
		result.Error = ErrCircuitBreakerOpen
		result.ErrorType = domain.ErrorTypeCircuitOpen
		result.Latency = time.Since(start)
		return result, ErrCircuitBreakerOpen
	}

	checkCtx, cancel := context.WithTimeout(ctx, endpoint.CheckTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(checkCtx, http.MethodGet, healthCheckUrl, nil)
	if err != nil {
		result.Latency = time.Since(start)
		result.Error = err
		result.ErrorType = classifyError(err)
		result.Status = determineStatus(0, result.Latency, err, result.ErrorType)
		hc.circuitBreaker.RecordFailure(healthCheckUrl)
		return result, err
	}

	resp, err := hc.client.Do(req)
	result.Latency = time.Since(start)

	if err != nil {
		result.Error = err
		result.ErrorType = classifyError(err)
		result.Status = determineStatus(0, result.Latency, err, result.ErrorType)
		hc.circuitBreaker.RecordFailure(healthCheckUrl)
		return result, err
	}
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)

	result.Status = determineStatus(resp.StatusCode, result.Latency, nil, domain.ErrorTypeNone)

	if result.Status == domain.StatusHealthy {
		hc.circuitBreaker.RecordSuccess(healthCheckUrl)
	} else {
		hc.circuitBreaker.RecordFailure(healthCheckUrl)
	}

	return result, nil
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

	// Double the backoff up to max
	multiplier := endpoint.BackoffMultiplier * 2
	if multiplier > MaxBackoffMultiplier {
		multiplier = MaxBackoffMultiplier
	}

	backoffInterval := endpoint.CheckInterval * time.Duration(multiplier)
	return backoffInterval, multiplier
}