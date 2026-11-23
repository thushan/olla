package services

import (
	"context"
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/core/ports"
	"github.com/thushan/olla/internal/logger"
)

func newTestLogger() logger.StyledLogger {
	cfg := &logger.Config{Level: "error", Theme: "default"}
	log, _, _ := logger.New(cfg)
	return logger.NewPlainStyledLogger(log)
}

// spyProxyService is a minimal ports.ProxyService that also implements the
// cleanupable interface. It counts Cleanup invocations for assertion.
type spyProxyService struct {
	cleanupCalls atomic.Int32
}

func (s *spyProxyService) ProxyRequest(_ context.Context, _ http.ResponseWriter, _ *http.Request, _ *ports.RequestStats, _ logger.StyledLogger) error {
	return nil
}

func (s *spyProxyService) ProxyRequestToEndpoints(_ context.Context, _ http.ResponseWriter, _ *http.Request, _ []*domain.Endpoint, _ *ports.RequestStats, _ logger.StyledLogger) error {
	return nil
}

func (s *spyProxyService) GetStats(_ context.Context) (ports.ProxyStats, error) {
	return ports.ProxyStats{}, nil
}

func (s *spyProxyService) UpdateConfig(_ ports.ProxyConfiguration) {}

// Cleanup satisfies the optional cleanupable interface.
func (s *spyProxyService) Cleanup() {
	s.cleanupCalls.Add(1)
}

// TestProxyServiceWrapper_Stop_CallsCleanup asserts that Stop invokes the proxy
// engine's Cleanup when the engine implements cleanupable.
func TestProxyServiceWrapper_Stop_CallsCleanup(t *testing.T) {
	t.Parallel()

	spy := &spyProxyService{}
	w := &ProxyServiceWrapper{
		logger:       newTestLogger(),
		proxyService: spy,
	}

	if err := w.Stop(t.Context()); err != nil {
		t.Fatalf("Stop returned unexpected error: %v", err)
	}
	if got := spy.cleanupCalls.Load(); got != 1 {
		t.Errorf("Cleanup called %d times, want 1", got)
	}
}

// TestProxyServiceWrapper_Stop_DoubleCallDoesNotPanic asserts that calling Stop
// twice does not panic. Engine-level idempotency (sync.Once) is exercised
// separately in the olla service tests.
func TestProxyServiceWrapper_Stop_DoubleCallDoesNotPanic(t *testing.T) {
	t.Parallel()

	spy := &spyProxyService{}
	w := &ProxyServiceWrapper{
		logger:       newTestLogger(),
		proxyService: spy,
	}

	for range 2 {
		if err := w.Stop(t.Context()); err != nil {
			t.Fatalf("Stop returned error: %v", err)
		}
	}
	// The wrapper does not deduplicate; each Stop call propagates to the engine.
	// The engine's own sync.Once handles its internal idempotency.
	if got := spy.cleanupCalls.Load(); got != 2 {
		t.Errorf("Cleanup called %d times, want 2", got)
	}
}

// bareProxyService implements ports.ProxyService but NOT cleanupable.
type bareProxyService struct{}

func (b *bareProxyService) ProxyRequest(_ context.Context, _ http.ResponseWriter, _ *http.Request, _ *ports.RequestStats, _ logger.StyledLogger) error {
	return nil
}

func (b *bareProxyService) ProxyRequestToEndpoints(_ context.Context, _ http.ResponseWriter, _ *http.Request, _ []*domain.Endpoint, _ *ports.RequestStats, _ logger.StyledLogger) error {
	return nil
}

func (b *bareProxyService) GetStats(_ context.Context) (ports.ProxyStats, error) {
	return ports.ProxyStats{}, nil
}

func (b *bareProxyService) UpdateConfig(_ ports.ProxyConfiguration) {}

// TestProxyServiceWrapper_Stop_NoCleanupable asserts Stop does not panic when the
// proxy engine does not implement cleanupable.
func TestProxyServiceWrapper_Stop_NoCleanupable(t *testing.T) {
	t.Parallel()

	w := &ProxyServiceWrapper{
		logger:       newTestLogger(),
		proxyService: &bareProxyService{},
	}
	if err := w.Stop(t.Context()); err != nil {
		t.Fatalf("Stop returned unexpected error: %v", err)
	}
}
