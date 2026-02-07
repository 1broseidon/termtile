package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
)

// TUI represents the terminal user interface.
type TUI struct {
	configPath string
}

// New creates a new TUI instance.
func New(configPath string) *TUI {
	return &TUI{
		configPath: configPath,
	}
}

// Run starts the bubbletea TUI program.
func (t *TUI) Run() error {
	m := newModel(t.configPath)
	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("tui error: %w", err)
	}
	return nil
}
