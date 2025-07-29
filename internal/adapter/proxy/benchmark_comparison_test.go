package proxy

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/thushan/olla/internal/adapter/proxy/olla"
	"github.com/thushan/olla/internal/adapter/proxy/sherpa"
	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/core/ports"
)

// BenchmarkProxyComparison compares the refactored implementations
func BenchmarkProxyComparison(b *testing.B) {
	// Set up test server
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("hello world"))
	}))
	defer upstream.Close()

	endpoint := createTestEndpoint("test", upstream.URL, domain.StatusHealthy)
	endpoints := []*domain.Endpoint{endpoint}

	// Create refactored proxies
	refactoredSherpa, err := createRefactoredSherpaProxy(endpoints)
	if err != nil {
		b.Fatal(err)
	}

	refactoredOlla, err := createRefactoredOllaProxy(endpoints)
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()

	b.Run("Sherpa_Refactored", func(b *testing.B) {
		benchmarkProxy(b, refactoredSherpa)
	})

	b.Run("Olla_Refactored", func(b *testing.B) {
		benchmarkProxy(b, refactoredOlla)
	})
}

func createRefactoredSherpaProxy(endpoints []*domain.Endpoint) (ports.ProxyService, error) {
	discovery := &mockDiscoveryService{endpoints: endpoints}
	selector := &mockEndpointSelector{}
	collector := createTestStatsCollector()
	logger := createTestLogger()

	config := &sherpa.Configuration{
		ResponseTimeout:  30 * time.Second,
		ReadTimeout:      10 * time.Second,
		StreamBufferSize: 8192,
	}
	return sherpa.NewService(discovery, selector, config, collector, logger)
}

func createRefactoredOllaProxy(endpoints []*domain.Endpoint) (ports.ProxyService, error) {
	discovery := &mockDiscoveryService{endpoints: endpoints}
	selector := &mockEndpointSelector{}
	collector := createTestStatsCollector()
	logger := createTestLogger()

	config := &olla.Configuration{
		ResponseTimeout:  30 * time.Second,
		ReadTimeout:      10 * time.Second,
		StreamBufferSize: 8192,
		MaxIdleConns:     200,
		IdleConnTimeout:  90 * time.Second,
		MaxConnsPerHost:  50,
	}
	return olla.NewService(discovery, selector, config, collector, logger)
}

func benchmarkProxy(b *testing.B, proxy ports.ProxyService) {
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

// BenchmarkStreamingComparison benchmarks streaming performance
func BenchmarkStreamingComparison(b *testing.B) {
	// 10KB response
	responseData := strings.Repeat("x", 10*1024)

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(responseData))
	}))
	defer upstream.Close()

	endpoint := createTestEndpoint("test", upstream.URL, domain.StatusHealthy)
	endpoints := []*domain.Endpoint{endpoint}

	// Create refactored proxies
	refactoredSherpa, err := createRefactoredSherpaProxy(endpoints)
	if err != nil {
		b.Fatal(err)
	}

	refactoredOlla, err := createRefactoredOllaProxy(endpoints)
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()

	b.Run("Sherpa_Refactored_Streaming", func(b *testing.B) {
		benchmarkStreamingProxy(b, refactoredSherpa)
	})

	b.Run("Olla_Refactored_Streaming", func(b *testing.B) {
		benchmarkStreamingProxy(b, refactoredOlla)
	})
}

func benchmarkStreamingProxy(b *testing.B, proxy ports.ProxyService) {
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
