package services

import (
	"context"
	"sync"
	"testing"
	"time"
)

// fakeManagedService is a minimal ManagedService for exercising ServiceManager
// without pulling in real dependencies. Stop sleeps briefly so concurrent
// Register calls have a real window to race against stopServices' map read.
type fakeManagedService struct {
	name string
	deps []string
}

func (f *fakeManagedService) Name() string                { return f.name }
func (f *fakeManagedService) Dependencies() []string      { return f.deps }
func (f *fakeManagedService) Start(context.Context) error { return nil }
func (f *fakeManagedService) Stop(context.Context) error {
	time.Sleep(time.Millisecond)
	return nil
}

// TestServiceManager_StopServices_ConcurrentRegister is the regression test
// for a lockless read of sm.services in stopServices: Register takes sm.mu
// (an exclusive lock) to write the map, but stopServices used to read it with
// no lock at all, racing with concurrent registrations. Run with -race.
func TestServiceManager_StopServices_ConcurrentRegister(t *testing.T) {
	sm := NewServiceManager(newTestLogger())

	const n = 20

	// Register an initial batch so stopServices has real entries to find.
	initial := make([]string, 0, n)
	for i := range n {
		name := "initial-" + string(rune('a'+i%26)) + string(rune('0'+i/26))
		if err := sm.Register(&fakeManagedService{name: name}); err != nil {
			t.Fatalf("Register failed: %v", err)
		}
		initial = append(initial, name)
	}

	var wg sync.WaitGroup

	// Concurrently stop the initial batch while registering a second batch -
	// stopServices' map read and Register's map write must not race.
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := sm.stopServices(context.Background(), initial); err != nil {
			t.Errorf("stopServices failed: %v", err)
		}
	}()

	for i := range n {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			name := "extra-" + string(rune('a'+i%26)) + string(rune('0'+i/26))
			_ = sm.Register(&fakeManagedService{name: name})
		}(i)
	}

	wg.Wait()
}
