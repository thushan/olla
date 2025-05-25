package theme

import (
	"github.com/charmbracelet/lipgloss"
)

func ColourSplashLipgloss(message string) string {
	style := lipgloss.NewStyle().Foreground(lipgloss.Color("10")) // Light green
	return style.Render(message)
}

func ColourVersionLipgloss(message string) string {
	style := lipgloss.NewStyle().Foreground(lipgloss.Color("11")) // Light yellow
	return style.Render(message)
}

func StyleUrlLipgloss(message string) string {
	style := lipgloss.NewStyle().Foreground(lipgloss.Color("12")) // Light blue
	return style.Render(message)
}
