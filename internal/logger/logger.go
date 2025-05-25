package logger

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"gopkg.in/natefinch/lumberjack.v2"

	"github.com/thushan/olla/internal/util"
	"github.com/thushan/olla/theme"
)

// Config holds the logger configuration
type Config struct {
	Level      string // debug, info, warn, error
	FileOutput bool
	LogDir     string
	MaxSize    int // megabytes
	MaxBackups int
	MaxAge     int // days
	Theme      string
}

const (
	DefaultLogOutputName = "olla.log"
)

// New creates a new logger with dual output (terminal + file) and rotation
func New(cfg *Config) (*slog.Logger, func(), error) {
	level := parseLevel(cfg.Level)
	appTheme := theme.GetTheme(cfg.Theme)

	var cleanupFuncs []func()

	// Create terminal handler using our custom handler instead of pterm
	terminalHandler := createTerminalHandler(level, appTheme)

	var logger *slog.Logger

	if cfg.FileOutput {
		fileHandler, cleanup, err := createFileHandler(cfg, level)
		if err != nil {
			return nil, nil, err
		}

		cleanupFuncs = append(cleanupFuncs, cleanup)
		handler := &multiHandler{
			terminalHandler: terminalHandler,
			fileHandler:     fileHandler,
		}
		logger = slog.New(handler)
	} else {
		// Terminal only
		logger = slog.New(terminalHandler)
	}

	cleanup := func() {
		for _, fn := range cleanupFuncs {
			fn()
		}
	}

	return logger, cleanup, nil
}

// createTerminalHandler creates a custom terminal handler with colors
func createTerminalHandler(level slog.Level, appTheme *theme.Theme) slog.Handler {
	opts := &slog.HandlerOptions{
		Level: level,
	}

	// Use our custom formatter that maintains the nice CLI output
	return &colorHandler{
		handler: &prettyHandler{
			opts:  opts,
			theme: appTheme,
		},
		theme: appTheme,
	}
}

// prettyHandler formats logs in a human-readable way like pterm did
type prettyHandler struct {
	opts  *slog.HandlerOptions
	theme *theme.Theme
	attrs []slog.Attr
	group string
}

func (h *prettyHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return level >= h.opts.Level.Level()
}

func (h *prettyHandler) Handle(ctx context.Context, record slog.Record) error {
	var output strings.Builder

	// new lipgloss style for gray
	timestampStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	output.WriteString(timestampStyle.Render(record.Time.Format("2006-01-02 15:04:05")))
	output.WriteString(" ")

	// Add colored level
	var levelStyle lipgloss.Style
	switch record.Level {
	case slog.LevelDebug:
		levelStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("12")) // Light blue
	case slog.LevelInfo:
		levelStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("10")) // Light green
	case slog.LevelWarn:
		levelStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Bold(true) // Light yellow, bold
	case slog.LevelError:
		levelStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true) // Light red, bold
	}

	if util.IsTerminal() {
		output.WriteString(levelStyle.Render(record.Level.String()))
	} else {
		output.WriteString(record.Level.String())
	}
	output.WriteString(" ")

	// Add the message (which already contains ANSI codes from styled logger)
	if util.IsTerminal() {
		output.WriteString(record.Message)
	} else {
		output.WriteString(stripANSI(record.Message))
	}

	// Add attributes in key=value format (like pterm structured logging)
	record.Attrs(func(attr slog.Attr) bool {
		output.WriteString(" ")
		output.WriteString(attr.Key)
		output.WriteString("=")

		// Format the value properly
		value := attr.Value.String()
		// If value contains spaces, quote it
		if strings.Contains(value, " ") {
			output.WriteString(`"`)
			output.WriteString(value)
			output.WriteString(`"`)
		} else {
			output.WriteString(value)
		}
		return true
	})

	output.WriteString("\n")

	fmt.Print(output.String())
	return nil
}

func (h *prettyHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &prettyHandler{
		opts:  h.opts,
		theme: h.theme,
		attrs: append(h.attrs, attrs...),
		group: h.group,
	}
}

func (h *prettyHandler) WithGroup(name string) slog.Handler {
	return &prettyHandler{
		opts:  h.opts,
		theme: h.theme,
		attrs: h.attrs,
		group: name,
	}
}

// stripANSI removes ANSI escape sequences from text
func stripANSI(text string) string {
	// Simple ANSI removal - could be enhanced with regex if needed
	result := ""
	inEscape := false

	for _, r := range text {
		if r == '\033' { // ESC character
			inEscape = true
			continue
		}

		if inEscape {
			if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') {
				inEscape = false
			}
			continue
		}

		result += string(r)
	}

	return result
}

// colorHandler wraps a handler to add colors to terminal output
type colorHandler struct {
	handler slog.Handler
	theme   *theme.Theme
}

func (h *colorHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.handler.Enabled(ctx, level)
}

func (h *colorHandler) Handle(ctx context.Context, record slog.Record) error {
	// Only add colors if we're in a terminal
	if util.IsTerminal() {
		// Create a styled version of the record for terminal output
		record = h.styleRecord(record)
	}
	return h.handler.Handle(ctx, record)
}

func (h *colorHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &colorHandler{
		handler: h.handler.WithAttrs(attrs),
		theme:   h.theme,
	}
}

func (h *colorHandler) WithGroup(name string) slog.Handler {
	return &colorHandler{
		handler: h.handler.WithGroup(name),
		theme:   h.theme,
	}
}

// styleRecord applies color styling to log records
func (h *colorHandler) styleRecord(record slog.Record) slog.Record {
	// For now, we'll keep this simple and not modify the record
	// Color styling will be handled by the underlying text handler
	// when we implement custom formatting later

	// Future enhancement: modify the record message with colors based on level
	return record
}

// createFileHandler creates a rotating file handler
func createFileHandler(cfg *Config, level slog.Level) (slog.Handler, func(), error) {
	if err := os.MkdirAll(cfg.LogDir, 0755); err != nil {
		return nil, nil, err
	}

	// Setup log rotation
	rotator := &lumberjack.Logger{
		Filename:   filepath.Join(cfg.LogDir, DefaultLogOutputName),
		MaxSize:    cfg.MaxSize,
		MaxBackups: cfg.MaxBackups,
		MaxAge:     cfg.MaxAge,
		Compress:   true,
	}

	handler := slog.NewJSONHandler(rotator, &slog.HandlerOptions{
		Level:     level,
		AddSource: true,
	})

	cleanup := func() {
		rotator.Close()
	}

	return handler, cleanup, nil
}

// multiHandler implements slog.Handler to write to multiple handlers
type multiHandler struct {
	terminalHandler slog.Handler
	fileHandler     slog.Handler
}

func (mh *multiHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return mh.terminalHandler.Enabled(ctx, level) || mh.fileHandler.Enabled(ctx, level)
}

func (mh *multiHandler) Handle(ctx context.Context, record slog.Record) error {
	// terminal:
	if mh.terminalHandler.Enabled(ctx, record.Level) {
		if err := mh.terminalHandler.Handle(ctx, record); err != nil {
			return err
		}
	}

	// file out:
	if mh.fileHandler.Enabled(ctx, record.Level) {
		if err := mh.fileHandler.Handle(ctx, record); err != nil {
			return err
		}
	}

	return nil
}

func (mh *multiHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &multiHandler{
		terminalHandler: mh.terminalHandler.WithAttrs(attrs),
		fileHandler:     mh.fileHandler.WithAttrs(attrs),
	}
}

func (mh *multiHandler) WithGroup(name string) slog.Handler {
	return &multiHandler{
		terminalHandler: mh.terminalHandler.WithGroup(name),
		fileHandler:     mh.fileHandler.WithGroup(name),
	}
}

// parseLevel converts string level to slog.Level
func parseLevel(level string) slog.Level {
	switch strings.ToLower(level) {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
