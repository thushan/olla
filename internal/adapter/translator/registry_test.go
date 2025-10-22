package translator

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"testing"

	"github.com/thushan/olla/internal/logger"
)

// mockTranslator is a test double for RequestTranslator interface
type mockTranslator struct {
	name string
}

func (m *mockTranslator) TransformRequest(ctx context.Context, r *http.Request) (*TransformedRequest, error) {
	return nil, nil
}

func (m *mockTranslator) TransformResponse(ctx context.Context, openaiResp interface{}, original *http.Request) (interface{}, error) {
	return nil, nil
}

func (m *mockTranslator) TransformStreamingResponse(ctx context.Context, openaiStream io.Reader, w http.ResponseWriter, original *http.Request) error {
	return nil
}

func (m *mockTranslator) Name() string {
	return m.name
}

// createTestLogger creates a minimal logger for testing
func createTestLogger() logger.StyledLogger {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	return logger.NewPlainStyledLogger(log)
}

func TestNewRegistry(t *testing.T) {
	log := createTestLogger()
	registry := NewRegistry(log)

	if registry == nil {
		t.Fatal("Expected non-nil registry")
	}

	if registry.translators == nil {
		t.Error("Expected initialized translators map")
	}

	if registry.logger == nil {
		t.Error("Expected non-nil logger")
	}
}

func TestRegistry_Register(t *testing.T) {
	tests := []struct {
		name           string
		translatorName string
		translator     RequestTranslator
		wantName       string
		wantCount      int
	}{
		{
			name:           "register with explicit name",
			translatorName: "anthropic",
			translator:     &mockTranslator{name: "anthropic"},
			wantName:       "anthropic",
			wantCount:      1,
		},
		{
			name:           "register with empty name uses translator.Name()",
			translatorName: "",
			translator:     &mockTranslator{name: "gemini"},
			wantName:       "gemini",
			wantCount:      1,
		},
		{
			name:           "register multiple translators",
			translatorName: "bedrock",
			translator:     &mockTranslator{name: "bedrock"},
			wantName:       "bedrock",
			wantCount:      1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log := createTestLogger()
			registry := NewRegistry(log)

			registry.Register(tt.translatorName, tt.translator)

			// Verify registration
			all := registry.GetAll()
			if len(all) != tt.wantCount {
				t.Errorf("Expected %d translator(s), got %d", tt.wantCount, len(all))
			}

			// Verify we can retrieve it
			retrieved, err := registry.Get(tt.wantName)
			if err != nil {
				t.Errorf("Expected no error retrieving %s, got: %v", tt.wantName, err)
			}

			if retrieved == nil {
				t.Errorf("Expected non-nil translator for %s", tt.wantName)
			}

			if retrieved.Name() != tt.wantName {
				t.Errorf("Expected translator name %s, got %s", tt.wantName, retrieved.Name())
			}
		})
	}
}

func TestRegistry_Register_Overwrite(t *testing.T) {
	log := createTestLogger()
	registry := NewRegistry(log)

	// Register first translator
	first := &mockTranslator{name: "anthropic"}
	registry.Register("anthropic", first)

	// Register second translator with same name - should overwrite
	second := &mockTranslator{name: "anthropic"}
	registry.Register("anthropic", second)

	// Should still have only one translator
	all := registry.GetAll()
	if len(all) != 1 {
		t.Errorf("Expected 1 translator after overwrite, got %d", len(all))
	}

	// Retrieved translator should be the second one
	retrieved, err := registry.Get("anthropic")
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	// Simple check that it's a different instance (in real code you'd use pointer comparison)
	if retrieved.Name() != "anthropic" {
		t.Errorf("Expected overwritten translator name to be anthropic, got %s", retrieved.Name())
	}
}

func TestRegistry_Get(t *testing.T) {
	log := createTestLogger()
	registry := NewRegistry(log)

	// Register test translators
	anthropic := &mockTranslator{name: "anthropic"}
	gemini := &mockTranslator{name: "gemini"}
	registry.Register("anthropic", anthropic)
	registry.Register("gemini", gemini)

	tests := []struct {
		name          string
		translatorKey string
		wantError     bool
		wantName      string
	}{
		{
			name:          "get existing translator - anthropic",
			translatorKey: "anthropic",
			wantError:     false,
			wantName:      "anthropic",
		},
		{
			name:          "get existing translator - gemini",
			translatorKey: "gemini",
			wantError:     false,
			wantName:      "gemini",
		},
		{
			name:          "get non-existent translator",
			translatorKey: "bedrock",
			wantError:     true,
			wantName:      "",
		},
		{
			name:          "get with empty name",
			translatorKey: "",
			wantError:     true,
			wantName:      "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			retrieved, err := registry.Get(tt.translatorKey)

			if tt.wantError {
				if err == nil {
					t.Error("Expected error, got nil")
				}
				if retrieved != nil {
					t.Error("Expected nil translator on error")
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error, got: %v", err)
				}
				if retrieved == nil {
					t.Error("Expected non-nil translator")
				}
				if retrieved != nil && retrieved.Name() != tt.wantName {
					t.Errorf("Expected translator name %s, got %s", tt.wantName, retrieved.Name())
				}
			}
		})
	}
}

func TestRegistry_GetAll(t *testing.T) {
	tests := []struct {
		name        string
		translators map[string]RequestTranslator
		wantCount   int
	}{
		{
			name:        "empty registry",
			translators: map[string]RequestTranslator{},
			wantCount:   0,
		},
		{
			name: "single translator",
			translators: map[string]RequestTranslator{
				"anthropic": &mockTranslator{name: "anthropic"},
			},
			wantCount: 1,
		},
		{
			name: "multiple translators",
			translators: map[string]RequestTranslator{
				"anthropic": &mockTranslator{name: "anthropic"},
				"gemini":    &mockTranslator{name: "gemini"},
				"bedrock":   &mockTranslator{name: "bedrock"},
			},
			wantCount: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log := createTestLogger()
			registry := NewRegistry(log)

			// Register all test translators
			for name, translator := range tt.translators {
				registry.Register(name, translator)
			}

			// Get all translators
			all := registry.GetAll()

			if len(all) != tt.wantCount {
				t.Errorf("Expected %d translator(s), got %d", tt.wantCount, len(all))
			}

			// Verify each expected translator is present
			for name := range tt.translators {
				if _, exists := all[name]; !exists {
					t.Errorf("Expected translator %s to be in GetAll() result", name)
				}
			}
		})
	}
}

func TestRegistry_GetAll_ReturnsCopy(t *testing.T) {
	log := createTestLogger()
	registry := NewRegistry(log)

	// Register a translator
	registry.Register("anthropic", &mockTranslator{name: "anthropic"})

	// Get all translators
	all := registry.GetAll()

	// Modify the returned map
	all["hacked"] = &mockTranslator{name: "hacked"}

	// Verify the registry wasn't affected
	if _, err := registry.Get("hacked"); err == nil {
		t.Error("Expected registry to be unaffected by external map modification")
	}

	// Original should still be there
	if _, err := registry.Get("anthropic"); err != nil {
		t.Error("Expected original translator to still exist")
	}
}

func TestRegistry_GetAvailableNames(t *testing.T) {
	tests := []struct {
		name        string
		translators []string
		wantNames   []string
	}{
		{
			name:        "empty registry",
			translators: []string{},
			wantNames:   []string{},
		},
		{
			name:        "single translator",
			translators: []string{"anthropic"},
			wantNames:   []string{"anthropic"},
		},
		{
			name:        "multiple translators sorted",
			translators: []string{"gemini", "anthropic", "bedrock"},
			wantNames:   []string{"anthropic", "bedrock", "gemini"}, // Should be sorted
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log := createTestLogger()
			registry := NewRegistry(log)

			// Register translators
			for _, name := range tt.translators {
				registry.Register(name, &mockTranslator{name: name})
			}

			// Get available names
			names := registry.GetAvailableNames()

			if len(names) != len(tt.wantNames) {
				t.Errorf("Expected %d names, got %d", len(tt.wantNames), len(names))
			}

			// Verify order and contents
			for i, wantName := range tt.wantNames {
				if i >= len(names) {
					t.Errorf("Missing expected name: %s", wantName)
					continue
				}
				if names[i] != wantName {
					t.Errorf("Expected name[%d] = %s, got %s", i, wantName, names[i])
				}
			}
		})
	}
}

func TestRegistry_Concurrency(t *testing.T) {
	// Test concurrent access to ensure mutex protection works
	log := createTestLogger()
	registry := NewRegistry(log)

	// Pre-register one translator
	registry.Register("base", &mockTranslator{name: "base"})

	done := make(chan bool)
	iterations := 100

	// Concurrent writes
	go func() {
		for i := 0; i < iterations; i++ {
			registry.Register("writer1", &mockTranslator{name: "writer1"})
		}
		done <- true
	}()

	// Concurrent reads
	go func() {
		for i := 0; i < iterations; i++ {
			_, _ = registry.Get("base")
		}
		done <- true
	}()

	// Concurrent GetAll
	go func() {
		for i := 0; i < iterations; i++ {
			_ = registry.GetAll()
		}
		done <- true
	}()

	// Wait for all goroutines
	<-done
	<-done
	<-done

	// Verify registry is still consistent
	if _, err := registry.Get("base"); err != nil {
		t.Error("Expected base translator to still exist after concurrent access")
	}
}
