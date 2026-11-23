package logger

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/thushan/olla/theme"
)

// Characterisation tests for the two raw-string keys this package owns across
// the context-key rework (docs/specs/go-1.27-modernise.md, section 6):
// "request_id" as the slog key the styled loggers attach via WithRequestID,
// and DefaultDetailedCookie ("detailed") as the context flag that keeps a log
// record out of the terminal while the file handler still receives it.

// WithRequestID must attach the ID under the slog key "request_id" in both
// styled loggers, so every lifecycle line a handler emits names the request.
func TestStyledLoggers_RequestIDUsesTheRequestIDSlogKey(t *testing.T) {
	t.Parallel()

	newCapturing := func() (StyledLogger, *bytes.Buffer) {
		buf := &bytes.Buffer{}
		base := slog.New(slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
		return NewStyledLogger(base, &theme.Theme{}, false), buf
	}

	t.Run("plain logger", func(t *testing.T) {
		t.Parallel()
		styled, buf := newCapturing()
		styled.WithRequestID("huacaya-grazing-0042").Info("Request completed")
		assertRequestIDKey(t, buf, "huacaya-grazing-0042")
	})

	t.Run("pretty logger", func(t *testing.T) {
		t.Parallel()
		buf := &bytes.Buffer{}
		base := slog.New(slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
		log := NewPrettyStyledLogger(base, &theme.Theme{})
		log.WithRequestID("suri-trekking-00ff").Info("Request completed")
		assertRequestIDKey(t, buf, "suri-trekking-00ff")
	})
}

func assertRequestIDKey(t *testing.T, buf *bytes.Buffer, want string) {
	t.Helper()
	line := strings.TrimSpace(buf.String())
	if line == "" {
		t.Fatal("styled logger produced no output")
	}
	var record map[string]any
	if err := json.Unmarshal([]byte(line), &record); err != nil {
		t.Fatalf("log line is not JSON: %v (%q)", err, line)
	}
	if got, ok := record["request_id"].(string); !ok || got != want {
		t.Errorf("log line request_id = %v, want %q", record["request_id"], want)
	}
}

// A record emitted with the detailed context flag must be kept out of the
// terminal handler while the file handler still writes it, and a record
// without the flag must reach both. That is the whole contract of the
// "detailed" cookie key: dual logging (clean terminal, detailed file).
func TestDetailedContextFlag_TerminalFiltersFileKeeps(t *testing.T) {
	// No t.Parallel(): the test swaps os.Stdout while the logger chain is
	// constructed, and a parallel neighbour logging to the real stdout would
	// interleave with the capture.

	// NO_COLOR forces the plain text terminal handler, which is the one wrapped
	// by the detailed filter; the JSON stdout handler is unfiltered by design.
	t.Setenv("NO_COLOR", "1")

	logDir := t.TempDir()
	cfg := &Config{
		Level:      LogLevelDebug,
		PrettyLogs: true,
		FileOutput: true,
		LogDir:     logDir,
	}

	readEnd, writeEnd, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	realStdout := os.Stdout
	os.Stdout = writeEnd
	base, cleanup, err := New(cfg)
	os.Stdout = realStdout
	if err != nil {
		t.Fatalf("logger.New: %v", err)
	}
	styled := NewPlainStyledLogger(base)
	pretty := NewPrettyStyledLogger(base, theme.GetTheme("default"))

	styled.Info("plain line")
	styled.InfoWithContext("Endpoint status", "ollama-1", LogContext{
		UserArgs:     []any{"model", "mistral-small3.2"},
		DetailedArgs: []any{"latency_ms", 42},
	})
	// The pretty styled logger shares the contract: its detailed record must
	// stay out of the terminal while the file keeps it.
	pretty.Info("pretty line")
	pretty.InfoWithContext("Endpoint status", "ollama-2", LogContext{
		UserArgs:     []any{"model", "qwen3-32b"},
		DetailedArgs: []any{"queue_depth", 7},
	})

	// Nothing else writes to the pipe: closing the write end lets ReadAll return.
	if err := writeEnd.Close(); err != nil {
		t.Fatalf("close pipe write end: %v", err)
	}
	terminal, err := io.ReadAll(readEnd)
	if err != nil {
		t.Fatalf("read terminal output: %v", err)
	}
	readEnd.Close()
	cleanup()

	termOut := string(terminal)
	if !strings.Contains(termOut, "plain line") || !strings.Contains(termOut, "pretty line") {
		t.Errorf("terminal output missing a plain record:\n%s", termOut)
	}
	// The plain logger's clean contextual record names its endpoint directly;
	// the pretty one styles the name with ANSI the text handler then escapes,
	// so its clean record is identified by its unstyled model arg instead.
	if !strings.Contains(termOut, "Endpoint status ollama-1") || !strings.Contains(termOut, "model=qwen3-32b") {
		t.Errorf("terminal output missing the clean contextual records:\n%s", termOut)
	}
	if strings.Contains(termOut, "latency_ms") || strings.Contains(termOut, "queue_depth") {
		t.Errorf("terminal output must not carry either logger's detailed record:\n%s", termOut)
	}

	fileBytes, err := os.ReadFile(filepath.Join(logDir, DefaultLogOutputName))
	if err != nil {
		t.Fatalf("read log file: %v", err)
	}
	var fileRecords []map[string]any
	for line := range strings.SplitSeq(strings.TrimSpace(string(fileBytes)), "\n") {
		if line == "" {
			continue
		}
		var rec map[string]any
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatalf("file line is not JSON: %v (%q)", err, line)
		}
		fileRecords = append(fileRecords, rec)
	}
	if len(fileRecords) != 6 {
		t.Fatalf("log file has %d records, want 6 (per styled logger: plain, clean contextual, detailed):\n%s", len(fileRecords), fileBytes)
	}
	detailedFields := map[string]string{"latency_ms": "ollama-1", "queue_depth": "ollama-2"}
	for _, rec := range fileRecords {
		for field, endpoint := range detailedFields {
			if rec[field] != nil && rec["endpoint_name"] != endpoint {
				t.Errorf("detailed record %s has endpoint_name = %v, want %s", field, rec["endpoint_name"], endpoint)
			}
		}
	}
	for field := range detailedFields {
		var saw bool
		for _, rec := range fileRecords {
			if rec[field] != nil {
				saw = true
			}
		}
		if !saw {
			t.Errorf("log file missing the %s detailed record:\n%s", field, fileBytes)
		}
	}
}
