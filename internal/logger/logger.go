package logger

import (
	"context"
	"github.com/thushan/olla/internal/util"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/pterm/pterm"
	"gopkg.in/natefinch/lumberjack.v2"

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

	terminalLogger := createTerminalLogger(level, appTheme)

	var logger *slog.Logger

	if cfg.FileOutput {

		fileLogger, cleanup, err := createFileLogger(cfg, level)
		if err != nil {
			return nil, nil, err
		}

		cleanupFuncs = append(cleanupFuncs, cleanup)
		handler := &multiHandler{
			terminalHandler: pterm.NewSlogHandler(terminalLogger),
			fileHandler:     fileLogger,
		}
		logger = slog.New(handler)
	} else {
		// Terminal only
		logger = slog.New(pterm.NewSlogHandler(terminalLogger))
	}

	cleanup := func() {
		for _, fn := range cleanupFuncs {
			fn()
		}
	}

	return logger, cleanup, nil
}

// createTerminalLogger creates a PTerm logger with Theme colours
func createTerminalLogger(level slog.Level, appTheme *theme.Theme) *pterm.Logger {
	plogger := pterm.DefaultLogger.
		WithLevel(convertToPTermLevel(level)).
		WithWriter(os.Stdout)

	// Only use colours if we're in a terminal
	if util.IsTerminal() {
		plogger = plogger.WithFormatter(pterm.LogFormatterColorful)

		keyStyles := map[string]pterm.Style{
			"level": *appTheme.Info,
			"msg":   *appTheme.Info,
			"time":  *appTheme.Muted,
		}
		plogger = plogger.WithKeyStyles(keyStyles)
	} else {
		plogger = plogger.WithFormatter(pterm.LogFormatterJSON)
	}

	return plogger
}

// createFileLogger creates a rotating file logger
func createFileLogger(cfg *Config, level slog.Level) (slog.Handler, func(), error) {
	if err := os.MkdirAll(cfg.LogDir, 0755); err != nil {
		return nil, nil, err
	}

	// Setsup log rotation
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

// convertToPTermLevel converts slog.Level to pterm.LogLevel
func convertToPTermLevel(level slog.Level) pterm.LogLevel {
	switch level {
	case slog.LevelDebug:
		return pterm.LogLevelTrace
	case slog.LevelInfo:
		return pterm.LogLevelInfo
	case slog.LevelWarn:
		return pterm.LogLevelWarn
	case slog.LevelError:
		return pterm.LogLevelError
	default:
		return pterm.LogLevelInfo
	}
}
