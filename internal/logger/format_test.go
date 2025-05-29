package logger

import (
	"strings"
	"testing"
)

var (
	ansiSample  = "\x1b[31mError:\x1b[0m Something went \x1b[1;33mwrong\x1b[0m"
	strippedOut = "Error: Something went wrong"
)

func TestStripAnsiCodes(t *testing.T) {
	got := stripAnsiCodes(ansiSample)
	if got != strippedOut {
		t.Errorf("stripAnsiCodes failed: got %q, want %q", got, strippedOut)
	}
}

func buildLargeAnsiInput(n int) string {
	var b strings.Builder
	for i := 0; i < n; i++ {
		b.WriteString(ansiSample)
	}
	return b.String()
}

func BenchmarkStripAnsiCodes_Small(b *testing.B) {
	for i := 0; i < b.N; i++ {
		stripAnsiCodes(ansiSample)
	}
}

func BenchmarkStripAnsiCodes_Large(b *testing.B) {
	large := buildLargeAnsiInput(1000) // ~60 KB input
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		stripAnsiCodes(large)
	}
}

