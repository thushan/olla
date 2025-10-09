package olla

import (
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/thushan/olla/internal/core/domain"
)

// BenchmarkBuildTargetURL benchmarks URL building with fast path vs slow path
func BenchmarkBuildTargetURL(b *testing.B) {
	// Create a test service
	config := &Configuration{}
	config.ResponseTimeout = 30 * time.Second
	config.ReadTimeout = 10 * time.Second
	config.StreamBufferSize = 8192
	config.MaxIdleConns = 200
	config.IdleConnTimeout = 90 * time.Second
	config.MaxConnsPerHost = 50

	service, err := NewService(&mockDiscoveryService{}, &mockEndpointSelector{}, config, &mockStatsCollector{}, nil, createTestLogger())
	if err != nil {
		b.Fatalf("failed to create service: %v", err)
	}

	b.Run("FastPath_SimpleEndpoint", func(b *testing.B) {
		// Test fast path: endpoint with no path (majority of use cases)
		endpointURL, _ := url.Parse("http://localhost:11434")
		endpoint := &domain.Endpoint{
			Name: "ollama",
			URL:  endpointURL,
		}

		req, _ := http.NewRequest("POST", "/olla/v1/chat/completions?model=llama3", nil)

		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			targetURL := service.buildTargetURL(req, endpoint)
			if targetURL.Path != "/v1/chat/completions" {
				b.Fatalf("unexpected path: %s", targetURL.Path)
			}
			if targetURL.RawQuery != "model=llama3" {
				b.Fatalf("unexpected query: %s", targetURL.RawQuery)
			}
		}
	})

	b.Run("FastPath_RootPath", func(b *testing.B) {
		// Test fast path: endpoint with "/" path
		endpointURL, _ := url.Parse("http://localhost:1234/")
		endpoint := &domain.Endpoint{
			Name: "lmstudio",
			URL:  endpointURL,
		}

		req, _ := http.NewRequest("POST", "/olla/v1/completions", nil)

		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			targetURL := service.buildTargetURL(req, endpoint)
			if targetURL.Path != "/v1/completions" {
				b.Fatalf("unexpected path: %s", targetURL.Path)
			}
		}
	})

	b.Run("FastPath_WithQueryString", func(b *testing.B) {
		// Test fast path with query string
		endpointURL, _ := url.Parse("http://localhost:11434")
		endpoint := &domain.Endpoint{
			Name: "ollama",
			URL:  endpointURL,
		}

		req, _ := http.NewRequest("GET", "/olla/api/tags?format=json&details=true", nil)

		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			targetURL := service.buildTargetURL(req, endpoint)
			if targetURL.Path != "/api/tags" {
				b.Fatalf("unexpected path: %s", targetURL.Path)
			}
			if targetURL.RawQuery != "format=json&details=true" {
				b.Fatalf("unexpected query: %s", targetURL.RawQuery)
			}
		}
	})

	b.Run("SlowPath_ComplexEndpoint", func(b *testing.B) {
		// Test slow path: endpoint with complex path (rare)
		endpointURL, _ := url.Parse("http://localhost:8000/api/v1")
		endpoint := &domain.Endpoint{
			Name: "vllm",
			URL:  endpointURL,
		}

		req, _ := http.NewRequest("POST", "/olla/models", nil)

		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			targetURL := service.buildTargetURL(req, endpoint)
			// ResolveReference should join paths correctly
			if targetURL.Host != "localhost:8000" {
				b.Fatalf("unexpected host: %s", targetURL.Host)
			}
		}
	})
}
