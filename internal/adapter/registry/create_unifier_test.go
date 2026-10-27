package registry

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/thushan/olla/internal/core/ports"
)

func TestCreateUnifier_FallsBackToDefaultOnError(t *testing.T) {
	t.Parallel()

	for name, create := range map[string]func() (ports.ModelUnifier, error){
		"factory error": func() (ports.ModelUnifier, error) { return nil, errors.New("boom") },
		"nil unifier":   func() (ports.ModelUnifier, error) { return nil, nil },
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			u := createUnifier(createTestLogger(), create)
			require.NotNil(t, u, "a failed factory call must still yield a working unifier")
		})
	}
}
