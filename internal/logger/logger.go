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
}

const (
	DefaultLogOutputName  = "olla.log"
	DefaultDetailedCookie = "detailed"
)

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
		logger = slog.New(pterm.NewSlogHandler(terminalLogger))
	}

	cleanup := func() {
		for _, fn := range cleanupFuncs {
			fn()
		}
	}

	return logger, cleanup, nil
}

func createTerminalLogger(level slog.Level, appTheme *theme.Theme) *pterm.Logger {
	plogger := pterm.DefaultLogger.
		WithLevel(convertToPTermLevel(level)).
		WithWriter(os.Stdout)

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

func createFileLogger(cfg *Config, level slog.Level) (slog.Handler, func(), error) {
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
		Level:     level,
		AddSource: false,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if a.Key == slog.TimeKey {
				return slog.Attr{
					Key:   "timestamp",
					Value: slog.StringValue(a.Value.Time().Format("2006-01-02 15:04:05")),
				}
			}
			// kinda strip ANSI codes from string values
			if a.Value.Kind() == slog.KindString {
				cleanValue := stripAnsiCodes(a.Value.String())
				if cleanValue != a.Value.String() {
					return slog.Attr{Key: a.Key, Value: slog.StringValue(cleanValue)}
				}
			}
			return a
		},
	})

	cleanup := func() {
		_ = rotator.Close()
	}

	return handler, cleanup, nil
}

type multiHandler struct {
	terminalHandler slog.Handler
	fileHandler     slog.Handler
}

func (mh *multiHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return mh.terminalHandler.Enabled(ctx, level) || mh.fileHandler.Enabled(ctx, level)
}

func (mh *multiHandler) Handle(ctx context.Context, record slog.Record) error {
	isDetailed := false
	if detailed := ctx.Value(DefaultDetailedCookie); detailed != nil {
		if d, ok := detailed.(bool); ok {
			isDetailed = d
		}
	}

	if !isDetailed && mh.terminalHandler.Enabled(ctx, record.Level) {
		if err := mh.terminalHandler.Handle(ctx, record); err != nil {
			return err
		}
	}

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
