package inspector

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/thushan/olla/internal/logger"
)

// Entry represents a single request or response entry in the inspector log
type Entry struct {
	Type      string          `json:"type"` // "request" or "response"
	Timestamp string          `json:"ts"`   // ISO8601 timestamp
	Model     string          `json:"model,omitempty"`
	Body      json.RawMessage `json:"body"`
}

// Simple is a minimal inspector that dumps requests/responses to disk
// Designed for quick debugging without fancy features
type Simple struct {
	logger        logger.StyledLogger
	outputDir     string
	sessionHeader string
	mu            sync.Mutex // protects file operations
	enabled       bool
}

// NewSimple creates a new simple inspector
func NewSimple(enabled bool, outputDir, sessionHeader string, log logger.StyledLogger) *Simple {
	if sessionHeader == "" {
		sessionHeader = "X-Session-ID"
	}

	return &Simple{
		enabled:       enabled,
		outputDir:     outputDir,
		sessionHeader: sessionHeader,
		logger:        log,
	}
}

// LogRequest logs an incoming request
func (s *Simple) LogRequest(sessionID, model string, body []byte) error {
	if !s.enabled {
		return nil
	}

	entry := Entry{
		Type:      "request",
		Timestamp: time.Now().Format(time.RFC3339),
		Model:     model,
		Body:      json.RawMessage(body),
	}

	return s.writeEntry(sessionID, entry)
}

// LogResponse logs an outgoing response
func (s *Simple) LogResponse(sessionID string, body []byte) error {
	if !s.enabled {
		return nil
	}

	entry := Entry{
		Type:      "response",
		Timestamp: time.Now().Format(time.RFC3339),
		Body:      json.RawMessage(body),
	}

	return s.writeEntry(sessionID, entry)
}

// writeEntry writes an entry to the appropriate session file
func (s *Simple) writeEntry(sessionID string, entry Entry) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Create directory structure: {outputDir}/{date}/{session-id}.jsonl
	today := time.Now().Format("2006-01-02")
	dirPath := filepath.Join(s.outputDir, today)

	if err := os.MkdirAll(dirPath, 0755); err != nil {
		s.logger.Error("Failed to create inspector directory", "path", dirPath, "error", err)
		return fmt.Errorf("create inspector dir: %w", err)
	}

	filePath := filepath.Join(dirPath, sessionID+".jsonl")

	// Open file in append mode, create if not exists
	f, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		s.logger.Error("Failed to open inspector file", "path", filePath, "error", err)
		return fmt.Errorf("open inspector file: %w", err)
	}
	defer f.Close()

	// Write entry as JSON line
	if err := json.NewEncoder(f).Encode(entry); err != nil {
		s.logger.Error("Failed to write inspector entry", "error", err)
		return fmt.Errorf("write inspector entry: %w", err)
	}

	s.logger.Debug("Logged inspector entry",
		"session", sessionID,
		"type", entry.Type,
		"file", filePath)

	return nil
}

// GetSessionHeader returns the header name used for session IDs
func (s *Simple) GetSessionHeader() string {
	return s.sessionHeader
}

// Enabled returns whether the inspector is enabled
func (s *Simple) Enabled() bool {
	return s.enabled
}
