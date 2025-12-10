package util

import "testing"

func TestJoinURLPath(t *testing.T) {
	tests := []struct {
		name     string
		baseURL  string
		path     string
		expected string
	}{
		{
			name:     "base with trailing slash, path with leading slash",
			baseURL:  "http://localhost:12434/engines/llama.cpp/",
			path:     "/v1/models",
			expected: "http://localhost:12434/engines/llama.cpp/v1/models",
		},
		{
			name:     "base without trailing slash, path with leading slash",
			baseURL:  "http://localhost:11434",
			path:     "/api/tags",
			expected: "http://localhost:11434/api/tags",
		},
		{
			name:     "base with trailing slash, path without leading slash",
			baseURL:  "http://localhost:12434/api/",
			path:     "v1/models",
			expected: "http://localhost:12434/api/v1/models",
		},
		{
			name:     "base without trailing slash, path without leading slash",
			baseURL:  "http://localhost:11434",
			path:     "api/tags",
			expected: "http://localhost:11434/api/tags",
		},
		{
			name:     "empty base",
			baseURL:  "",
			path:     "/v1/models",
			expected: "/v1/models",
		},
		{
			name:     "empty path",
			baseURL:  "http://localhost:11434",
			path:     "",
			expected: "http://localhost:11434",
		},
		{
			name:     "Docker nested path case",
			baseURL:  "http://localhost:12434/engines/llama.cpp/",
			path:     "/v1/models",
			expected: "http://localhost:12434/engines/llama.cpp/v1/models",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := JoinURLPath(tc.baseURL, tc.path)
			if result != tc.expected {
				t.Errorf("JoinURLPath(%q, %q) = %q, expected %q",
					tc.baseURL, tc.path, result, tc.expected)
			}
		})
	}
}
