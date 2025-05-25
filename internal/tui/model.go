package tui

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/thushan/olla/internal/core/ports"
	"github.com/thushan/olla/internal/logger"
)

// InputMode represents the current input mode
type InputMode int

const (
	NavigationMode InputMode = iota
	CommandMode
)

// Model represents the main TUI model
type Model struct {
	// UI components
	viewport  viewport.Model
	textInput textinput.Model

	// Application state
	discoveryService ports.DiscoveryService
	logger           *logger.StyledLogger
	content          []string
	mode             InputMode

	// Display info
	appInfo AppInfo

	// Dimensions
	width  int
	height int

	// Context for operations
	ctx context.Context
}

// AppInfo holds application information for display
type AppInfo struct {
	Version           string
	PID               int
	TotalEndpoints    int
	HealthyEndpoints  int
	RoutableEndpoints int
}

// NewModel creates a new TUI model
func NewModel(discoveryService ports.DiscoveryService, logger *logger.StyledLogger) Model {
	// Create viewport for scrollable content
	vp := viewport.New(80, 20)
	vp.Style = lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("62"))

	// Create text input for commands
	ti := textinput.New()
	ti.Placeholder = "Press : to enter command mode..."
	ti.CharLimit = 100

	return Model{
		viewport:         vp,
		textInput:        ti,
		discoveryService: discoveryService,
		logger:           logger,
		content:          []string{},
		mode:             NavigationMode,
		appInfo: AppInfo{
			Version: "v0.0.1", // This would come from version package
			PID:     os.Getpid(),
		},
		ctx: context.Background(),
	}
}

// Init implements tea.Model
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		tea.EnterAltScreen,
		tickCmd(), // Start periodic updates
	)
}

// Update implements tea.Model
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.updateLayout()

	case tea.KeyMsg:
		switch m.mode {
		case NavigationMode:
			return m.handleNavigationKeys(msg)
		case CommandMode:
			return m.handleCommandKeys(msg)
		}

	case tickMsg:
		// Update app info and content periodically
		m.updateAppInfo()
		cmds = append(cmds, tickCmd())

	case contentMsg:
		// Add new content to the display
		m.addContent(string(msg))
	}

	// Update viewport
	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

// handleNavigationKeys handles input in navigation mode
func (m Model) handleNavigationKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit

	case ":", "/":
		m.mode = CommandMode
		m.textInput.Focus()
		m.textInput.SetValue("")
		return m, nil

	case "h":
		m.executeCommand("health")
		return m, nil

	case "n":
		m.executeCommand("nodes")
		return m, nil

	case "s":
		m.executeCommand("status")
		return m, nil

	case "r":
		m.executeCommand("refresh")
		return m, nil

	case "c":
		m.executeCommand("config")
		return m, nil

	case "l":
		m.executeCommand("logs")
		return m, nil

	default:
		// Let viewport handle scrolling
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		return m, cmd
	}
}

// handleCommandKeys handles input in command mode
func (m Model) handleCommandKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEnter:
		command := strings.TrimSpace(m.textInput.Value())
		if command != "" {
			m.executeCommand(command)
		}
		m.mode = NavigationMode
		m.textInput.Blur()
		return m, nil

	case tea.KeyEsc:
		m.mode = NavigationMode
		m.textInput.Blur()
		return m, nil

	default:
		var cmd tea.Cmd
		m.textInput, cmd = m.textInput.Update(msg)
		return m, cmd
	}
}

// View implements tea.Model
func (m Model) View() string {
	if m.width == 0 || m.height == 0 {
		return "Loading..."
	}

	return lipgloss.JoinVertical(
		lipgloss.Left,
		m.renderHeader(),
		m.renderStatusLine(),
		m.renderContent(),
		m.renderFooter(),
	)
}

// renderHeader renders the top header
func (m Model) renderHeader() string {
	title := fmt.Sprintf("─ Olla %s (PID: %d) ", m.appInfo.Version, m.appInfo.PID)
	padding := m.width - len(title) - 2
	if padding < 0 {
		padding = 0
	}

	headerStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("12")). // Light blue
		Bold(true)

	return headerStyle.Render(fmt.Sprintf("┌%s%s┐", title, strings.Repeat("─", padding)))
}

// renderStatusLine renders the cluster status line
func (m Model) renderStatusLine() string {
	timestamp := time.Now().Format("15:04")
	statusText := fmt.Sprintf("├─ Cluster: %d/%d healthy | %d routable | %s ",
		m.appInfo.HealthyEndpoints, m.appInfo.TotalEndpoints, m.appInfo.RoutableEndpoints, timestamp)

	padding := m.width - len(statusText) - 1
	if padding < 0 {
		padding = 0
	}

	var statusStyle lipgloss.Style
	if m.appInfo.HealthyEndpoints == 0 {
		statusStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("9")) // Red
	} else if m.appInfo.HealthyEndpoints < m.appInfo.TotalEndpoints/2 {
		statusStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("11")) // Yellow
	} else {
		statusStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("10")) // Green
	}

	return statusStyle.Render(fmt.Sprintf("%s%s┤", statusText, strings.Repeat("─", padding)))
}

// renderContent renders the scrollable content area
func (m Model) renderContent() string {
	// Update viewport content
	m.viewport.SetContent(strings.Join(m.content, "\n"))
	return m.viewport.View()
}

// renderFooter renders the bottom section
func (m Model) renderFooter() string {
	// Separator line
	separator := lipgloss.NewStyle().
		Foreground(lipgloss.Color("8")).
		Render(fmt.Sprintf("├%s┤", strings.Repeat("─", m.width-2)))

	// Help line
	helpText := "│ h:Health | n:Nodes | s:Status | r:Refresh | q:Quit"
	helpPadding := m.width - len(helpText) - 1
	if helpPadding < 0 {
		helpPadding = 0
	}
	helpLine := fmt.Sprintf("%s%s│", helpText, strings.Repeat(" ", helpPadding))

	// Input line
	var inputLine string
	if m.mode == CommandMode {
		inputLine = fmt.Sprintf("└> %s", m.textInput.View())
		inputPadding := m.width - lipgloss.Width(inputLine)
		if inputPadding > 0 {
			inputLine += strings.Repeat(" ", inputPadding)
		}
	} else {
		prompt := "└> Press : to enter command mode"
		inputPadding := m.width - len(prompt)
		if inputPadding < 0 {
			inputPadding = 0
		}
		inputLine = fmt.Sprintf("%s%s", prompt, strings.Repeat(" ", inputPadding))
	}

	return lipgloss.JoinVertical(lipgloss.Left, separator, helpLine, inputLine)
}

// updateLayout updates component sizes based on terminal dimensions
func (m *Model) updateLayout() {
	headerHeight := 1
	statusHeight := 1
	footerHeight := 3
	contentHeight := m.height - headerHeight - statusHeight - footerHeight - 2 // -2 for borders

	if contentHeight < 1 {
		contentHeight = 1
	}

	m.viewport.Width = m.width - 2 // Account for borders
	m.viewport.Height = contentHeight
	m.textInput.Width = m.width - 4 // Account for prompt
}

// updateAppInfo fetches current application state
func (m *Model) updateAppInfo() {
	if endpoints, err := m.discoveryService.GetEndpoints(m.ctx); err == nil {
		m.appInfo.TotalEndpoints = len(endpoints)
	}

	if healthy, err := m.discoveryService.GetHealthyEndpoints(m.ctx); err == nil {
		m.appInfo.HealthyEndpoints = len(healthy)
		// Use healthy as approximation for routable
		m.appInfo.RoutableEndpoints = len(healthy)
	}
}

// addContent adds a line to the content buffer
func (m *Model) addContent(content string) {
	m.content = append(m.content, content)

	// Keep buffer size reasonable (last 1000 lines)
	if len(m.content) > 1000 {
		m.content = m.content[len(m.content)-1000:]
	}

	// Auto-scroll to bottom
	m.viewport.GotoBottom()
}

// executeCommand executes a TUI command
func (m *Model) executeCommand(command string) {
	switch strings.ToLower(strings.TrimSpace(command)) {
	case "health", "h":
		m.showHealth()
	case "nodes", "n":
		m.showNodes()
	case "status", "s":
		m.showStatus()
	case "refresh", "r":
		m.refreshEndpoints()
	case "config", "c":
		m.showConfig()
	case "logs", "l":
		m.addContent("Log level toggle functionality would be implemented here")
	case "help":
		m.showHelp()
	case "clear":
		m.content = []string{}
	default:
		m.addContent(fmt.Sprintf("Unknown command: %s. Type 'help' for available commands.", command))
	}
}

// Command implementations (similar to the original CLI but output to content)
func (m *Model) showHealth() {
	m.addContent("=== Health Summary ===")

	m.addContent(fmt.Sprintf("Total Endpoints: %d", m.appInfo.TotalEndpoints))
	m.addContent(fmt.Sprintf("Healthy: %d", m.appInfo.HealthyEndpoints))
	m.addContent(fmt.Sprintf("Routable: %d", m.appInfo.RoutableEndpoints))

	healthPercentage := float64(m.appInfo.HealthyEndpoints) / float64(m.appInfo.TotalEndpoints) * 100
	if m.appInfo.TotalEndpoints > 0 {
		m.addContent(fmt.Sprintf("Health Percentage: %.1f%%", healthPercentage))
	}
}

func (m *Model) showNodes() {
	m.addContent("=== Node List ===")

	endpoints, err := m.discoveryService.GetEndpoints(m.ctx)
	if err != nil {
		m.addContent(fmt.Sprintf("Error getting endpoints: %v", err))
		return
	}

	if len(endpoints) == 0 {
		m.addContent("No endpoints configured")
		return
	}

	// Sort by priority
	for i := 0; i < len(endpoints)-1; i++ {
		for j := i + 1; j < len(endpoints); j++ {
			if endpoints[i].Priority < endpoints[j].Priority {
				endpoints[i], endpoints[j] = endpoints[j], endpoints[i]
			}
		}
	}

	m.addContent(fmt.Sprintf("%-15s %-10s %-8s %-30s %s", "Name", "Status", "Priority", "URL", "Latency"))
	m.addContent(strings.Repeat("-", 80))

	for _, ep := range endpoints {
		latency := "N/A"
		if ep.LastLatency > 0 {
			latency = ep.LastLatency.Round(time.Millisecond).String()
		}

		m.addContent(fmt.Sprintf("%-15s %-10s %-8d %-30s %s",
			ep.Name,
			ep.Status.String(),
			ep.Priority,
			ep.URL.String(),
			latency,
		))
	}
}

func (m *Model) showStatus() {
	m.addContent("=== Detailed Status ===")
	// Implementation would be similar to showNodes but with more detail
	m.addContent("Detailed status implementation would go here")
}

func (m *Model) refreshEndpoints() {
	m.addContent("Refreshing endpoints...")
	start := time.Now()

	if err := m.discoveryService.RefreshEndpoints(m.ctx); err != nil {
		m.addContent(fmt.Sprintf("Error refreshing endpoints: %v", err))
		return
	}

	elapsed := time.Since(start)
	m.addContent(fmt.Sprintf("Refresh completed in %v", elapsed.Round(time.Millisecond)))
}

func (m *Model) showConfig() {
	m.addContent("=== Current Configuration ===")
	m.addContent("TUI Mode: Enabled")
	m.addContent(fmt.Sprintf("PID: %d", m.appInfo.PID))
	m.addContent(fmt.Sprintf("Version: %s", m.appInfo.Version))
}

func (m *Model) showHelp() {
	m.addContent("=== Available Commands ===")
	m.addContent("")
	m.addContent("Navigation:")
	m.addContent("  ↑/↓ - Scroll line by line")
	m.addContent("  PgUp/PgDn - Scroll page by page")
	m.addContent("  : or / - Enter command mode")
	m.addContent("  Esc - Exit command mode")
	m.addContent("")
	m.addContent("Commands:")
	m.addContent("  h, health  - Show health summary")
	m.addContent("  n, nodes   - Show node list with status")
	m.addContent("  s, status  - Show detailed endpoint status")
	m.addContent("  r, refresh - Force refresh health checks")
	m.addContent("  c, config  - Show current configuration")
	m.addContent("  l, logs    - Toggle log level")
	m.addContent("  clear      - Clear content buffer")
	m.addContent("  help       - Show this help message")
	m.addContent("  q, quit    - Shutdown olla")
}

// Custom messages for Bubble Tea
type tickMsg time.Time
type contentMsg string

// tickCmd returns a command that sends periodic tick messages
func tickCmd() tea.Cmd {
	return tea.Tick(1*time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}
