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

	"github.com/thushan/olla/internal/adapter/proxy/olla"
	"github.com/thushan/olla/internal/adapter/proxy/sherpa"
	"github.com/thushan/olla/internal/core/constants"
	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/core/ports"
)

// mockResponseWriter tracks flush calls for testing
type mockResponseWriter struct {
	httptest.ResponseRecorder
	flushCount int
	mu         sync.Mutex
}

func (m *mockResponseWriter) Flush() {
	m.mu.Lock()
	m.flushCount++
	m.mu.Unlock()
}

func (m *mockResponseWriter) getFlushCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.flushCount
}

// TestStreamingProfiles tests that proxy profiles correctly control flushing behavior
func TestStreamingProfiles(t *testing.T) {
	suites := []ProxyTestSuite{
		SherpaTestSuite{},
		OllaTestSuite{},
	}

	for _, suite := range suites {
		t.Run(suite.Name(), func(t *testing.T) {
			testStreamingProfilesForSuite(t, suite)
		})
	}
}

func testStreamingProfilesForSuite(t *testing.T, suite ProxyTestSuite) {
	tests := []struct {
		name           string
		profile        string
		contentType    string
		responseChunks int
		expectFlushes  bool
		description    string
	}{
		// standard profile - should NEVER flush
		{
			name:           "standard_profile_sse_content",
			profile:        constants.ConfigurationProxyProfileStandard,
			contentType:    constants.ContentTypeEventStream,
			responseChunks: 5,
			expectFlushes:  false,
			description:    "standard profile should not flush even for SSE content",
		},
		{
			name:           "standard_profile_json_streaming",
			profile:        constants.ConfigurationProxyProfileStandard,
			contentType:    constants.ContentTypeStreamJSON,
			responseChunks: 5,
			expectFlushes:  false,
			description:    "standard profile should not flush streaming JSON",
		},
		{
			name:           "standard_profile_plain_text",
			profile:        constants.ConfigurationProxyProfileStandard,
			contentType:    constants.ContentTypeText,
			responseChunks: 5,
			expectFlushes:  false,
			description:    "standard profile should not flush plain text",
		},

		// Streaming profile - should ALWAYS flush
		{
			name:           "streaming_profile_binary_content",
			profile:        constants.ConfigurationProxyProfileStreaming,
			contentType:    constants.ContentTypeImagePNG,
			responseChunks: 5,
			expectFlushes:  true,
			description:    "Streaming profile should flush even for binary content",
		},
		{
			name:           "streaming_profile_json",
			profile:        constants.ConfigurationProxyProfileStreaming,
			contentType:    constants.ContentTypeJSON,
			responseChunks: 5,
			expectFlushes:  true,
			description:    "Streaming profile should flush JSON responses",
		},
		{
			name:           "streaming_profile_pdf",
			profile:        constants.ConfigurationProxyProfileStreaming,
			contentType:    constants.ContentTypePDF,
			responseChunks: 5,
			expectFlushes:  true,
			description:    "Streaming profile should flush even PDFs",
		},

		// Auto profile - flush based on content type detection
		{
			name:           "auto_profile_sse_content",
			profile:        constants.ConfigurationProxyProfileAuto,
			contentType:    constants.ContentTypeEventStream,
			responseChunks: 5,
			expectFlushes:  true,
			description:    "Auto profile should flush SSE content",
		},
		{
			name:           "auto_profile_ndjson",
			profile:        constants.ConfigurationProxyProfileAuto,
			contentType:    constants.ContentTypeNDJSON,
			responseChunks: 5,
			expectFlushes:  true,
			description:    "Auto profile should flush NDJSON",
		},
		{
			name:           "auto_profile_binary_image",
			profile:        constants.ConfigurationProxyProfileAuto,
			contentType:    constants.ContentTypeImageJPEG,
			responseChunks: 5,
			expectFlushes:  false,
			description:    "Auto profile should NOT flush binary images",
		},
		{
			name:           "auto_profile_pdf",
			profile:        constants.ConfigurationProxyProfileAuto,
			contentType:    constants.ContentTypePDF,
			responseChunks: 5,
			expectFlushes:  false,
			description:    "Auto profile should NOT flush PDFs",
		},
		{
			name:           "auto_profile_plain_text",
			profile:        constants.ConfigurationProxyProfileAuto,
			contentType:    constants.ContentTypeTextUTF8,
			responseChunks: 5,
			expectFlushes:  true,
			description:    "Auto profile should flush plain text (common for LLMs)",
		},
		{
			name:           "auto_profile_json",
			profile:        constants.ConfigurationProxyProfileAuto,
			contentType:    constants.ContentTypeJSON,
			responseChunks: 5,
			expectFlushes:  true,
			description:    "Auto profile should flush JSON by default",
		},

		// Edge cases and mixed scenarios
		{
			name:           "standard_profile_with_chunked_encoding",
			profile:        constants.ConfigurationProxyProfileStandard,
			contentType:    constants.ContentTypeEventStream,
			responseChunks: 10,
			expectFlushes:  false,
			description:    "standard profile should not flush even with many chunks",
		},
		{
			name:           "streaming_profile_empty_content_type",
			profile:        constants.ConfigurationProxyProfileStreaming,
			contentType:    "",
			responseChunks: 5,
			expectFlushes:  true,
			description:    "Streaming profile should flush even with no content type",
		},
		{
			name:           "auto_profile_unknown_content_type",
			profile:        constants.ConfigurationProxyProfileAuto,
			contentType:    "application/x-custom-type",
			responseChunks: 5,
			expectFlushes:  true,
			description:    "Auto profile should default to streaming for unknown types",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create upstream server that sends chunked responses
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set(constants.HeaderContentType, tt.contentType)
				w.WriteHeader(http.StatusOK)

				// Send response in chunks
				for i := 0; i < tt.responseChunks; i++ {
					chunk := fmt.Sprintf("chunk %d\n", i)
					w.Write([]byte(chunk))
					// Upstream flushes to simulate streaming
					if f, ok := w.(http.Flusher); ok {
						f.Flush()
					}
					// Small delay to simulate streaming
					time.Sleep(5 * time.Millisecond)
				}
			}))
			defer upstream.Close()

			// Setup proxy with the test profile
			endpoint := createTestEndpoint("test", upstream.URL, domain.StatusHealthy)
			discovery := &mockDiscoveryService{endpoints: []*domain.Endpoint{endpoint}}
			selector := &mockEndpointSelector{endpoint: endpoint}
			collector := createTestStatsCollector()

			// Create config with the test profile
			var proxy ports.ProxyService
			if suite.Name() == "Sherpa" {
				config := &sherpa.Configuration{
					Profile:          tt.profile,
					ResponseTimeout:  5 * time.Second,
					ReadTimeout:      5 * time.Second,
					StreamBufferSize: 8192,
				}
				proxy = suite.CreateProxy(discovery, selector, config, collector)
			} else {
				config := &olla.Configuration{
					Profile:          tt.profile,
					ResponseTimeout:  5 * time.Second,
					ReadTimeout:      5 * time.Second,
					StreamBufferSize: 8192,
					MaxIdleConns:     10,
					IdleConnTimeout:  90 * time.Second,
					MaxConnsPerHost:  5,
				}
				proxy = suite.CreateProxy(discovery, selector, config, collector)
			}

			// Create request
			req, stats, rlog := createTestRequestWithBody("GET", "/api/test", "")

			// Use our mock response writer to track flushes
			w := &mockResponseWriter{
				ResponseRecorder: *httptest.NewRecorder(),
			}

			// Execute proxy request
			err := proxy.ProxyRequest(req.Context(), w, req, stats, rlog)
			if err != nil {
				t.Fatalf("Proxy request failed: %v", err)
			}

			// Verify response was received
			result := w.Result()
			body, _ := io.ReadAll(result.Body)
			if !strings.Contains(string(body), "chunk") {
				t.Errorf("Response body doesn't contain expected chunks: %s", body)
			}

			// Check flush behavior
			flushCount := w.getFlushCount()
			if tt.expectFlushes && flushCount == 0 {
				t.Errorf("%s - Expected flushes but got none. Profile: %s, Content-Type: %s",
					tt.description, tt.profile, tt.contentType)
			} else if !tt.expectFlushes && flushCount > 0 {
				t.Errorf("%s - Expected no flushes but got %d. Profile: %s, Content-Type: %s",
					tt.description, flushCount, tt.profile, tt.contentType)
			}

			// Log results for debugging
			t.Logf("Test: %s, Profile: %s, Content-Type: %s, Flushes: %d, Expected flushes: %v",
				tt.name, tt.profile, tt.contentType, flushCount, tt.expectFlushes)
		})
	}
}

// TestStreamingProfilesWithContextOverride tests that context stream value works with profiles
func TestStreamingProfilesWithContextOverride(t *testing.T) {
	suites := []ProxyTestSuite{
		SherpaTestSuite{},
		OllaTestSuite{},
	}

	for _, suite := range suites {
		t.Run(suite.Name(), func(t *testing.T) {
			// Create upstream that sends binary content
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set(constants.HeaderContentType, constants.ContentTypeImagePNG)
				w.WriteHeader(http.StatusOK)

				// Send fake binary data in chunks
				for i := 0; i < 3; i++ {
					w.Write([]byte{0xFF, 0xD8, 0xFF, 0xE0}) // Fake JPEG header
					if f, ok := w.(http.Flusher); ok {
						f.Flush()
					}
					time.Sleep(5 * time.Millisecond)
				}
			}))
			defer upstream.Close()

			// Setup proxy with auto profile
			endpoint := createTestEndpoint("test", upstream.URL, domain.StatusHealthy)
			discovery := &mockDiscoveryService{endpoints: []*domain.Endpoint{endpoint}}
			selector := &mockEndpointSelector{endpoint: endpoint}
			collector := createTestStatsCollector()

			var proxy ports.ProxyService
			if suite.Name() == "Sherpa" {
				config := &sherpa.Configuration{
					Profile:          constants.ConfigurationProxyProfileAuto,
					ResponseTimeout:  5 * time.Second,
					ReadTimeout:      5 * time.Second,
					StreamBufferSize: 8192,
				}
				proxy = suite.CreateProxy(discovery, selector, config, collector)
			} else {
				config := &olla.Configuration{
					Profile:          constants.ConfigurationProxyProfileAuto,
					ResponseTimeout:  5 * time.Second,
					ReadTimeout:      5 * time.Second,
					StreamBufferSize: 8192,
					MaxIdleConns:     10,
					IdleConnTimeout:  90 * time.Second,
					MaxConnsPerHost:  5,
				}
				proxy = suite.CreateProxy(discovery, selector, config, collector)
			}

			// Test 1: Auto profile with binary content and stream=true in context
			t.Run("context_stream_true_overrides_binary_detection", func(t *testing.T) {
				req, stats, rlog := createTestRequestWithBody("GET", "/api/test", "")
				// Add stream=true to context
				ctx := context.WithValue(req.Context(), constants.ContextKeyStream, true)
				req = req.WithContext(ctx)

				w := &mockResponseWriter{
					ResponseRecorder: *httptest.NewRecorder(),
				}

				err := proxy.ProxyRequest(req.Context(), w, req, stats, rlog)
				if err != nil {
					t.Fatalf("Proxy request failed: %v", err)
				}

				// Should flush because context stream=true overrides binary detection
				if w.getFlushCount() == 0 {
					t.Error("Expected flushes when context stream=true for binary content in auto mode")
				}
			})

			// Test 2: Auto profile with binary content and no context override
			t.Run("auto_profile_buffers_binary_without_context_override", func(t *testing.T) {
				req, stats, rlog := createTestRequestWithBody("GET", "/api/test", "")

				w := &mockResponseWriter{
					ResponseRecorder: *httptest.NewRecorder(),
				}

				err := proxy.ProxyRequest(req.Context(), w, req, stats, rlog)
				if err != nil {
					t.Fatalf("Proxy request failed: %v", err)
				}

				// Should NOT flush binary content in auto mode
				if w.getFlushCount() > 0 {
					t.Error("Expected no flushes for binary content in auto mode without context override")
				}
			})
		})
	}
}

// TestStreamingProfilesComprehensive tests all profile combinations with different scenarios
func TestStreamingProfilesComprehensive(t *testing.T) {
	suites := []ProxyTestSuite{
		SherpaTestSuite{},
		OllaTestSuite{},
	}

	for _, suite := range suites {
		t.Run(suite.Name(), func(t *testing.T) {
			// Test matrix covering all profile/context combinations
			tests := []struct {
				name          string
				profile       string
				contentType   string
				contextStream interface{} // nil, true, or false
				expectFlushes bool
				description   string
			}{
				// standard profile - should NEVER flush regardless of context
				{
					name:          "standard_sse_no_context",
					profile:       constants.ConfigurationProxyProfileStandard,
					contentType:   "text/event-stream",
					contextStream: nil,
					expectFlushes: false,
					description:   "standard profile should not flush SSE without context",
				},
				{
					name:          "standard_sse_stream_true",
					profile:       constants.ConfigurationProxyProfileStandard,
					contentType:   "text/event-stream",
					contextStream: true,
					expectFlushes: false,
					description:   "standard profile ignores context stream=true",
				},
				{
					name:          "standard_binary_stream_true",
					profile:       constants.ConfigurationProxyProfileStandard,
					contentType:   "image/png",
					contextStream: true,
					expectFlushes: false,
					description:   "standard profile should not flush binary even with stream=true",
				},

				// Streaming profile - should ALWAYS flush regardless of context
				{
					name:          "streaming_binary_no_context",
					profile:       constants.ConfigurationProxyProfileStreaming,
					contentType:   "image/png",
					contextStream: nil,
					expectFlushes: true,
					description:   "Streaming profile should flush binary without context",
				},
				{
					name:          "streaming_binary_stream_false",
					profile:       constants.ConfigurationProxyProfileStreaming,
					contentType:   "image/png",
					contextStream: false,
					expectFlushes: true,
					description:   "Streaming profile ignores context stream=false",
				},
				{
					name:          "streaming_sse_stream_false",
					profile:       constants.ConfigurationProxyProfileStreaming,
					contentType:   "text/event-stream",
					contextStream: false,
					expectFlushes: true,
					description:   "Streaming profile should flush SSE even with stream=false",
				},

				// Auto profile - respects content type and context
				{
					name:          "auto_sse_no_context",
					profile:       constants.ConfigurationProxyProfileAuto,
					contentType:   "text/event-stream",
					contextStream: nil,
					expectFlushes: true,
					description:   "Auto profile should flush SSE by default",
				},
				{
					name:          "auto_binary_no_context",
					profile:       constants.ConfigurationProxyProfileAuto,
					contentType:   "image/png",
					contextStream: nil,
					expectFlushes: false,
					description:   "Auto profile should buffer binary by default",
				},
				{
					name:          "auto_binary_stream_true",
					profile:       constants.ConfigurationProxyProfileAuto,
					contentType:   "image/png",
					contextStream: true,
					expectFlushes: true,
					description:   "Auto profile with stream=true should override binary detection",
				},
				{
					name:          "auto_sse_stream_false",
					profile:       constants.ConfigurationProxyProfileAuto,
					contentType:   "text/event-stream",
					contextStream: false,
					expectFlushes: true,
					description:   "Auto profile still flushes SSE even with stream=false",
				},
			}

			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					// Create upstream server
					upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						w.Header().Set(constants.HeaderContentType, tt.contentType)
						w.WriteHeader(http.StatusOK)

						// Send appropriate content based on type
						for i := 0; i < 3; i++ {
							if strings.HasPrefix(tt.contentType, "image/") {
								w.Write([]byte{0xFF, 0xD8, 0xFF, 0xE0})
							} else if tt.contentType == "text/event-stream" {
								w.Write([]byte(fmt.Sprintf("data: event %d\n\n", i)))
							} else {
								w.Write([]byte(fmt.Sprintf("chunk %d\n", i)))
							}
							if f, ok := w.(http.Flusher); ok {
								f.Flush()
							}
							time.Sleep(5 * time.Millisecond)
						}
					}))
					defer upstream.Close()

					// Setup proxy
					endpoint := createTestEndpoint("test", upstream.URL, domain.StatusHealthy)
					discovery := &mockDiscoveryService{endpoints: []*domain.Endpoint{endpoint}}
					selector := &mockEndpointSelector{endpoint: endpoint}
					collector := createTestStatsCollector()

					var proxy ports.ProxyService
					if suite.Name() == "Sherpa" {
						config := &sherpa.Configuration{
							Profile:          tt.profile,
							ResponseTimeout:  5 * time.Second,
							ReadTimeout:      5 * time.Second,
							StreamBufferSize: 8192,
						}
						proxy = suite.CreateProxy(discovery, selector, config, collector)
					} else {
						config := &olla.Configuration{
							Profile:          tt.profile,
							ResponseTimeout:  5 * time.Second,
							ReadTimeout:      5 * time.Second,
							StreamBufferSize: 8192,
							MaxIdleConns:     10,
							IdleConnTimeout:  90 * time.Second,
							MaxConnsPerHost:  5,
						}
						proxy = suite.CreateProxy(discovery, selector, config, collector)
					}

					// Create request
					req, stats, rlog := createTestRequestWithBody("GET", "/api/test", "")
					if tt.contextStream != nil {
						ctx := context.WithValue(req.Context(), constants.ContextKeyStream, tt.contextStream)
						req = req.WithContext(ctx)
					}

					w := &mockResponseWriter{
						ResponseRecorder: *httptest.NewRecorder(),
					}

					// Execute request
					err := proxy.ProxyRequest(req.Context(), w, req, stats, rlog)
					if err != nil {
						t.Fatalf("Proxy request failed: %v", err)
					}

					// Verify flush behavior
					flushCount := w.getFlushCount()
					if tt.expectFlushes && flushCount == 0 {
						t.Errorf("%s - Expected flushes but got none. Profile: %s, Content-Type: %s, Context: %v",
							tt.description, tt.profile, tt.contentType, tt.contextStream)
					} else if !tt.expectFlushes && flushCount > 0 {
						t.Errorf("%s - Expected no flushes but got %d. Profile: %s, Content-Type: %s, Context: %v",
							tt.description, flushCount, tt.profile, tt.contentType, tt.contextStream)
					}

					t.Logf("Suite: %s, Test: %s, Profile: %s, Content-Type: %s, Context: %v, Flushes: %d, Expected: %v",
						suite.Name(), tt.name, tt.profile, tt.contentType, tt.contextStream, flushCount, tt.expectFlushes)
				})
			}
		})
	}
}
