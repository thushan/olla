package version

import (
	"fmt"
	"log"
	"runtime"
	"strings"

	"github.com/thushan/olla/internal/util"
	"github.com/thushan/olla/theme"
)

var (
	Name         = "Olla"
	ShortName    = "olla"
	Edition      = "Community"
	Authors      = "Thushan Fernando"
	Description  = "The AI Proxy for your LLMs"
	Version      = "v0.0.x"
	Commit       = "none"
	Date         = "nowish"
	User         = "local"
	Tool         = "make"
	Runtime      = "Go 1.2x.0"
	Capabilities = []string{
		"load_balancing",
		"health_checking",
		"rate_limiting",
		"model_unification",
		"endpoint_discovery",
	}
	SupportedBackends = []string{
		"ollama",
		"lemonade",
		"llamacpp",
		"lm_studio",
		"sglang",
		"vllm",
		"openai_compatible",
	}
)

const (
	GithubHomeText  = "github.com/thushan/olla"
	GithubHomeUri   = "https://github.com/thushan/olla"
	GithubLatestUri = "https://github.com/thushan/olla/releases/latest"
	BoxWidth        = 70
)

func PrintVersionInfo(extendedInfo bool, vlog *log.Logger) {

	var b strings.Builder

	if util.ShouldUseColors() {
		b.WriteString(formatAsciiBanner())
	} else {
		b.WriteString(formatPlainBanner())
	}

	if extendedInfo {
		b.WriteString("\n")
		b.WriteString(fmt.Sprintf(" Commit: %s\n", Commit))
		b.WriteString(fmt.Sprintf("  Built: %s\n", Date))
		b.WriteString(fmt.Sprintf("  Using: %s\n", Tool))
		b.WriteString(fmt.Sprintf("   With: %s\n", Runtime))
		b.WriteString(fmt.Sprintf("    For: %s\n", runtime.GOOS))
		b.WriteString(fmt.Sprintf("     On: %s\n", runtime.GOARCH))
		b.WriteString(fmt.Sprintf("     By: %s\n", User))
	}

	vlog.Println(b.String())
}
func formatAsciiBanner() string {
	var b strings.Builder

	// When we use GoReleaser, for internal builds, we get:
	// Version = "v0.0.6-d4f9eb5eb"
	// For releases, we get:
	// Version = "v0.0.6"
	// so for internal builds, we'll show a truncated commit hash
	availableSpace := 33
	version := Version

	githubUri := theme.Hyperlink(GithubHomeUri, GithubHomeText)
	latestUri := theme.Hyperlink(GithubLatestUri, version)
	llamaArt := "   ⢸⡅⠀⠀⠀⠀⠀⠀⠀⠀⠀⢀⡿  │"

	// Calculate the content lengths
	githubTextLen := len(GithubHomeText)
	versionLen := len(version)

	bufferSpace := availableSpace - githubTextLen - versionLen

	b.WriteString(theme.ColourSplash(`╔─────────────────────────────────────────────────────╗
│                                      ⠀⠀⣀⣀⠀⠀⠀⠀⠀⣀⣀⠀⠀  │
│                                      ⠀⢰⡏⢹⡆⠀⠀⠀⢰⡏⢹⡆⡀  │ 
│   ██████╗ ██╗     ██╗      █████╗    ⠀⢸⡇⣸⡷⠟⠛⠻⢾⣇⣸⡇   │
│  ██╔═══██╗██║     ██║     ██╔══██╗   ⢠⡾⠛⠉⠁⠀⠀⠀⠈⠉⠛⢷⡄  │
│  ██║   ██║██║     ██║     ███████║   ⣿⠀⢀⣄⢀⣠⣤⣄⡀⣠⡀⠀⣿  │
│  ██║   ██║██║     ██║     ██╔══██║   ⢻⣄⠘⠋⡞⠉⢤⠉⢳⠙⠃⢠⡿⡀ │
│  ╚██████╔╝███████╗███████╗██║  ██║   ⣼⠃⠀⠀⠳⠤⠬⠤⠞⠀⠀⠘⣷  │
│                                      ⢸⡟⠀⠀⠀⠀⠀⠀⠀⠀⠀⢸⡇  │` + "\n"))

	b.WriteString(theme.ColourSplash("│  "))
	b.WriteString(theme.StyleUrl(githubUri))
	b.WriteString(strings.Repeat(" ", bufferSpace)) // Add dynamic spacing between GitHub URL and version
	b.WriteString(theme.ColourVersion(latestUri))
	b.WriteString(theme.ColourSplash(llamaArt + "\n"))
	b.WriteString(theme.ColourSplash("╚─────────────────────────────────────────────────────╝"))

	return b.String()
}
func formatPlainBanner() string {
	var b strings.Builder

	t := centerLine(fmt.Sprintf("%s %s", Name, Version))
	g := centerLine(fmt.Sprintf("%s", GithubHomeText))

	b.WriteString("┌" + strings.Repeat("─", BoxWidth-2) + "┐\n")

	b.WriteString(t)
	b.WriteString(g)

	b.WriteString("└" + strings.Repeat("─", BoxWidth-2) + "┘")

	return b.String()
}

func centerLine(text string) string {
	if text == "" {
		return "│" + strings.Repeat(" ", BoxWidth-2) + "│\n"
	}

	textLen := len(text)
	if textLen >= BoxWidth-4 { // Leave 4 chars for borders and min padding
		text = text[:BoxWidth-7] + "..."
		textLen = BoxWidth - 4
	}

	totalPadding := BoxWidth - 2 - textLen
	leftPad := totalPadding / 2
	rightPad := totalPadding - leftPad

	return fmt.Sprintf("│%s%s%s│\n",
		strings.Repeat(" ", leftPad),
		text,
		strings.Repeat(" ", rightPad))
}
