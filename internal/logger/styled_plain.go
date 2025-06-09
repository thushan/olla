package logger

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/thushan/olla/internal/core/domain"
)

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
	case LogLevelInfo:
		sl.logger.Info(styledMsg, ctx.UserArgs...)
	case LogLevelWarn:
		sl.logger.Warn(styledMsg, ctx.UserArgs...)
	case LogLevelError:
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
		case LogLevelInfo:
			sl.logger.InfoContext(detailedCtx, msg, allArgs...)
		case LogLevelWarn:
			sl.logger.WarnContext(detailedCtx, msg, allArgs...)
		case LogLevelError:
			sl.logger.ErrorContext(detailedCtx, msg, allArgs...)
		}
	}
}
