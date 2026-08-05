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

func TestResolveRuntimeLogFormat_NonTTY_UsesConfiguredValue(t *testing.T) {
	t.Parallel()

	warned := false
	got := resolveRuntimeLogFormat("text", false, false, func(string, ...any) { warned = true })

	if got != "text" {
		t.Errorf("expected configured format 'text', got %q", got)
	}
	if warned {
		t.Error("did not expect a warning for a valid format")
	}
}

func TestResolveRuntimeLogFormat_NonTTY_JSONHonoured(t *testing.T) {
	t.Parallel()

	got := resolveRuntimeLogFormat("json", false, false, func(string, ...any) {})

	if got != LogFormatJSON {
		t.Errorf("expected non-TTY output to honour configured json format, got %q", got)
	}
}

func TestResolveRuntimeLogFormat_NonTTY_EmptyFallsBackToDefault(t *testing.T) {
	t.Parallel()

	warned := false
	got := resolveRuntimeLogFormat("", false, false, func(string, ...any) { warned = true })

	if got != DefaultLoggerFormat {
		t.Errorf("expected default format %q for unset config, got %q", DefaultLoggerFormat, got)
	}
	if warned {
		t.Error("an unset logging.format is not an error, so no warning should fire")
	}
}

func TestResolveRuntimeLogFormat_NonTTY_InvalidFallsBackAndWarns(t *testing.T) {
	t.Parallel()

	var gotMsg string
	got := resolveRuntimeLogFormat("yaml", false, false, func(msg string, args ...any) { gotMsg = msg })

	if got != DefaultLoggerFormat {
		t.Errorf("expected fallback to default format %q, got %q", DefaultLoggerFormat, got)
	}
	if gotMsg == "" {
		t.Error("expected a warning to be raised for an invalid format")
	}
}

// TestResolveRuntimeLogFormat_TTY_PrettyWinsRegardlessOfConfig pins the core
// regression fix: a dev terminal keeps its pretty logs even when config.yaml
// (or a local override) says "format: json" for headless/Docker use, so
// startup output doesn't flip formats mid-stream.
func TestResolveRuntimeLogFormat_TTY_PrettyWinsRegardlessOfConfig(t *testing.T) {
	t.Parallel()

	for _, configured := range []string{"", "json", "text", "bogus"} {
		got := resolveRuntimeLogFormat(configured, false, true, func(string, ...any) {})
		if got != LogFormatText {
			t.Errorf("configured=%q: expected TTY to force text (pretty), got %q", configured, got)
		}
	}
}

// TestResolveRuntimeLogFormat_TTY_ExplicitEnvOverridesTTY pins the other half:
// an explicit OLLA_LOGGING_FORMAT is deliberate operator intent and forces
// its value even on an interactive TTY.
func TestResolveRuntimeLogFormat_TTY_ExplicitEnvOverridesTTY(t *testing.T) {
	t.Parallel()

	got := resolveRuntimeLogFormat("json", true, true, func(string, ...any) {})

	if got != LogFormatJSON {
		t.Errorf("expected explicit env format to override TTY pretty-wins default, got %q", got)
	}
}

// TestResolveRuntimeLogFormat_NonTTY_ExplicitEnvInvalidWarnsAndDefaults checks
// the env-forced path still validates rather than passing through garbage.
func TestResolveRuntimeLogFormat_NonTTY_ExplicitEnvInvalidWarnsAndDefaults(t *testing.T) {
	t.Parallel()

	var gotMsg string
	got := resolveRuntimeLogFormat("bogus", true, false, func(msg string, args ...any) { gotMsg = msg })

	if got != DefaultLoggerFormat {
		t.Errorf("expected fallback to default format %q, got %q", DefaultLoggerFormat, got)
	}
	if gotMsg == "" {
		t.Error("expected a warning to be raised for an invalid env-forced format")
	}
}

func TestResolveRuntimeLogOutput_UsesConfiguredValue(t *testing.T) {
	t.Parallel()

	warned := false
	got := resolveRuntimeLogOutput("file", func(string, ...any) { warned = true })

	if got != "file" {
		t.Errorf("expected configured output 'file', got %q", got)
	}
	if warned {
		t.Error("did not expect a warning for a valid output")
	}
}

func TestResolveRuntimeLogOutput_EmptyFallsBackToDefault(t *testing.T) {
	t.Parallel()

	warned := false
	got := resolveRuntimeLogOutput("", func(string, ...any) { warned = true })

	if got != DefaultLoggerOutput {
		t.Errorf("expected default output %q for unset config, got %q", DefaultLoggerOutput, got)
	}
	if warned {
		t.Error("an unset logging.output is not an error, so no warning should fire")
	}
}

func TestResolveRuntimeLogOutput_InvalidFallsBackAndWarns(t *testing.T) {
	t.Parallel()

	var gotMsg string
	got := resolveRuntimeLogOutput("syslog", func(msg string, args ...any) { gotMsg = msg })

	if got != DefaultLoggerOutput {
		t.Errorf("expected fallback to default output %q, got %q", DefaultLoggerOutput, got)
	}
	if gotMsg == "" {
		t.Error("expected a warning to be raised for an invalid output")
	}
}
