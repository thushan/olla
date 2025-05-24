// internal/logger/styled.go
package logger

import (
	"fmt"
	"github.com/thushan/olla/internal/core/domain"
	"log/slog"

	"github.com/pterm/pterm"
	"github.com/thushan/olla/theme"
)

// StyledLogger wraps slog.Logger with theme-aware formatting methods
type StyledLogger struct {
	logger *slog.Logger
	theme  *theme.Theme
}

// NewStyledLogger creates a new styled logger with the given theme
func NewStyledLogger(logger *slog.Logger, theme *theme.Theme) *StyledLogger {
	return &StyledLogger{
		logger: logger,
		theme:  theme,
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
	styledMsg := fmt.Sprintf("%s %s", msg, pterm.Style{sl.theme.Counts}.Sprint("(", count, ")"))
	sl.logger.Info(styledMsg, args...)
}

func (sl *StyledLogger) InfoWithEndpoint(msg string, endpoint string, args ...any) {
	styledMsg := fmt.Sprintf("%s %s", msg, pterm.Style{sl.theme.Endpoint}.Sprint(endpoint))
	sl.logger.Info(styledMsg, args...)
}

func (sl *StyledLogger) InfoWithHealthCheck(msg string, endpoint string, args ...any) {
	styledMsg := fmt.Sprintf("%s %s", msg, pterm.Style{sl.theme.HealthCheck}.Sprint(endpoint))
	sl.logger.Info(styledMsg, args...)
}

func (sl *StyledLogger) InfoWithNumbers(msg string, numbers ...int64) {
	var formattedNums []string
	for _, num := range numbers {
		formattedNums = append(formattedNums, pterm.Style{sl.theme.Numbers}.Sprint(num))
	}

	// Build message with styled numbers
	styledMsg := fmt.Sprintf(msg, toInterfaceSlice(formattedNums)...)
	sl.logger.Info(styledMsg)
}

func (sl *StyledLogger) WarnWithEndpoint(msg string, endpoint string, args ...any) {
	styledMsg := fmt.Sprintf("%s %s", msg, pterm.Style{sl.theme.Endpoint}.Sprint(endpoint))
	sl.logger.Warn(styledMsg, args...)
}

func (sl *StyledLogger) ErrorWithEndpoint(msg string, endpoint string, args ...any) {
	styledMsg := fmt.Sprintf("%s %s", msg, pterm.Style{sl.theme.Endpoint}.Sprint(endpoint))
	sl.logger.Error(styledMsg, args...)
}

func (sl *StyledLogger) InfoHealthy(msg string, endpoint string, args ...any) {
	styledMsg := fmt.Sprintf("%s %s", msg, pterm.Style{sl.theme.HealthHealthy}.Sprint(endpoint))
	sl.logger.Info(styledMsg, args...)
}

func (sl *StyledLogger) InfoHealthStatus(msg string, name string, status domain.EndpointStatus, args ...any) {

	var statusColor pterm.Color
	var statusText string

	switch status {
	case domain.StatusHealthy:
		statusColor = sl.theme.HealthHealthy
		statusText = "Healthy"
	case domain.StatusUnhealthy:
		statusColor = sl.theme.HealthUnhealthy
		statusText = "Unhealthy"
	case domain.StatusUnknown:
		statusColor = sl.theme.HealthUnknown
		statusText = "Unknown"
	}
	styledMsg := fmt.Sprintf("%s %s is %s", msg, pterm.Style{sl.theme.Endpoint}.Sprint(name), pterm.Style{statusColor}.Sprint(statusText))
	sl.logger.Info(styledMsg, args...)
}
func (sl *StyledLogger) InfoUnhealthy(msg string, endpoint string, args ...any) {
	styledMsg := fmt.Sprintf("%s %s", msg, pterm.Style{sl.theme.HealthUnhealthy}.Sprint(endpoint))
	sl.logger.Info(styledMsg, args...)
}

func (sl *StyledLogger) WarnUnknownHealth(msg string, endpoint string, args ...any) {
	styledMsg := fmt.Sprintf("%s %s", msg, pterm.Style{sl.theme.HealthUnknown}.Sprint(endpoint))
	sl.logger.Warn(styledMsg, args...)
}

func (sl *StyledLogger) InfoWithHealthStats(msg string, healthy, unhealthy, unknown int, args ...any) {
	// Create styled values for the health stats - more efficient for integers
	healthyStyled := pterm.Style{sl.theme.HealthHealthy}.Sprint(healthy)
	unhealthyStyled := pterm.Style{sl.theme.HealthUnhealthy}.Sprint(unhealthy)
	unknownStyled := pterm.Style{sl.theme.HealthUnknown}.Sprint(unknown)

	// Combine the styled health stats with any additional args
	allArgs := make([]any, 0, len(args)+6)
	allArgs = append(allArgs, args...)
	allArgs = append(allArgs,
		"healthy", healthyStyled,
		"unhealthy", unhealthyStyled,
		"unknown", unknownStyled,
	)

	sl.logger.Info(msg, allArgs...)
}

// GetUnderlying returns the underlying slog.Logger for cases where direct access is needed
func (sl *StyledLogger) GetUnderlying() *slog.Logger {
	return sl.logger
}

// WithAttrs creates a new StyledLogger with additional structured attributes
func (sl *StyledLogger) WithAttrs(attrs ...slog.Attr) *StyledLogger {
	// Convert slog.Attr to key-value pairs
	args := make([]any, 0, len(attrs)*2)
	for _, attr := range attrs {
		args = append(args, attr.Key, attr.Value)
	}

	return &StyledLogger{
		logger: sl.logger.With(args...),
		theme:  sl.theme,
	}
}

// With creates a new StyledLogger with additional key-value pairs
func (sl *StyledLogger) With(args ...any) *StyledLogger {
	return &StyledLogger{
		logger: sl.logger.With(args...),
		theme:  sl.theme,
	}
}

// Helper function to convert string slice to interface slice
func toInterfaceSlice(strs []string) []interface{} {
	result := make([]interface{}, len(strs))
	for i, s := range strs {
		result[i] = s
	}
	return result
}

// NewWithTheme creates both a regular logger and a styled logger
func NewWithTheme(cfg *Config) (*slog.Logger, *StyledLogger, func(), error) {
	logger, cleanup, err := New(cfg)
	if err != nil {
		return nil, nil, nil, err
	}

	appTheme := theme.GetTheme(cfg.Theme)
	styledLogger := NewStyledLogger(logger, appTheme)

	return logger, styledLogger, cleanup, nil
}
