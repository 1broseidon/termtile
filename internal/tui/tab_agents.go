package tui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/1broseidon/termtile/internal/config"
)

// agentItem is a list item representing a configured or detected agent.
type agentItem struct {
	name       string
	configured bool
	detected   bool
	agentCfg   config.AgentConfig
}

func (i agentItem) Title() string {
	if i.configured {
		return lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Render("✓") + " " + i.name
	}
	if i.detected {
		return lipgloss.NewStyle().Foreground(lipgloss.Color("226")).Render("+") + " " + i.name
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Render("·") + " " + i.name
}

func (i agentItem) Description() string {
	if i.configured {
		return i.agentCfg.Command + " (configured)"
	}
	if i.detected {
		return i.agentCfg.Command + " (detected)"
	}
	return "(not found)"
}

func (i agentItem) FilterValue() string { return i.name }

// AgentsTab is the sub-model for the Agents tab.
type AgentsTab struct {
	list     list.Model
	cfg      *config.Config
	detected []config.DetectedAgent
	width    int
	height   int
}

// NewAgentsTab creates a new AgentsTab, scanning for agents immediately.
func NewAgentsTab(cfg *config.Config) AgentsTab {
	var agents map[string]config.AgentConfig
	if cfg != nil {
		agents = cfg.Agents
	}
	detected := config.DetectAgents(agents)
	items := buildAgentItems(cfg, detected)

	delegate := list.NewDefaultDelegate()
	delegate.Styles.SelectedTitle = delegate.Styles.SelectedTitle.
		Foreground(lipgloss.Color("15")).
		BorderForeground(lipgloss.Color("62"))
	delegate.Styles.SelectedDesc = delegate.Styles.SelectedDesc.
		Foreground(lipgloss.Color("250")).
		BorderForeground(lipgloss.Color("62"))

	l := list.New(items, delegate, 0, 0)
	l.Title = "Agents"
	l.Styles.Title = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("15")).
		Background(lipgloss.Color("62")).
		Padding(0, 1)
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(false)
	l.SetShowHelp(false)
	l.KeyMap.Quit.SetEnabled(false)

	return AgentsTab{
		list:     l,
		cfg:      cfg,
		detected: detected,
	}
}

// Init implements tea.Model.
func (a AgentsTab) Init() tea.Cmd { return nil }

// Update handles messages for the agents tab.
func (a AgentsTab) Update(msg tea.Msg) (AgentsTab, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height
		leftWidth := a.width * 2 / 5
		if leftWidth < 20 {
			leftWidth = 20
		}
		a.list.SetSize(leftWidth, a.height)
		return a, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "D":
			var agents map[string]config.AgentConfig
			if a.cfg != nil {
				agents = a.cfg.Agents
			}
			a.detected = config.DetectAgents(agents)
			items := buildAgentItems(a.cfg, a.detected)
			a.list.SetItems(items)
			return a, nil
		}
	}

	var cmd tea.Cmd
	a.list, cmd = a.list.Update(msg)
	return a, cmd
}

// View implements tea.Model.
func (a AgentsTab) View() string {
	if a.width == 0 || a.height == 0 {
		return ""
	}

	leftWidth := a.width * 2 / 5
	if leftWidth < 20 {
		leftWidth = 20
	}
	rightWidth := a.width - leftWidth
	if rightWidth < 10 {
		rightWidth = 10
	}

	left := lipgloss.NewStyle().
		Width(leftWidth).
		Height(a.height).
		Render(a.list.View())

	var right string
	if item, ok := a.list.SelectedItem().(agentItem); ok {
		right = renderAgentDetail(item, rightWidth, a.height)
	} else {
		right = lipgloss.NewStyle().
			Width(rightWidth).
			Height(a.height).
			Foreground(lipgloss.Color("241")).
			Align(lipgloss.Center, lipgloss.Center).
			Render("No agents found\nPress D to scan")
	}

	return lipgloss.JoinHorizontal(lipgloss.Top, left, right)
}

// buildAgentItems merges configured and detected agents into list items.
func buildAgentItems(cfg *config.Config, detected []config.DetectedAgent) []list.Item {
	detectedMap := make(map[string]config.DetectedAgent, len(detected))
	for _, d := range detected {
		detectedMap[d.Name] = d
	}

	seen := make(map[string]bool)
	var items []list.Item

	// Configured agents first (sorted).
	if cfg != nil && len(cfg.Agents) > 0 {
		names := make([]string, 0, len(cfg.Agents))
		for name := range cfg.Agents {
			names = append(names, name)
		}
		sort.Strings(names)

		for _, name := range names {
			ac := cfg.Agents[name]
			_, found := detectedMap[name]
			items = append(items, agentItem{
				name:       name,
				configured: true,
				detected:   found,
				agentCfg:   ac,
			})
			seen[name] = true
		}
	}

	// Detected-but-not-configured agents.
	for _, d := range detected {
		if seen[d.Name] {
			continue
		}
		items = append(items, agentItem{
			name:       d.Name,
			configured: false,
			detected:   true,
			agentCfg:   d.ProposedConfig,
		})
	}

	return items
}

// renderAgentDetail renders the right-side detail pane for the selected agent.
func renderAgentDetail(item agentItem, width, height int) string {
	var b strings.Builder

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15"))
	b.WriteString(titleStyle.Render(item.name))
	b.WriteString("\n\n")

	// Status indicator.
	switch {
	case item.configured && item.detected:
		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Render("● configured & detected"))
	case item.configured:
		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Render("● configured"))
		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Render("  (binary not found)"))
	case item.detected:
		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("226")).Render("+ detected — not yet configured"))
	}
	b.WriteString("\n\n")

	labelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("248")).Width(18)
	valueStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("15"))

	field := func(label, value string) {
		if value == "" {
			return
		}
		b.WriteString(labelStyle.Render(label))
		b.WriteString(valueStyle.Render(value))
		b.WriteString("\n")
	}

	ac := item.agentCfg
	field("command:", ac.Command)
	if len(ac.Args) > 0 {
		field("args:", strings.Join(ac.Args, " "))
	}
	field("spawn_mode:", ac.SpawnMode)
	field("ready_pattern:", ac.ReadyPattern)
	field("idle_pattern:", ac.IdlePattern)
	field("default_model:", ac.DefaultModel)
	field("model_flag:", ac.ModelFlag)
	field("description:", ac.Description)
	if ac.PromptAsArg {
		field("prompt_as_arg:", "true")
	}
	if ac.ResponseFence {
		field("response_fence:", "true")
	}
	if len(ac.Models) > 0 {
		field("models:", strings.Join(ac.Models, ", "))
	}

	// Action hint for unconfigured agents.
	if !item.configured {
		b.WriteString("\n")
		hint := lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Italic(true)
		b.WriteString(hint.Render(fmt.Sprintf("Add %q to your config to use this agent", item.name)))
	}

	style := lipgloss.NewStyle().
		Width(width).
		Height(height).
		Padding(1, 2).
		BorderStyle(lipgloss.NormalBorder()).
		BorderLeft(true).
		BorderForeground(lipgloss.Color("236"))

	return style.Render(b.String())
}
