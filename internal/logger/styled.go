package logger

import (
	"context"
	"fmt"
	"github.com/thushan/olla/internal/core/domain"
	"log/slog"

	"github.com/pterm/pterm"
	"github.com/thushan/olla/theme"
)

// StyledLogger wraps slog.Logger with Theme-aware formatting
type StyledLogger struct {
	logger *slog.Logger
	Theme  *theme.Theme
}

func NewStyledLogger(logger *slog.Logger, theme *theme.Theme) *StyledLogger {
	return &StyledLogger{
		logger: logger,
		Theme:  theme,
	}
}

func (sl *StyledLogger) Debug(msg string, args ...any) {
	sl.logger.Debug(msg, args...)
}

func (sl *StyledLogger) Info(msg string, args ...any) {
	sl.logger.Info(msg, args...)
}

func (sl *StyledLogger) Warn(msg string, args ...any) {
	sl.logger.Warn(msg, args...)
}

func (sl *StyledLogger) Error(msg string, args ...any) {
	sl.logger.Error(msg, args...)
}

func (sl *StyledLogger) InfoWithCount(msg string, count int, args ...any) {
	styledMsg := fmt.Sprintf("%s %s", msg, pterm.Style{sl.Theme.Counts}.Sprint("(", count, ")"))
	sl.logger.Info(styledMsg, args...)
}

func (sl *StyledLogger) InfoWithEndpoint(msg string, endpoint string, args ...any) {
	styledMsg := fmt.Sprintf("%s %s", msg, pterm.Style{sl.Theme.Endpoint}.Sprint(endpoint))
	sl.logger.Info(styledMsg, args...)
}

func (sl *StyledLogger) InfoWithHealthCheck(msg string, endpoint string, args ...any) {
	styledMsg := fmt.Sprintf("%s %s", msg, pterm.Style{sl.Theme.HealthCheck}.Sprint(endpoint))
	sl.logger.Info(styledMsg, args...)
}

func (sl *StyledLogger) InfoWithNumbers(msg string, numbers ...int64) {
	var formattedNums []string
	for _, num := range numbers {
		formattedNums = append(formattedNums, pterm.Style{sl.Theme.Numbers}.Sprint(num))
	}

	styledMsg := fmt.Sprintf(msg, toInterfaceSlice(formattedNums)...)
	sl.logger.Info(styledMsg)
}

func (sl *StyledLogger) WarnWithEndpoint(msg string, endpoint string, args ...any) {
	styledMsg := fmt.Sprintf("%s %s", msg, pterm.Style{sl.Theme.Endpoint}.Sprint(endpoint))
	sl.logger.Warn(styledMsg, args...)
}

func (sl *StyledLogger) ErrorWithEndpoint(msg string, endpoint string, args ...any) {
	styledMsg := fmt.Sprintf("%s %s", msg, pterm.Style{sl.Theme.Endpoint}.Sprint(endpoint))
	sl.logger.Error(styledMsg, args...)
}

func (sl *StyledLogger) InfoHealthy(msg string, endpoint string, args ...any) {
	styledMsg := fmt.Sprintf("%s %s", msg, pterm.Style{sl.Theme.HealthHealthy}.Sprint(endpoint))
	sl.logger.Info(styledMsg, args...)
}

func (sl *StyledLogger) InfoHealthStatus(msg string, name string, status domain.EndpointStatus, args ...any) {
	var statusColor pterm.Color
	var statusText string

	switch status {
	case domain.StatusHealthy:
		statusColor = sl.Theme.HealthHealthy
		statusText = "Healthy"
	case domain.StatusBusy:
		statusColor = sl.Theme.HealthBusy
		statusText = "Busy"
	case domain.StatusOffline:
		statusColor = sl.Theme.HealthOffline
		statusText = "Offline"
	case domain.StatusWarming:
		statusColor = sl.Theme.HealthWarming
		statusText = "Warming"
	case domain.StatusUnhealthy:
		statusColor = sl.Theme.HealthUnhealthy
		statusText = "Unhealthy"
	case domain.StatusUnknown:
		statusColor = sl.Theme.HealthUnknown
		statusText = "Unknown"
	}
	styledMsg := fmt.Sprintf("%s %s is %s", msg, pterm.Style{sl.Theme.Endpoint}.Sprint(name), pterm.Style{statusColor}.Sprint(statusText))
	sl.logger.Info(styledMsg, args...)
}

func (sl *StyledLogger) GetUnderlying() *slog.Logger {
	return sl.logger
}

func (sl *StyledLogger) WithRequestID(requestID string) *StyledLogger {
	return sl.With("request_id", requestID)
}

func (sl *StyledLogger) InfoConfigChange(oldName, newName string) {
	styledMsg := fmt.Sprintf("Endpoint configuration changed for %s to: %s",
		pterm.Style{sl.Theme.Endpoint}.Sprint(oldName),
		pterm.Style{sl.Theme.Endpoint}.Sprint(newName))
	sl.logger.Info(styledMsg)
}

func (sl *StyledLogger) WithAttrs(attrs ...slog.Attr) *StyledLogger {
	args := make([]any, 0, len(attrs)*2)
	for _, attr := range attrs {
		args = append(args, attr.Key, attr.Value)
	}

	return &StyledLogger{
		logger: sl.logger.With(args...),
		Theme:  sl.Theme,
	}
}

func (sl *StyledLogger) With(args ...any) *StyledLogger {
	return &StyledLogger{
		logger: sl.logger.With(args...),
		Theme:  sl.Theme,
	}
}

func toInterfaceSlice(strs []string) []interface{} {
	result := make([]interface{}, len(strs))
	for i, s := range strs {
		result[i] = s
	}
	return result
}

func NewWithTheme(cfg *Config) (*slog.Logger, *StyledLogger, func(), error) {
	logger, cleanup, err := New(cfg)
	if err != nil {
		return nil, nil, nil, err
	}

	appTheme := theme.GetTheme(cfg.Theme)
	styledLogger := NewStyledLogger(logger, appTheme)

	return logger, styledLogger, cleanup, nil
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

func (sl *StyledLogger) InfoWithContext(msg string, endpoint string, ctx LogContext) {
	sl.logWithContext("info", msg, endpoint, ctx)
}

func (sl *StyledLogger) WarnWithContext(msg string, endpoint string, ctx LogContext) {
	sl.logWithContext("warn", msg, endpoint, ctx)
}

func (sl *StyledLogger) ErrorWithContext(msg string, endpoint string, ctx LogContext) {
	sl.logWithContext("error", msg, endpoint, ctx)
}

// logWithContext is the internal method that handles the dual logging logic
func (sl *StyledLogger) logWithContext(level string, msg string, endpoint string, ctx LogContext) {
	// CLI: clean messaging
	styledMsg := fmt.Sprintf("%s %s", msg, pterm.Style{sl.Theme.Endpoint}.Sprint(endpoint))

	switch level {
	case "info":
		sl.logger.Info(styledMsg, ctx.UserArgs...)
	case "warn":
		sl.logger.Warn(styledMsg, ctx.UserArgs...)
	case "error":
		sl.logger.Error(styledMsg, ctx.UserArgs...)
	}

	// log file: detailed hopefully
	if len(ctx.DetailedArgs) > 0 {
		allArgs := make([]interface{}, 0, len(ctx.UserArgs)+len(ctx.DetailedArgs)+2)
		allArgs = append(allArgs, "endpoint_name", endpoint)
		allArgs = append(allArgs, ctx.UserArgs...)
		allArgs = append(allArgs, ctx.DetailedArgs...)

		detailedCtx := context.WithValue(context.Background(), DefaultDetailedCookie, true)

		switch level {
		case "info":
			sl.logger.InfoContext(detailedCtx, msg, allArgs...)
		case "warn":
			sl.logger.WarnContext(detailedCtx, msg, allArgs...)
		case "error":
			sl.logger.ErrorContext(detailedCtx, msg, allArgs...)
		}
	}
}
