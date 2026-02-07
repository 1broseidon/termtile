package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Tab identifies a TUI tab.
type Tab int

const (
	TabGeneral Tab = iota
	TabLayouts
	TabAgents
	TabTerminalClasses
	tabCount // sentinel for iteration
)

func (t Tab) String() string {
	switch t {
	case TabGeneral:
		return "General"
	case TabLayouts:
		return "Layouts"
	case TabAgents:
		return "Agents"
	case TabTerminalClasses:
		return "Terminal Classes"
	default:
		return "?"
	}
}

var (
	activeTabStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("15")).
			Background(lipgloss.Color("62")).
			Padding(0, 2)

	inactiveTabStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("250")).
				Background(lipgloss.Color("236")).
				Padding(0, 2)

	tabBarStyle = lipgloss.NewStyle().
			MarginBottom(1)

	tabGap = lipgloss.NewStyle().
		Background(lipgloss.Color("235")).
		SetString(" ")
)

// renderTabBar renders the tab bar with the given active tab and width.
func renderTabBar(active Tab, width int) string {
	var tabs []string
	for i := Tab(0); i < tabCount; i++ {
		label := i.String()
		shortcut := ""
		switch i {
		case TabGeneral:
			shortcut = "1"
		case TabLayouts:
			shortcut = "2"
		case TabAgents:
			shortcut = "3"
		case TabTerminalClasses:
			shortcut = "4"
		}
		label = shortcut + ":" + label
		if i == active {
			tabs = append(tabs, activeTabStyle.Render(label))
		} else {
			tabs = append(tabs, inactiveTabStyle.Render(label))
		}
	}

	row := lipgloss.JoinHorizontal(lipgloss.Top, intersperse(tabs, tabGap.Render())...)
	bar := tabBarStyle.Width(width).Render(row)
	return bar
}

// intersperse inserts sep between each element of items.
func intersperse(items []string, sep string) []string {
	if len(items) <= 1 {
		return items
	}
	result := make([]string, 0, len(items)*2-1)
	for i, item := range items {
		if i > 0 {
			result = append(result, sep)
		}
		result = append(result, item)
	}
	return result
}

// renderPlaceholder renders placeholder content for a tab.
func renderPlaceholder(tab Tab, width, height int) string {
	msg := tab.String() + " (coming soon)"
	style := lipgloss.NewStyle().
		Width(width).
		Height(height).
		Foreground(lipgloss.Color("241")).
		Align(lipgloss.Center, lipgloss.Center)
	return style.Render(msg)
}

// statusBarStyle renders the daemon connection status bar.
func renderStatusBar(connected bool, activeLayout, defaultLayout string, width int) string {
	var status string
	if connected {
		dot := lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Render("●")
		parts := []string{dot + " daemon connected"}
		if activeLayout != "" {
			parts = append(parts, "active:"+activeLayout)
		}
		if defaultLayout != "" {
			parts = append(parts, "default:"+defaultLayout)
		}
		status = strings.Join(parts, "  ")
	} else {
		dot := lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Render("●")
		status = dot + " daemon not running"
	}

	style := lipgloss.NewStyle().
		Width(width).
		Background(lipgloss.Color("235")).
		Foreground(lipgloss.Color("250")).
		Padding(0, 1)
	return style.Render(status)
}

// renderHelpBar renders the bottom help/keybinding bar.
func renderHelpBar(width int) string {
	help := "tab/shift-tab: switch tabs  1-4: jump to tab  ctrl-s: save  q/ctrl-c: quit"
	style := lipgloss.NewStyle().
		Width(width).
		Foreground(lipgloss.Color("241")).
		Padding(0, 1)
	return style.Render(help)
}
