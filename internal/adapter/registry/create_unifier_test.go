package registry

import (
	"bytes"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/thushan/olla/internal/adapter/unifier"
	"github.com/thushan/olla/internal/core/ports"
	"github.com/thushan/olla/internal/logger"
)

func TestCreateUnifier_UsesCallerFallbackAndLogsOnce(t *testing.T) {
	t.Parallel()

	cfg := unifier.DefaultConfig()
	for name, create := range map[string]func() (ports.ModelUnifier, error){
		"factory error": func() (ports.ModelUnifier, error) { return nil, errors.New("boom") },
		"nil unifier":   func() (ports.ModelUnifier, error) { return nil, nil },
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var buf bytes.Buffer
			log := logger.NewPlainStyledLogger(slog.New(slog.NewTextHandler(&buf, nil)))

			def := createUnifier(log, create, func() ports.ModelUnifier { return unifier.NewDefaultUnifierWithConfig(cfg) })
			require.IsType(t, &unifier.DefaultUnifier{}, def)
			require.Equal(t, 1, strings.Count(buf.String(), "falling back"), "one error log per failed creation")

			// the lifecycle registry relies on *LifecycleUnifier assertions, so its fallback must keep the type
			lc := createUnifier(log, create, func() ports.ModelUnifier { return unifier.NewLifecycleUnifier(cfg, log) })
			require.IsType(t, &unifier.LifecycleUnifier{}, lc)
			require.Equal(t, 2, strings.Count(buf.String(), "falling back"))
		})
	}
}

func TestCreateUnifier_HealthyFactoryDoesNotFallBack(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	log := logger.NewPlainStyledLogger(slog.New(slog.NewTextHandler(&buf, nil)))
	want := unifier.NewDefaultUnifier()
	got := createUnifier(log, func() (ports.ModelUnifier, error) { return want, nil },
		func() ports.ModelUnifier { t.Error("fallback must not run"); return nil })
	require.Same(t, want, got)
	require.Empty(t, buf.String())
}
