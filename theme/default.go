package theme

import (
	"github.com/pterm/pterm"
)

// Theme defines the colour scheme and styling for the application
type Theme struct {
	// Log level colours
	Debug *pterm.Style
	Info  *pterm.Style
	Warn  *pterm.Style
	Error *pterm.Style
	Fatal *pterm.Style

	// Component colours
	Success   *pterm.Style
	Highlight *pterm.Style
	Muted     *pterm.Style
	Accent    *pterm.Style

	// Functional colours (now pre-built styles)
	Primary   *pterm.Style
	Secondary *pterm.Style
	Danger    *pterm.Style
	Warning   *pterm.Style
	Good      *pterm.Style

	// Application colours (now pre-built styles)
	Counts      *pterm.Style // Colour for counts like Microsofty apps Eg. Outlook (234) unread emails
	Numbers     *pterm.Style
	Endpoint    *pterm.Style
	HealthCheck *pterm.Style

	// Health check colours (now pre-built styles)
	HealthHealthy   *pterm.Style
	HealthUnhealthy *pterm.Style
	HealthUnknown   *pterm.Style
	HealthBusy      *pterm.Style
	HealthOffline   *pterm.Style
	HealthWarming   *pterm.Style
}

// Default returns the default application theme
func Default() *Theme {
	return &Theme{
		// Log level styling
		Debug: pterm.NewStyle(pterm.FgLightBlue),
		Info:  pterm.NewStyle(pterm.FgGreen),
		Warn:  pterm.NewStyle(pterm.FgYellow, pterm.Bold),
		Error: pterm.NewStyle(pterm.FgRed, pterm.Bold),
		Fatal: pterm.NewStyle(pterm.FgWhite, pterm.BgRed, pterm.Bold),

		// Component styling
		Success:   pterm.NewStyle(pterm.FgGreen, pterm.Bold),
		Highlight: pterm.NewStyle(pterm.FgCyan, pterm.Bold),
		Muted:     pterm.NewStyle(pterm.FgGray),
		Accent:    pterm.NewStyle(pterm.FgMagenta),

		// Functional styles (converted from Color to Style)
		Primary:   pterm.NewStyle(pterm.FgBlue),
		Secondary: pterm.NewStyle(pterm.FgCyan),
		Danger:    pterm.NewStyle(pterm.FgRed),
		Warning:   pterm.NewStyle(pterm.FgYellow),
		Good:      pterm.NewStyle(pterm.FgGreen),

		// Application styles (converted from Color to Style)
		Counts:      pterm.NewStyle(pterm.FgLightBlue),
		Numbers:     pterm.NewStyle(pterm.FgLightCyan),
		Endpoint:    pterm.NewStyle(pterm.FgLightMagenta),
		HealthCheck: pterm.NewStyle(pterm.FgGreen),

		// Health check styles (converted from Color to Style)
		HealthHealthy:   pterm.NewStyle(pterm.FgGreen),
		HealthUnhealthy: pterm.NewStyle(pterm.FgRed),
		HealthUnknown:   pterm.NewStyle(pterm.FgGray),
		HealthBusy:      pterm.NewStyle(pterm.FgYellow),
		HealthOffline:   pterm.NewStyle(pterm.FgLightRed),
		HealthWarming:   pterm.NewStyle(pterm.FgLightBlue),
	}
}

// Dark returns a dark theme variant
func Dark() *Theme {
	return &Theme{
		Debug: pterm.NewStyle(pterm.FgLightBlue),
		Info:  pterm.NewStyle(pterm.FgLightGreen),
		Warn:  pterm.NewStyle(pterm.FgLightYellow, pterm.Bold),
		Error: pterm.NewStyle(pterm.FgLightRed, pterm.Bold),
		Fatal: pterm.NewStyle(pterm.FgWhite, pterm.BgRed, pterm.Bold),

		Success:   pterm.NewStyle(pterm.FgLightGreen, pterm.Bold),
		Highlight: pterm.NewStyle(pterm.FgLightCyan, pterm.Bold),
		Muted:     pterm.NewStyle(pterm.FgGray),
		Accent:    pterm.NewStyle(pterm.FgLightMagenta),

		// Functional styles for dark theme
		Primary:   pterm.NewStyle(pterm.FgLightBlue),
		Secondary: pterm.NewStyle(pterm.FgLightCyan),
		Danger:    pterm.NewStyle(pterm.FgLightRed),
		Warning:   pterm.NewStyle(pterm.FgLightYellow),
		Good:      pterm.NewStyle(pterm.FgLightGreen),

		// Application styles for dark theme
		Counts:      pterm.NewStyle(pterm.FgLightBlue),
		Numbers:     pterm.NewStyle(pterm.FgLightCyan),
		Endpoint:    pterm.NewStyle(pterm.FgLightMagenta),
		HealthCheck: pterm.NewStyle(pterm.FgLightGreen),

		// Health styles for dark theme
		HealthHealthy:   pterm.NewStyle(pterm.FgLightGreen),
		HealthUnhealthy: pterm.NewStyle(pterm.FgLightRed),
		HealthUnknown:   pterm.NewStyle(pterm.FgGray),
		HealthBusy:      pterm.NewStyle(pterm.FgLightYellow),
		HealthOffline:   pterm.NewStyle(pterm.FgRed),
		HealthWarming:   pterm.NewStyle(pterm.FgLightCyan),
	}
}

// Light returns a light theme variant
func Light() *Theme {
	return &Theme{
		Debug: pterm.NewStyle(pterm.FgBlue),
		Info:  pterm.NewStyle(pterm.FgBlack),
		Warn:  pterm.NewStyle(pterm.FgRed, pterm.Bold),
		Error: pterm.NewStyle(pterm.FgRed, pterm.Bold),
		Fatal: pterm.NewStyle(pterm.FgWhite, pterm.BgRed, pterm.Bold),

		Success:   pterm.NewStyle(pterm.FgGreen, pterm.Bold),
		Highlight: pterm.NewStyle(pterm.FgBlue, pterm.Bold),
		Muted:     pterm.NewStyle(pterm.FgGray),
		Accent:    pterm.NewStyle(pterm.FgMagenta),

		// Functional styles for light theme
		Primary:   pterm.NewStyle(pterm.FgBlue),
		Secondary: pterm.NewStyle(pterm.FgCyan),
		Danger:    pterm.NewStyle(pterm.FgRed),
		Warning:   pterm.NewStyle(pterm.FgRed),
		Good:      pterm.NewStyle(pterm.FgGreen),

		// Application styles for light theme
		Counts:      pterm.NewStyle(pterm.FgBlue),
		Numbers:     pterm.NewStyle(pterm.FgCyan),
		Endpoint:    pterm.NewStyle(pterm.FgMagenta),
		HealthCheck: pterm.NewStyle(pterm.FgGreen),

		// Health styles for light theme
		HealthHealthy:   pterm.NewStyle(pterm.FgGreen),
		HealthUnhealthy: pterm.NewStyle(pterm.FgRed),
		HealthUnknown:   pterm.NewStyle(pterm.FgGray),
		HealthBusy:      pterm.NewStyle(pterm.FgRed), // More visible on light backgrounds
		HealthOffline:   pterm.NewStyle(pterm.FgRed),
		HealthWarming:   pterm.NewStyle(pterm.FgBlue),
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

// ColourSplash Colours for the splash screen
func ColourSplash(message ...any) string {
	return pterm.LightGreen(message...)
}

// ColourVersion Colours Version numbers, used for the splash screen
func ColourVersion(message ...any) string {
	return pterm.LightYellow(message...)
}
func ColourProfiler(message ...any) string {
	return pterm.LightMagenta(message...)
}

// StyleUrl Colours for URLs and hyperlinks
func StyleUrl(message ...any) string {
	return pterm.LightBlue(message...)
}

// Hyperlink creates a hyperlink in the terminal
func Hyperlink(uri string, text string) string {
	return "\x1b]8;;" + uri + "\x07" + text + "\x1b]8;;\x07" + "\u001b[0m"
}
