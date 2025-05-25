// internal/logger/styled.go - Converted to use Lipgloss instead of pterm
package logger

import (
	"fmt"
	"log/slog"

	"github.com/charmbracelet/lipgloss"
	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/theme"
)

// StyledLogger wraps slog.Logger with theme-aware formatting methods using Lipgloss
type StyledLogger struct {
	logger *slog.Logger
	theme  *theme.Theme
	styles *ThemeStyles
}

// ThemeStyles holds pre-configured Lipgloss styles based on theme
type ThemeStyles struct {
	Counts      lipgloss.Style
	Numbers     lipgloss.Style
	Endpoint    lipgloss.Style
	HealthCheck lipgloss.Style

	// Health status styles
	HealthHealthy   lipgloss.Style
	HealthBusy      lipgloss.Style
	HealthOffline   lipgloss.Style
	HealthWarming   lipgloss.Style
	HealthUnhealthy lipgloss.Style
	HealthUnknown   lipgloss.Style
}

// NewStyledLogger creates a new styled logger with the given theme
func NewStyledLogger(logger *slog.Logger, theme *theme.Theme) *StyledLogger {
	styles := &ThemeStyles{
		Counts:      theme.CountsStyle(),
		Numbers:     theme.NumbersStyle(),
		Endpoint:    theme.EndpointStyle(),
		HealthCheck: theme.HealthCheckStyle(),

		// Health status styles
		HealthHealthy:   theme.HealthHealthyStyle(),
		HealthBusy:      theme.HealthBusyStyle(),
		HealthOffline:   theme.HealthOfflineStyle(),
		HealthWarming:   theme.HealthWarmingStyle(),
		HealthUnhealthy: theme.HealthUnhealthyStyle(),
		HealthUnknown:   theme.HealthUnknownStyle(),
	}

	return &StyledLogger{
		logger: logger,
		theme:  theme,
		styles: styles,
	}
}

func (sl *StyledLogger) Debug(msg string, args ...any) {
	sl.logger.Debug(msg, args...)
}

func (sl *StyledLogger) Info(msg string, args ...any) {
	// Check if this looks like a multi-line structured log entry
	if len(args) >= 6 && containsEndpointInfo(args) {
		sl.infoWithTree(msg, args...)
	} else {
		sl.logger.Info(msg, args...)
	}
}

// Helper function to detect endpoint info logging
func containsEndpointInfo(args []any) bool {
	// Look for common endpoint info keys
	for i := 0; i < len(args)-1; i += 2 {
		if key, ok := args[i].(string); ok {
			if key == "name" || key == "endpoint" || key == "model_url" || key == "health_check_url" {
				return true
			}
		}
	}
	return false
}

// infoWithTree formats structured data with tree-style box drawing characters
func (sl *StyledLogger) infoWithTree(msg string, args ...any) {
	// Log the main message first
	sl.logger.Info(msg)

	// Then log each key-value pair with tree formatting
	for i := 0; i < len(args)-1; i += 2 {
		if key, ok := args[i].(string); ok {
			value := args[i+1]

			var prefix string
			if i == len(args)-2 { // Last item
				prefix = "└"
			} else {
				prefix = "├"
			}

			// Format with tree characters and colors
			var formattedLine string
			switch key {
			case "name":
				formattedLine = fmt.Sprintf("%s %s: %s", prefix, key, sl.styles.Endpoint.Render(fmt.Sprintf("%v", value)))
			case "endpoint", "model_url", "health_check_url":
				formattedLine = fmt.Sprintf("%s %s: %v", prefix, key, value)
			default:
				formattedLine = fmt.Sprintf("%s %s: %v", prefix, key, value)
			}

			// Log each line directly to avoid extra formatting
			fmt.Println(formattedLine)
		}
	}
}

func (sl *StyledLogger) Warn(msg string, args ...any) {
	sl.logger.Warn(msg, args...)
}

func (sl *StyledLogger) Error(msg string, args ...any) {
	sl.logger.Error(msg, args...)
}

func (sl *StyledLogger) InfoWithCount(msg string, count int, args ...any) {
	styledMsg := fmt.Sprintf("%s %s", msg, sl.styles.Counts.Render(fmt.Sprintf("(%d)", count)))
	sl.logger.Info(styledMsg, args...)
}

func (sl *StyledLogger) InfoWithEndpoint(msg string, endpoint string, args ...any) {
	styledMsg := fmt.Sprintf("%s %s", msg, sl.styles.Endpoint.Render(endpoint))
	sl.logger.Info(styledMsg, args...)
}

func (sl *StyledLogger) InfoWithHealthCheck(msg string, endpoint string, args ...any) {
	styledMsg := fmt.Sprintf("%s %s", msg, sl.styles.HealthCheck.Render(endpoint))
	sl.logger.Info(styledMsg, args...)
}

func (sl *StyledLogger) InfoWithNumbers(msg string, numbers ...int64) {
	var formattedNums []string
	for _, num := range numbers {
		formattedNums = append(formattedNums, sl.styles.Numbers.Render(fmt.Sprintf("%d", num)))
	}

	// Build message with styled numbers
	styledMsg := fmt.Sprintf(msg, toInterfaceSlice(formattedNums)...)
	sl.logger.Info(styledMsg)
}

func (sl *StyledLogger) WarnWithEndpoint(msg string, endpoint string, args ...any) {
	styledMsg := fmt.Sprintf("%s %s", msg, sl.styles.Endpoint.Render(endpoint))
	sl.logger.Warn(styledMsg, args...)
}

func (sl *StyledLogger) ErrorWithEndpoint(msg string, endpoint string, args ...any) {
	styledMsg := fmt.Sprintf("%s %s", msg, sl.styles.Endpoint.Render(endpoint))
	sl.logger.Error(styledMsg, args...)
}

func (sl *StyledLogger) InfoHealthy(msg string, endpoint string, args ...any) {
	styledMsg := fmt.Sprintf("%s %s", msg, sl.styles.HealthHealthy.Render(endpoint))
	sl.logger.Info(styledMsg, args...)
}

func (sl *StyledLogger) InfoHealthStatus(msg string, name string, status domain.EndpointStatus, args ...any) {
	var statusStyle lipgloss.Style
	var statusText string

	switch status {
	case domain.StatusHealthy:
		statusStyle = sl.styles.HealthHealthy
		statusText = "Healthy"
	case domain.StatusBusy:
		statusStyle = sl.styles.HealthBusy
		statusText = "Busy"
	case domain.StatusOffline:
		statusStyle = sl.styles.HealthOffline
		statusText = "Offline"
	case domain.StatusWarming:
		statusStyle = sl.styles.HealthWarming
		statusText = "Warming"
	case domain.StatusUnhealthy:
		statusStyle = sl.styles.HealthUnhealthy
		statusText = "Unhealthy"
	case domain.StatusUnknown:
		statusStyle = sl.styles.HealthUnknown
		statusText = "Unknown"
	}

	styledMsg := fmt.Sprintf("%s %s is %s",
		msg,
		sl.styles.Endpoint.Render(name),
		statusStyle.Render(statusText))
	sl.logger.Info(styledMsg, args...)
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
		styles: sl.styles,
	}
}

// With creates a new StyledLogger with additional key-value pairs
func (sl *StyledLogger) With(args ...any) *StyledLogger {
	return &StyledLogger{
		logger: sl.logger.With(args...),
		theme:  sl.theme,
		styles: sl.styles,
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
