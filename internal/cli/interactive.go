package cli

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/pterm/pterm"
	"github.com/thushan/olla/internal/adapter/discovery"
	"github.com/thushan/olla/internal/config"
	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/core/ports"
	"github.com/thushan/olla/internal/logger"
)

type InteractiveCLI struct {
	discoveryService ports.DiscoveryService
	config           *config.Config
	logger           *logger.StyledLogger
	shutdownCh       chan struct{}
	running          bool
}

const (
	helpText = `
Interactive Commands:
  h, health    - Show health summary
  n, nodes     - Show node list with status  
  s, status    - Show detailed endpoint status
  r, refresh   - Force refresh health checks
  c, config    - Show current configuration
  l, logs      - Toggle log level (info <-> debug)
  help         - Show this help message
  q, quit      - Shutdown olla

Press any key + Enter to execute command...
`
)

func NewInteractiveCLI(discoveryService ports.DiscoveryService, config *config.Config, logger *logger.StyledLogger) *InteractiveCLI {
	return &InteractiveCLI{
		discoveryService: discoveryService,
		config:           config,
		logger:           logger,
		shutdownCh:       make(chan struct{}),
		running:          false,
	}
}

func (cli *InteractiveCLI) Start(ctx context.Context) {
	if cli.running {
		return
	}
	cli.running = true

	cli.showWelcome()

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

func (cli *InteractiveCLI) showWelcome() {
	pterm.DefaultHeader.WithFullWidth().
		WithBackgroundStyle(pterm.NewStyle(pterm.BgLightBlue)).
		WithTextStyle(pterm.NewStyle(pterm.FgBlack)).
		Println("Olla Interactive CLI")

	pterm.Info.Println("Interactive mode enabled. Type 'help' for commands or 'q' to quit.")
	pterm.Println()
}

func (cli *InteractiveCLI) inputLoop(ctx context.Context) {
	scanner := bufio.NewScanner(os.Stdin)

	for cli.running {
		select {
		case <-ctx.Done():
			return
		default:
			if scanner.Scan() {
				input := strings.TrimSpace(scanner.Text())
				if input != "" {
					cli.handleCommand(ctx, input)
				}
			}
		}
	}
}

func (cli *InteractiveCLI) handleCommand(ctx context.Context, cmd string) {
	switch strings.ToLower(cmd) {
	case "h", "health":
		cli.showHealthSummary(ctx)
	case "n", "nodes":
		cli.showNodeList(ctx)
	case "s", "status":
		cli.showDetailedStatus(ctx)
	case "r", "refresh":
		cli.forceRefresh(ctx)
	case "c", "config":
		cli.showConfig()
	case "l", "logs":
		cli.toggleLogLevel()
	case "help":
		cli.showHelp()
	case "q", "quit", "exit":
		cli.initiateShutdown()
	default:
		pterm.Warning.Printfln("Unknown command: %s. Type 'help' for available commands.", cmd)
	}
}

func (cli *InteractiveCLI) showHealthSummary(ctx context.Context) {
	pterm.DefaultSection.Println("Health Summary")

	if ds, ok := cli.discoveryService.(*discovery.StaticDiscoveryService); ok {
		status, err := ds.GetHealthStatus(ctx)
		if err != nil {
			pterm.Error.Printfln("Error getting health status: %v", err)
			return
		}

		total := status["total_endpoints"].(int)
		healthy := status["healthy_endpoints"].(int)
		routable := status["routable_endpoints"].(int)
		unhealthy := status["unhealthy_endpoints"].(int)

		// Create summary table
		tableData := pterm.TableData{
			{"Metric", "Count"},
			{"Total Endpoints", pterm.Sprintf("%d", total)},
			{"Healthy", pterm.Green(fmt.Sprintf("%d", healthy))},
			{"Routable", pterm.Cyan(fmt.Sprintf("%d", routable))},
			{"Unhealthy", pterm.Red(fmt.Sprintf("%d", unhealthy))},
		}

		pterm.DefaultTable.WithHasHeader().WithData(tableData).Render()

		healthPercentage := float64(healthy) / float64(total) * 100
		if healthPercentage >= 80 {
			pterm.Success.Printfln("Health Percentage: %.1f%%", healthPercentage)
		} else if healthPercentage >= 50 {
			pterm.Warning.Printfln("Health Percentage: %.1f%%", healthPercentage)
		} else {
			pterm.Error.Printfln("Health Percentage: %.1f%%", healthPercentage)
		}
	} else {
		pterm.Warning.Println("Health status not available")
	}
	pterm.Println()
}

func (cli *InteractiveCLI) showNodeList(ctx context.Context) {
	pterm.DefaultSection.Println("Node List")

	endpoints, err := cli.discoveryService.GetEndpoints(ctx)
	if err != nil {
		pterm.Error.Printfln("Error getting endpoints: %v", err)
		return
	}

	if len(endpoints) == 0 {
		pterm.Warning.Println("No endpoints configured")
		return
	}

	// Sort by priority (highest first)
	for i := 0; i < len(endpoints)-1; i++ {
		for j := i + 1; j < len(endpoints); j++ {
			if endpoints[i].Priority < endpoints[j].Priority {
				endpoints[i], endpoints[j] = endpoints[j], endpoints[i]
			}
		}
	}

	// Create table data
	tableData := pterm.TableData{
		{"Name", "Status", "Priority", "URL", "Latency"},
	}

	for _, ep := range endpoints {
		statusText := cli.getStatusText(ep.Status)
		latency := "N/A"
		if ep.LastLatency > 0 {
			latency = ep.LastLatency.Round(time.Millisecond).String()
		}

		tableData = append(tableData, []string{
			ep.Name,
			statusText,
			fmt.Sprintf("%d", ep.Priority),
			ep.URL.String(),
			latency,
		})
	}

	pterm.DefaultTable.WithHasHeader().WithData(tableData).Render()
	pterm.Println()
}

func (cli *InteractiveCLI) showDetailedStatus(ctx context.Context) {
	pterm.DefaultSection.Println("Detailed Status")

	if ds, ok := cli.discoveryService.(*discovery.StaticDiscoveryService); ok {
		status, err := ds.GetHealthStatus(ctx)
		if err != nil {
			pterm.Error.Printfln("Error getting detailed status: %v", err)
			return
		}

		endpoints, ok := status["endpoints"].([]discovery.EndpointStatusResponse)
		if !ok {
			pterm.Warning.Println("Unable to parse endpoint details")
			return
		}

		for i, ep := range endpoints {
			if i > 0 {
				pterm.Println()
			}

			// Create a box for each endpoint
			pterm.DefaultBox.WithTitle(ep.Name).WithTitleTopCenter().Printfln(
				"URL: %s\nStatus: %s\nPriority: %d\nLast Checked: %s\nLatency: %s\nConsecutive Failures: %d\nNext Check: %s",
				pterm.Cyan(ep.URL),
				cli.getStatusTextByString(ep.Status),
				ep.Priority,
				ep.LastChecked.Format("15:04:05"),
				ep.LastLatency,
				ep.ConsecutiveFailures,
				ep.NextCheckTime.Format("15:04:05"))
		}
	} else {
		pterm.Warning.Println("Detailed status not available")
	}
	pterm.Println()
}

func (cli *InteractiveCLI) forceRefresh(ctx context.Context) {
	pterm.DefaultSection.Println("Force Health Check Refresh")

	spinner, _ := pterm.DefaultSpinner.Start("Refreshing endpoints...")
	start := time.Now()

	if err := cli.discoveryService.RefreshEndpoints(ctx); err != nil {
		spinner.Fail(fmt.Sprintf("Error refreshing endpoints: %v", err))
		return
	}

	elapsed := time.Since(start)
	spinner.Success(fmt.Sprintf("Refresh completed in %v", elapsed.Round(time.Millisecond)))
	pterm.Println()
}

func (cli *InteractiveCLI) showConfig() {
	pterm.DefaultSection.Println("Current Configuration")

	configData := pterm.TableData{
		{"Setting", "Value"},
		{"Server", fmt.Sprintf("%s:%d", cli.config.Server.Host, cli.config.Server.Port)},
		{"Discovery Type", cli.config.Discovery.Type},
		{"Log Level", cli.config.Logging.Level},
		{"Endpoints", fmt.Sprintf("%d configured", len(cli.config.Discovery.Static.Endpoints))},
	}

	if cli.config.Telemetry.Metrics.Enabled {
		configData = append(configData, []string{"Metrics", fmt.Sprintf("Enabled (%s)", cli.config.Telemetry.Metrics.Address)})
	} else {
		configData = append(configData, []string{"Metrics", "Disabled"})
	}

	pterm.DefaultTable.WithHasHeader().WithData(configData).Render()
	pterm.Println()
}

func (cli *InteractiveCLI) toggleLogLevel() {
	pterm.DefaultSection.Println("Toggle Log Level")

	currentLevel := cli.config.Logging.Level
	var newLevel string

	switch strings.ToLower(currentLevel) {
	case "info":
		newLevel = "debug"
	case "debug":
		newLevel = "info"
	default:
		newLevel = "info"
	}

	cli.config.Logging.Level = newLevel
	pterm.Success.Printfln("Log level changed: %s → %s", currentLevel, newLevel)
	pterm.Warning.Println("Note: This change is temporary and won't persist after restart")
	pterm.Println()
}

func (cli *InteractiveCLI) showHelp() {
	pterm.DefaultSection.Println("Interactive Commands")

	helpData := pterm.TableData{
		{"Command", "Aliases", "Description"},
		{"health", "h", "Show health summary"},
		{"nodes", "n", "Show node list with status"},
		{"status", "s", "Show detailed endpoint status"},
		{"refresh", "r", "Force refresh health checks"},
		{"config", "c", "Show current configuration"},
		{"logs", "l", "Toggle log level (info ↔ debug)"},
		{"help", "", "Show this help message"},
		{"quit", "q, exit", "Shutdown olla"},
	}

	pterm.DefaultTable.WithHasHeader().WithData(helpData).Render()
	pterm.Info.Println("Press any key + Enter to execute command...")
	pterm.Println()
}

func (cli *InteractiveCLI) initiateShutdown() {
	pterm.Success.Println("Initiating graceful shutdown...")
	cli.Stop()
}

func (cli *InteractiveCLI) getStatusText(status domain.EndpointStatus) string {
	switch status {
	case domain.StatusHealthy:
		return pterm.Green("HEALTHY")
	case domain.StatusBusy:
		return pterm.Yellow("BUSY")
	case domain.StatusOffline:
		return pterm.Red("OFFLINE")
	case domain.StatusWarming:
		return pterm.Cyan("WARMING")
	case domain.StatusUnhealthy:
		return pterm.Red("UNHEALTHY")
	default:
		return pterm.Gray("UNKNOWN")
	}
}

func (cli *InteractiveCLI) getStatusTextByString(status string) string {
	switch strings.ToLower(status) {
	case "healthy":
		return pterm.Green("HEALTHY")
	case "busy":
		return pterm.Yellow("BUSY")
	case "offline":
		return pterm.Red("OFFLINE")
	case "warming":
		return pterm.Cyan("WARMING")
	case "unhealthy":
		return pterm.Red("UNHEALTHY")
	default:
		return pterm.Gray("UNKNOWN")
	}
}
