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
		"message_translation",
		"translation_anthropic",
		"passthrough_anthropic",
	}
	SupportedBackends = []string{
		"ollama",
		"lemonade",
		"litellm",
		"llamacpp",
		"lmdeploy",
		"lm_studio",
		"sglang",
		"vllm",
		"vllm-mlx",
		"openai_compatible",
		"docker-model-runner",
	}
	ExperimentalCapabilities = []string{
		"translation_anthropic",
		"passthrough_anthropic",
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

	availableSpace := 33
	version := Version

	// Truncate long dev versions (e.g. "v0.0.24-15-g94c1784-dirty") to fit the banner.
	// Keep the semver prefix (e.g. "v0.0.24") and append "-dev" as a short indicator.
	const devSuffix = "-dev"
	maxVersionLen := availableSpace - len(GithubHomeText) - 2
	if len(version) > maxVersionLen {
		// Find the end of the semver portion (up to the first hyphen after the patch number)
		trimAt := maxVersionLen - len(devSuffix)
		if idx := strings.Index(version[1:], "-"); idx > 0 && idx+1 < trimAt {
			trimAt = idx + 1
		}
		if trimAt > 0 {
			version = version[:trimAt] + devSuffix
		}
	}

	githubUri := theme.Hyperlink(GithubHomeUri, GithubHomeText)
	latestUri := theme.Hyperlink(GithubLatestUri, version)
	llamaArt := "   ⢸⡅⠀⠀⠀⠀⠀⠀⠀⠀⠀⢀⡿  │"

	githubTextLen := len(GithubHomeText)
	versionLen := len(version)

	bufferSpace := availableSpace - githubTextLen - versionLen
	if bufferSpace < 1 {
		bufferSpace = 1
	}

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
	g := centerLine(GithubHomeText)

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
