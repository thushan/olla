package util

import "testing"

func TestResolveURLPath(t *testing.T) {
	tests := []struct {
		name      string
		baseURL   string
		pathOrURL string
		expected  string
	}{
		{
			name:      "base with trailing slash, path with leading slash",
			baseURL:   "http://localhost:12434/engines/llama.cpp/",
			pathOrURL: "/v1/models",
			expected:  "http://localhost:12434/engines/llama.cpp/v1/models",
		},
		{
			name:      "base without trailing slash, path with leading slash",
			baseURL:   "http://localhost:11434",
			pathOrURL: "/api/tags",
			expected:  "http://localhost:11434/api/tags",
		},
		{
			name:      "base with trailing slash, path without leading slash",
			baseURL:   "http://localhost:12434/api/",
			pathOrURL: "v1/models",
			expected:  "http://localhost:12434/api/v1/models",
		},
		{
			name:      "base without trailing slash, path without leading slash",
			baseURL:   "http://localhost:11434",
			pathOrURL: "api/tags",
			expected:  "http://localhost:11434/api/tags",
		},
		{
			name:      "empty base",
			baseURL:   "",
			pathOrURL: "/v1/models",
			expected:  "/v1/models",
		},
		{
			name:      "empty path",
			baseURL:   "http://localhost:11434",
			pathOrURL: "",
			expected:  "http://localhost:11434",
		},
		{
			name:      "absolute URL with http scheme",
			baseURL:   "http://localhost:12434/api/",
			pathOrURL: "http://other-host:9000/v1/models",
			expected:  "http://other-host:9000/v1/models",
		},
		{
			name:      "absolute URL with https scheme",
			baseURL:   "http://localhost:12434/api/",
			pathOrURL: "https://api.example.com/models",
			expected:  "https://api.example.com/models",
		},
		{
			name:      "absolute URL overrides base completely",
			baseURL:   "http://localhost:11434",
			pathOrURL: "https://api.openai.com/v1/models",
			expected:  "https://api.openai.com/v1/models",
		},
		{
			name:      "path with multiple segments",
			baseURL:   "http://localhost:8080",
			pathOrURL: "/api/v1/models/list",
			expected:  "http://localhost:8080/api/v1/models/list",
		},
		{
			name:      "base URL with query params",
			baseURL:   "http://localhost:8080/api?key=123",
			pathOrURL: "/models",
			expected:  "http://localhost:8080/api/models?key=123",
		},
		{
			name:      "path that is just slash",
			baseURL:   "http://localhost:8080/api",
			pathOrURL: "/",
			expected:  "http://localhost:8080/api",
		},
		{
			name:      "absolute URL with port",
			baseURL:   "http://localhost:8080/api/",
			pathOrURL: "http://192.168.1.100:9000/models",
			expected:  "http://192.168.1.100:9000/models",
		},
		{
			name:      "absolute URL with path",
			baseURL:   "http://localhost:8080/",
			pathOrURL: "https://api.example.com/v1/chat/completions",
			expected:  "https://api.example.com/v1/chat/completions",
		},
		{
			name:      "absolute URL with query string",
			baseURL:   "http://localhost:8080/api/",
			pathOrURL: "http://other:9000/models?format=json",
			expected:  "http://other:9000/models?format=json",
		},
		{
			name:      "both empty",
			baseURL:   "",
			pathOrURL: "",
			expected:  "",
		},
		{
			name:      "base URL with fragment",
			baseURL:   "http://localhost:8080/api#section",
			pathOrURL: "/models",
			expected:  "http://localhost:8080/api/models#section",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := ResolveURLPath(tc.baseURL, tc.pathOrURL)
			if result != tc.expected {
				t.Errorf("ResolveURLPath(%q, %q) = %q, expected %q",
					tc.baseURL, tc.pathOrURL, result, tc.expected)
			}
		})
	}
}
