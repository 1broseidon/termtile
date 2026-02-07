package tui

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"

	"github.com/1broseidon/termtile/internal/config"
)

// GeneralTab is the sub-model for the General settings tab.
type GeneralTab struct {
	cfg *config.Config

	// Display dimensions
	width  int
	height int

	// Edit mode
	editing bool
	form    *huh.Form

	// Form-bound values (strings for huh, converted on submit)
	fGapSize           string
	fDefaultLayout     string
	fPreferredTerminal string
	fHotkey            string
	fTerminalSort      string
	fPaddingTop        string
	fPaddingBottom     string
	fPaddingLeft       string
	fPaddingRight      string
}

// NewGeneralTab creates a GeneralTab from the loaded config.
func NewGeneralTab(cfg *config.Config) GeneralTab {
	return GeneralTab{cfg: cfg}
}

// SetConfig updates the config reference.
func (g *GeneralTab) SetConfig(cfg *config.Config) {
	g.cfg = cfg
}

// Init implements tea.Model.
func (g GeneralTab) Init() tea.Cmd {
	return nil
}

// Update implements tea.Model.
func (g GeneralTab) Update(msg tea.Msg) (GeneralTab, tea.Cmd) {
	if g.editing {
		return g.updateEditing(msg)
	}
	return g.updateDisplay(msg)
}

func (g GeneralTab) updateDisplay(msg tea.Msg) (GeneralTab, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "e" {
			g.startEditing()
			return g, g.form.Init()
		}
	case tea.WindowSizeMsg:
		g.width = msg.Width
		g.height = msg.Height
	}
	return g, nil
}

func (g GeneralTab) updateEditing(msg tea.Msg) (GeneralTab, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "esc" {
			g.editing = false
			g.form = nil
			return g, nil
		}
	case tea.WindowSizeMsg:
		g.width = msg.Width
		g.height = msg.Height
	}

	form, cmd := g.form.Update(msg)
	if f, ok := form.(*huh.Form); ok {
		g.form = f
	}

	if g.form.State == huh.StateCompleted {
		g.applyForm()
		g.editing = false
		g.form = nil
		return g, nil
	}

	return g, cmd
}

func (g *GeneralTab) startEditing() {
	cfg := g.cfg
	if cfg == nil {
		cfg = config.DefaultConfig()
	}

	g.fGapSize = strconv.Itoa(cfg.GapSize)
	g.fDefaultLayout = cfg.DefaultLayout
	g.fPreferredTerminal = cfg.PreferredTerminal
	g.fHotkey = cfg.Hotkey
	g.fTerminalSort = cfg.TerminalSort
	g.fPaddingTop = strconv.Itoa(cfg.ScreenPadding.Top)
	g.fPaddingBottom = strconv.Itoa(cfg.ScreenPadding.Bottom)
	g.fPaddingLeft = strconv.Itoa(cfg.ScreenPadding.Left)
	g.fPaddingRight = strconv.Itoa(cfg.ScreenPadding.Right)

	// Build layout options from config
	layoutOpts := g.layoutOptions()

	sortOpts := []huh.Option[string]{
		huh.NewOption("position", "position"),
		huh.NewOption("window_id", "window_id"),
		huh.NewOption("client_list", "client_list"),
		huh.NewOption("active_first", "active_first"),
	}

	w := g.width - 4
	if w < 40 {
		w = 40
	}

	g.form = huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Key("hotkey").
				Title("Hotkey").
				Description("X11 keybinding to trigger tiling").
				Value(&g.fHotkey),

			huh.NewInput().
				Key("gap_size").
				Title("Gap Size").
				Description("Pixels between tiled windows").
				Value(&g.fGapSize),

			huh.NewSelect[string]().
				Key("default_layout").
				Title("Default Layout").
				Description("Layout applied on startup").
				Options(layoutOpts...).
				Value(&g.fDefaultLayout),

			huh.NewInput().
				Key("preferred_terminal").
				Title("Preferred Terminal").
				Description("Terminal emulator class to prefer").
				Value(&g.fPreferredTerminal),

			huh.NewSelect[string]().
				Key("terminal_sort").
				Title("Terminal Sort").
				Description("How to order terminals for tiling").
				Options(sortOpts...).
				Value(&g.fTerminalSort),
		),
		huh.NewGroup(
			huh.NewInput().
				Key("padding_top").
				Title("Screen Padding: Top").
				Value(&g.fPaddingTop),
			huh.NewInput().
				Key("padding_bottom").
				Title("Screen Padding: Bottom").
				Value(&g.fPaddingBottom),
			huh.NewInput().
				Key("padding_left").
				Title("Screen Padding: Left").
				Value(&g.fPaddingLeft),
			huh.NewInput().
				Key("padding_right").
				Title("Screen Padding: Right").
				Value(&g.fPaddingRight),
		),
	).WithWidth(w).WithShowHelp(true).WithShowErrors(true)

	g.editing = true
}

func (g *GeneralTab) layoutOptions() []huh.Option[string] {
	cfg := g.cfg
	if cfg == nil {
		cfg = config.DefaultConfig()
	}

	names := make([]string, 0, len(cfg.Layouts))
	for name := range cfg.Layouts {
		names = append(names, name)
	}
	sort.Strings(names)

	opts := make([]huh.Option[string], 0, len(names))
	for _, name := range names {
		opts = append(opts, huh.NewOption(name, name))
	}
	return opts
}

func (g *GeneralTab) applyForm() {
	if g.cfg == nil {
		return
	}

	if v, err := strconv.Atoi(g.fGapSize); err == nil && v >= 0 {
		g.cfg.GapSize = v
	}
	if g.fDefaultLayout != "" {
		g.cfg.DefaultLayout = g.fDefaultLayout
	}
	g.cfg.PreferredTerminal = g.fPreferredTerminal
	if g.fHotkey != "" {
		g.cfg.Hotkey = g.fHotkey
	}
	if g.fTerminalSort != "" {
		g.cfg.TerminalSort = g.fTerminalSort
	}
	if v, err := strconv.Atoi(g.fPaddingTop); err == nil && v >= 0 {
		g.cfg.ScreenPadding.Top = v
	}
	if v, err := strconv.Atoi(g.fPaddingBottom); err == nil && v >= 0 {
		g.cfg.ScreenPadding.Bottom = v
	}
	if v, err := strconv.Atoi(g.fPaddingLeft); err == nil && v >= 0 {
		g.cfg.ScreenPadding.Left = v
	}
	if v, err := strconv.Atoi(g.fPaddingRight); err == nil && v >= 0 {
		g.cfg.ScreenPadding.Right = v
	}
}

// View implements tea.Model.
func (g GeneralTab) View() string {
	if g.editing && g.form != nil {
		return g.viewEditing()
	}
	return g.viewDisplay()
}

func (g GeneralTab) viewDisplay() string {
	cfg := g.cfg
	if cfg == nil {
		style := lipgloss.NewStyle().
			Width(g.width).
			Height(g.height).
			Foreground(lipgloss.Color("241")).
			Align(lipgloss.Center, lipgloss.Center)
		return style.Render("No config loaded")
	}

	labelStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("250")).
		Width(22).
		Align(lipgloss.Right).
		PaddingRight(2)

	valueStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("15")).
		Bold(true)

	dimStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("241"))

	row := func(label, value string) string {
		return labelStyle.Render(label) + valueStyle.Render(value)
	}

	padding := fmt.Sprintf("top:%d bottom:%d left:%d right:%d",
		cfg.ScreenPadding.Top, cfg.ScreenPadding.Bottom,
		cfg.ScreenPadding.Left, cfg.ScreenPadding.Right)

	lines := []string{
		"",
		row("Hotkey", cfg.Hotkey),
		row("Move Mode Hotkey", cfg.MoveModeHotkey),
		row("Palette Hotkey", cfg.PaletteHotkey),
		"",
		row("Gap Size", strconv.Itoa(cfg.GapSize)),
		row("Screen Padding", padding),
		row("Default Layout", cfg.DefaultLayout),
		row("Terminal Sort", cfg.TerminalSort),
		"",
		row("Preferred Terminal", displayOrDefault(cfg.PreferredTerminal, "(auto)")),
		row("Palette Backend", cfg.PaletteBackend),
		row("Log Level", cfg.LogLevel),
		"",
		dimStyle.Render("  Press 'e' to edit settings"),
	}

	content := strings.Join(lines, "\n")

	contentStyle := lipgloss.NewStyle().
		Width(g.width).
		Height(g.height).
		Padding(1, 2)

	return contentStyle.Render(content)
}

func (g GeneralTab) viewEditing() string {
	header := lipgloss.NewStyle().
		Foreground(lipgloss.Color("62")).
		Bold(true).
		Render("Editing General Settings") +
		lipgloss.NewStyle().
			Foreground(lipgloss.Color("241")).
			Render("  (esc to cancel)")

	formView := g.form.View()

	content := header + "\n\n" + formView

	style := lipgloss.NewStyle().
		Width(g.width).
		Height(g.height).
		Padding(1, 2)

	return style.Render(content)
}

func displayOrDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}
