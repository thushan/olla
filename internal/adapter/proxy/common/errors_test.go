package common

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"
)

func TestMakeUserFriendlyErrorEOFPreservesCauseAcrossContexts(t *testing.T) {
	tests := []struct {
		name           string
		duration       time.Duration
		errorContext   string
		expectedPhrase string
	}{
		{
			name:           "short streaming response",
			duration:       4 * time.Second,
			errorContext:   "streaming",
			expectedPhrase: "response ended prematurely",
		},
		{
			name:           "long streaming response",
			duration:       5 * time.Second,
			errorContext:   "streaming",
			expectedPhrase: "response stream ended unexpectedly",
		},
		{
			name:           "non streaming response",
			duration:       10 * time.Second,
			errorContext:   "backend",
			expectedPhrase: "AI backend ended communication unexpectedly",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cause := fmt.Errorf("round trip failed: %w", io.EOF)
			result := MakeUserFriendlyError(cause, tt.duration, tt.errorContext, 30*time.Second)

			if result == nil {
				t.Fatal("expected user-friendly error")
			}
			if !errors.Is(result, cause) {
				t.Fatalf("expected result to preserve wrapped cause, got: %v", result)
			}
			if !errors.Is(result, io.EOF) {
				t.Fatalf("expected result to preserve io.EOF, got: %v", result)
			}
			if !strings.Contains(result.Error(), tt.expectedPhrase) {
				t.Errorf("expected error to contain %q, got: %s", tt.expectedPhrase, result)
			}
		})
	}
}
