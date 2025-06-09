package logger

import (
	"log/slog"

	"github.com/thushan/olla/internal/core/domain"

	"github.com/thushan/olla/theme"
)

// StyledLogger interface for different formatting strategies
type StyledLogger interface {
	Debug(msg string, args ...any)
	Info(msg string, args ...any)
	Warn(msg string, args ...any)
	Error(msg string, args ...any)
	InfoWithCount(msg string, count int, args ...any)
	InfoWithEndpoint(msg string, endpoint string, args ...any)
	InfoWithHealthCheck(msg string, endpoint string, args ...any)
	InfoWithNumbers(msg string, numbers ...int64)
	WarnWithEndpoint(msg string, endpoint string, args ...any)
	ErrorWithEndpoint(msg string, endpoint string, args ...any)
	InfoHealthy(msg string, endpoint string, args ...any)
	InfoHealthStatus(msg string, name string, status domain.EndpointStatus, args ...any)
	GetUnderlying() *slog.Logger
	WithRequestID(requestID string) StyledLogger
	InfoConfigChange(oldName, newName string)
	WithAttrs(attrs ...slog.Attr) StyledLogger
	With(args ...any) StyledLogger
	InfoWithContext(msg string, endpoint string, ctx LogContext)
	WarnWithContext(msg string, endpoint string, ctx LogContext)
	ErrorWithContext(msg string, endpoint string, ctx LogContext)
}

func NewStyledLogger(logger *slog.Logger, theme *theme.Theme, prettyMode bool) StyledLogger {
	if prettyMode {
		return NewPrettyStyledLogger(logger, theme)
	}
	return NewPlainStyledLogger(logger)
}

func NewWithTheme(cfg *Config) (*slog.Logger, StyledLogger, func(), error) {
	logger, cleanup, err := New(cfg)
	if err != nil {
		return nil, nil, nil, err
	}

	appTheme := theme.GetTheme(cfg.Theme)
	styledLogger := NewStyledLogger(logger, appTheme, cfg.PrettyLogs)

	return logger, styledLogger, cleanup, nil
}

func toInterfaceSlice(strs []string) []interface{} {
	result := make([]interface{}, len(strs))
	for i, s := range strs {
		result[i] = s
	}
	return result
}

/**
 * LogContext provides a structured way to separate user-facing and detailed logging context.
 * This allows for cleaner terminal output while still capturing all necessary details in the log file.
 * That way, we get a clean TUI output with user-friendly messages, and detailed logs for debugging.
 */

// LogContext separates user-facing from detailed logging context
type LogContext struct {
	UserArgs     []interface{}
	DetailedArgs []interface{}
}
