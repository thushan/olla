package sherpa

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thushan/olla/internal/adapter/proxy/core"
	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/core/ports"
	"github.com/thushan/olla/pkg/pool"
)

// TestSherpa_PreservePath_URLBuilding tests the URL building logic in Sherpa's proxyToSingleEndpoint
func TestSherpa_PreservePath_URLBuilding(t *testing.T) {
	tests := []struct {
		name         string
		endpoint     *domain.Endpoint
		requestPath  string
		proxyPrefix  string
		expectedPath string
		description  string
	}{
		// Backward compatibility tests (preserve_path = false)
		{
			name: "backward_compatibility_no_path",
			endpoint: &domain.Endpoint{
				URL: &url.URL{
					Scheme: "http",
					Host:   "localhost:8080",
				},
				PreservePath: false,
			},
			requestPath:  "/olla/proxy/chat/completions",
			proxyPrefix:  "/olla/proxy",
			expectedPath: "/chat/completions",
			description:  "Current behaviour with no endpoint path",
		},
		{
			name: "backward_compatibility_with_slash",
			endpoint: &domain.Endpoint{
				URL: &url.URL{
					Scheme: "http",
					Host:   "localhost:8080",
					Path:   "/",
				},
				PreservePath: false,
			},
			requestPath:  "/olla/proxy/chat/completions",
			proxyPrefix:  "/olla/proxy",
			expectedPath: "/chat/completions",
			description:  "Current behaviour with root path",
		},
		{
			name: "backward_compatibility_with_api_path",
			endpoint: &domain.Endpoint{
				URL: &url.URL{
					Scheme: "http",
					Host:   "localhost:8080",
					Path:   "/api/v1/",
				},
				PreservePath: false,
			},
			requestPath:  "/olla/proxy/chat/completions",
			proxyPrefix:  "/olla/proxy",
			expectedPath: "/chat/completions",
			description:  "ResolveReference drops base path when preserve_path=false",
		},

		// preserve_path = true tests
		{
			name: "preserve_path_true_with_endpoint_path",
			endpoint: &domain.Endpoint{
				URL: &url.URL{
					Scheme: "http",
					Host:   "api.example.com",
					Path:   "/v1/api",
				},
				PreservePath: true,
			},
			requestPath:  "/olla/proxy/chat/completions",
			proxyPrefix:  "/olla/proxy",
			expectedPath: "/v1/api/chat/completions",
			description:  "Concatenates paths when preserve_path=true",
		},
		{
			name: "preserve_path_true_with_trailing_slash",
			endpoint: &domain.Endpoint{
				URL: &url.URL{
					Scheme: "http",
					Host:   "api.example.com",
					Path:   "/api/v1/",
				},
				PreservePath: true,
			},
			requestPath:  "/olla/proxy/chat/completions",
			proxyPrefix:  "/olla/proxy",
			expectedPath: "/api/v1/chat/completions",
			description:  "Handles trailing slashes correctly",
		},
		{
			name: "preserve_path_true_llamacpp_engine",
			endpoint: &domain.Endpoint{
				URL: &url.URL{
					Scheme: "http",
					Host:   "localhost:8080",
					Path:   "/engines/llama.cpp/",
				},
				PreservePath: true,
			},
			requestPath:  "/olla/proxy/completions",
			proxyPrefix:  "/olla/proxy",
			expectedPath: "/engines/llama.cpp/completions",
			description:  "Real-world llama.cpp engine path",
		},
		{
			name: "preserve_path_true_but_no_endpoint_path",
			endpoint: &domain.Endpoint{
				URL: &url.URL{
					Scheme: "http",
					Host:   "api.example.com",
					Path:   "",
				},
				PreservePath: true,
			},
			requestPath:  "/olla/proxy/chat/completions",
			proxyPrefix:  "/olla/proxy",
			expectedPath: "/chat/completions",
			description:  "No path to preserve",
		},
		{
			name: "preserve_path_true_with_nested_paths",
			endpoint: &domain.Endpoint{
				URL: &url.URL{
					Scheme: "http",
					Host:   "api.example.com",
					Path:   "/api/v2/llm",
				},
				PreservePath: true,
			},
			requestPath:  "/olla/proxy/models/gpt-4/generate",
			proxyPrefix:  "/olla/proxy",
			expectedPath: "/api/v2/llm/models/gpt-4/generate",
			description:  "Deep nested paths",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Create a mock backend server to capture the request
			var capturedPath string
			backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				capturedPath = r.URL.Path
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("OK"))
			}))
			defer backend.Close()

			// Update endpoint URL to point to test server
			backendURL, err := url.Parse(backend.URL)
			require.NoError(t, err)
			tt.endpoint.URL.Scheme = backendURL.Scheme
			tt.endpoint.URL.Host = backendURL.Host
			tt.endpoint.Status = domain.StatusHealthy

			// Create Sherpa service
			service := createTestSherpaService(t, tt.proxyPrefix)

			// Create test request
			req := httptest.NewRequest("POST", tt.requestPath, nil)
			w := httptest.NewRecorder()

			// Execute the proxy request
			stats := &ports.RequestStats{}
			ctx := context.Background()
			err = service.proxyToSingleEndpoint(ctx, w, req, tt.endpoint, stats, createTestLogger())

			// Assert no error and check captured path
			assert.NoError(t, err, tt.description)
			assert.Equal(t, tt.expectedPath, capturedPath, tt.description)
		})
	}
}

// TestSherpa_PreservePath_EdgeCases tests edge cases and weird paths
func TestSherpa_PreservePath_EdgeCases(t *testing.T) {
	tests := []struct {
		name         string
		endpoint     *domain.Endpoint
		requestPath  string
		proxyPrefix  string
		expectedPath string
		description  string
	}{
		// Double slash handling
		{
			name: "double_slashes_in_request",
			endpoint: &domain.Endpoint{
				URL: &url.URL{
					Scheme: "http",
					Host:   "api.example.com",
					Path:   "/api/v1",
				},
				PreservePath: true,
			},
			requestPath:  "/olla/proxy//double//slashes//",
			proxyPrefix:  "/olla/proxy",
			expectedPath: "/api/v1/double/slashes",
			description:  "path.Join normalises double slashes",
		},
		{
			name: "double_slashes_preserve_false",
			endpoint: &domain.Endpoint{
				URL: &url.URL{
					Scheme: "http",
					Host:   "api.example.com",
					Path:   "/api/v1",
				},
				PreservePath: false,
			},
			requestPath:  "/olla/proxy//double//slashes//",
			proxyPrefix:  "/olla/proxy",
			expectedPath: "//double//slashes//",
			description:  "ResolveReference preserves double slashes when preserve_path=false",
		},

		// Path traversal attempts (security)
		{
			name: "path_traversal_preserve_true",
			endpoint: &domain.Endpoint{
				URL: &url.URL{
					Scheme: "http",
					Host:   "api.example.com",
					Path:   "/api/v1",
				},
				PreservePath: true,
			},
			requestPath:  "/olla/proxy/../../../etc/passwd",
			proxyPrefix:  "/olla/proxy",
			expectedPath: "/etc/passwd",
			description:  "path.Join resolves .. when preserve_path=true",
		},
		{
			name: "path_traversal_preserve_false",
			endpoint: &domain.Endpoint{
				URL: &url.URL{
					Scheme: "http",
					Host:   "api.example.com",
					Path:   "/api/v1",
				},
				PreservePath: false,
			},
			requestPath:  "/olla/proxy/../../../etc/passwd",
			proxyPrefix:  "/olla/proxy",
			expectedPath: "/etc/passwd",
			description:  "ResolveReference resolves path traversal",
		},

		// Empty and special cases
		{
			name: "empty_request_path_after_strip",
			endpoint: &domain.Endpoint{
				URL: &url.URL{
					Scheme: "http",
					Host:   "api.example.com",
					Path:   "/api/v1",
				},
				PreservePath: true,
			},
			requestPath:  "/olla/proxy",
			proxyPrefix:  "/olla/proxy",
			expectedPath: "/api/v1",
			description:  "Empty path after stripping prefix",
		},

		// Port-only endpoints
		{
			name: "port_only_no_path",
			endpoint: &domain.Endpoint{
				URL: &url.URL{
					Scheme: "http",
					Host:   "localhost:11434",
				},
				PreservePath: false,
			},
			requestPath:  "/olla/proxy/api/generate",
			proxyPrefix:  "/olla/proxy",
			expectedPath: "/api/generate",
			description:  "Port-only endpoint without path",
		},
		{
			name: "port_only_preserve_true_no_effect",
			endpoint: &domain.Endpoint{
				URL: &url.URL{
					Scheme: "http",
					Host:   "localhost:11434",
				},
				PreservePath: true,
			},
			requestPath:  "/olla/proxy/api/generate",
			proxyPrefix:  "/olla/proxy",
			expectedPath: "/api/generate",
			description:  "preserve_path=true has no effect when endpoint has no path",
		},

		// Special characters and encoding
		{
			name: "spaces_in_path",
			endpoint: &domain.Endpoint{
				URL: &url.URL{
					Scheme: "http",
					Host:   "api.example.com",
					Path:   "/api/v1",
				},
				PreservePath: true,
			},
			requestPath:  "/olla/proxy/path%20with%20spaces",
			proxyPrefix:  "/olla/proxy",
			expectedPath: "/api/v1/path with spaces",
			description:  "URL-encoded spaces are decoded in path",
		},
		{
			name: "special_chars_in_path",
			endpoint: &domain.Endpoint{
				URL: &url.URL{
					Scheme: "http",
					Host:   "api.example.com",
					Path:   "/api/v1",
				},
				PreservePath: true,
			},
			requestPath:  "/olla/proxy/model@latest/generate",
			proxyPrefix:  "/olla/proxy",
			expectedPath: "/api/v1/model@latest/generate",
			description:  "Special characters like @ preserved",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Create a mock backend server
			var capturedPath string
			backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				capturedPath = r.URL.Path
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("OK"))
			}))
			defer backend.Close()

			// Update endpoint URL
			backendURL, err := url.Parse(backend.URL)
			require.NoError(t, err)
			tt.endpoint.URL.Scheme = backendURL.Scheme
			tt.endpoint.URL.Host = backendURL.Host
			tt.endpoint.Status = domain.StatusHealthy

			// Create service and execute
			service := createTestSherpaService(t, tt.proxyPrefix)
			req := httptest.NewRequest("POST", tt.requestPath, nil)
			w := httptest.NewRecorder()
			stats := &ports.RequestStats{}
			ctx := context.Background()

			err = service.proxyToSingleEndpoint(ctx, w, req, tt.endpoint, stats, createTestLogger())

			assert.NoError(t, err, tt.description)
			assert.Equal(t, tt.expectedPath, capturedPath, tt.description)
		})
	}
}

// TestSherpa_PreservePath_QueryStrings tests query string preservation
func TestSherpa_PreservePath_QueryStrings(t *testing.T) {
	tests := []struct {
		name          string
		endpoint      *domain.Endpoint
		requestPath   string
		expectedPath  string
		expectedQuery string
		description   string
	}{
		{
			name: "query_string_with_preserve_true",
			endpoint: &domain.Endpoint{
				URL: &url.URL{
					Scheme: "http",
					Host:   "api.example.com",
					Path:   "/v1/api",
				},
				PreservePath: true,
			},
			requestPath:   "/olla/proxy/models?filter=gpt&limit=10",
			expectedPath:  "/v1/api/models",
			expectedQuery: "filter=gpt&limit=10",
			description:   "Query strings preserved with preserve_path=true",
		},
		{
			name: "query_string_with_preserve_false",
			endpoint: &domain.Endpoint{
				URL: &url.URL{
					Scheme: "http",
					Host:   "api.example.com",
					Path:   "/v1/api",
				},
				PreservePath: false,
			},
			requestPath:   "/olla/proxy/models?filter=gpt&limit=10",
			expectedPath:  "/models",
			expectedQuery: "filter=gpt&limit=10",
			description:   "Query strings preserved with preserve_path=false",
		},
		{
			name: "complex_query_string",
			endpoint: &domain.Endpoint{
				URL: &url.URL{
					Scheme: "http",
					Host:   "api.example.com",
					Path:   "/api/v1",
				},
				PreservePath: true,
			},
			requestPath:   "/olla/proxy/search?q=hello%20world&type=model&tags[]=llm&tags[]=chat",
			expectedPath:  "/api/v1/search",
			expectedQuery: "q=hello%20world&type=model&tags[]=llm&tags[]=chat",
			description:   "Complex query with arrays and encoding",
		},
		{
			name: "empty_query_string",
			endpoint: &domain.Endpoint{
				URL: &url.URL{
					Scheme: "http",
					Host:   "api.example.com",
					Path:   "/api/v1",
				},
				PreservePath: true,
			},
			requestPath:   "/olla/proxy/models?",
			expectedPath:  "/api/v1/models",
			expectedQuery: "",
			description:   "Empty query string",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Create mock backend to capture request
			var capturedPath string
			var capturedQuery string
			backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				capturedPath = r.URL.Path
				capturedQuery = r.URL.RawQuery
				w.WriteHeader(http.StatusOK)
			}))
			defer backend.Close()

			// Setup endpoint
			backendURL, err := url.Parse(backend.URL)
			require.NoError(t, err)
			tt.endpoint.URL.Scheme = backendURL.Scheme
			tt.endpoint.URL.Host = backendURL.Host
			tt.endpoint.Status = domain.StatusHealthy

			// Execute proxy request
			service := createTestSherpaService(t, "/olla/proxy")
			req := httptest.NewRequest("GET", tt.requestPath, nil)
			w := httptest.NewRecorder()
			stats := &ports.RequestStats{}
			ctx := context.Background()

			err = service.proxyToSingleEndpoint(ctx, w, req, tt.endpoint, stats, createTestLogger())

			assert.NoError(t, err, tt.description)
			assert.Equal(t, tt.expectedPath, capturedPath, "Path: "+tt.description)
			assert.Equal(t, tt.expectedQuery, capturedQuery, "Query: "+tt.description)
		})
	}
}

// TestSherpa_PreservePath_RealWorldProviders tests real-world provider configurations
func TestSherpa_PreservePath_RealWorldProviders(t *testing.T) {
	tests := []struct {
		name         string
		provider     string
		endpoint     *domain.Endpoint
		requestPath  string
		expectedPath string
		description  string
	}{
		// OpenAI-compatible services
		{
			name:     "openai_api_direct",
			provider: "openai",
			endpoint: &domain.Endpoint{
				URL: &url.URL{
					Scheme: "https",
					Host:   "api.openai.com",
					Path:   "/v1",
				},
				PreservePath: true,
			},
			requestPath:  "/olla/proxy/chat/completions",
			expectedPath: "/v1/chat/completions",
			description:  "OpenAI API with /v1 base path",
		},
		{
			name:     "local_lmstudio",
			provider: "lmstudio",
			endpoint: &domain.Endpoint{
				URL: &url.URL{
					Scheme: "http",
					Host:   "localhost:1234",
					Path:   "/v1",
				},
				PreservePath: true,
			},
			requestPath:  "/olla/proxy/chat/completions",
			expectedPath: "/v1/chat/completions",
			description:  "LM Studio with OpenAI-compatible API",
		},

		// Ollama
		{
			name:     "ollama_default",
			provider: "ollama",
			endpoint: &domain.Endpoint{
				URL: &url.URL{
					Scheme: "http",
					Host:   "localhost:11434",
				},
				PreservePath: false,
			},
			requestPath:  "/olla/proxy/api/generate",
			expectedPath: "/api/generate",
			description:  "Ollama with no base path",
		},

		// vLLM with custom paths
		{
			name:     "vllm_custom_path",
			provider: "vllm",
			endpoint: &domain.Endpoint{
				URL: &url.URL{
					Scheme: "http",
					Host:   "gpu-server:8000",
					Path:   "/v1",
				},
				PreservePath: true,
			},
			requestPath:  "/olla/proxy/completions",
			expectedPath: "/v1/completions",
			description:  "vLLM with OpenAI-compatible path",
		},

		// Anthropic Claude API
		{
			name:     "anthropic_messages",
			provider: "anthropic",
			endpoint: &domain.Endpoint{
				URL: &url.URL{
					Scheme: "https",
					Host:   "api.anthropic.com",
					Path:   "/v1",
				},
				PreservePath: true,
			},
			requestPath:  "/olla/proxy/messages",
			expectedPath: "/v1/messages",
			description:  "Anthropic Messages API",
		},

		// Custom enterprise deployment
		{
			name:     "enterprise_nested_path",
			provider: "custom",
			endpoint: &domain.Endpoint{
				URL: &url.URL{
					Scheme: "https",
					Host:   "ai.company.com",
					Path:   "/api/ml/v2/inference",
				},
				PreservePath: true,
			},
			requestPath:  "/olla/proxy/predict",
			expectedPath: "/api/ml/v2/inference/predict",
			description:  "Enterprise deployment with nested paths",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Create mock backend
			var capturedPath string
			backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				capturedPath = r.URL.Path
				w.WriteHeader(http.StatusOK)
			}))
			defer backend.Close()

			// Setup endpoint
			backendURL, err := url.Parse(backend.URL)
			require.NoError(t, err)
			tt.endpoint.URL.Scheme = backendURL.Scheme
			tt.endpoint.URL.Host = backendURL.Host
			tt.endpoint.Status = domain.StatusHealthy

			// Execute test
			service := createTestSherpaService(t, "/olla/proxy")
			req := httptest.NewRequest("POST", tt.requestPath, nil)
			w := httptest.NewRecorder()
			stats := &ports.RequestStats{}
			ctx := context.Background()

			err = service.proxyToSingleEndpoint(ctx, w, req, tt.endpoint, stats, createTestLogger())

			assert.NoError(t, err, "Provider: %s - %s", tt.provider, tt.description)
			assert.Equal(t, tt.expectedPath, capturedPath,
				"Provider: %s - %s", tt.provider, tt.description)
		})
	}
}

// Helper function to create a test Sherpa service with mock selector
func createTestSherpaService(t *testing.T, proxyPrefix string) *Service {
	t.Helper()

	config := &Configuration{}
	config.ProxyPrefix = proxyPrefix
	config.ResponseTimeout = 5 * time.Second
	config.ConnectionTimeout = 2 * time.Second
	config.ConnectionKeepAlive = 30 * time.Second
	config.ReadTimeout = 5 * time.Second
	config.StreamBufferSize = 8192

	// Create mock selector that does nothing
	mockSelector := &mockEndpointSelector{}

	// Create minimal components
	baseComponents := core.NewBaseProxyComponents(
		nil, // discovery service not needed for these tests
		mockSelector,
		nil, // stats collector not needed
		nil, // metrics extractor not needed
		createTestLogger(),
	)

	bufferPool, err := pool.NewLitePool(func() *[]byte {
		buf := make([]byte, config.GetStreamBufferSize())
		return &buf
	})
	require.NoError(t, err)

	service := &Service{
		BaseProxyComponents: baseComponents,
		configuration:       config,
		bufferPool:          bufferPool,
		retryHandler:        core.NewRetryHandler(nil, createTestLogger()),
		transport:           http.DefaultTransport.(*http.Transport),
	}

	return service
}

// mockEndpointSelector is a mock implementation of EndpointSelector
type mockEndpointSelector struct{}

func (m *mockEndpointSelector) Name() string {
	return "mock"
}

func (m *mockEndpointSelector) Select(ctx context.Context, endpoints []*domain.Endpoint) (*domain.Endpoint, error) {
	if len(endpoints) > 0 {
		return endpoints[0], nil
	}
	return nil, nil
}

func (m *mockEndpointSelector) IncrementConnections(endpoint *domain.Endpoint) {}
func (m *mockEndpointSelector) DecrementConnections(endpoint *domain.Endpoint) {}
