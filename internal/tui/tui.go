package tui

import (
	"context"
	"fmt"
	"io"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"golang.org/x/term"

	"github.com/thushan/olla/internal/core/ports"
	"github.com/thushan/olla/internal/logger"
)

// TUI manages the Bubble Tea TUI interface
type TUI struct {
	model            Model
	program          *tea.Program
	originalOutput   io.Writer
	contentWriter    *ContentWriter
	logger           *logger.StyledLogger
	discoveryService ports.DiscoveryService
}

// ContentWriter captures logger output and sends it to the TUI
type ContentWriter struct {
	program *tea.Program
}

func (cw *ContentWriter) Write(p []byte) (n int, err error) {
	if cw.program != nil {
		// Send content to TUI
		cw.program.Send(contentMsg(string(p)))
	}
	return len(p), nil
}

// NewTUI creates a new TUI instance
func NewTUI(discoveryService ports.DiscoveryService, logger *logger.StyledLogger) *TUI {
	model := NewModel(discoveryService, logger)

	return &TUI{
		model:            model,
		logger:           logger,
		discoveryService: discoveryService,
	}
}

// Start starts the TUI interface
func (t *TUI) Start(ctx context.Context) error {
	// Check if terminal supports TUI
	if !isTerminalCapable() {
		return fmt.Errorf("terminal does not support TUI interface")
	}

	// Create content writer to capture logger output
	t.contentWriter = &ContentWriter{}

	// Create Bubble Tea program
	t.program = tea.NewProgram(
		t.model,
		tea.WithAltScreen(),       // Use alternate screen buffer
		tea.WithMouseCellMotion(), // Enable mouse support
	)

	// Set up content writer to send to program
	t.contentWriter.program = t.program

	// Redirect logger output to TUI (optional - for now we'll just show commands)
	// This would require modifying the logger to support custom writers

	// Add welcome message
	t.program.Send(contentMsg("Olla TUI started - Press 'h' for health, 'q' to quit"))
	t.program.Send(contentMsg("Use arrow keys to scroll, ':' to enter commands"))

	// Start the program
	model, err := t.program.Run()
	if err != nil {
		return fmt.Errorf("TUI error: %w", err)
	}

	// Update our model with the final state
	if finalModel, ok := model.(Model); ok {
		t.model = finalModel
	}

	return nil
}

// Stop stops the TUI interface
func (t *TUI) Stop() {
	if t.program != nil {
		t.program.Quit()
	}
}

// isTerminalCapable checks if the terminal supports TUI features
func isTerminalCapable() bool {
	// Check if stdout is a terminal
	if !term.IsTerminal(int(os.Stdout.Fd())) {
		return false
	}

	// Check terminal size
	width, height, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil {
		return false
	}

	// Ensure minimum dimensions
	return width >= 80 && height >= 20
}

// SendContent sends content to the TUI (for external use)
func (t *TUI) SendContent(content string) {
	if t.program != nil {
		t.program.Send(contentMsg(content))
	}
}
