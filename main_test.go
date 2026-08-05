package main

import "testing"

func TestResolveRuntimeLogLevel_UsesConfiguredValue(t *testing.T) {
	t.Parallel()

	warned := false
	got := resolveRuntimeLogLevel("debug", func(string, ...any) { warned = true })

	if got != "debug" {
		t.Errorf("expected configured level 'debug', got %q", got)
	}
	if warned {
		t.Error("did not expect a warning for a valid level")
	}
}

func TestResolveRuntimeLogLevel_EmptyFallsBackToDefault(t *testing.T) {
	t.Parallel()

	warned := false
	got := resolveRuntimeLogLevel("", func(string, ...any) { warned = true })

	if got != DefaultLoggerLevel {
		t.Errorf("expected default level %q for unset config, got %q", DefaultLoggerLevel, got)
	}
	if warned {
		t.Error("an unset logging.level is not an error, so no warning should fire")
	}
}

func TestResolveRuntimeLogLevel_InvalidFallsBackAndWarns(t *testing.T) {
	t.Parallel()

	var gotMsg string
	got := resolveRuntimeLogLevel("verbose", func(msg string, args ...any) { gotMsg = msg })

	if got != DefaultLoggerLevel {
		t.Errorf("expected fallback to default level %q, got %q", DefaultLoggerLevel, got)
	}
	if gotMsg == "" {
		t.Error("expected a warning to be raised for an invalid level")
	}
}
