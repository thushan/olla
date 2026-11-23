package olla

import (
	"sync"
	"testing"
	"time"

	"github.com/puzpuzpuz/xsync/v4"
	"github.com/thushan/olla/internal/adapter/proxy/config"
	"github.com/thushan/olla/internal/adapter/proxy/core"
)

// TestUpdateConfig_ConcurrentReadWrite verifies that concurrent UpdateConfig calls
// and configuration reads do not produce a data race. The race detector catches
// torn pointer reads, so this test is only meaningful under -race.
func TestUpdateConfig_ConcurrentReadWrite(t *testing.T) {
	t.Parallel()

	s := &Service{
		BaseProxyComponents: &core.BaseProxyComponents{
			Logger: createTestLogger(),
		},
		endpointPools:   *xsync.NewMap[string, *connectionPool](),
		circuitBreakers: *xsync.NewMap[string, *circuitBreaker](),
	}
	initial := &Configuration{}
	initial.ReadTimeout = 10 * time.Second
	initial.ResponseTimeout = 20 * time.Second
	s.configuration.Store(initial)

	const (
		writers = 4
		readers = 8
		iters   = 500
	)

	var wg sync.WaitGroup

	// Writers: repeatedly swap in a new config
	for w := range writers {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := range iters {
				cfg := &Configuration{}
				// Alternate between two timeout values so readers can verify
				// they always see a valid (non-zero) value, never a partial write.
				if (id+i)%2 == 0 {
					cfg.ReadTimeout = 10 * time.Second
					cfg.ResponseTimeout = 20 * time.Second
				} else {
					cfg.ReadTimeout = 30 * time.Second
					cfg.ResponseTimeout = 60 * time.Second
				}
				s.UpdateConfig(cfg)
			}
		}(w)
	}

	// Readers: load the config and assert it is coherent (both fields set together)
	for range readers {
		wg.Go(func() {
			for range iters {
				cfg := s.configuration.Load()
				rt := cfg.GetReadTimeout()
				rsp := cfg.GetResponseTimeout()
				// A torn write would produce a mix from two different configs
				// that cannot appear as a valid pair from either generation.
				// We validate that read timeout is always a recognised value.
				if rt != 10*time.Second && rt != 30*time.Second {
					t.Errorf("GetReadTimeout returned unexpected value %v — possible torn read", rt)
				}
				if rsp != 20*time.Second && rsp != 60*time.Second {
					t.Errorf("GetResponseTimeout returned unexpected value %v — possible torn read", rsp)
				}
			}
		})
	}

	wg.Wait()
}

// TestGetOrCreateEndpointPool_ComputeOnce verifies that concurrent first-use of
// getOrCreateEndpointPool for the same key returns the same *connectionPool to all
// callers. Before LoadOrCompute, losing goroutines allocated a transport that was
// silently discarded; this test proves exactly one instance wins.
func TestGetOrCreateEndpointPool_ComputeOnce(t *testing.T) {
	t.Parallel()

	s := &Service{
		BaseProxyComponents: &core.BaseProxyComponents{
			Logger: createTestLogger(),
		},
		endpointPools:   *xsync.NewMap[string, *connectionPool](),
		circuitBreakers: *xsync.NewMap[string, *circuitBreaker](),
	}
	cfg := &Configuration{}
	cfg.MaxIdleConns = 10
	cfg.MaxConnsPerHost = 5
	cfg.IdleConnTimeout = 30 * time.Second
	s.configuration.Store(cfg)

	const goroutines = 50
	results := make([]*connectionPool, goroutines)
	var wg sync.WaitGroup

	for i := range goroutines {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			results[idx] = s.getOrCreateEndpointPool("test-endpoint")
		}(i)
	}
	wg.Wait()

	// All goroutines must have received the exact same pointer.
	first := results[0]
	for i, p := range results {
		if p != first {
			t.Errorf("goroutine %d received a different *connectionPool (%p vs %p) — compute-once violated", i, p, first)
		}
	}
}

// TestGetCircuitBreaker_ComputeOnce verifies that concurrent first-use of
// GetCircuitBreaker for the same key returns the same *circuitBreaker to all callers.
func TestGetCircuitBreaker_ComputeOnce(t *testing.T) {
	t.Parallel()

	s := &Service{
		BaseProxyComponents: &core.BaseProxyComponents{
			Logger: createTestLogger(),
		},
		endpointPools:   *xsync.NewMap[string, *connectionPool](),
		circuitBreakers: *xsync.NewMap[string, *circuitBreaker](),
	}
	s.configuration.Store(&Configuration{})

	const goroutines = 50
	results := make([]*circuitBreaker, goroutines)
	var wg sync.WaitGroup

	for i := range goroutines {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			results[idx] = s.GetCircuitBreaker("test-endpoint")
		}(i)
	}
	wg.Wait()

	first := results[0]
	for i, cb := range results {
		if cb != first {
			t.Errorf("goroutine %d received a different *circuitBreaker (%p vs %p) — compute-once violated", i, cb, first)
		}
	}
}

// TestUpdateConfig_NilGuard verifies that calling UpdateConfig on a Service whose
// configuration pointer has never been stored (nil atomic) does not panic.
// This covers the else-branch nil guard added in T0-2: if current is nil we fall
// through without dereferencing it, and the new config is stored correctly.
func TestUpdateConfig_NilGuard(t *testing.T) {
	t.Parallel()

	// Construct a Service without going through NewService so the atomic is nil.
	s := &Service{
		BaseProxyComponents: &core.BaseProxyComponents{
			Logger: createTestLogger(),
		},
		endpointPools:   *xsync.NewMap[string, *connectionPool](),
		circuitBreakers: *xsync.NewMap[string, *circuitBreaker](),
	}
	// configuration atomic is zero-value — Load() returns nil.

	// Use a non-*Configuration to trigger the else-branch (which used to dereference nil).
	nonOlla := &config.SherpaConfig{}
	nonOlla.ReadTimeout = 5 * time.Second

	// Must not panic.
	s.UpdateConfig(nonOlla)

	// Config should now be stored.
	stored := s.configuration.Load()
	if stored == nil {
		t.Fatal("expected configuration to be stored after UpdateConfig, got nil")
	}
	if stored.ReadTimeout != 5*time.Second {
		t.Errorf("ReadTimeout: want 5s, got %v", stored.ReadTimeout)
	}
}
