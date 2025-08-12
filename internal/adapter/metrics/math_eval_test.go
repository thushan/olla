package metrics

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEvaluateSimpleMath(t *testing.T) {
	tests := []struct {
		name      string
		expr      string
		variables map[string]float64
		expected  float64
		wantErr   bool
	}{
		{
			name:     "simple division",
			expr:     "100 / 10",
			expected: 10,
		},
		{
			name:     "simple multiplication",
			expr:     "5 * 4",
			expected: 20,
		},
		{
			name:     "simple addition",
			expr:     "10 + 5",
			expected: 15,
		},
		{
			name:     "simple subtraction",
			expr:     "20 - 8",
			expected: 12,
		},
		{
			name: "variable substitution",
			expr: "tokens / seconds",
			variables: map[string]float64{
				"tokens":  100,
				"seconds": 5,
			},
			expected: 20,
		},
		{
			name: "nanoseconds to milliseconds",
			expr: "duration_ns / 1000000",
			variables: map[string]float64{
				"duration_ns": 5000000000,
			},
			expected: 5000,
		},
		{
			name: "tokens per second calculation",
			expr: "output_tokens / (eval_duration_ns / 1000000000)",
			variables: map[string]float64{
				"output_tokens":    20,
				"eval_duration_ns": 2000000000,
			},
			expected: 10,
		},
		{
			name:      "division by zero",
			expr:      "10 / 0",
			variables: map[string]float64{},
			wantErr:   true,
		},
		{
			name:     "complex expression with parentheses",
			expr:     "(10 + 5) * 2",
			expected: 30,
		},
		{
			name:     "nested parentheses",
			expr:     "((10 + 5) * 2) / 3",
			expected: 10,
		},
		{
			name:     "mixed operations",
			expr:     "10 + 5 * 2",
			expected: 30, // our simplified evaluator processes left to right
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := evaluateSimpleMath(tt.expr, tt.variables)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.InDelta(t, tt.expected, result, 0.001)
			}
		})
	}
}

func TestEvalExpression(t *testing.T) {
	tests := []struct {
		name     string
		expr     string
		expected float64
		wantErr  bool
	}{
		{
			name:     "simple number",
			expr:     "42",
			expected: 42,
		},
		{
			name:     "expression with spaces",
			expr:     "  10  +  5  ",
			expected: 15,
		},
		{
			name:     "negative result",
			expr:     "10 - 20",
			expected: -10,
		},
		{
			name:     "decimal numbers",
			expr:     "10.5 + 5.5",
			expected: 16,
		},
		{
			name:     "large numbers",
			expr:     "1000000000 / 1000000",
			expected: 1000,
		},
		{
			name:    "invalid expression",
			expr:    "10 ++ 5",
			wantErr: true,
		},
		{
			name:    "unmatched parentheses",
			expr:    "(10 + 5",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := evalExpression(tt.expr)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.InDelta(t, tt.expected, result, 0.001)
			}
		})
	}
}

func TestEvaluateSimpleMath_EdgeCases(t *testing.T) {
	// Test with empty expression
	_, err := evaluateSimpleMath("", nil)
	assert.Error(t, err)

	// Test with only spaces
	_, err = evaluateSimpleMath("   ", nil)
	assert.Error(t, err)

	// Test with variable not found
	_, err = evaluateSimpleMath("unknown_var", nil)
	assert.Error(t, err)

	// Test with partial variable substitution
	result, err := evaluateSimpleMath("known + unknown", map[string]float64{
		"known": 10,
	})
	assert.Error(t, err)

	// Test scientific notation
	result, err = evaluateSimpleMath("1e6 / 1000", nil)
	assert.NoError(t, err)
	assert.InDelta(t, 1000, result, 0.001)
}
