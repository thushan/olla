package logger

import (
	"context"
	"fmt"
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

	if cfg.PrettyLogs {
		terminalHandler := createTerminalHandler(level, appTheme)
		handlers = append(handlers, terminalHandler)
	} else {
		jsonHandler := createJSONHandler(level)
		handlers = append(handlers, jsonHandler)
	}

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
		logger = slog.New(&simpleMultiHandler{handlers: handlers})
	}

	cleanup := func() {
		for _, fn := range cleanupFuncs {
			fn()
		}
	}

	return logger, cleanup, nil
}

func createTerminalHandler(level slog.Level, appTheme *theme.Theme) slog.Handler {
	if util.ShouldUseColors() {
		// Colourful terminal output - use pterm
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
		return pterm.NewSlogHandler(plogger)
	}

	// JSON output for non-TTY - use standard slog JSON handler
	return slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level:       level,
		AddSource:   false,
		ReplaceAttr: fastReplaceAttr,
	})
}

func createJSONHandler(level slog.Level) slog.Handler {
	return slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level:       level,
		AddSource:   false,
		ReplaceAttr: fastReplaceAttr,
	})
}

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
		ReplaceAttr: fastReplaceAttr,
	})

	cleanup := func() {
		_ = rotator.Close()
	}

	return handler, cleanup, nil
}

// fastReplaceAttr - handles complex types and ANSI codes
func fastReplaceAttr(groups []string, a slog.Attr) slog.Attr {
	switch a.Key {
	case slog.TimeKey:
		return slog.Attr{
			Key:   "timestamp",
			Value: slog.StringValue(a.Value.Time().Format("2006-01-02 15:04:05")),
		}
	default:
		switch a.Value.Kind() {
		case slog.KindString:
			str := a.Value.String()
			if strings.ContainsRune(str, '\x1b') {
				return slog.Attr{Key: a.Key, Value: slog.StringValue(stripAnsiCodes(str))}
			}
		case slog.KindAny:
		default:
			return slog.Attr{Key: a.Key, Value: slog.StringValue(fmt.Sprintf("%v", a.Value.Any()))}
		}
	}
	return a
}

// simpleMultiHandler sends logs to multiple handlers without dual processing
type simpleMultiHandler struct {
	handlers []slog.Handler
}

func (h *simpleMultiHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, handler := range h.handlers {
		if handler.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

func (h *simpleMultiHandler) Handle(ctx context.Context, record slog.Record) error {
	for _, handler := range h.handlers {
		if handler.Enabled(ctx, record.Level) {
			if err := handler.Handle(ctx, record); err != nil {
				return err
			}
		}
	}
	return nil
}

func (h *simpleMultiHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	newHandlers := make([]slog.Handler, len(h.handlers))
	for i, handler := range h.handlers {
		newHandlers[i] = handler.WithAttrs(attrs)
	}
	return &simpleMultiHandler{handlers: newHandlers}
}

func (h *simpleMultiHandler) WithGroup(name string) slog.Handler {
	newHandlers := make([]slog.Handler, len(h.handlers))
	for i, handler := range h.handlers {
		newHandlers[i] = handler.WithGroup(name)
	}
	return &simpleMultiHandler{handlers: newHandlers}
}
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
