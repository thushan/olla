package domain

import (
	"fmt"
	"time"
)

type EndpointError struct {
	Err       error
	Operation string
	URL       string
}

func (e *EndpointError) Error() string {
	return fmt.Sprintf("%s failed for endpoint %s: %v", e.Operation, e.URL, e.Err)
}

func (e *EndpointError) Unwrap() error {
	return e.Err
}

type HealthCheckError struct {
	Err                 error
	EndpointURL         string
	EndpointName        string
	CheckType           string
	StatusCode          int
	Latency             time.Duration
	ConsecutiveFailures int
}

func (e *HealthCheckError) Error() string {
	if e.StatusCode > 0 {
		return fmt.Sprintf("health check failed for %s (%s): HTTP %d after %v (failures: %d): %v",
			e.EndpointName, e.EndpointURL, e.StatusCode, e.Latency, e.ConsecutiveFailures, e.Err)
	}
	return fmt.Sprintf("health check failed for %s (%s): %v after %v (failures: %d)",
		e.EndpointName, e.EndpointURL, e.Err, e.Latency, e.ConsecutiveFailures)
}

func (e *HealthCheckError) Unwrap() error {
	return e.Err
}

type ProxyError struct {
	Err        error
	RequestID  string
	TargetURL  string
	Method     string
	Path       string
	StatusCode int
	Latency    time.Duration
	BytesRead  int
}

func (e *ProxyError) Error() string {
	if e.StatusCode > 0 {
		return fmt.Sprintf("proxy request failed [%s] %s %s -> %s: HTTP %d after %v (%d bytes): %v",
			e.RequestID, e.Method, e.Path, e.TargetURL, e.StatusCode, e.Latency, e.BytesRead, e.Err)
	}
	return fmt.Sprintf("proxy request failed [%s] %s %s -> %s: %v after %v (%d bytes)",
		e.RequestID, e.Method, e.Path, e.TargetURL, e.Err, e.Latency, e.BytesRead)
}

func (e *ProxyError) Unwrap() error {
	return e.Err
}

type ConfigValidationError struct {
	Field  string
	Value  interface{}
	Reason string
}

func (e *ConfigValidationError) Error() string {
	return fmt.Sprintf("invalid configuration for %s=%v: %s", e.Field, e.Value, e.Reason)
}

type LoadBalancerError struct {
	Err           error
	Strategy      string
	EndpointCount int
}

func (e *LoadBalancerError) Error() string {
	return fmt.Sprintf("load balancer %s failed with %d endpoints: %v", e.Strategy, e.EndpointCount, e.Err)
}

func (e *LoadBalancerError) Unwrap() error {
	return e.Err
}

func NewEndpointError(operation, url string, err error) *EndpointError {
	return &EndpointError{
		Operation: operation,
		URL:       url,
		Err:       err,
	}
}

func NewHealthCheckError(endpoint *Endpoint, checkType string, statusCode int, latency time.Duration, err error) *HealthCheckError {
	return &HealthCheckError{
		EndpointURL:         endpoint.GetURLString(),
		EndpointName:        endpoint.Name,
		CheckType:           checkType,
		StatusCode:          statusCode,
		Latency:             latency,
		ConsecutiveFailures: endpoint.ConsecutiveFailures,
		Err:                 err,
	}
}

func NewProxyError(requestID, targetURL, method, path string, statusCode int, latency time.Duration, bytesRead int, err error) *ProxyError {
	return &ProxyError{
		RequestID:  requestID,
		TargetURL:  targetURL,
		Method:     method,
		Path:       path,
		StatusCode: statusCode,
		Latency:    latency,
		BytesRead:  bytesRead,
		Err:        err,
	}
}

/*
func NewConfigValidationError(field string, value interface{}, reason string) *ConfigValidationError {
	return &ConfigValidationError{
		Field:  field,
		Value:  value,
		Reason: reason,
	}
}

func NewLoadBalancerError(strategy string, endpointCount int, err error) *LoadBalancerError {
	return &LoadBalancerError{
		Strategy:      strategy,
		EndpointCount: endpointCount,
		Err:           err,
	}
}
*/
