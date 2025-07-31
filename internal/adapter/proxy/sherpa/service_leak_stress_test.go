package sherpa

import (
	"context"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestPerformTimedRead_NoGoroutineLeak_Stress runs the original comprehensive leak test
// This test is skipped in CI (when -short flag is used)
func TestPerformTimedRead_NoGoroutineLeak_Stress(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}
	// Get initial goroutine count
	runtime.GC()
	time.Sleep(10 * time.Millisecond)
	initialGoroutines := runtime.NumGoroutine()

	s := &Service{
		configuration: &Configuration{
			ReadTimeout: 100 * time.Millisecond, // Original timeout
		},
	}

	state := &streamState{}
	logger := createTestLogger()

	// Run many reads that will timeout (original 50 iterations)
	for i := 0; i < 50; i++ {
		ctx := context.Background()

		// Use a reader that delays then returns data
		reader := &delayedReader{delay: 150 * time.Millisecond} // Longer than read timeout

		// Create a new buffer for each iteration to avoid race
		localBuffer := make([]byte, 1024)

		// This should timeout after 100ms
		result, err := s.performTimedRead(ctx, reader, localBuffer, 100*time.Millisecond, state, logger)
		if err == nil && result != nil {
			t.Fatal("Expected timeout but got result")
		}
	}

	// Also test context cancellation with non-blocking reader
	for i := 0; i < 50; i++ {
		ctx, cancel := context.WithCancel(context.Background())

		// Use a reader that returns data immediately
		reader := strings.NewReader("test data")

		// Create a new buffer for each iteration to avoid race
		localBuffer := make([]byte, 1024)

		// Cancel context immediately
		cancel()

		// This should return nil due to context cancellation
		s.performTimedRead(ctx, reader, localBuffer, 100*time.Millisecond, state, logger)
	}

	// Give goroutines time to exit
	time.Sleep(100 * time.Millisecond)
	runtime.GC()
	time.Sleep(10 * time.Millisecond)

	// Check goroutine count
	finalGoroutines := runtime.NumGoroutine()
	leaked := finalGoroutines - initialGoroutines

	if leaked > 2 { // Allow small variance
		t.Errorf("Goroutine leak detected: initial=%d, final=%d, leaked=%d",
			initialGoroutines, finalGoroutines, leaked)
	}

	t.Logf("STRESS TEST completed - tested 100 timeout scenarios sequentially")
}

// TestPerformTimedRead_ConcurrentStress tests many concurrent reads
func TestPerformTimedRead_ConcurrentStress(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping concurrent stress test in short mode")
	}
	// Get initial goroutine count
	runtime.GC()
	time.Sleep(10 * time.Millisecond)
	initialGoroutines := runtime.NumGoroutine()

	s := &Service{
		configuration: &Configuration{
			ReadTimeout: 50 * time.Millisecond,
		},
	}

	state := &streamState{}
	logger := createTestLogger()

	// Run MANY concurrent reads
	var wg sync.WaitGroup
	const numConcurrent = 100

	for i := 0; i < numConcurrent; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx := context.Background()

			// Mix of timeout and successful reads
			if i%2 == 0 {
				reader := &delayedReader{delay: 100 * time.Millisecond}
				localBuffer := make([]byte, 1024)
				s.performTimedRead(ctx, reader, localBuffer, 50*time.Millisecond, state, logger)
			} else {
				reader := strings.NewReader("quick data")
				localBuffer := make([]byte, 1024)
				s.performTimedRead(ctx, reader, localBuffer, 50*time.Millisecond, state, logger)
			}
		}()
	}

	wg.Wait()

	// Give goroutines time to exit
	time.Sleep(200 * time.Millisecond)
	runtime.GC()
	time.Sleep(10 * time.Millisecond)

	// Check goroutine count
	finalGoroutines := runtime.NumGoroutine()
	leaked := finalGoroutines - initialGoroutines

	if leaked > 5 { // Allow slightly more variance for high concurrency
		t.Errorf("Goroutine leak detected under high concurrency: initial=%d, final=%d, leaked=%d",
			initialGoroutines, finalGoroutines, leaked)
	}

	t.Logf("CONCURRENT STRESS TEST completed - tested %d concurrent reads", numConcurrent)
}
