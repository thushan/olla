package domain_test

import (
	"testing"

	"github.com/thushan/olla/internal/core/domain"
)

func TestLlamaCppProfileConstant(t *testing.T) {
	expected := "llamacpp"
	if domain.ProfileLlamaCpp != expected {
		t.Errorf("ProfileLlamaCpp: expected %q, got %q", expected, domain.ProfileLlamaCpp)
	}
}
