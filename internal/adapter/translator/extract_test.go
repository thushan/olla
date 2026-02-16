package translator

import (
	"encoding/json"
	"testing"
)

func TestExtractModelName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		body      []byte
		wantModel string
		wantErr   bool
	}{
		{
			name:      "standard anthropic request",
			body:      []byte(`{"model":"claude-3-opus-20240229","max_tokens":1024,"messages":[{"role":"user","content":"Hello"}]}`),
			wantModel: "claude-3-opus-20240229",
		},
		{
			name:      "model field first",
			body:      []byte(`{"model":"gpt-4","temperature":0.7}`),
			wantModel: "gpt-4",
		},
		{
			name:      "model field last",
			body:      []byte(`{"max_tokens":1024,"messages":[],"model":"llama-3.1-70b"}`),
			wantModel: "llama-3.1-70b",
		},
		{
			name:      "model field in middle of large request",
			body:      []byte(`{"messages":[{"role":"user","content":"Tell me a story"}],"model":"claude-3-haiku-20240307","max_tokens":4096,"stream":true,"temperature":0.5}`),
			wantModel: "claude-3-haiku-20240307",
		},
		{
			name:      "model with slashes and colons",
			body:      []byte(`{"model":"org/repo:latest","max_tokens":100}`),
			wantModel: "org/repo:latest",
		},
		{
			name: "nested model string in content does not match",
			body: []byte(`{"messages":[{"role":"user","content":"{\"model\":\"wrong\"}"}],"model":"correct-model","max_tokens":100}`),
			// gjson extracts the top-level "model" field
			wantModel: "correct-model",
		},
		{
			name:    "missing model field",
			body:    []byte(`{"max_tokens":1024,"messages":[]}`),
			wantErr: true,
		},
		{
			name:    "empty model string",
			body:    []byte(`{"model":"","max_tokens":1024}`),
			wantErr: true,
		},
		{
			name:    "model as number",
			body:    []byte(`{"model":42,"max_tokens":1024}`),
			wantErr: true,
		},
		{
			name:    "model as boolean",
			body:    []byte(`{"model":true,"max_tokens":1024}`),
			wantErr: true,
		},
		{
			name:    "model as array",
			body:    []byte(`{"model":["a","b"],"max_tokens":1024}`),
			wantErr: true,
		},
		{
			name:    "model as object",
			body:    []byte(`{"model":{"name":"test"},"max_tokens":1024}`),
			wantErr: true,
		},
		{
			name:    "model as null",
			body:    []byte(`{"model":null,"max_tokens":1024}`),
			wantErr: true,
		},
		{
			name:    "invalid JSON",
			body:    []byte(`{invalid json`),
			wantErr: true,
		},
		{
			name:    "empty body",
			body:    []byte{},
			wantErr: true,
		},
		{
			name:    "nil body",
			body:    nil,
			wantErr: true,
		},
		{
			name:    "just whitespace",
			body:    []byte(`   `),
			wantErr: true,
		},
		{
			name:    "empty JSON object",
			body:    []byte(`{}`),
			wantErr: true,
		},
		{
			name:    "HTML error page",
			body:    []byte(`<html><body>502 Bad Gateway</body></html>`),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := ExtractModelName(tt.body)
			if tt.wantErr {
				if err == nil {
					t.Errorf("ExtractModelName() expected error, got model=%q", got)
				}
				return
			}
			if err != nil {
				t.Errorf("ExtractModelName() unexpected error: %v", err)
				return
			}
			if got != tt.wantModel {
				t.Errorf("ExtractModelName() = %q, want %q", got, tt.wantModel)
			}
		})
	}
}

// BenchmarkExtractModelName measures the lightweight gjson-based extraction
func BenchmarkExtractModelName(b *testing.B) {
	// Realistic Anthropic Messages API request body
	body := []byte(`{
		"model": "claude-3-opus-20240229",
		"max_tokens": 4096,
		"system": "You are a helpful assistant.",
		"messages": [
			{"role": "user", "content": "What is the capital of France?"}
		],
		"stream": true,
		"temperature": 0.7
	}`)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = ExtractModelName(body)
	}
}

// BenchmarkFullUnmarshal compares against a minimal struct unmarshal.
// Note: This understates the real TransformRequest cost, which also includes:
// - Field validation with custom rules
// - Format conversion from Anthropic to OpenAI structure
// - Message content transformation
// - System prompt injection
// - Buffer pool operations
// The actual performance improvement over TransformRequest is larger than shown here.
func BenchmarkFullUnmarshal(b *testing.B) {
	type anthropicReq struct {
		Model     string      `json:"model"`
		MaxTokens int         `json:"max_tokens"`
		System    interface{} `json:"system,omitempty"`
		Messages  []struct {
			Content interface{} `json:"content"`
			Role    string      `json:"role"`
		} `json:"messages"`
		Stream      bool     `json:"stream,omitempty"`
		Temperature *float64 `json:"temperature,omitempty"`
	}

	body := []byte(`{
		"model": "claude-3-opus-20240229",
		"max_tokens": 4096,
		"system": "You are a helpful assistant.",
		"messages": [
			{"role": "user", "content": "What is the capital of France?"}
		],
		"stream": true,
		"temperature": 0.7
	}`)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var req anthropicReq
		_ = json.Unmarshal(body, &req)
	}
}

// BenchmarkExtractModelName_LargeBody simulates a larger, multi-turn conversation
func BenchmarkExtractModelName_LargeBody(b *testing.B) {
	body := []byte(`{
		"model": "claude-3-opus-20240229",
		"max_tokens": 4096,
		"system": "You are a coding assistant. Help the user write Go code.",
		"messages": [
			{"role": "user", "content": "Write me a function that sorts a slice of integers"},
			{"role": "assistant", "content": "Here is a function that sorts integers:\n\nfunc sortInts(nums []int) []int {\n\tsort.Ints(nums)\n\treturn nums\n}"},
			{"role": "user", "content": "Now write a benchmark for it"},
			{"role": "assistant", "content": "Here is a benchmark:\n\nfunc BenchmarkSortInts(b *testing.B) {\n\tnums := make([]int, 1000)\n\tfor i := range nums {\n\t\tnums[i] = rand.Intn(10000)\n\t}\n\tb.ResetTimer()\n\tfor i := 0; i < b.N; i++ {\n\t\tcopy := make([]int, len(nums))\n\t\tcopy(copy, nums)\n\t\tsortInts(copy)\n\t}\n}"},
			{"role": "user", "content": "Good, now add error handling and make it generic"}
		],
		"stream": true,
		"temperature": 0.3
	}`)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = ExtractModelName(body)
	}
}
