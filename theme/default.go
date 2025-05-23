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

	// Functional colours
	Primary   pterm.Color
	Secondary pterm.Color
	Danger    pterm.Color
	Warning   pterm.Color
	Good      pterm.Color

	// Application colours
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

		// Colour palette
		Primary:   pterm.FgBlue,
		Secondary: pterm.FgCyan,
		Danger:    pterm.FgRed,
		Warning:   pterm.FgYellow,
		Good:      pterm.FgGreen,
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

		Primary:   pterm.FgLightBlue,
		Secondary: pterm.FgLightCyan,
		Danger:    pterm.FgLightRed,
		Warning:   pterm.FgLightYellow,
		Good:      pterm.FgLightGreen,
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

		Primary:   pterm.FgBlue,
		Secondary: pterm.FgCyan,
		Danger:    pterm.FgRed,
		Warning:   pterm.FgRed,
		Good:      pterm.FgGreen,
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

// StyleUrl Colours for URLs and hyperlinks
func StyleUrl(message ...any) string {
	return pterm.LightBlue(message...)
}

// Hyperlink creates a hyperlink in the terminal
func Hyperlink(uri string, text string) string {
	return "\x1b]8;;" + uri + "\x07" + text + "\x1b]8;;\x07" + "\u001b[0m"
}
