package services

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"
)

// fakeManagedService is a minimal ManagedService for exercising ServiceManager
// without pulling in real dependencies. Stop sleeps briefly so concurrent
// Register calls have a real window to race against stopServices' map read.
// startDelay does the same for Start's map read when set.
type fakeManagedService struct {
	name       string
	deps       []string
	startDelay time.Duration
	startErr   error
	rec        *lifecycleRecorder // optional; nil for tests that ignore ordering
}

// lifecycleRecorder captures Start/Stop calls across fakes in call order. The
// mutex is needed because fakes are also driven from concurrent tests.
type lifecycleRecorder struct {
	mu          sync.Mutex
	starts      []string
	stops       []string
	stopCtxErrs []error
}

func (r *lifecycleRecorder) started(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.starts = append(r.starts, name)
}

func (r *lifecycleRecorder) stopped(name string, ctxErr error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.stops = append(r.stops, name)
	r.stopCtxErrs = append(r.stopCtxErrs, ctxErr)
}

func (r *lifecycleRecorder) snapshot() (starts, stops []string, ctxErrs []error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.starts), slices.Clone(r.stops), slices.Clone(r.stopCtxErrs)
}

func (f *fakeManagedService) Name() string           { return f.name }
func (f *fakeManagedService) Dependencies() []string { return f.deps }
func (f *fakeManagedService) Start(context.Context) error {
	if f.startDelay > 0 {
		time.Sleep(f.startDelay)
	}
	if f.startErr != nil {
		return f.startErr
	}
	if f.rec != nil {
		f.rec.started(f.name)
	}
	return nil
}
func (f *fakeManagedService) Stop(ctx context.Context) error {
	time.Sleep(time.Millisecond)
	if f.rec != nil {
		f.rec.stopped(f.name, ctx.Err())
	}
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

// TestServiceManager_Start_ConcurrentRegister is the Start-side counterpart:
// Start's per-service loop read sm.services[name] with the same missing lock
// as stopServices.
func TestServiceManager_Start_ConcurrentRegister(t *testing.T) {
	sm := NewServiceManager(newTestLogger())

	const n = 20

	// Register an initial batch so Start has real entries to iterate.
	// Each Start call sleeps briefly, the same trick fakeManagedService.Stop
	// uses above, so the Register goroutines below have a real window to
	// overlap Start's per-service map read instead of racing to finish first.
	for i := range n {
		name := "initial-" + string(rune('a'+i%26)) + string(rune('0'+i/26))
		if err := sm.Register(&fakeManagedService{name: name, startDelay: time.Millisecond}); err != nil {
			t.Fatalf("Register failed: %v", err)
		}
	}

	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := sm.Start(context.Background()); err != nil {
			t.Errorf("Start failed: %v", err)
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

// registerChain registers a -> b -> c -> d where each service depends on the previous
// one, so the start order is a, b, c, d regardless of map iteration.
func registerChain(t *testing.T, sm *ServiceManager, rec *lifecycleRecorder, failAt string, startErr error) {
	t.Helper()
	names := []string{"a", "b", "c", "d"}
	for i, n := range names {
		svc := &fakeManagedService{name: n, rec: rec}
		if i > 0 {
			svc.deps = []string{names[i-1]}
		}
		if n == failAt {
			svc.startErr = startErr
		}
		if err := sm.Register(svc); err != nil {
			t.Fatalf("Register %s: %v", n, err)
		}
	}
}

func TestServiceManager_Start_RollbackOrder(t *testing.T) {
	t.Parallel()

	boom := errors.New("boom")
	tests := []struct {
		name       string
		failAt     string
		wantStarts []string
		wantStops  []string
	}{
		{"fails midway", "c", []string{"a", "b"}, []string{"b", "a"}},
		{"fails on last", "d", []string{"a", "b", "c"}, []string{"c", "b", "a"}},
		{"first fails", "a", nil, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			rec := &lifecycleRecorder{}
			sm := NewServiceManager(newTestLogger())
			registerChain(t, sm, rec, tt.failAt, boom)

			err := sm.Start(context.Background())
			if !errors.Is(err, boom) {
				t.Fatalf("Start error = %v, want it to wrap %v", err, boom)
			}
			starts, stops, _ := rec.snapshot()
			if !slices.Equal(starts, tt.wantStarts) {
				t.Errorf("starts = %v, want %v", starts, tt.wantStarts)
			}
			if !slices.Equal(stops, tt.wantStops) {
				t.Errorf("stops = %v, want %v", stops, tt.wantStops)
			}
		})
	}
}

func TestServiceManager_Start_Success_ThenStopReversed(t *testing.T) {
	t.Parallel()

	rec := &lifecycleRecorder{}
	sm := NewServiceManager(newTestLogger())
	registerChain(t, sm, rec, "", nil)

	if err := sm.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	starts, stops, _ := rec.snapshot()
	if want := []string{"a", "b", "c", "d"}; !slices.Equal(starts, want) {
		t.Errorf("starts = %v, want %v", starts, want)
	}
	if len(stops) != 0 {
		t.Errorf("Start stopped %v on success, want none", stops)
	}

	if err := sm.Stop(context.Background()); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	_, stops, _ = rec.snapshot()
	if want := []string{"d", "c", "b", "a"}; !slices.Equal(stops, want) {
		t.Errorf("stops = %v, want %v", stops, want)
	}
}

// A cancelled ctx is a common reason Start fails; rollback must still hand
// Stop a live ctx or services like HTTPService would abort their shutdown.
func TestServiceManager_Start_RollbackIgnoresCancelledContext(t *testing.T) {
	t.Parallel()

	rec := &lifecycleRecorder{}
	sm := NewServiceManager(newTestLogger())
	registerChain(t, sm, rec, "c", context.Canceled)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := sm.Start(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("Start error = %v, want context.Canceled", err)
	}
	_, stops, ctxErrs := rec.snapshot()
	if want := []string{"b", "a"}; !slices.Equal(stops, want) {
		t.Fatalf("stops = %v, want %v", stops, want)
	}
	for i, e := range ctxErrs {
		if e != nil {
			t.Errorf("Stop(%s) saw ctx error %v, want nil", stops[i], e)
		}
	}
}

func TestServiceManager_Rollback_DoesNotMutateInput(t *testing.T) {
	t.Parallel()

	rec := &lifecycleRecorder{}
	sm := NewServiceManager(newTestLogger())
	registerChain(t, sm, rec, "", nil)

	started := []string{"a", "b", "c"}
	sm.rollback(context.Background(), started)

	if want := []string{"a", "b", "c"}; !slices.Equal(started, want) {
		t.Errorf("rollback mutated input: %v, want %v", started, want)
	}
	if _, stops, _ := rec.snapshot(); !slices.Equal(stops, []string{"c", "b", "a"}) {
		t.Errorf("stops = %v, want [c b a]", stops)
	}
}
