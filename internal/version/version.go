package version

import (
	"fmt"
	"github.com/thushan/olla/theme"
	"log"
	"strings"
)

var (
	Name        = "olla"
	Authors     = "Thushan Fernando"
	Description = "Your Ollama Proxy Sherpa"
	Version     = "v0.0.1"
	Commit      = "none"
	Date        = "nowish"
	User        = "local"
)

const (
	GithubHomeText  = "github.com/thushan/olla"
	GithubHomeUri   = "https://github.com/thushan/olla"
	GithubLatestUri = "https://github.com/thushan/olla/releases/latest"
)

func PrintVersionInfo(extendedInfo bool, vlog *log.Logger) {
	githubUri := theme.Hyperlink(GithubHomeUri, GithubHomeText)
	latestUri := theme.Hyperlink(GithubLatestUri, Version)
	padLatest := fmt.Sprintf("%*s", 1-len(Version), "")
	padBuffer := fmt.Sprintf("%*s", 2, "")

	var b strings.Builder

	b.WriteString(theme.ColourSplash(`
╔────────────────────────────────────────────────────────╗
│                                      ⠀⠀⣀⣀⠀⠀⠀⠀⠀⣀⣀⠀⠀     │
│                                      ⠀⢰⡏⢹⡆⠀⠀⠀⢰⡏⢹⡆⡀     │ 
│   ██████╗ ██╗     ██╗      █████╗    ⠀⢸⡇⣸⡷⠟⠛⠻⢾⣇⣸⡇      │
│  ██╔═══██╗██║     ██║     ██╔══██╗   ⢠⡾⠛⠉⠁⠀⠀⠀⠈⠉⠛⢷⡄     │
│  ██║   ██║██║     ██║     ███████║   ⣿⠀⢀⣄⢀⣠⣤⣄⡀⣠⡀⠀⣿     │
│  ██║   ██║██║     ██║     ██╔══██║   ⢻⣄⠘⠋⡞⠉⢤⠉⢳⠙⠃⢠⡿⡀    │
│  ╚██████╔╝███████╗███████╗██║  ██║   ⣼⠃⠀⠀⠳⠤⠬⠤⠞⠀⠀⠘⣷     │
│                                      ⢸⡟⠀⠀⠀⠀⠀⠀⠀⠀⠀⢸⡇     │` + "\n"))

	b.WriteString(theme.ColourSplash("│ "))
	b.WriteString(theme.StyleUrl(githubUri))
	b.WriteString(padLatest)
	b.WriteString(theme.ColourVersion(latestUri))
	b.WriteString(padBuffer)
	b.WriteString(theme.ColourSplash(" ⢸⡅⠀⠀⠀⠀⠀⠀⠀⠀⠀⢀⡿     │\n"))
	b.WriteString(theme.ColourSplash("╚────────────────────────────────────────────────────────╝"))

	if extendedInfo {
		b.WriteString("\n")
		b.WriteString(fmt.Sprintf(" Commit: %s\n", Commit))
		b.WriteString(fmt.Sprintf("  Built: %s\n", Date))
		b.WriteString(fmt.Sprintf("  Using: %s\n", User))
	}

	vlog.Println(b.String())
}
