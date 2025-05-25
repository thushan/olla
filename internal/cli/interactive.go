// internal/cli/interactive.go
package cli

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/pterm/pterm"
	"github.com/thushan/olla/internal/adapter/discovery"
	"github.com/thushan/olla/internal/config"
	"github.com/thushan/olla/internal/core/ports"
	"github.com/thushan/olla/internal/logger"
	"github.com/thushan/olla/internal/version"
)

type InteractiveCLI struct {
	discoveryService ports.DiscoveryService
	config           *config.Config
	logger           *logger.StyledLogger
	shutdownCh       chan struct{}
	running          bool

	// Content management
	logMessages []string
	logMutex    sync.RWMutex
	maxLogLines int
}

const (
	updateInterval = 10 * time.Second
	maxLogMessages = 50 // Keep last 50 log messages
)

func NewInteractiveCLI(discoveryService ports.DiscoveryService, config *config.Config, logger *logger.StyledLogger) *InteractiveCLI {
	return &InteractiveCLI{
		discoveryService: discoveryService,
		config:           config,
		logger:           logger,
		shutdownCh:       make(chan struct{}),
		running:          false,
		logMessages:      make([]string, 0),
		maxLogLines:      maxLogMessages,
	}
}

func (cli *InteractiveCLI) Start(ctx context.Context) {
	if cli.running {
		return
	}
	cli.running = true

	// Show initial interface
	cli.showInterface(ctx)

	// Start background updater for cluster status
	go cli.backgroundUpdater(ctx)

	// Start input handler
	go cli.inputLoop(ctx)
}

func (cli *InteractiveCLI) Stop() {
	if !cli.running {
		return
	}
	cli.running = false
	close(cli.shutdownCh)
}

func (cli *InteractiveCLI) GetShutdownChannel() <-chan struct{} {
	return cli.shutdownCh
}

func (cli *InteractiveCLI) showInterface(ctx context.Context) {
	// Clear screen completely
	pterm.Print("\033[2J\033[H")

	// Show header
	cli.showHeader()

	// Show cluster status
	cli.showClusterStatus(ctx)

	// Show log area with current messages
	cli.showLogArea()

	// Show command bar
	cli.showCommandBar()

	// Position cursor at input
	pterm.Print("> ")
}

func (cli *InteractiveCLI) refreshInterface(ctx context.Context) {
	// Save cursor position
	pterm.Print("\033[s")

	// Move to top and redraw just the dynamic parts
	pterm.Print("\033[H")

	// Redraw cluster status (skip header)
	pterm.Print("\033[4;1H") // Move to line 4
	cli.showClusterStatus(ctx)

	// Redraw log area
	cli.showLogArea()

	// Restore cursor to input line
	pterm.Print("\033[u> ")
}

func (cli *InteractiveCLI) showHeader() {
	pid := os.Getpid()
	headerText := fmt.Sprintf("Olla %s (PID: %d)", version.Version, pid)

	pterm.DefaultBox.
		WithTitle(headerText).
		WithTitleTopCenter().
		WithBoxStyle(pterm.NewStyle(pterm.FgBlue)).
		Println("")
}

func (cli *InteractiveCLI) showClusterStatus(ctx context.Context) {
	statusText := cli.getClusterStatusText(ctx)

	pterm.DefaultBox.
		WithTitle("Cluster Status").
		WithTitleTopLeft().
		WithBoxStyle(pterm.NewStyle(pterm.FgCyan)).
		Println(statusText)
}

func (cli *InteractiveCLI) showLogArea() {
	cli.logMutex.RLock()
	logs := make([]string, len(cli.logMessages))
	copy(logs, cli.logMessages)
	cli.logMutex.RUnlock()

	// Create log content
	logContent := "System logs will appear here..."
	if len(logs) > 0 {
		// Show last 10 messages
		start := 0
		if len(logs) > 10 {
			start = len(logs) - 10
		}
		logContent = strings.Join(logs[start:], "\n")
	}

	pterm.DefaultBox.
		WithTitle("Application Logs").
		WithTitleTopLeft().
		WithBoxStyle(pterm.NewStyle(pterm.FgGreen)).
		Println(logContent)
}

func (cli *InteractiveCLI) showCommandBar() {
	commands := []string{
		"h:Health", "n:Nodes", "s:Status", "r:Refresh",
		"c:Config", "l:Logs", "q:Quit", "help:Commands",
	}

	commandText := strings.Join(commands, " | ")

	pterm.DefaultBox.
		WithBoxStyle(pterm.NewStyle(pterm.BgBlue, pterm.FgWhite)).
		Println(commandText)
}

func (cli *InteractiveCLI) getClusterStatusText(ctx context.Context) string {
	if ds, ok := cli.discoveryService.(*discovery.StaticDiscoveryService); ok {
		status, err := ds.GetHealthStatus(ctx)
		if err != nil {
			return pterm.Red("Error getting cluster status")
		}

		total := status["total_endpoints"].(int)
		healthy := status["healthy_endpoints"].(int)
		routable := status["routable_endpoints"].(int)

		timestamp := time.Now().Format("15:04:05")

		return fmt.Sprintf("%s | %s | Updated: %s",
			pterm.Green(fmt.Sprintf("%d/%d healthy", healthy, total)),
			pterm.Cyan(fmt.Sprintf("%d routable", routable)),
			pterm.Gray(timestamp))
	}

	return pterm.Gray("Cluster status unavailable")
}

func (cli *InteractiveCLI) backgroundUpdater(ctx context.Context) {
	ticker := time.NewTicker(updateInterval)
	defer ticker.Stop()

	for cli.running {
		select {
		case <-ctx.Done():
			return
		case <-cli.shutdownCh:
			return
		case <-ticker.C:
			// Just update cluster status, don't redraw everything
			cli.updateClusterStatus(ctx)
		}
	}
}

func (cli *InteractiveCLI) updateClusterStatus(ctx context.Context) {
	// Move cursor to cluster status area and update just that section
	// This is a simplified approach - in practice you'd need precise cursor positioning
	statusText := cli.getClusterStatusText(ctx)

	// For now, just add to log that status updated
	cli.addLogMessage("info", fmt.Sprintf("Cluster status: %s", statusText))
}

func (cli *InteractiveCLI) inputLoop(ctx context.Context) {
	scanner := bufio.NewScanner(os.Stdin)

	for cli.running {
		select {
		case <-ctx.Done():
			return
		case <-cli.shutdownCh:
			return
		default:
			if scanner.Scan() {
				input := strings.TrimSpace(scanner.Text())
				if input != "" {
					cli.handleCommand(ctx, input)
				}
				// Show prompt again after command
				pterm.Print("> ")
			}
		}
	}
}

func (cli *InteractiveCLI) handleCommand(ctx context.Context, cmd string) {
	switch strings.ToLower(cmd) {
	case "h", "health":
		cli.showHealthCommand(ctx)
	case "n", "nodes":
		cli.showNodesCommand(ctx)
	case "s", "status":
		cli.showStatusCommand(ctx)
	case "r", "refresh":
		cli.refreshCommand(ctx)
	case "c", "config":
		cli.showConfigCommand()
	case "l", "logs":
		cli.toggleLogLevel()
	case "help":
		cli.showHelpCommand()
	case "q", "quit", "exit":
		cli.initiateShutdown()
		return
	case "clear":
		cli.showInterface(ctx) // Redraw interface
		return
	default:
		cli.addLogMessage("warn", fmt.Sprintf("Unknown command: %s (type 'help' for commands)", cmd))
	}

	// Refresh the interface after any command
	cli.refreshInterface(ctx)
}

func (cli *InteractiveCLI) showHealthCommand(ctx context.Context) {
	if ds, ok := cli.discoveryService.(*discovery.StaticDiscoveryService); ok {
		status, err := ds.GetHealthStatus(ctx)
		if err != nil {
			cli.addLogMessage("error", fmt.Sprintf("Error getting health: %v", err))
			return
		}

		total := status["total_endpoints"].(int)
		healthy := status["healthy_endpoints"].(int)
		routable := status["routable_endpoints"].(int)
		unhealthy := status["unhealthy_endpoints"].(int)

		healthMsg := fmt.Sprintf("Health: %d total, %d healthy, %d routable, %d unhealthy",
			total, healthy, routable, unhealthy)
		cli.addLogMessage("info", healthMsg)

		healthPercentage := float64(healthy) / float64(total) * 100
		cli.addLogMessage("info", fmt.Sprintf("Health percentage: %.1f%%", healthPercentage))
	}
}

func (cli *InteractiveCLI) showNodesCommand(ctx context.Context) {
	endpoints, err := cli.discoveryService.GetEndpoints(ctx)
	if err != nil {
		cli.addLogMessage("error", fmt.Sprintf("Error getting nodes: %v", err))
		return
	}

	cli.addLogMessage("info", fmt.Sprintf("Found %d nodes:", len(endpoints)))
	for _, ep := range endpoints {
		statusText := strings.ToLower(string(ep.Status))
		nodeMsg := fmt.Sprintf("  %s: %s (priority: %d)", ep.Name, statusText, ep.Priority)
		cli.addLogMessage("info", nodeMsg)
	}
}

func (cli *InteractiveCLI) showStatusCommand(ctx context.Context) {
	cli.addLogMessage("info", "Detailed status requested")
	// Show last few log messages as status
	cli.logMutex.RLock()
	recentLogs := len(cli.logMessages)
	cli.logMutex.RUnlock()

	cli.addLogMessage("info", fmt.Sprintf("Recent activity: %d log messages", recentLogs))
}

func (cli *InteractiveCLI) refreshCommand(ctx context.Context) {
	cli.addLogMessage("info", "Refreshing endpoints...")

	start := time.Now()
	if err := cli.discoveryService.RefreshEndpoints(ctx); err != nil {
		cli.addLogMessage("error", fmt.Sprintf("Refresh failed: %v", err))
	} else {
		elapsed := time.Since(start)
		cli.addLogMessage("info", fmt.Sprintf("Refresh completed in %v", elapsed.Round(time.Millisecond)))
	}
}

func (cli *InteractiveCLI) showConfigCommand() {
	cli.addLogMessage("info", fmt.Sprintf("Server: %s:%d", cli.config.Server.Host, cli.config.Server.Port))
	cli.addLogMessage("info", fmt.Sprintf("Discovery: %s", cli.config.Discovery.Type))
	cli.addLogMessage("info", fmt.Sprintf("Log level: %s", cli.config.Logging.Level))
	cli.addLogMessage("info", fmt.Sprintf("Endpoints: %d configured", len(cli.config.Discovery.Static.Endpoints)))
}

func (cli *InteractiveCLI) toggleLogLevel() {
	currentLevel := cli.config.Logging.Level
	newLevel := "info"
	if currentLevel == "info" {
		newLevel = "debug"
	}

	cli.config.Logging.Level = newLevel
	cli.addLogMessage("info", fmt.Sprintf("Log level: %s → %s", currentLevel, newLevel))
}

func (cli *InteractiveCLI) showHelpCommand() {
	commands := []string{
		"h,health - Show health summary",
		"n,nodes - Show node list",
		"s,status - Show detailed status",
		"r,refresh - Force refresh",
		"c,config - Show configuration",
		"l,logs - Toggle log level",
		"clear - Redraw interface",
		"q,quit - Exit application",
	}

	cli.addLogMessage("info", "Available commands:")
	for _, cmd := range commands {
		cli.addLogMessage("info", fmt.Sprintf("  %s", cmd))
	}
}

func (cli *InteractiveCLI) initiateShutdown() {
	cli.addLogMessage("info", "Shutting down...")
	cli.Stop()
}

// HandleLogMessage receives log messages from the styled logger
func (cli *InteractiveCLI) HandleLogMessage(level, message string, args ...interface{}) {
	if !cli.running {
		return
	}

	cli.addLogMessage(level, message)

	// Auto-refresh interface when new logs arrive (but not too frequently)
	go func() {
		time.Sleep(50 * time.Millisecond) // Small delay to batch updates
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()
		cli.refreshInterface(ctx)
	}()
}

func (cli *InteractiveCLI) addLogMessage(level, message string) {
	cli.logMutex.Lock()
	defer cli.logMutex.Unlock()

	timestamp := time.Now().Format("15:04:05")

	var levelColor func(...interface{}) string
	switch strings.ToLower(level) {
	case "debug":
		levelColor = pterm.Gray
	case "info":
		levelColor = pterm.Blue
	case "warn":
		levelColor = pterm.Yellow
	case "error":
		levelColor = pterm.Red
	default:
		levelColor = pterm.White
	}

	logLine := fmt.Sprintf("[%s] %s %s",
		pterm.Gray(timestamp),
		levelColor(strings.ToUpper(level)),
		message)

	cli.logMessages = append(cli.logMessages, logLine)

	// Keep only recent messages
	if len(cli.logMessages) > cli.maxLogLines {
		cli.logMessages = cli.logMessages[len(cli.logMessages)-cli.maxLogLines:]
	}
}
