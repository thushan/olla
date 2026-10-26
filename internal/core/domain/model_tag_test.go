package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOllamaDefaultTagName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		in   string
		want string
		ok   bool
	}{
		{"all-minilm", "all-minilm:latest", true},
		{"all-minilm:latest", "", false},
		{"qwen2.5:0.5b", "", false},
		{"library/foo", "library/foo:latest", true},
		{"registry.example.com:5000/foo", "registry.example.com:5000/foo:latest", true},
		{"registry.example.com:5000/foo:7b", "", false},
		{"", "", false},
		{"foo:", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			t.Parallel()
			got, ok := OllamaDefaultTagName(tt.in)
			assert.Equal(t, tt.ok, ok)
			assert.Equal(t, tt.want, got)
		})
	}
}
