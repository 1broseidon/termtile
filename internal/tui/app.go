package tui

import (
	"github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/1broseidon/termtile/internal/config"
	"github.com/1broseidon/termtile/internal/ipc"
)

// model is the root bubbletea model for the TUI.
type model struct {
	configPath string
	result     *config.LoadResult
	ipcClient  *ipc.Client

	// Tab navigation
	activeTab Tab

	// Sub-models
	generalTab   GeneralTab
	layoutsTab   LayoutsTab
	agentsTab    AgentsTab
	terminalsTab TerminalsTab

	// Save overlay
	originalConfig *config.Config
	saveOverlay    SaveOverlay

	// Daemon state
	daemonConnected bool
	activeLayout    string
	defaultLayout   string

	// Terminal dimensions
	width  int
	height int
}

func newModel(configPath string) model {
	m := model{
		configPath: configPath,
		activeTab:  TabGeneral,
	}

	// Load config
	m.loadConfig()

	// Snapshot original config for diff preview on save
	if m.result != nil {
		m.originalConfig = cloneConfig(m.result.Config)
	}

	// Connect to daemon
	m.ipcClient = ipc.NewClient()
	if err := m.ipcClient.Ping(); err == nil {
		m.refreshDaemonStatus()
	}

	// Initialize sub-models
	var cfg *config.Config
	if m.result != nil {
		cfg = m.result.Config
	}
	m.generalTab = NewGeneralTab(cfg)
	m.layoutsTab = NewLayoutsTab(m.ipcClient, cfg, m.activeLayout, m.defaultLayout)
	m.agentsTab = NewAgentsTab(cfg)
	m.terminalsTab = NewTerminalsTab(cfg)

	return m
}

func (m *model) loadConfig() {
	var res *config.LoadResult
	var err error

	if m.configPath == "" {
		res, err = config.LoadWithSources()
	} else {
		res, err = config.LoadFromPath(m.configPath)
	}

	if err != nil {
		return
	}
	m.result = res
}

func (m *model) refreshDaemonStatus() {
	if m.ipcClient == nil {
		return
	}
	data, err := m.ipcClient.ListLayouts()
	if err != nil {
		m.daemonConnected = false
		m.activeLayout = ""
		m.defaultLayout = ""
		return
	}
	m.daemonConnected = true
	m.activeLayout = data.ActiveLayout
	m.defaultLayout = data.DefaultLayout
}

// contentHeight returns the height available for tab content.
func (m model) contentHeight() int {
	// Approximate: status bar (1) + tab bar (2 with margin) + help bar (1) = 4 lines
	h := m.height - 4
	if h < 1 {
		h = 1
	}
	return h
}

// Init implements tea.Model.
func (m model) Init() tea.Cmd {
	return nil
}

// Update implements tea.Model.
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Save overlay captures all input when active
	if m.saveOverlay.Active() {
		switch msg := msg.(type) {
		case tea.KeyMsg:
			if msg.String() == "ctrl+c" {
				return m, tea.Quit
			}
			prevPhase := m.saveOverlay.phase
			m.saveOverlay = m.saveOverlay.Update(msg, m.result.Config, m.ipcClient, m.daemonConnected)
			// After successful save, update the original snapshot
			if prevPhase == savePreview && m.saveOverlay.SaveSucceeded() {
				m.originalConfig = cloneConfig(m.result.Config)
			}
		case tea.WindowSizeMsg:
			m.width = msg.Width
			m.height = msg.Height
		}
		return m, nil
	}

	// ctrl+s triggers save overlay from any context (including form editing)
	if km, ok := msg.(tea.KeyMsg); ok && km.String() == "ctrl+s" {
		if m.result != nil && m.result.Config != nil {
			m.saveOverlay.Show(m.originalConfig, m.result.Config)
		}
		return m, nil
	}

	// When a sub-model captures input, delegate all messages to it
	// (the form/input consumes keys; only ctrl+c escapes to quit)
	capturing := (m.activeTab == TabGeneral && m.generalTab.editing) ||
		(m.activeTab == TabTerminalClasses && m.terminalsTab.adding)
	if capturing {
		switch msg := msg.(type) {
		case tea.KeyMsg:
			if msg.String() == "ctrl+c" {
				return m, tea.Quit
			}
		case tea.WindowSizeMsg:
			m.width = msg.Width
			m.height = msg.Height
			contentHeight := m.contentHeight()
			subMsg := tea.WindowSizeMsg{Width: m.width, Height: contentHeight}
			m.generalTab, _ = m.generalTab.Update(subMsg)
			m.layoutsTab, _ = m.layoutsTab.Update(subMsg)
			m.agentsTab, _ = m.agentsTab.Update(subMsg)
			m.terminalsTab, _ = m.terminalsTab.Update(subMsg)
			return m, nil
		}
		var cmd tea.Cmd
		switch m.activeTab {
		case TabGeneral:
			m.generalTab, cmd = m.generalTab.Update(msg)
		case TabTerminalClasses:
			m.terminalsTab, cmd = m.terminalsTab.Update(msg)
		}
		return m, cmd
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit

		case "tab":
			m.activeTab = (m.activeTab + 1) % tabCount
			return m, nil

		case "shift+tab":
			m.activeTab = (m.activeTab - 1 + tabCount) % tabCount
			return m, nil

		case "1":
			m.activeTab = TabGeneral
			return m, nil
		case "2":
			// On layouts tab, 2 is tile count — delegate below
			if m.activeTab != TabLayouts {
				m.activeTab = TabLayouts
				return m, nil
			}
		case "3":
			m.activeTab = TabAgents
			return m, nil
		case "4":
			// On layouts tab, 4 is tile count — delegate below
			if m.activeTab != TabLayouts {
				m.activeTab = TabTerminalClasses
				return m, nil
			}
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		// Forward to sub-models with content dimensions
		contentHeight := m.contentHeight()
		subMsg := tea.WindowSizeMsg{Width: m.width, Height: contentHeight}
		m.generalTab, _ = m.generalTab.Update(subMsg)
		m.layoutsTab, _ = m.layoutsTab.Update(subMsg)
		m.agentsTab, _ = m.agentsTab.Update(subMsg)
		m.terminalsTab, _ = m.terminalsTab.Update(subMsg)
		return m, nil
	}

	// Delegate to active tab's sub-model
	switch m.activeTab {
	case TabGeneral:
		var cmd tea.Cmd
		m.generalTab, cmd = m.generalTab.Update(msg)
		return m, cmd
	case TabLayouts:
		var cmd tea.Cmd
		m.layoutsTab, cmd = m.layoutsTab.Update(msg)
		return m, cmd
	case TabAgents:
		var cmd tea.Cmd
		m.agentsTab, cmd = m.agentsTab.Update(msg)
		return m, cmd
	case TabTerminalClasses:
		var cmd tea.Cmd
		m.terminalsTab, cmd = m.terminalsTab.Update(msg)
		return m, cmd
	}

	return m, nil
}

// View implements tea.Model.
func (m model) View() string {
	if m.width == 0 || m.height == 0 {
		return ""
	}

	// Status bar (top)
	statusBar := renderStatusBar(m.daemonConnected, m.activeLayout, m.defaultLayout, m.width)

	// Tab bar
	tabBar := renderTabBar(m.activeTab, m.width)

	// Help bar (bottom)
	helpBar := renderHelpBar(m.width)

	// Calculate content height: total - statusBar - tabBar - helpBar
	usedHeight := lipgloss.Height(statusBar) + lipgloss.Height(tabBar) + lipgloss.Height(helpBar)
	contentHeight := m.height - usedHeight
	if contentHeight < 1 {
		contentHeight = 1
	}

	// Tab content (or save overlay)
	var content string
	if m.saveOverlay.Active() {
		content = m.saveOverlay.View(m.width, contentHeight)
	} else {
		switch m.activeTab {
		case TabGeneral:
			content = m.generalTab.View()
		case TabLayouts:
			content = m.layoutsTab.View()
		case TabAgents:
			content = m.agentsTab.View()
		case TabTerminalClasses:
			content = m.terminalsTab.View()
		default:
			content = renderPlaceholder(m.activeTab, m.width, contentHeight)
		}
	}

	return lipgloss.JoinVertical(lipgloss.Left,
		statusBar,
		tabBar,
		content,
		helpBar,
	)
}
