package middleware

import "testing"

func TestIsProxyRequest(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected bool
	}{
		// Proxy requests
		{
			name:     "olla proxy ollama path",
			path:     "/olla/ollama/api/chat",
			expected: true,
		},
		{
			name:     "olla proxy lmstudio path",
			path:     "/olla/lmstudio/v1/models",
			expected: true,
		},
		{
			name:     "olla proxy generic path",
			path:     "/olla/proxy/some/endpoint",
			expected: true,
		},
		{
			name:     "direct api path",
			path:     "/api/chat",
			expected: true,
		},
		{
			name:     "api models path",
			path:     "/api/models",
			expected: true,
		},

		// Non-proxy requests
		{
			name:     "health check endpoint",
			path:     "/internal/health",
			expected: false,
		},
		{
			name:     "status endpoint",
			path:     "/internal/status",
			expected: false,
		},
		{
			name:     "version endpoint",
			path:     "/version",
			expected: false,
		},
		{
			name:     "root path",
			path:     "/",
			expected: false,
		},
		{
			name:     "internal api v0 models",
			path:     "/api/v0/models",
			expected: false,
		},
		{
			name:     "internal api v0 status",
			path:     "/api/v0/status",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsProxyRequest(tt.path)
			if result != tt.expected {
				t.Errorf("IsProxyRequest(%q) = %v, want %v", tt.path, result, tt.expected)
			}
		})
	}
}
