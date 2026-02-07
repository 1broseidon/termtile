package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/1broseidon/termtile/internal/config"
)

// terminalItem is a list item representing a terminal class.
type terminalItem struct {
	class       string
	isDefault   bool
	hasSpawnCmd bool
}

func (i terminalItem) Title() string {
	if i.isDefault {
		return lipgloss.NewStyle().Foreground(lipgloss.Color("226")).Render("★") + " " + i.class
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Render("✓") + " " + i.class
}

func (i terminalItem) Description() string {
	var parts []string
	if i.isDefault {
		parts = append(parts, "default")
	}
	if i.hasSpawnCmd {
		parts = append(parts, "spawn command configured")
	}
	if len(parts) == 0 {
		return "terminal class"
	}
	return strings.Join(parts, " | ")
}

func (i terminalItem) FilterValue() string { return i.class }

// TerminalsTab is the sub-model for the Terminal Classes tab.
type TerminalsTab struct {
	list   list.Model
	cfg    *config.Config
	width  int
	height int

	// Add mode
	adding    bool
	textInput textinput.Model
}

// NewTerminalsTab creates a new TerminalsTab from the loaded config.
func NewTerminalsTab(cfg *config.Config) TerminalsTab {
	items := buildTerminalItems(cfg)

	delegate := list.NewDefaultDelegate()
	delegate.Styles.SelectedTitle = delegate.Styles.SelectedTitle.
		Foreground(lipgloss.Color("15")).
		BorderForeground(lipgloss.Color("62"))
	delegate.Styles.SelectedDesc = delegate.Styles.SelectedDesc.
		Foreground(lipgloss.Color("250")).
		BorderForeground(lipgloss.Color("62"))

	l := list.New(items, delegate, 0, 0)
	l.Title = "Terminal Classes"
	l.Styles.Title = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("15")).
		Background(lipgloss.Color("62")).
		Padding(0, 1)
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(false)
	l.SetShowHelp(false)
	l.KeyMap.Quit.SetEnabled(false)

	ti := textinput.New()
	ti.Placeholder = "e.g. kitty, Alacritty, wezterm"
	ti.CharLimit = 64

	return TerminalsTab{
		list:      l,
		cfg:       cfg,
		textInput: ti,
	}
}

// Init implements tea.Model.
func (t TerminalsTab) Init() tea.Cmd { return nil }

// Update handles messages for the terminals tab.
func (t TerminalsTab) Update(msg tea.Msg) (TerminalsTab, tea.Cmd) {
	if t.adding {
		return t.updateAdding(msg)
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		t.width = msg.Width
		t.height = msg.Height
		leftWidth := t.listWidth()
		t.list.SetSize(leftWidth, t.height)
		return t, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "a":
			t.adding = true
			t.textInput.Reset()
			t.textInput.Focus()
			return t, textinput.Blink
		case "x", "delete":
			if item, ok := t.list.SelectedItem().(terminalItem); ok {
				t.removeTerminalClass(item.class)
				items := buildTerminalItems(t.cfg)
				t.list.SetItems(items)
			}
			return t, nil
		case "d":
			if item, ok := t.list.SelectedItem().(terminalItem); ok {
				t.toggleDefault(item.class)
				items := buildTerminalItems(t.cfg)
				t.list.SetItems(items)
			}
			return t, nil
		}
	}

	var cmd tea.Cmd
	t.list, cmd = t.list.Update(msg)
	return t, cmd
}

func (t TerminalsTab) updateAdding(msg tea.Msg) (TerminalsTab, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			value := strings.TrimSpace(t.textInput.Value())
			if value != "" {
				t.addTerminalClass(value)
				items := buildTerminalItems(t.cfg)
				t.list.SetItems(items)
			}
			t.adding = false
			t.textInput.Blur()
			return t, nil
		case "esc":
			t.adding = false
			t.textInput.Blur()
			return t, nil
		}
	case tea.WindowSizeMsg:
		t.width = msg.Width
		t.height = msg.Height
		return t, nil
	}

	var cmd tea.Cmd
	t.textInput, cmd = t.textInput.Update(msg)
	return t, cmd
}

func (t TerminalsTab) listWidth() int {
	w := t.width * 2 / 5
	if w < 20 {
		w = 20
	}
	return w
}

func (t *TerminalsTab) addTerminalClass(class string) {
	if t.cfg == nil {
		return
	}
	for _, tc := range t.cfg.TerminalClasses {
		if strings.EqualFold(tc.Class, class) {
			return
		}
	}
	t.cfg.TerminalClasses = append(t.cfg.TerminalClasses, config.TerminalClass{Class: class})
}

func (t *TerminalsTab) removeTerminalClass(class string) {
	if t.cfg == nil || len(t.cfg.TerminalClasses) <= 1 {
		return
	}
	for i, tc := range t.cfg.TerminalClasses {
		if tc.Class == class {
			t.cfg.TerminalClasses = append(t.cfg.TerminalClasses[:i], t.cfg.TerminalClasses[i+1:]...)
			return
		}
	}
}

func (t *TerminalsTab) toggleDefault(class string) {
	if t.cfg == nil {
		return
	}
	for i, tc := range t.cfg.TerminalClasses {
		if tc.Class == class {
			t.cfg.TerminalClasses[i].Default = !tc.Default
		} else {
			t.cfg.TerminalClasses[i].Default = false
		}
	}
}

// View implements tea.Model.
func (t TerminalsTab) View() string {
	if t.width == 0 || t.height == 0 {
		return ""
	}

	leftWidth := t.listWidth()
	rightWidth := t.width - leftWidth
	if rightWidth < 10 {
		rightWidth = 10
	}

	var leftContent string
	if t.adding {
		inputStyle := lipgloss.NewStyle().Padding(0, 1).Width(leftWidth)
		prompt := lipgloss.NewStyle().
			Foreground(lipgloss.Color("62")).
			Bold(true).
			Render("Add terminal class:") + "\n" +
			t.textInput.View() + "\n" +
			lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Render("enter: confirm  esc: cancel")
		inputBlock := inputStyle.Render(prompt)
		inputHeight := lipgloss.Height(inputBlock)
		listHeight := t.height - inputHeight
		if listHeight < 1 {
			listHeight = 1
		}
		t.list.SetSize(leftWidth, listHeight)
		leftContent = inputBlock + "\n" + t.list.View()
	} else {
		leftContent = t.list.View()
	}

	left := lipgloss.NewStyle().
		Width(leftWidth).
		Height(t.height).
		Render(leftContent)

	var right string
	if item, ok := t.list.SelectedItem().(terminalItem); ok {
		right = renderTerminalDetail(item, t.cfg, rightWidth, t.height)
	} else {
		right = lipgloss.NewStyle().
			Width(rightWidth).
			Height(t.height).
			Foreground(lipgloss.Color("241")).
			Align(lipgloss.Center, lipgloss.Center).
			Render("No terminal classes configured")
	}

	return lipgloss.JoinHorizontal(lipgloss.Top, left, right)
}

// buildTerminalItems creates list items from config terminal classes.
func buildTerminalItems(cfg *config.Config) []list.Item {
	if cfg == nil {
		return nil
	}
	items := make([]list.Item, 0, len(cfg.TerminalClasses))
	for _, tc := range cfg.TerminalClasses {
		_, hasSpawn := cfg.TerminalSpawnCommands[tc.Class]
		items = append(items, terminalItem{
			class:       tc.Class,
			isDefault:   tc.Default,
			hasSpawnCmd: hasSpawn,
		})
	}
	return items
}

// renderTerminalDetail renders the right-side detail pane for the selected terminal class.
func renderTerminalDetail(item terminalItem, cfg *config.Config, width, height int) string {
	var b strings.Builder

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15"))
	b.WriteString(titleStyle.Render(item.class))
	b.WriteString("\n\n")

	if item.isDefault {
		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("226")).Render("★ default terminal class"))
	} else {
		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Render("● configured"))
	}
	b.WriteString("\n\n")

	labelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("248")).Width(18)
	valueStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("15"))

	field := func(label, value string) {
		b.WriteString(labelStyle.Render(label))
		b.WriteString(valueStyle.Render(value))
		b.WriteString("\n")
	}

	if cfg != nil {
		if cmd, ok := cfg.TerminalSpawnCommands[item.class]; ok {
			field("spawn command:", cmd)
		} else {
			field("spawn command:", "(none)")
		}

		margins := cfg.GetMargins(item.class)
		if margins != (config.Margins{}) {
			field("margins:", fmt.Sprintf("T:%d B:%d L:%d R:%d",
				margins.Top, margins.Bottom, margins.Left, margins.Right))
		}
	}

	b.WriteString("\n")
	helpStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Italic(true)
	b.WriteString(helpStyle.Render("a: add  x: remove  d: toggle default"))

	style := lipgloss.NewStyle().
		Width(width).
		Height(height).
		Padding(1, 2).
		BorderStyle(lipgloss.NormalBorder()).
		BorderLeft(true).
		BorderForeground(lipgloss.Color("236"))

	return style.Render(b.String())
}
