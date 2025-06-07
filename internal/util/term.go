package util

import (
	"github.com/mattn/go-isatty"
	"os"
	"strings"
)

/*
   references:
   - https://no-color.org/
   - https://github.com/sitkevij/no_color
*/

// IsTerminal checks if stdout is a terminal using go-isatty
func IsTerminal() bool {
	return isatty.IsTerminal(os.Stdout.Fd())
}

// ShouldUseColors determines if coloured output should be used
func ShouldUseColors() bool {
	if noColor := os.Getenv("NO_COLOR"); noColor != "" {
		return false
	}

	if forceColor := os.Getenv("FORCE_COLOR"); forceColor != "" {
		return forceColor != "0"
	}

	if ollaColors := os.Getenv("OLLA_FORCE_COLORS"); ollaColors != "" {
		return strings.ToLower(ollaColors) == "true"
	}

	return IsTerminal()
}
