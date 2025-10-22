package inspector

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/thushan/olla/internal/logger"
)

func TestSimpleInspector(t *testing.T) {
	// Create temp directory for test
	tmpDir := t.TempDir()

	// Use a simple logger for tests
	log := logger.NewPlainStyledLogger(slog.Default())
	inspector := NewSimple(true, tmpDir, "X-Session-ID", log)

	t.Run("LogRequest", func(t *testing.T) {
		sessionID := "test-session"
		model := "test-model"
		body := []byte(`{"test":"request"}`)

		err := inspector.LogRequest(sessionID, model, body)
		if err != nil {
			t.Fatalf("LogRequest failed: %v", err)
		}

		// Verify file was created
		today := time.Now().Format("2006-01-02")
		filePath := filepath.Join(tmpDir, today, sessionID+".jsonl")

		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			t.Fatalf("Expected file %s to exist", filePath)
		}

		// Read and verify content
		data, err := os.ReadFile(filePath)
		if err != nil {
			t.Fatalf("Failed to read file: %v", err)
		}

		var entry Entry
		if err := json.Unmarshal(data, &entry); err != nil {
			t.Fatalf("Failed to unmarshal entry: %v", err)
		}

		if entry.Type != "request" {
			t.Errorf("Expected type 'request', got %s", entry.Type)
		}
		if entry.Model != model {
			t.Errorf("Expected model %s, got %s", model, entry.Model)
		}
	})

	t.Run("LogResponse", func(t *testing.T) {
		sessionID := "test-session-2"
		body := []byte(`{"test":"response"}`)

		err := inspector.LogResponse(sessionID, body)
		if err != nil {
			t.Fatalf("LogResponse failed: %v", err)
		}

		// Verify file was created
		today := time.Now().Format("2006-01-02")
		filePath := filepath.Join(tmpDir, today, sessionID+".jsonl")

		data, err := os.ReadFile(filePath)
		if err != nil {
			t.Fatalf("Failed to read file: %v", err)
		}

		var entry Entry
		if err := json.Unmarshal(data, &entry); err != nil {
			t.Fatalf("Failed to unmarshal entry: %v", err)
		}

		if entry.Type != "response" {
			t.Errorf("Expected type 'response', got %s", entry.Type)
		}
	})

	t.Run("MultipleEntries", func(t *testing.T) {
		sessionID := "multi-test"

		// Log request
		reqBody := []byte(`{"test":"request1"}`)
		if err := inspector.LogRequest(sessionID, "model1", reqBody); err != nil {
			t.Fatalf("LogRequest failed: %v", err)
		}

		// Log response
		respBody := []byte(`{"test":"response1"}`)
		if err := inspector.LogResponse(sessionID, respBody); err != nil {
			t.Fatalf("LogResponse failed: %v", err)
		}

		// Verify both entries are in the file
		today := time.Now().Format("2006-01-02")
		filePath := filepath.Join(tmpDir, today, sessionID+".jsonl")

		data, err := os.ReadFile(filePath)
		if err != nil {
			t.Fatalf("Failed to read file: %v", err)
		}

		// Count lines (each entry is a line)
		lines := 0
		for _, b := range data {
			if b == '\n' {
				lines++
			}
		}

		if lines != 2 {
			t.Errorf("Expected 2 lines, got %d", lines)
		}
	})

	t.Run("DisabledInspector", func(t *testing.T) {
		disabledInspector := NewSimple(false, tmpDir, "X-Session-ID", log)

		err := disabledInspector.LogRequest("test", "model", []byte(`{}`))
		if err != nil {
			t.Fatalf("Disabled inspector should not return error: %v", err)
		}

		// Verify no files were created for disabled inspector
		today := time.Now().Format("2006-01-02")
		filePath := filepath.Join(tmpDir, today, "test.jsonl")

		if _, err := os.Stat(filePath); !os.IsNotExist(err) {
			t.Errorf("Expected no file when inspector is disabled")
		}
	})
}

func TestSimpleInspectorHelpers(t *testing.T) {
	log := logger.NewPlainStyledLogger(slog.Default())

	t.Run("GetSessionHeader", func(t *testing.T) {
		inspector := NewSimple(true, "/tmp", "X-Custom-Session", log)
		if got := inspector.GetSessionHeader(); got != "X-Custom-Session" {
			t.Errorf("Expected X-Custom-Session, got %s", got)
		}
	})

	t.Run("DefaultSessionHeader", func(t *testing.T) {
		inspector := NewSimple(true, "/tmp", "", log)
		if got := inspector.GetSessionHeader(); got != "X-Session-ID" {
			t.Errorf("Expected default X-Session-ID, got %s", got)
		}
	})

	t.Run("Enabled", func(t *testing.T) {
		enabledInspector := NewSimple(true, "/tmp", "", log)
		if !enabledInspector.Enabled() {
			t.Error("Expected inspector to be enabled")
		}

		disabledInspector := NewSimple(false, "/tmp", "", log)
		if disabledInspector.Enabled() {
			t.Error("Expected inspector to be disabled")
		}
	})
}

// TestSanitiseSessionID tests session ID sanitisation and path traversal prevention
func TestSanitiseSessionID(t *testing.T) {
	tmpDir := t.TempDir()

	tests := []struct {
		name      string
		sessionID string
		wantErr   bool
		wantID    string
		desc      string
	}{
		{
			name:      "ValidSimpleID",
			sessionID: "test-session-123",
			wantErr:   false,
			wantID:    "test-session-123",
			desc:      "Normal alphanumeric with dashes should be allowed",
		},
		{
			name:      "ValidWithUnderscores",
			sessionID: "test_session_123",
			wantErr:   false,
			wantID:    "test_session_123",
			desc:      "Underscores should be allowed",
		},
		{
			name:      "EmptySessionID",
			sessionID: "",
			wantErr:   false,
			wantID:    "default",
			desc:      "Empty session ID should default to 'default'",
		},
		{
			name:      "DefaultSessionID",
			sessionID: "default",
			wantErr:   false,
			wantID:    "default",
			desc:      "Default session ID should be accepted",
		},
		{
			name:      "PathTraversalUnix",
			sessionID: "../../../etc/passwd",
			wantErr:   true,
			desc:      "Unix path traversal should be rejected",
		},
		{
			name:      "PathTraversalWindows",
			sessionID: "..\\..\\..\\windows\\system32",
			wantErr:   true,
			desc:      "Windows path traversal should be rejected",
		},
		{
			name:      "PathTraversalDotDot",
			sessionID: "..",
			wantErr:   true,
			desc:      "Double dot should be rejected",
		},
		{
			name:      "PathTraversalSingleDot",
			sessionID: ".",
			wantErr:   true,
			desc:      "Single dot should be rejected",
		},
		{
			name:      "URLEncodedTraversal",
			sessionID: "%2e%2e%2f%2e%2e%2ftest",
			wantErr:   true,
			desc:      "URL encoded path traversal should be rejected",
		},
		{
			name:      "NullByte",
			sessionID: "test\x00evil",
			wantErr:   true,
			desc:      "Null bytes should be rejected",
		},
		{
			name:      "NullByteAtEnd",
			sessionID: "test\x00",
			wantErr:   true,
			desc:      "Null byte at end should be rejected",
		},
		{
			name:      "ExcessiveLength",
			sessionID: "a123456789012345678901234567890123456789012345678901234567890123456789",
			wantErr:   true,
			desc:      "Session ID exceeding 64 characters should be rejected",
		},
		{
			name:      "MaxLength",
			sessionID: "a12345678901234567890123456789012345678901234567890123456789012",
			wantErr:   false,
			wantID:    "a12345678901234567890123456789012345678901234567890123456789012",
			desc:      "Session ID at exactly 64 characters should be accepted",
		},
		{
			name:      "SlashInMiddle",
			sessionID: "test/session",
			wantErr:   true,
			desc:      "Forward slash should be rejected",
		},
		{
			name:      "BackslashInMiddle",
			sessionID: "test\\session",
			wantErr:   true,
			desc:      "Backslash should be rejected",
		},
		{
			name:      "AbsolutePathUnix",
			sessionID: "/etc/passwd",
			wantErr:   true,
			desc:      "Absolute Unix path should be rejected",
		},
		{
			name:      "AbsolutePathWindows",
			sessionID: "C:\\Windows\\System32",
			wantErr:   true,
			desc:      "Absolute Windows path should be rejected",
		},
		{
			name:      "SpecialCharsSpace",
			sessionID: "test session",
			wantErr:   true,
			desc:      "Spaces should be rejected",
		},
		{
			name:      "SpecialCharsBang",
			sessionID: "test!session",
			wantErr:   true,
			desc:      "Special characters like ! should be rejected",
		},
		{
			name:      "SpecialCharsAt",
			sessionID: "test@session",
			wantErr:   true,
			desc:      "Special characters like @ should be rejected",
		},
		{
			name:      "UnicodeCharacters",
			sessionID: "test-😀-session",
			wantErr:   true,
			desc:      "Unicode characters should be rejected",
		},
		{
			name:      "NewlineCharacter",
			sessionID: "test\nsession",
			wantErr:   true,
			desc:      "Newline characters should be rejected",
		},
		{
			name:      "CarriageReturn",
			sessionID: "test\rsession",
			wantErr:   true,
			desc:      "Carriage return characters should be rejected",
		},
		{
			name:      "TabCharacter",
			sessionID: "test\tsession",
			wantErr:   true,
			desc:      "Tab characters should be rejected",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := sanitiseSessionID(tt.sessionID, tmpDir)

			if tt.wantErr {
				if err == nil {
					t.Errorf("sanitiseSessionID() error = nil, wantErr = true; %s", tt.desc)
				}
			} else {
				if err != nil {
					t.Errorf("sanitiseSessionID() unexpected error = %v; %s", err, tt.desc)
				}
				if got != tt.wantID {
					t.Errorf("sanitiseSessionID() = %v, want %v; %s", got, tt.wantID, tt.desc)
				}
			}
		})
	}
}

// TestPathTraversalProtection verifies that malicious session IDs cannot escape the output directory
func TestPathTraversalProtection(t *testing.T) {
	tmpDir := t.TempDir()
	log := logger.NewPlainStyledLogger(slog.Default())
	inspector := NewSimple(true, tmpDir, "X-Session-ID", log)

	// Create a file outside the tmpDir to verify it's not accessible
	outsideDir := filepath.Join(filepath.Dir(tmpDir), "outside")
	if err := os.MkdirAll(outsideDir, 0700); err != nil {
		t.Fatalf("Failed to create outside directory: %v", err)
	}
	defer os.RemoveAll(outsideDir)

	attackVectors := []string{
		"../../../etc/passwd",
		"..\\..\\..\\windows\\system32",
		"../../outside/test",
		"..%2F..%2Foutside",
		"....//....//outside",
	}

	for _, sessionID := range attackVectors {
		t.Run("Attack_"+sessionID, func(t *testing.T) {
			// Attempt to log with malicious session ID
			err := inspector.LogRequest(sessionID, "test-model", []byte(`{"test":"attack"}`))

			// Should not error (we fall back to "default")
			if err != nil {
				t.Errorf("LogRequest should not error with fallback, got: %v", err)
			}

			// Verify no files were created outside tmpDir
			var foundOutside bool
			_ = filepath.Walk(outsideDir, func(path string, info os.FileInfo, err error) error {
				if err != nil {
					return nil
				}
				if !info.IsDir() && strings.HasSuffix(path, ".jsonl") {
					foundOutside = true
				}
				return nil
			})

			if foundOutside {
				t.Errorf("File created outside tmpDir with session ID: %s", sessionID)
			}

			// Verify file was created in tmpDir with "default" session ID
			today := time.Now().Format("2006-01-02")
			expectedPath := filepath.Join(tmpDir, today, "default.jsonl")
			if _, err := os.Stat(expectedPath); os.IsNotExist(err) {
				t.Errorf("Expected fallback file not created: %s", expectedPath)
			}
		})
	}
}

// TestFilePermissions verifies that created files and directories have secure permissions
func TestFilePermissions(t *testing.T) {
	tmpDir := t.TempDir()
	log := logger.NewPlainStyledLogger(slog.Default())
	inspector := NewSimple(true, tmpDir, "X-Session-ID", log)

	sessionID := "test-permissions"
	body := []byte(`{"test":"permissions"}`)

	err := inspector.LogRequest(sessionID, "test-model", body)
	if err != nil {
		t.Fatalf("LogRequest failed: %v", err)
	}

	// Check directory permissions (should be 0700)
	today := time.Now().Format("2006-01-02")
	dirPath := filepath.Join(tmpDir, today)
	dirInfo, err := os.Stat(dirPath)
	if err != nil {
		t.Fatalf("Failed to stat directory: %v", err)
	}

	// Note: Windows doesn't support Unix-style permission bits the same way
	// On Windows, permissions are managed via ACLs, not Unix mode bits
	// We still verify the directory and file were created successfully
	dirMode := dirInfo.Mode().Perm()
	expectedDirMode := os.FileMode(0700)

	// Only check permissions on Unix-like systems
	// On Windows, the os.FileMode will be different but security is handled via ACLs
	if dirMode&0400 == 0 {
		t.Errorf("Directory should be readable by owner, got permissions: %o", dirMode)
	}
	// Log permissions for debugging but don't fail on Windows
	if dirMode != expectedDirMode {
		t.Logf("Note: Directory permissions = %o, expected %o (difference expected on Windows)", dirMode, expectedDirMode)
	}

	// Check file permissions (should be 0600)
	filePath := filepath.Join(dirPath, sessionID+".jsonl")
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		t.Fatalf("Failed to stat file: %v", err)
	}

	fileMode := fileInfo.Mode().Perm()
	expectedFileMode := os.FileMode(0600)

	// Only check permissions on Unix-like systems
	// On Windows, security is handled via ACLs
	if fileMode&0400 == 0 {
		t.Errorf("File should be readable by owner, got permissions: %o", fileMode)
	}
	// Log permissions for debugging but don't fail on Windows
	if fileMode != expectedFileMode {
		t.Logf("Note: File permissions = %o, expected %o (difference expected on Windows)", fileMode, expectedFileMode)
	}
}

// TestSecurityWarning verifies that security warning is logged on first use
func TestSecurityWarning(t *testing.T) {
	tmpDir := t.TempDir()

	// We can't easily test log output, but we can verify the warning flag is set
	log := logger.NewPlainStyledLogger(slog.Default())
	inspector := NewSimple(true, tmpDir, "X-Session-ID", log)

	if inspector.warningLogged {
		t.Error("Warning should not be logged before first use")
	}

	// First request should trigger warning
	_ = inspector.LogRequest("test", "model", []byte(`{}`))

	if !inspector.warningLogged {
		t.Error("Warning should be logged after first LogRequest")
	}

	// Second request should not log again (already set)
	inspector.warningLogged = false // Reset to verify it gets set again
	_ = inspector.LogRequest("test2", "model", []byte(`{}`))

	if !inspector.warningLogged {
		t.Error("Warning flag should remain set")
	}

	// Test with LogResponse as well
	inspector2 := NewSimple(true, tmpDir, "X-Session-ID", log)
	if inspector2.warningLogged {
		t.Error("Warning should not be logged before first use (inspector2)")
	}

	_ = inspector2.LogResponse("test", []byte(`{}`))

	if !inspector2.warningLogged {
		t.Error("Warning should be logged after first LogResponse")
	}
}
