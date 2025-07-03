package logger

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/pterm/pterm"

	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/theme"
)

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
func (sl *PrettyStyledLogger) InfoWithStatus(msg string, status string, args ...any) {
	styledMsg := fmt.Sprintf("[ %s ] %s", sl.Theme.Good.Sprint(status), msg)
	sl.logger.Info(styledMsg, args...)
}

func (sl *PrettyStyledLogger) ResetLine() {
	fmt.Print("\033[1A\033[2K")
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
	styledMsg := fmt.Sprintf("%s %s", msg, sl.Theme.Counts.Sprint("(", count, ")"))
	sl.logger.Info(styledMsg, args...)
}

func (sl *PrettyStyledLogger) InfoWithEndpoint(msg string, endpoint string, args ...any) {
	styledMsg := fmt.Sprintf("%s %s", msg, sl.Theme.Endpoint.Sprint(endpoint))
	sl.logger.Info(styledMsg, args...)
}

func (sl *PrettyStyledLogger) InfoWithHealthCheck(msg string, endpoint string, args ...any) {
	styledMsg := fmt.Sprintf("%s %s", msg, sl.Theme.HealthCheck.Sprint(endpoint))
	sl.logger.Info(styledMsg, args...)
}

func (sl *PrettyStyledLogger) InfoWithNumbers(msg string, numbers ...int64) {
	var formattedNums []string
	for _, num := range numbers {
		formattedNums = append(formattedNums, sl.Theme.Numbers.Sprint(num))
	}

	styledMsg := fmt.Sprintf(msg, toInterfaceSlice(formattedNums)...)
	sl.logger.Info(styledMsg)
}

func (sl *PrettyStyledLogger) WarnWithEndpoint(msg string, endpoint string, args ...any) {
	styledMsg := fmt.Sprintf("%s %s", msg, sl.Theme.Endpoint.Sprint(endpoint))
	sl.logger.Warn(styledMsg, args...)
}

func (sl *PrettyStyledLogger) ErrorWithEndpoint(msg string, endpoint string, args ...any) {
	styledMsg := fmt.Sprintf("%s %s", msg, sl.Theme.Endpoint.Sprint(endpoint))
	sl.logger.Error(styledMsg, args...)
}

func (sl *PrettyStyledLogger) InfoHealthy(msg string, endpoint string, args ...any) {
	styledMsg := fmt.Sprintf("%s %s", msg, sl.Theme.HealthHealthy.Sprint(endpoint))
	sl.logger.Info(styledMsg, args...)
}
func (sl *PrettyStyledLogger) InfoHealthStatus(msg string, name string, status domain.EndpointStatus, args ...any) {
	var statusStyle *pterm.Style
	var statusText string

	switch status {
	case domain.StatusHealthy:
		statusStyle = sl.Theme.HealthHealthy
		statusText = "Healthy"
	case domain.StatusBusy:
		statusStyle = sl.Theme.HealthBusy
		statusText = "Busy"
	case domain.StatusOffline:
		statusStyle = sl.Theme.HealthOffline
		statusText = "Offline"
	case domain.StatusWarming:
		statusStyle = sl.Theme.HealthWarming
		statusText = "Warming"
	case domain.StatusUnhealthy:
		statusStyle = sl.Theme.HealthUnhealthy
		statusText = "Unhealthy"
	case domain.StatusUnknown:
	default:
		statusStyle = sl.Theme.HealthUnknown
		statusText = "Unknown"
	}

	styledMsg := fmt.Sprintf("%s %s is %s",
		msg,
		sl.Theme.Endpoint.Sprint(name), statusStyle.Sprint(statusText))

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
		sl.Theme.Endpoint.Sprint(oldName),
		sl.Theme.Endpoint.Sprint(newName))
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
	sl.logWithContext(LogLevelInfo, msg, endpoint, ctx)
}

func (sl *PrettyStyledLogger) WarnWithContext(msg string, endpoint string, ctx LogContext) {
	sl.logWithContext(LogLevelWarn, msg, endpoint, ctx)
}

func (sl *PrettyStyledLogger) ErrorWithContext(msg string, endpoint string, ctx LogContext) {
	sl.logWithContext(LogLevelError, msg, endpoint, ctx)
}

// logWithContext is the internal method that handles the dual logging logic
func (sl *PrettyStyledLogger) logWithContext(level string, msg string, endpoint string, ctx LogContext) {
	// CLI: clean messaging
	styledMsg := fmt.Sprintf("%s %s", msg, sl.Theme.Endpoint.Sprint(endpoint))

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
