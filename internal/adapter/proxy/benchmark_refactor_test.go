package proxy

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/thushan/olla/internal/adapter/proxy/core"
	"github.com/thushan/olla/internal/adapter/proxy/olla"
	"github.com/thushan/olla/internal/adapter/proxy/sherpa"
	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/core/ports"
	"github.com/thushan/olla/pkg/eventbus"
	"github.com/thushan/olla/pkg/pool"
)

// BenchmarkRefactoredImplementations benchmarks the refactored proxy implementations
func BenchmarkRefactoredImplementations(b *testing.B) {
	b.Run("Sherpa_Refactored", func(b *testing.B) {
		runRefactoredProxyBenchmarks(b, "sherpa")
	})

	b.Run("Olla_Refactored", func(b *testing.B) {
		runRefactoredProxyBenchmarks(b, "olla")
	})
}

func runRefactoredProxyBenchmarks(b *testing.B, proxyType string) {
	b.Run("SimpleRequest", func(b *testing.B) {
		benchmarkRefactoredSimpleRequest(b, proxyType)
	})

	b.Run("ErrorHandling", func(b *testing.B) {
		benchmarkRefactoredErrorHandling(b, proxyType)
	})

	b.Run("HeaderProcessing", func(b *testing.B) {
		benchmarkRefactoredHeaderProcessing(b, proxyType)
	})

	b.Run("StreamingResponse", func(b *testing.B) {
		benchmarkRefactoredStreamingResponse(b, proxyType)
	})

	b.Run("ConfigUpdates", func(b *testing.B) {
		benchmarkRefactoredConfigUpdates(b, proxyType)
	})

	b.Run("StatsCollection", func(b *testing.B) {
		benchmarkRefactoredStatsCollection(b, proxyType)
	})
}

func createRefactoredProxy(proxyType string, endpoints []*domain.Endpoint) (ports.ProxyService, error) {
	discovery := &mockDiscoveryService{endpoints: endpoints}
	selector := &mockEndpointSelector{}
	collector := createTestStatsCollector()
	logger := createTestLogger()

	switch proxyType {
	case "sherpa":
		config := &sherpa.Configuration{
			ResponseTimeout:  30 * time.Second,
			ReadTimeout:      10 * time.Second,
			StreamBufferSize: 8192,
		}
		return sherpa.NewService(discovery, selector, config, collector, logger)
	case "olla":
		config := &olla.Configuration{
			ResponseTimeout:  30 * time.Second,
			ReadTimeout:      10 * time.Second,
			StreamBufferSize: 8192,
			MaxIdleConns:     200,
			IdleConnTimeout:  90 * time.Second,
			MaxConnsPerHost:  50,
		}
		return olla.NewService(discovery, selector, config, collector, logger)
	default:
		return nil, fmt.Errorf("unknown proxy type: %s", proxyType)
	}
}

func benchmarkRefactoredSimpleRequest(b *testing.B, proxyType string) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}))
	defer upstream.Close()

	endpoint := createTestEndpoint("test", upstream.URL, domain.StatusHealthy)
	proxy, err := createRefactoredProxy(proxyType, []*domain.Endpoint{endpoint})
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest("GET", "/api/test", nil)
		w := httptest.NewRecorder()
		stats := &ports.RequestStats{
			RequestID: fmt.Sprintf("bench-%d", i),
			StartTime: time.Now(),
		}

		err := proxy.ProxyRequest(context.Background(), w, req, stats, createTestLogger())
		if err != nil {
			b.Fatal(err)
		}

		if w.Code != http.StatusOK {
			b.Fatalf("unexpected status code: %d", w.Code)
		}
	}
}

func benchmarkRefactoredErrorHandling(b *testing.B, proxyType string) {
	// Failing upstream
	endpoint := createTestEndpoint("test", "http://localhost:99999", domain.StatusHealthy)
	proxy, err := createRefactoredProxy(proxyType, []*domain.Endpoint{endpoint})
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest("GET", "/api/test", nil)
		w := httptest.NewRecorder()
		stats := &ports.RequestStats{
			RequestID: fmt.Sprintf("bench-%d", i),
			StartTime: time.Now(),
		}

		_ = proxy.ProxyRequest(context.Background(), w, req, stats, createTestLogger())
	}
}

func benchmarkRefactoredHeaderProcessing(b *testing.B, proxyType string) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Echo back headers
		for k, v := range r.Header {
			w.Header()[k] = v
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	endpoint := createTestEndpoint("test", upstream.URL, domain.StatusHealthy)
	proxy, err := createRefactoredProxy(proxyType, []*domain.Endpoint{endpoint})
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest("GET", "/api/test", nil)
		// Add multiple headers
		for j := 0; j < 10; j++ {
			req.Header.Add(fmt.Sprintf("X-Custom-%d", j), fmt.Sprintf("value-%d", j))
		}

		w := httptest.NewRecorder()
		stats := &ports.RequestStats{
			RequestID: fmt.Sprintf("bench-%d", i),
			StartTime: time.Now(),
		}

		err := proxy.ProxyRequest(context.Background(), w, req, stats, createTestLogger())
		if err != nil {
			b.Fatal(err)
		}
	}
}

func benchmarkRefactoredStreamingResponse(b *testing.B, proxyType string) {
	// 10KB response
	responseData := strings.Repeat("x", 10*1024)

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)

		// Stream in chunks
		for i := 0; i < 10; i++ {
			w.Write([]byte(responseData[:1024]))
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
		}
	}))
	defer upstream.Close()

	endpoint := createTestEndpoint("test", upstream.URL, domain.StatusHealthy)
	proxy, err := createRefactoredProxy(proxyType, []*domain.Endpoint{endpoint})
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest("GET", "/api/test", nil)
		w := &discardResponseWriter{header: make(http.Header)}
		stats := &ports.RequestStats{
			RequestID: fmt.Sprintf("bench-%d", i),
			StartTime: time.Now(),
		}

		err := proxy.ProxyRequest(context.Background(), w, req, stats, createTestLogger())
		if err != nil {
			b.Fatal(err)
		}
	}
}

func benchmarkRefactoredConfigUpdates(b *testing.B, proxyType string) {
	proxy, err := createRefactoredProxy(proxyType, []*domain.Endpoint{})
	if err != nil {
		b.Fatal(err)
	}

	// Create a simple config that implements ProxyConfiguration interface
	configs := make([]*Configuration, 10)
	for i := 0; i < 10; i++ {
		configs[i] = &Configuration{
			ResponseTimeout:  time.Duration(i+1) * time.Second,
			ReadTimeout:      time.Duration(i+1) * time.Second,
			StreamBufferSize: 8192 * (i + 1),
		}
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		configIndex := i % len(configs)
		proxy.UpdateConfig(configs[configIndex])
	}
}

func benchmarkRefactoredStatsCollection(b *testing.B, proxyType string) {
	proxy, err := createRefactoredProxy(proxyType, []*domain.Endpoint{})
	if err != nil {
		b.Fatal(err)
	}

	ctx := context.Background()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := proxy.GetStats(ctx)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkEventBusPublish benchmarks the EventBus publish performance
func BenchmarkEventBusPublish(b *testing.B) {
	bus := eventbus.New[core.ProxyEvent]()
	defer bus.Shutdown()

	// Create some subscribers
	for i := 0; i < 10; i++ {
		ch, cleanup := bus.Subscribe(context.Background())
		defer cleanup()

		// Drain events in background
		go func() {
			for range ch {
				// Discard
			}
		}()
	}

	event := core.ProxyEvent{
		Type: core.EventType("test.event"),
		Metadata: core.ProxyEventMetadata{
			Counter: 1,
			Message: "test",
		},
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		bus.Publish(event)
	}
}

// BenchmarkPoolGetPut benchmarks the LitePool performance
func BenchmarkPoolGetPut(b *testing.B) {
	type testStruct struct {
		data []byte
	}

	pool, err := pool.NewLitePool(func() *testStruct {
		return &testStruct{
			data: make([]byte, 8192),
		}
	})
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		obj := pool.Get()
		// Use the object
		obj.data[0] = byte(i)
		pool.Put(obj)
	}
}

// Specific benchmarks for Olla features
func BenchmarkOllaCircuitBreaker(b *testing.B) {
	proxy, err := createRefactoredProxy("olla", []*domain.Endpoint{})
	if err != nil {
		b.Fatal(err)
	}

	// Type assert to access Olla-specific features
	ollaProxy, ok := proxy.(*olla.Service)
	if !ok {
		b.Fatal("expected Olla proxy")
	}

	cb := ollaProxy.GetCircuitBreaker("test-endpoint")

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// Alternate between checking state and recording success/failure
		if i%3 == 0 {
			cb.RecordFailure()
		} else if i%3 == 1 {
			cb.RecordSuccess()
		} else {
			cb.IsOpen()
		}
	}
}

// BenchmarkOllaObjectPools benchmarks Olla's object pool usage
func BenchmarkOllaObjectPools(b *testing.B) {
	b.Run("BufferPool", func(b *testing.B) {
		bufferPool, err := pool.NewLitePool(func() *[]byte {
			buf := make([]byte, 64*1024)
			return &buf
		})
		if err != nil {
			b.Fatal(err)
		}

		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			buf := bufferPool.Get()
			bufferPool.Put(buf)
		}
	})

	b.Run("RequestContextPool", func(b *testing.B) {
		type requestContext struct {
			requestID string
			startTime time.Time
			endpoint  string
			targetURL string
		}

		reqPool, err := pool.NewLitePool(func() *requestContext {
			return &requestContext{}
		})
		if err != nil {
			b.Fatal(err)
		}

		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			ctx := reqPool.Get()
			reqPool.Put(ctx)
		}
	})
}

// discardResponseWriter is a ResponseWriter that discards all output
type discardResponseWriter struct {
	header http.Header
	code   int
}

func (d *discardResponseWriter) Header() http.Header {
	return d.header
}

func (d *discardResponseWriter) Write(b []byte) (int, error) {
	return len(b), nil
}

func (d *discardResponseWriter) WriteHeader(statusCode int) {
	d.code = statusCode
}
