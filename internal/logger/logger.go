package logger

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/pterm/pterm"
	"gopkg.in/natefinch/lumberjack.v2"

	"github.com/thushan/olla/internal/util"
	"github.com/thushan/olla/theme"
)

type Config struct {
	Level      string
	LogDir     string
	Theme      string
	MaxSize    int // megabytes
	MaxBackups int
	MaxAge     int // days
	FileOutput bool
	PrettyLogs bool
}

const (
	DefaultLogOutputName  = "olla.log"
	DefaultDetailedCookie = "detailed"

	LogLevelDebug   = "debug"
	LogLevelInfo    = "info"
	LogLevelWarn    = "warn"
	LogLevelWarning = "warning"
	LogLevelError   = "error"
	LogLevelFatal   = "fatal"
	LogLevelPanic   = "panic"
)

func New(cfg *Config) (*slog.Logger, func(), error) {
	level := parseLevel(cfg.Level)
	appTheme := theme.GetTheme(cfg.Theme)

	var cleanupFuncs []func()
	var handlers []slog.Handler

	// if folks want pretty, prioritise that
	// unless we have NO_COLOR set, then we use text
	if cfg.PrettyLogs {
		if util.ShouldUseColors() {
			handlers = append(handlers, createPTermHandler(level, appTheme))
		} else {
			handlers = append(handlers, createTextHandler(level))
		}
	} else {
		handlers = append(handlers, createJSONHandler(level, os.Stdout))
	}

	// Optional file handler if they really want it
	if cfg.FileOutput {
		fileHandler, cleanup, err := createFileHandler(cfg, level)
		if err != nil {
			return nil, nil, err
		}
		cleanupFuncs = append(cleanupFuncs, cleanup)
		handlers = append(handlers, fileHandler)
	}

	var logger *slog.Logger
	if len(handlers) == 1 {
		logger = slog.New(handlers[0])
	} else {
		logger = slog.New(&multiHandler{handlers: handlers})
	}

	cleanup := func() {
		for _, fn := range cleanupFuncs {
			fn()
		}
	}

	return logger, cleanup, nil
}

// createPTermHandler creates a colorful pterm-based handler for terminals
func createPTermHandler(level slog.Level, appTheme *theme.Theme) slog.Handler {
	plogger := pterm.DefaultLogger.
		WithLevel(convertToPTermLevel(level)).
		WithWriter(os.Stdout).
		WithFormatter(pterm.LogFormatterColorful)

	keyStyles := map[string]pterm.Style{
		"level": *appTheme.Info,
		"msg":   *appTheme.Info,
		"time":  *appTheme.Muted,
	}
	plogger = plogger.WithKeyStyles(keyStyles)

	return &terminalFilterHandler{
		handler: pterm.NewSlogHandler(plogger),
	}
}

// createTextHandler creates a plain text handler for non-TTY environments
func createTextHandler(level slog.Level) slog.Handler {
	return &terminalFilterHandler{
		handler: slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
			Level:     level,
			AddSource: false,
		}),
	}
}

// createJSONHandler creates a JSON handler for the specified writer
func createJSONHandler(level slog.Level, writer *os.File) slog.Handler {
	return slog.NewJSONHandler(writer, &slog.HandlerOptions{
		Level:       level,
		AddSource:   false,
		ReplaceAttr: replaceTimeAttr,
	})
}

// createFileHandler creates a file-based JSON handler with rotation
func createFileHandler(cfg *Config, level slog.Level) (slog.Handler, func(), error) {
	if err := os.MkdirAll(cfg.LogDir, 0755); err != nil {
		return nil, nil, err
	}

	rotator := &lumberjack.Logger{
		Filename:   filepath.Join(cfg.LogDir, DefaultLogOutputName),
		MaxSize:    cfg.MaxSize,
		MaxBackups: cfg.MaxBackups,
		MaxAge:     cfg.MaxAge,
		Compress:   true,
	}

	handler := slog.NewJSONHandler(rotator, &slog.HandlerOptions{
		Level:       level,
		AddSource:   false,
		ReplaceAttr: replaceTimeAttr,
	})

	cleanup := func() {
		_ = rotator.Close()
	}

	return handler, cleanup, nil
}

// replaceTimeAttr formats timestamps consistently
func replaceTimeAttr(groups []string, a slog.Attr) slog.Attr {
	if a.Key == slog.TimeKey {
		return slog.Attr{
			Key:   "timestamp",
			Value: slog.StringValue(a.Value.Time().Format("2006-01-02 15:04:05")),
		}
	}
	return a
}

// multiHandler sends logs to multiple handlers
type multiHandler struct {
	handlers []slog.Handler
}

func (h *multiHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, handler := range h.handlers {
		if handler.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

func (h *multiHandler) Handle(ctx context.Context, record slog.Record) error {
	for _, handler := range h.handlers {
		if handler.Enabled(ctx, record.Level) {
			if err := handler.Handle(ctx, record); err != nil {
				return err
			}
		}
	}
	return nil
}

func (h *multiHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	newHandlers := make([]slog.Handler, len(h.handlers))
	for i, handler := range h.handlers {
		newHandlers[i] = handler.WithAttrs(attrs)
	}
	return &multiHandler{handlers: newHandlers}
}

func (h *multiHandler) WithGroup(name string) slog.Handler {
	newHandlers := make([]slog.Handler, len(h.handlers))
	for i, handler := range h.handlers {
		newHandlers[i] = handler.WithGroup(name)
	}
	return &multiHandler{handlers: newHandlers}
}

// terminalFilterHandler filters out detailed context logs from terminal output
type terminalFilterHandler struct {
	handler slog.Handler
}

func (h *terminalFilterHandler) Enabled(ctx context.Context, level slog.Level) bool {
	if isDetailedLog(ctx) {
		return false
	}
	return h.handler.Enabled(ctx, level)
}

func (h *terminalFilterHandler) Handle(ctx context.Context, record slog.Record) error {
	if isDetailedLog(ctx) {
		return nil
	}
	return h.handler.Handle(ctx, record)
}

func (h *terminalFilterHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &terminalFilterHandler{handler: h.handler.WithAttrs(attrs)}
}

func (h *terminalFilterHandler) WithGroup(name string) slog.Handler {
	return &terminalFilterHandler{handler: h.handler.WithGroup(name)}
}

// isDetailedLog checks if a log should only go to files (not terminal)
func isDetailedLog(ctx context.Context) bool {
	if detailed := ctx.Value(DefaultDetailedCookie); detailed != nil {
		if d, ok := detailed.(bool); ok && d {
			return true
		}
	}
	return false
}

// parseLevel converts string log level to slog.Level
func parseLevel(level string) slog.Level {
	switch strings.ToLower(level) {
	case LogLevelDebug:
		return slog.LevelDebug
	case LogLevelInfo:
		return slog.LevelInfo
	case LogLevelWarn, LogLevelWarning:
		return slog.LevelWarn
	case LogLevelError:
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// convertToPTermLevel converts slog level to pterm level
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
