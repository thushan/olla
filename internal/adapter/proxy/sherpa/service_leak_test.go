package sherpa

import (
	"context"
	"io"
	"net/http"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/thushan/olla/internal/logger"
)

// TestCreateCombinedContext_NoGoroutineLeak verifies that the context merger doesn't leak goroutines
func TestCreateCombinedContext_NoGoroutineLeak(t *testing.T) {
	// Get initial goroutine count
	runtime.GC()
	time.Sleep(10 * time.Millisecond)
	initialGoroutines := runtime.NumGoroutine()

	s := &Service{}

	// Create many contexts and ensure goroutines are cleaned up
	for i := 0; i < 100; i++ {
		ctx1, cancel1 := context.WithCancel(context.Background())
		ctx2, cancel2 := context.WithCancel(context.Background())

		combinedCtx, combinedCancel := s.createCombinedContext(ctx1, ctx2)

		// Cancel one of the contexts
		if i%2 == 0 {
			cancel1()
		} else {
			cancel2()
		}

		// Wait for the combined context to be cancelled
		<-combinedCtx.Done()

		// Clean up
		combinedCancel()
		cancel1()
		cancel2()
	}

	// Give goroutines time to exit
	time.Sleep(100 * time.Millisecond)
	runtime.GC()
	time.Sleep(10 * time.Millisecond)

	// Check goroutine count
	finalGoroutines := runtime.NumGoroutine()
	leaked := finalGoroutines - initialGoroutines

	if leaked > 2 { // Allow small variance for test framework
		t.Errorf("Goroutine leak detected: initial=%d, final=%d, leaked=%d",
			initialGoroutines, finalGoroutines, leaked)
	}
}

// TestPerformTimedRead_NoGoroutineLeak verifies read operations don't leak goroutines
func TestPerformTimedRead_NoGoroutineLeak(t *testing.T) {
	// Get initial goroutine count
	runtime.GC()
	time.Sleep(10 * time.Millisecond)
	initialGoroutines := runtime.NumGoroutine()

	s := &Service{
		configuration: &Configuration{
			ReadTimeout: 10 * time.Millisecond, // Much shorter for tests
		},
	}

	logger := createTestLogger()

	// Run timeout tests concurrently to speed up
	var wg sync.WaitGroup
	const numTimeoutTests = 20 // Reduced from 50

	for i := 0; i < numTimeoutTests; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx := context.Background()

			// Create a new state for each goroutine to avoid race conditions
			localState := &streamState{}

			// Use a reader that delays then returns data
			reader := &delayedReader{delay: 50 * time.Millisecond} // Still longer than timeout

			// Create a new buffer for each goroutine
			localBuffer := make([]byte, 1024)

			// This should timeout after 10ms
			result, err := s.performTimedRead(ctx, reader, localBuffer, 10*time.Millisecond, localState, logger)
			if err == nil && result != nil {
				t.Error("Expected timeout but got result")
			}
		}()
	}

	// Test context cancellation concurrently too
	const numCancelTests = 20 // Reduced from 50
	for i := 0; i < numCancelTests; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithCancel(context.Background())

			// Create a new state for each goroutine to avoid race conditions
			localState := &streamState{}

			// Use a reader that returns data immediately
			reader := strings.NewReader("test data")

			// Create a new buffer for each goroutine
			localBuffer := make([]byte, 1024)

			// Cancel context immediately
			cancel()

			// This should return nil due to context cancellation
			s.performTimedRead(ctx, reader, localBuffer, 10*time.Millisecond, localState, logger)
		}()
	}

	// Wait for all tests to complete
	wg.Wait()

	// Give goroutines time to exit
	time.Sleep(50 * time.Millisecond)
	runtime.GC()
	time.Sleep(10 * time.Millisecond)

	// Check goroutine count
	finalGoroutines := runtime.NumGoroutine()
	leaked := finalGoroutines - initialGoroutines

	if leaked > 5 { // Allow slightly more variance for concurrent execution
		t.Errorf("Goroutine leak detected: initial=%d, final=%d, leaked=%d",
			initialGoroutines, finalGoroutines, leaked)
	}
}

// TestStreamResponseWithTimeout_ClientDisconnect verifies no goroutine leaks on client disconnect
func TestStreamResponseWithTimeout_ClientDisconnect(t *testing.T) {
	// Get initial goroutine count
	runtime.GC()
	time.Sleep(10 * time.Millisecond)
	initialGoroutines := runtime.NumGoroutine()

	s := &Service{
		configuration: &Configuration{
			ReadTimeout: 100 * time.Millisecond,
		},
	}

	// Create a reader that streams data slowly
	reader := strings.NewReader(strings.Repeat("test data ", 1000))
	buffer := make([]byte, 1024)
	logger := createTestLogger()

	// Simulate multiple client disconnections
	for i := 0; i < 20; i++ {
		clientCtx, clientCancel := context.WithCancel(context.Background())
		upstreamCtx := context.Background()
		writer := &mockWriter{}

		// Start streaming in goroutine
		done := make(chan struct{})
		go func() {
			defer close(done)

			// create a mock response with the reader as body
			resp := &http.Response{
				Body: io.NopCloser(reader),
				Header: http.Header{
					"Content-Type": []string{"text/plain"},
				},
			}
			_, _, _ = s.streamResponseWithTimeout(clientCtx, upstreamCtx, writer, resp, buffer, logger)
		}()

		// Simulate client disconnect after short time
		time.Sleep(10 * time.Millisecond)
		clientCancel()

		// Wait for streaming to complete
		select {
		case <-done:
			// Good
		case <-time.After(500 * time.Millisecond):
			t.Fatal("streamResponseWithTimeout did not complete after client disconnect")
		}

		// Reset reader for next iteration
		reader.Seek(0, io.SeekStart)
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
}

// Helper types for testing

type delayedReader struct {
	delay time.Duration
	read  bool
}

func (d *delayedReader) Read(p []byte) (n int, err error) {
	if !d.read {
		time.Sleep(d.delay)
		d.read = true
		copy(p, []byte("delayed data"))
		return 12, nil
	}
	return 0, io.EOF
}

type mockWriter struct {
	data   []byte
	header http.Header
}

func (m *mockWriter) Write(p []byte) (n int, err error) {
	m.data = append(m.data, p...)
	return len(p), nil
}

func (m *mockWriter) Header() http.Header {
	if m.header == nil {
		m.header = make(http.Header)
	}
	return m.header
}

func (m *mockWriter) WriteHeader(statusCode int) {
	// Mock implementation
}

func createTestLogger() logger.StyledLogger {
	loggerCfg := &logger.Config{Level: "error", Theme: "default"}
	log, _, _ := logger.New(loggerCfg)
	return logger.NewPlainStyledLogger(log)
}
