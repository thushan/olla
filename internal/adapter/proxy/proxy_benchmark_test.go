package proxy

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/core/ports"
)

// BenchmarkProxyImplementations benchmarks both proxy implementations
func BenchmarkProxyImplementations(b *testing.B) {
	suites := []ProxyTestSuite{
		SherpaTestSuite{},
		OllaTestSuite{},
	}

	for _, suite := range suites {
		b.Run(suite.Name(), func(b *testing.B) {
			runProxyBenchmarks(b, suite)
		})
	}
}

func runProxyBenchmarks(b *testing.B, suite ProxyTestSuite) {
	b.Run("SimpleRequest", func(b *testing.B) {
		benchmarkSimpleRequest(b, suite)
	})

	b.Run("SmallPayload", func(b *testing.B) {
		benchmarkSmallPayload(b, suite)
	})

	b.Run("LargePayload", func(b *testing.B) {
		benchmarkLargePayload(b, suite)
	})

	b.Run("StreamingResponse", func(b *testing.B) {
		benchmarkStreamingResponse(b, suite)
	})

	b.Run("ConcurrentRequests", func(b *testing.B) {
		benchmarkConcurrentRequests(b, suite)
	})

	b.Run("HighThroughput", func(b *testing.B) {
		benchmarkHighThroughput(b, suite)
	})

	b.Run("ErrorHandling", func(b *testing.B) {
		benchmarkErrorHandling(b, suite)
	})

	b.Run("ConfigUpdates", func(b *testing.B) {
		benchmarkConfigUpdates(b, suite)
	})

	b.Run("StatsCollection", func(b *testing.B) {
		benchmarkStatsCollection(b, suite)
	})

	b.Run("HeaderProcessing", func(b *testing.B) {
		benchmarkHeaderProcessing(b, suite)
	})
}

// benchmarkSimpleRequest tests basic request/response performance
func benchmarkSimpleRequest(b *testing.B, suite ProxyTestSuite) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}))
	defer upstream.Close()

	endpoint := createTestEndpoint("bench", upstream.URL, domain.StatusHealthy)
	discovery := &mockDiscoveryService{endpoints: []*domain.Endpoint{endpoint}}
	selector := &mockEndpointSelector{endpoint: endpoint}
	collector := createTestStatsCollector()
	config := suite.CreateConfig()

	proxy := suite.CreateProxy(discovery, selector, config, collector)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		req, stats, rlog := createTestRequestWithBody("GET", "/api/test", "")
		w := httptest.NewRecorder()

		err := proxy.ProxyRequest(req.Context(), w, req, stats, rlog)
		if err != nil {
			b.Fatalf("Request failed: %v", err)
		}
	}
}

// benchmarkSmallPayload tests performance with small JSON payloads
func benchmarkSmallPayload(b *testing.B, suite ProxyTestSuite) {
	responseData := `{"status": "ok", "data": {"id": 123, "name": "test"}}`

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(responseData))
	}))
	defer upstream.Close()

	endpoint := createTestEndpoint("bench", upstream.URL, domain.StatusHealthy)
	discovery := &mockDiscoveryService{endpoints: []*domain.Endpoint{endpoint}}
	selector := &mockEndpointSelector{endpoint: endpoint}
	collector := createTestStatsCollector()
	config := suite.CreateConfig()

	proxy := suite.CreateProxy(discovery, selector, config, collector)

	requestBody := `{"query": "benchmark test", "limit": 10}`

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		req, stats, rlog := createTestRequestWithBody("POST", "/api/query", requestBody)
		w := httptest.NewRecorder()

		err := proxy.ProxyRequest(req.Context(), w, req, stats, rlog)
		if err != nil {
			b.Fatalf("Request failed: %v", err)
		}
	}
}

// benchmarkLargePayload tests performance with large payloads
func benchmarkLargePayload(b *testing.B, suite ProxyTestSuite) {
	// 100KB response
	largeData := strings.Repeat("data", 25000)

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(largeData))
	}))
	defer upstream.Close()

	endpoint := createTestEndpoint("bench", upstream.URL, domain.StatusHealthy)
	discovery := &mockDiscoveryService{endpoints: []*domain.Endpoint{endpoint}}
	selector := &mockEndpointSelector{endpoint: endpoint}
	collector := createTestStatsCollector()
	config := suite.CreateConfig()

	proxy := suite.CreateProxy(discovery, selector, config, collector)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		req, stats, rlog := createTestRequestWithBody("GET", "/api/large", "")
		w := httptest.NewRecorder()

		err := proxy.ProxyRequest(req.Context(), w, req, stats, rlog)
		if err != nil {
			b.Fatalf("Request failed: %v", err)
		}
	}
}

// benchmarkStreamingResponse tests streaming performance
func benchmarkStreamingResponse(b *testing.B, suite ProxyTestSuite) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)

		flusher := w.(http.Flusher)
		for i := 0; i < 10; i++ {
			fmt.Fprintf(w, "chunk %d\n", i)
			flusher.Flush()
		}
	}))
	defer upstream.Close()

	endpoint := createTestEndpoint("bench", upstream.URL, domain.StatusHealthy)
	discovery := &mockDiscoveryService{endpoints: []*domain.Endpoint{endpoint}}
	selector := &mockEndpointSelector{endpoint: endpoint}
	collector := createTestStatsCollector()
	config := suite.CreateConfig()

	proxy := suite.CreateProxy(discovery, selector, config, collector)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		req, stats, rlog := createTestRequestWithBody("GET", "/api/stream", "")
		w := httptest.NewRecorder()

		err := proxy.ProxyRequest(req.Context(), w, req, stats, rlog)
		if err != nil {
			b.Fatalf("Request failed: %v", err)
		}
	}
}

// benchmarkConcurrentRequests tests performance under concurrent load
func benchmarkConcurrentRequests(b *testing.B, suite ProxyTestSuite) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}))
	defer upstream.Close()

	endpoint := createTestEndpoint("bench", upstream.URL, domain.StatusHealthy)
	discovery := &mockDiscoveryService{endpoints: []*domain.Endpoint{endpoint}}
	selector := &mockEndpointSelector{endpoint: endpoint}
	collector := createTestStatsCollector()
	config := suite.CreateConfig()

	proxy := suite.CreateProxy(discovery, selector, config, collector)

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			req, stats, rlog := createTestRequestWithBody("GET", "/api/test", "")
			w := httptest.NewRecorder()

			err := proxy.ProxyRequest(req.Context(), w, req, stats, rlog)
			if err != nil {
				b.Fatalf("Request failed: %v", err)
			}
		}
	})
}

// benchmarkHighThroughput tests maximum throughput capabilities
func benchmarkHighThroughput(b *testing.B, suite ProxyTestSuite) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Minimal processing time
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}))
	defer upstream.Close()

	endpoint := createTestEndpoint("bench", upstream.URL, domain.StatusHealthy)
	discovery := &mockDiscoveryService{endpoints: []*domain.Endpoint{endpoint}}
	selector := &mockEndpointSelector{endpoint: endpoint}
	collector := createTestStatsCollector()
	config := suite.CreateConfig()

	proxy := suite.CreateProxy(discovery, selector, config, collector)

	// Warm up
	for i := 0; i < 100; i++ {
		req, stats, rlog := createTestRequestWithBody("GET", "/api/test", "")
		w := httptest.NewRecorder()
		proxy.ProxyRequest(req.Context(), w, req, stats, rlog)
	}

	b.ResetTimer()
	b.ReportAllocs()

	// Test with high concurrency
	const workers = 50
	var wg sync.WaitGroup
	requests := make(chan struct{}, b.N)

	for i := 0; i < b.N; i++ {
		requests <- struct{}{}
	}
	close(requests)

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range requests {
				req, stats, rlog := createTestRequestWithBody("GET", "/api/test", "")
				w := httptest.NewRecorder()

				err := proxy.ProxyRequest(req.Context(), w, req, stats, rlog)
				if err != nil {
					b.Errorf("Request failed: %v", err)
				}
			}
		}()
	}

	wg.Wait()
}

// benchmarkErrorHandling tests error path performance
func benchmarkErrorHandling(b *testing.B, suite ProxyTestSuite) {
	// Use unreachable endpoint to trigger errors
	endpoint := createTestEndpoint("bench", "http://localhost:99999", domain.StatusHealthy)
	discovery := &mockDiscoveryService{endpoints: []*domain.Endpoint{endpoint}}
	selector := &mockEndpointSelector{endpoint: endpoint}
	collector := createTestStatsCollector()
	config := suite.CreateConfig()

	proxy := suite.CreateProxy(discovery, selector, config, collector)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		req, stats, rlog := createTestRequestWithBody("GET", "/api/test", "")
		w := httptest.NewRecorder()

		// This should fail quickly
		err := proxy.ProxyRequest(req.Context(), w, req, stats, rlog)
		if err == nil {
			b.Fatal("Expected error but got none")
		}
	}
}

// benchmarkConfigUpdates tests configuration update performance
func benchmarkConfigUpdates(b *testing.B, suite ProxyTestSuite) {
	discovery := &mockDiscoveryService{}
	selector := &mockEndpointSelector{}
	collector := createTestStatsCollector()
	config := suite.CreateConfig()

	proxy := suite.CreateProxy(discovery, selector, config, collector)

	// Create alternate configs for updates
	var configs []ports.ProxyConfiguration
	if suite.Name() == "Sherpa" {
		for i := 0; i < 10; i++ {
			configs = append(configs, &Configuration{
				ResponseTimeout:  time.Duration(i+1) * time.Second,
				ReadTimeout:      time.Duration(i+1) * time.Second,
				StreamBufferSize: (i + 1) * 1024,
			})
		}
	} else {
		for i := 0; i < 10; i++ {
			configs = append(configs, &OllaConfiguration{
				ResponseTimeout:  time.Duration(i+1) * time.Second,
				ReadTimeout:      time.Duration(i+1) * time.Second,
				StreamBufferSize: (i + 1) * 1024,
				MaxIdleConns:     (i + 1) * 10,
				IdleConnTimeout:  time.Duration(i+1) * 10 * time.Second,
				MaxConnsPerHost:  (i + 1) * 5,
			})
		}
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		configIndex := i % len(configs)
		proxy.UpdateConfig(configs[configIndex])
	}
}

// benchmarkStatsCollection tests statistics collection performance
func benchmarkStatsCollection(b *testing.B, suite ProxyTestSuite) {
	discovery := &mockDiscoveryService{}
	selector := &mockEndpointSelector{}
	collector := createTestStatsCollector()
	config := suite.CreateConfig()

	proxy := suite.CreateProxy(discovery, selector, config, collector)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := proxy.GetStats(context.Background())
		if err != nil {
			b.Fatalf("GetStats failed: %v", err)
		}
	}
}

// benchmarkHeaderProcessing tests header copying performance
func benchmarkHeaderProcessing(b *testing.B, suite ProxyTestSuite) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}))
	defer upstream.Close()

	endpoint := createTestEndpoint("bench", upstream.URL, domain.StatusHealthy)
	discovery := &mockDiscoveryService{endpoints: []*domain.Endpoint{endpoint}}
	selector := &mockEndpointSelector{endpoint: endpoint}
	collector := createTestStatsCollector()
	config := suite.CreateConfig()

	proxy := suite.CreateProxy(discovery, selector, config, collector)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		req, stats, rlog := createTestRequestWithBody("POST", "/api/test", `{"data": "test"}`)
		// Add multiple headers to test header processing performance
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer token123")
		req.Header.Set("User-Agent", "benchmark-client/1.0")
		req.Header.Set("Accept", "application/json")
		req.Header.Set("Accept-Encoding", "gzip, deflate")
		req.Header.Set("X-Request-ID", fmt.Sprintf("req-%d", i))
		req.Header.Set("X-Custom-Header", "custom-value")

		w := httptest.NewRecorder()

		err := proxy.ProxyRequest(req.Context(), w, req, stats, rlog)
		if err != nil {
			b.Fatalf("Request failed: %v", err)
		}
	}
}

// BenchmarkProxyStats benchmarks statistics collection separately
func BenchmarkProxyStats(b *testing.B) {
	suites := []ProxyTestSuite{
		SherpaTestSuite{},
		OllaTestSuite{},
	}

	for _, suite := range suites {
		b.Run(suite.Name()+"_GetStats", func(b *testing.B) {
			discovery := &mockDiscoveryService{}
			selector := &mockEndpointSelector{}
			collector := createTestStatsCollector()
			config := suite.CreateConfig()

			proxy := suite.CreateProxy(discovery, selector, config, collector)

			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				_, err := proxy.GetStats(context.Background())
				if err != nil {
					b.Fatalf("GetStats failed: %v", err)
				}
			}
		})
	}
}

// BenchmarkErrorFunction benchmarks the error handling function
func BenchmarkErrorFunction(b *testing.B) {
	testErrors := []error{
		context.Canceled,
		context.DeadlineExceeded,
		io.EOF,
		fmt.Errorf("connection refused"),
		fmt.Errorf("connection reset"),
		fmt.Errorf("no such host"),
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		err := testErrors[i%len(testErrors)]
		duration := time.Duration(i%1000) * time.Millisecond
		_ = makeUserFriendlyError(err, duration, "benchmark", 30*time.Second)
	}
}

// BenchmarkMemoryUsage provides memory usage comparison
func BenchmarkMemoryUsage(b *testing.B) {
	suites := []ProxyTestSuite{
		SherpaTestSuite{},
		OllaTestSuite{},
	}

	for _, suite := range suites {
		b.Run(suite.Name()+"_MemoryAllocations", func(b *testing.B) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("ok"))
			}))
			defer upstream.Close()

			endpoint := createTestEndpoint("bench", upstream.URL, domain.StatusHealthy)
			discovery := &mockDiscoveryService{endpoints: []*domain.Endpoint{endpoint}}
			selector := &mockEndpointSelector{endpoint: endpoint}
			collector := createTestStatsCollector()
			config := suite.CreateConfig()

			proxy := suite.CreateProxy(discovery, selector, config, collector)

			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				req, stats, rlog := createTestRequestWithBody("GET", "/api/test", "")
				w := httptest.NewRecorder()

				err := proxy.ProxyRequest(req.Context(), w, req, stats, rlog)
				if err != nil {
					b.Fatalf("Request failed: %v", err)
				}
			}
		})
	}
}

// BenchmarkConnectionPooling tests connection pool performance
func BenchmarkConnectionPooling(b *testing.B) {
	suites := []ProxyTestSuite{
		SherpaTestSuite{},
		OllaTestSuite{},
	}

	for _, suite := range suites {
		b.Run(suite.Name()+"_ConnectionReuse", func(b *testing.B) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("ok"))
			}))
			defer upstream.Close()

			endpoint := createTestEndpoint("bench", upstream.URL, domain.StatusHealthy)
			discovery := &mockDiscoveryService{endpoints: []*domain.Endpoint{endpoint}}
			selector := &mockEndpointSelector{endpoint: endpoint}
			collector := createTestStatsCollector()
			config := suite.CreateConfig()

			proxy := suite.CreateProxy(discovery, selector, config, collector)

			b.ResetTimer()
			b.ReportAllocs()

			// Run concurrent requests to test connection pooling
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					req, stats, rlog := createTestRequestWithBody("GET", "/api/test", "")
					w := httptest.NewRecorder()

					err := proxy.ProxyRequest(req.Context(), w, req, stats, rlog)
					if err != nil {
						b.Fatalf("Request failed: %v", err)
					}
				}
			})
		})
	}
}

// BenchmarkCircuitBreaker tests circuit breaker performance (Olla specific)
func BenchmarkCircuitBreaker(b *testing.B) {
	b.Run("Olla_CircuitBreakerCheck", func(b *testing.B) {
		config := &OllaConfiguration{
			ResponseTimeout:  5 * time.Second,
			ReadTimeout:      2 * time.Second,
			StreamBufferSize: 8192,
			MaxIdleConns:     200,
			IdleConnTimeout:  90 * time.Second,
			MaxConnsPerHost:  50,
		}

		proxy := NewOllaService(&mockDiscoveryService{}, &mockEndpointSelector{}, config, createTestStatsCollector(), createTestLogger())

		// Get circuit breaker for testing
		cb := proxy.getCircuitBreaker("test-endpoint")

		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			// Alternate between checking state and recording success/failure
			if i%3 == 0 {
				cb.isOpen()
			} else if i%3 == 1 {
				cb.recordSuccess()
			} else {
				cb.recordFailure()
			}
		}
	})
}

// BenchmarkObjectPools tests object pool performance (Olla specific)
func BenchmarkObjectPools(b *testing.B) {
	b.Run("Olla_BufferPool", func(b *testing.B) {
		config := &OllaConfiguration{
			StreamBufferSize: 8192,
			MaxIdleConns:     200,
			IdleConnTimeout:  90 * time.Second,
			MaxConnsPerHost:  50,
		}

		proxy := NewOllaService(&mockDiscoveryService{}, &mockEndpointSelector{}, config, createTestStatsCollector(), createTestLogger())

		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			buf := proxy.bufferPool.Get()
			proxy.bufferPool.Put(buf)
		}
	})

	b.Run("Olla_RequestContextPool", func(b *testing.B) {
		config := &OllaConfiguration{
			StreamBufferSize: 8192,
			MaxIdleConns:     200,
			IdleConnTimeout:  90 * time.Second,
			MaxConnsPerHost:  50,
		}

		proxy := NewOllaService(&mockDiscoveryService{}, &mockEndpointSelector{}, config, createTestStatsCollector(), createTestLogger())

		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			ctx := proxy.requestPool.Get()
			proxy.requestPool.Put(ctx)
		}
	})
}
