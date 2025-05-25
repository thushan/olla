package theme

import (
	"fmt"
	"github.com/charmbracelet/lipgloss"
)

// Theme defines the colour scheme and styling for the application
type Theme struct {
	// Application colours (now stored as lipgloss colors)
	Counts      string // Color for counts like Microsoft apps Eg. Outlook (234) unread emails
	Numbers     string
	Endpoint    string
	HealthCheck string

	// Health check colours
	HealthHealthy   string
	HealthUnhealthy string
	HealthUnknown   string
	HealthBusy      string
	HealthOffline   string
	HealthWarming   string
}

// Default returns the default application theme
func Default() *Theme {
	return &Theme{
		// Application colours (using ANSI color codes)
		Counts:      "12", // Light blue
		Numbers:     "14", // Light cyan
		Endpoint:    "13", // Light magenta
		HealthCheck: "10", // Green

		// Health check colours
		HealthHealthy:   "10", // Green
		HealthUnhealthy: "9",  // Light red
		HealthUnknown:   "8",  // Gray
		HealthBusy:      "11", // Yellow
		HealthOffline:   "1",  // Red
		HealthWarming:   "12", // Light blue
	}
}

// Dark returns a dark theme variant
func Dark() *Theme {
	return &Theme{
		Counts:      "12", // Light blue
		Numbers:     "14", // Light cyan
		Endpoint:    "13", // Light magenta
		HealthCheck: "10", // Light green

		// Health colours for dark theme
		HealthHealthy:   "10", // Light green
		HealthUnhealthy: "9",  // Light red
		HealthUnknown:   "8",  // Gray
		HealthBusy:      "11", // Light yellow
		HealthOffline:   "1",  // Red
		HealthWarming:   "14", // Light cyan
	}
}

// Light returns a light theme variant
func Light() *Theme {
	return &Theme{
		Counts:      "4", // Blue
		Numbers:     "6", // Cyan
		Endpoint:    "5", // Magenta
		HealthCheck: "2", // Green

		// Health colours for light theme
		HealthHealthy:   "2", // Green
		HealthUnhealthy: "1", // Red
		HealthUnknown:   "8", // Gray
		HealthBusy:      "1", // Red (more visible on light backgrounds)
		HealthOffline:   "1", // Red
		HealthWarming:   "4", // Blue
	}
}

// GetTheme returns the appropriate theme based on environment or preference
func GetTheme(name string) *Theme {
	switch name {
	case "dark":
		return Dark()
	case "light":
		return Light()
	default:
		return Default()
	}
}

// Theme extension methods for Lipgloss styles
func (t *Theme) CountsStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(lipgloss.Color(t.Counts))
}

func (t *Theme) NumbersStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(lipgloss.Color(t.Numbers))
}

func (t *Theme) EndpointStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(lipgloss.Color(t.Endpoint))
}

func (t *Theme) HealthCheckStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(lipgloss.Color(t.HealthCheck))
}

// Health status styles
func (t *Theme) HealthHealthyStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(lipgloss.Color(t.HealthHealthy))
}

func (t *Theme) HealthBusyStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(lipgloss.Color(t.HealthBusy))
}

func (t *Theme) HealthOfflineStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(lipgloss.Color(t.HealthOffline))
}

func (t *Theme) HealthWarmingStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(lipgloss.Color(t.HealthWarming))
}

func (t *Theme) HealthUnhealthyStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(lipgloss.Color(t.HealthUnhealthy))
}

func (t *Theme) HealthUnknownStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(lipgloss.Color(t.HealthUnknown))
}

// Styling functions for splash screen and URLs
var (
	splashStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("10")) // Light green
	versionStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("11")) // Light yellow
	urlStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("12")) // Light blue
)

// ColourSplash colours for the splash screen
func ColourSplash(message ...any) string {
	var result string
	for _, msg := range message {
		result += splashStyle.Render(fmt.Sprintf("%v", msg))
	}
	return result
}

// ColourVersion colours Version numbers, used for the splash screen
func ColourVersion(message ...any) string {
	var result string
	for _, msg := range message {
		result += versionStyle.Render(fmt.Sprintf("%v", msg))
	}
	return result
}

// StyleUrl colours for URLs and hyperlinks
func StyleUrl(message ...any) string {
	var result string
	for _, msg := range message {
		result += urlStyle.Render(fmt.Sprintf("%v", msg))
	}
	return result
}

// Hyperlink creates a hyperlink in the terminal
func Hyperlink(uri string, text string) string {
	return "\x1b]8;;" + uri + "\x07" + text + "\x1b]8;;\x07" + "\u001b[0m"
}
