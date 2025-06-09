package logger

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/thushan/olla/internal/core/domain"

	"github.com/pterm/pterm"
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

// PrettyStyledLogger implements StyledLogger with pterm formatting
type PrettyStyledLogger struct {
	logger *slog.Logger
	Theme  *theme.Theme
}

func NewPrettyStyledLogger(logger *slog.Logger, theme *theme.Theme) *PrettyStyledLogger {
	return &PrettyStyledLogger{
		logger: logger,
		Theme:  theme,
	}
}

func (sl *PrettyStyledLogger) Debug(msg string, args ...any) {
	sl.logger.Debug(msg, args...)
}

func (sl *PrettyStyledLogger) Info(msg string, args ...any) {
	sl.logger.Info(msg, args...)
}

func (sl *PrettyStyledLogger) Warn(msg string, args ...any) {
	sl.logger.Warn(msg, args...)
}

func (sl *PrettyStyledLogger) Error(msg string, args ...any) {
	sl.logger.Error(msg, args...)
}

func (sl *PrettyStyledLogger) InfoWithCount(msg string, count int, args ...any) {
	styledMsg := fmt.Sprintf("%s %s", msg, pterm.Style{sl.Theme.Counts}.Sprint("(", count, ")"))
	sl.logger.Info(styledMsg, args...)
}

func (sl *PrettyStyledLogger) InfoWithEndpoint(msg string, endpoint string, args ...any) {
	styledMsg := fmt.Sprintf("%s %s", msg, pterm.Style{sl.Theme.Endpoint}.Sprint(endpoint))
	sl.logger.Info(styledMsg, args...)
}

func (sl *PrettyStyledLogger) InfoWithHealthCheck(msg string, endpoint string, args ...any) {
	styledMsg := fmt.Sprintf("%s %s", msg, pterm.Style{sl.Theme.HealthCheck}.Sprint(endpoint))
	sl.logger.Info(styledMsg, args...)
}

func (sl *PrettyStyledLogger) InfoWithNumbers(msg string, numbers ...int64) {
	var formattedNums []string
	for _, num := range numbers {
		formattedNums = append(formattedNums, pterm.Style{sl.Theme.Numbers}.Sprint(num))
	}

	styledMsg := fmt.Sprintf(msg, toInterfaceSlice(formattedNums)...)
	sl.logger.Info(styledMsg)
}

func (sl *PrettyStyledLogger) WarnWithEndpoint(msg string, endpoint string, args ...any) {
	styledMsg := fmt.Sprintf("%s %s", msg, pterm.Style{sl.Theme.Endpoint}.Sprint(endpoint))
	sl.logger.Warn(styledMsg, args...)
}

func (sl *PrettyStyledLogger) ErrorWithEndpoint(msg string, endpoint string, args ...any) {
	styledMsg := fmt.Sprintf("%s %s", msg, pterm.Style{sl.Theme.Endpoint}.Sprint(endpoint))
	sl.logger.Error(styledMsg, args...)
}

func (sl *PrettyStyledLogger) InfoHealthy(msg string, endpoint string, args ...any) {
	styledMsg := fmt.Sprintf("%s %s", msg, pterm.Style{sl.Theme.HealthHealthy}.Sprint(endpoint))
	sl.logger.Info(styledMsg, args...)
}

func (sl *PrettyStyledLogger) InfoHealthStatus(msg string, name string, status domain.EndpointStatus, args ...any) {
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

func (sl *PrettyStyledLogger) GetUnderlying() *slog.Logger {
	return sl.logger
}

func (sl *PrettyStyledLogger) WithRequestID(requestID string) StyledLogger {
	return sl.With("request_id", requestID)
}

func (sl *PrettyStyledLogger) InfoConfigChange(oldName, newName string) {
	styledMsg := fmt.Sprintf("Endpoint configuration changed for %s to: %s",
		pterm.Style{sl.Theme.Endpoint}.Sprint(oldName),
		pterm.Style{sl.Theme.Endpoint}.Sprint(newName))
	sl.logger.Info(styledMsg)
}

func (sl *PrettyStyledLogger) WithAttrs(attrs ...slog.Attr) StyledLogger {
	args := make([]any, 0, len(attrs)*2)
	for _, attr := range attrs {
		args = append(args, attr.Key, attr.Value)
	}

	return &PrettyStyledLogger{
		logger: sl.logger.With(args...),
		Theme:  sl.Theme,
	}
}

func (sl *PrettyStyledLogger) With(args ...any) StyledLogger {
	return &PrettyStyledLogger{
		logger: sl.logger.With(args...),
		Theme:  sl.Theme,
	}
}

func (sl *PrettyStyledLogger) InfoWithContext(msg string, endpoint string, ctx LogContext) {
	sl.logWithContext("info", msg, endpoint, ctx)
}

func (sl *PrettyStyledLogger) WarnWithContext(msg string, endpoint string, ctx LogContext) {
	sl.logWithContext("warn", msg, endpoint, ctx)
}

func (sl *PrettyStyledLogger) ErrorWithContext(msg string, endpoint string, ctx LogContext) {
	sl.logWithContext("error", msg, endpoint, ctx)
}

// logWithContext is the internal method that handles the dual logging logic
func (sl *PrettyStyledLogger) logWithContext(level string, msg string, endpoint string, ctx LogContext) {
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

// PlainStyledLogger implements StyledLogger without formatting
type PlainStyledLogger struct {
	logger *slog.Logger
}

func NewPlainStyledLogger(logger *slog.Logger) *PlainStyledLogger {
	return &PlainStyledLogger{
		logger: logger,
	}
}

func (sl *PlainStyledLogger) Debug(msg string, args ...any) {
	sl.logger.Debug(msg, args...)
}

func (sl *PlainStyledLogger) Info(msg string, args ...any) {
	sl.logger.Info(msg, args...)
}

func (sl *PlainStyledLogger) Warn(msg string, args ...any) {
	sl.logger.Warn(msg, args...)
}

func (sl *PlainStyledLogger) Error(msg string, args ...any) {
	sl.logger.Error(msg, args...)
}

func (sl *PlainStyledLogger) InfoWithCount(msg string, count int, args ...any) {
	styledMsg := fmt.Sprintf("%s (%d)", msg, count)
	sl.logger.Info(styledMsg, args...)
}

func (sl *PlainStyledLogger) InfoWithEndpoint(msg string, endpoint string, args ...any) {
	styledMsg := fmt.Sprintf("%s %s", msg, endpoint)
	sl.logger.Info(styledMsg, args...)
}

func (sl *PlainStyledLogger) InfoWithHealthCheck(msg string, endpoint string, args ...any) {
	styledMsg := fmt.Sprintf("%s %s", msg, endpoint)
	sl.logger.Info(styledMsg, args...)
}

func (sl *PlainStyledLogger) InfoWithNumbers(msg string, numbers ...int64) {
	var formattedNums []string
	for _, num := range numbers {
		formattedNums = append(formattedNums, fmt.Sprintf("%d", num))
	}

	styledMsg := fmt.Sprintf(msg, toInterfaceSlice(formattedNums)...)
	sl.logger.Info(styledMsg)
}

func (sl *PlainStyledLogger) WarnWithEndpoint(msg string, endpoint string, args ...any) {
	styledMsg := fmt.Sprintf("%s %s", msg, endpoint)
	sl.logger.Warn(styledMsg, args...)
}

func (sl *PlainStyledLogger) ErrorWithEndpoint(msg string, endpoint string, args ...any) {
	styledMsg := fmt.Sprintf("%s %s", msg, endpoint)
	sl.logger.Error(styledMsg, args...)
}

func (sl *PlainStyledLogger) InfoHealthy(msg string, endpoint string, args ...any) {
	styledMsg := fmt.Sprintf("%s %s", msg, endpoint)
	sl.logger.Info(styledMsg, args...)
}

func (sl *PlainStyledLogger) InfoHealthStatus(msg string, name string, status domain.EndpointStatus, args ...any) {
	var statusText string

	switch status {
	case domain.StatusHealthy:
		statusText = "Healthy"
	case domain.StatusBusy:
		statusText = "Busy"
	case domain.StatusOffline:
		statusText = "Offline"
	case domain.StatusWarming:
		statusText = "Warming"
	case domain.StatusUnhealthy:
		statusText = "Unhealthy"
	case domain.StatusUnknown:
		statusText = "Unknown"
	}
	styledMsg := fmt.Sprintf("%s %s is %s", msg, name, statusText)
	sl.logger.Info(styledMsg, args...)
}

func (sl *PlainStyledLogger) GetUnderlying() *slog.Logger {
	return sl.logger
}

func (sl *PlainStyledLogger) WithRequestID(requestID string) StyledLogger {
	return sl.With("request_id", requestID)
}

func (sl *PlainStyledLogger) InfoConfigChange(oldName, newName string) {
	styledMsg := fmt.Sprintf("Endpoint configuration changed for %s to: %s", oldName, newName)
	sl.logger.Info(styledMsg)
}

func (sl *PlainStyledLogger) WithAttrs(attrs ...slog.Attr) StyledLogger {
	args := make([]any, 0, len(attrs)*2)
	for _, attr := range attrs {
		args = append(args, attr.Key, attr.Value)
	}

	return &PlainStyledLogger{
		logger: sl.logger.With(args...),
	}
}

func (sl *PlainStyledLogger) With(args ...any) StyledLogger {
	return &PlainStyledLogger{
		logger: sl.logger.With(args...),
	}
}

func (sl *PlainStyledLogger) InfoWithContext(msg string, endpoint string, ctx LogContext) {
	sl.logWithContext("info", msg, endpoint, ctx)
}

func (sl *PlainStyledLogger) WarnWithContext(msg string, endpoint string, ctx LogContext) {
	sl.logWithContext("warn", msg, endpoint, ctx)
}

func (sl *PlainStyledLogger) ErrorWithContext(msg string, endpoint string, ctx LogContext) {
	sl.logWithContext("error", msg, endpoint, ctx)
}

// logWithContext is the internal method that handles the dual logging logic
func (sl *PlainStyledLogger) logWithContext(level string, msg string, endpoint string, ctx LogContext) {
	// CLI: clean messaging
	styledMsg := fmt.Sprintf("%s %s", msg, endpoint)

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

// Factory function to create the appropriate styled logger
func NewStyledLogger(logger *slog.Logger, theme *theme.Theme, prettyMode bool) StyledLogger {
	if prettyMode {
		return NewPrettyStyledLogger(logger, theme)
	}
	return NewPlainStyledLogger(logger)
}

func toInterfaceSlice(strs []string) []interface{} {
	result := make([]interface{}, len(strs))
	for i, s := range strs {
		result[i] = s
	}
	return result
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
