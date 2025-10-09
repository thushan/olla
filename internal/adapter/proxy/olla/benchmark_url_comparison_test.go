package olla

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/util"
)

// buildTargetURL_Old is the original implementation (for comparison)
func buildTargetURL_Old(r *http.Request, endpoint *domain.Endpoint, proxyPrefix string) *url.URL {
	targetPath := util.StripPrefix(r.URL.Path, proxyPrefix)
	targetURL := endpoint.URL.ResolveReference(&url.URL{Path: targetPath})
	if r.URL.RawQuery != "" {
		targetURL.RawQuery = r.URL.RawQuery
	}
	return targetURL
}

// buildTargetURL_New is the optimized implementation with fast path
func buildTargetURL_New(r *http.Request, endpoint *domain.Endpoint, proxyPrefix string) *url.URL {
	targetPath := util.StripPrefix(r.URL.Path, proxyPrefix)

	// Fast path: if endpoint URL has no path or only "/", avoid ResolveReference allocation
	if endpoint.URL.Path == "" || endpoint.URL.Path == "/" {
		targetURL := *endpoint.URL // Shallow copy is safe
		targetURL.Path = targetPath
		if r.URL.RawQuery != "" {
			targetURL.RawQuery = r.URL.RawQuery
		}
		return &targetURL
	}

	// Slow path: complex URL resolution
	targetURL := endpoint.URL.ResolveReference(&url.URL{Path: targetPath})
	if r.URL.RawQuery != "" {
		targetURL.RawQuery = r.URL.RawQuery
	}
	return targetURL
}

// BenchmarkBuildTargetURL_Comparison compares old vs new implementation
func BenchmarkBuildTargetURL_Comparison(b *testing.B) {
	// Test with simple endpoint (the common case)
	endpointURL, _ := url.Parse("http://localhost:11434")
	endpoint := &domain.Endpoint{
		Name: "ollama",
		URL:  endpointURL,
	}
	proxyPrefix := "/olla/"

	b.Run("Old_SimpleEndpoint", func(b *testing.B) {
		req, _ := http.NewRequest("POST", "/olla/v1/chat/completions", nil)
		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			targetURL := buildTargetURL_Old(req, endpoint, proxyPrefix)
			if targetURL.Path != "/v1/chat/completions" {
				b.Fatalf("unexpected path: %s", targetURL.Path)
			}
		}
	})

	b.Run("New_SimpleEndpoint", func(b *testing.B) {
		req, _ := http.NewRequest("POST", "/olla/v1/chat/completions", nil)
		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			targetURL := buildTargetURL_New(req, endpoint, proxyPrefix)
			if targetURL.Path != "/v1/chat/completions" {
				b.Fatalf("unexpected path: %s", targetURL.Path)
			}
		}
	})

	b.Run("Old_WithQueryString", func(b *testing.B) {
		req, _ := http.NewRequest("GET", "/olla/api/tags?format=json", nil)
		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			targetURL := buildTargetURL_Old(req, endpoint, proxyPrefix)
			if targetURL.Path != "/api/tags" {
				b.Fatalf("unexpected path: %s", targetURL.Path)
			}
			if targetURL.RawQuery != "format=json" {
				b.Fatalf("unexpected query: %s", targetURL.RawQuery)
			}
		}
	})

	b.Run("New_WithQueryString", func(b *testing.B) {
		req, _ := http.NewRequest("GET", "/olla/api/tags?format=json", nil)
		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			targetURL := buildTargetURL_New(req, endpoint, proxyPrefix)
			if targetURL.Path != "/api/tags" {
				b.Fatalf("unexpected path: %s", targetURL.Path)
			}
			if targetURL.RawQuery != "format=json" {
				b.Fatalf("unexpected query: %s", targetURL.RawQuery)
			}
		}
	})

	// Test with complex endpoint (the rare case)
	complexEndpointURL, _ := url.Parse("http://localhost:8000/api/v1")
	complexEndpoint := &domain.Endpoint{
		Name: "vllm",
		URL:  complexEndpointURL,
	}

	b.Run("Old_ComplexEndpoint", func(b *testing.B) {
		req, _ := http.NewRequest("POST", "/olla/models", nil)
		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			targetURL := buildTargetURL_Old(req, complexEndpoint, proxyPrefix)
			_ = targetURL
		}
	})

	b.Run("New_ComplexEndpoint", func(b *testing.B) {
		req, _ := http.NewRequest("POST", "/olla/models", nil)
		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			targetURL := buildTargetURL_New(req, complexEndpoint, proxyPrefix)
			_ = targetURL
		}
	})
}
