package olla

import (
	"testing"
	"time"

	"github.com/puzpuzpuz/xsync/v4"
	"github.com/thushan/olla/internal/adapter/health"
	"github.com/thushan/olla/internal/core/domain"
)

func TestServiceCircuitBreaker_PerEndpointOverrides(t *testing.T) {
	s := &Service{
		circuitBreakers: *xsync.NewMap[string, *circuitBreaker](),
	}
	endpoint := &domain.Endpoint{
		Name:                    "slow-vllm",
		CircuitBreakerTimeout:   10 * time.Millisecond,
		CircuitBreakerThreshold: 2,
	}

	cb := s.getCircuitBreakerForEndpoint(endpoint)
	if cb.threshold != 2 {
		t.Fatalf("threshold = %d, want 2", cb.threshold)
	}
	if cb.timeout != 10*time.Millisecond {
		t.Fatalf("timeout = %v, want 10ms", cb.timeout)
	}

	cb.RecordFailure()
	if cb.IsOpen() {
		t.Fatal("breaker opened before endpoint threshold")
	}

	cb.RecordFailure()
	if !cb.IsOpen() {
		t.Fatal("breaker did not open at endpoint threshold")
	}

	time.Sleep(20 * time.Millisecond)
	if cb.IsOpen() {
		t.Fatal("breaker did not use endpoint timeout for half-open transition")
	}
}

func TestServiceCircuitBreaker_DefaultsRemainUnchanged(t *testing.T) {
	s := &Service{
		circuitBreakers: *xsync.NewMap[string, *circuitBreaker](),
	}

	cb := s.GetCircuitBreaker("default-endpoint")
	if cb.threshold != proxyCircuitBreakerThreshold {
		t.Fatalf("threshold = %d, want %d", cb.threshold, proxyCircuitBreakerThreshold)
	}
	if cb.timeout != health.DefaultCircuitBreakerTimeout {
		t.Fatalf("timeout = %v, want %v", cb.timeout, health.DefaultCircuitBreakerTimeout)
	}
}
