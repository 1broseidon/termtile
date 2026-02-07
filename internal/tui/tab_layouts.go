package tui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/1broseidon/termtile/internal/config"
	"github.com/1broseidon/termtile/internal/ipc"
)

// layoutItem implements list.Item for the layout picker sidebar.
type layoutItem struct {
	name      string
	isActive  bool
	isDefault bool
}

func (i layoutItem) Title() string {
	prefix := "  "
	if i.isActive {
		prefix = "* "
	}
	suffix := ""
	if i.isDefault {
		suffix = " (default)"
	}
	return prefix + i.name + suffix
}

func (i layoutItem) Description() string { return "" }
func (i layoutItem) FilterValue() string { return i.name }

// statusMsg is sent after an IPC action completes.
type statusMsg struct {
	text string
}

// clearStatusMsg clears the status message after a delay.
type clearStatusMsg struct{}

// refreshLayoutsMsg triggers a refresh of layout data from daemon.
type refreshLayoutsMsg struct{}

// LayoutsTab is the sub-model for the Layouts browser tab.
type LayoutsTab struct {
	list      list.Model
	ipcClient *ipc.Client
	cfg       *config.Config

	activeLayout  string
	defaultLayout string
	tileCount     int

	statusText string

	width  int
	height int
	ready  bool
}

// NewLayoutsTab creates a new LayoutsTab sub-model.
func NewLayoutsTab(ipcClient *ipc.Client, cfg *config.Config, activeLayout, defaultLayout string) LayoutsTab {
	items := buildLayoutItems(cfg, activeLayout, defaultLayout)

	delegate := list.NewDefaultDelegate()
	delegate.ShowDescription = false
	delegate.SetSpacing(0)

	l := list.New(items, delegate, 0, 0)
	l.Title = "Layouts"
	l.SetShowStatusBar(false)
	l.SetShowHelp(false)
	l.SetFilteringEnabled(false)
	l.DisableQuitKeybindings()

	return LayoutsTab{
		list:          l,
		ipcClient:     ipcClient,
		cfg:           cfg,
		activeLayout:  activeLayout,
		defaultLayout: defaultLayout,
		tileCount:     4,
	}
}

func buildLayoutItems(cfg *config.Config, activeLayout, defaultLayout string) []list.Item {
	if cfg == nil {
		return nil
	}

	names := make([]string, 0, len(cfg.Layouts))
	for name := range cfg.Layouts {
		names = append(names, name)
	}
	sort.Strings(names)

	items := make([]list.Item, 0, len(names))
	for _, name := range names {
		items = append(items, layoutItem{
			name:      name,
			isActive:  name == activeLayout,
			isDefault: name == defaultLayout,
		})
	}
	return items
}

// Init implements tea.Model.
func (lt LayoutsTab) Init() tea.Cmd {
	return nil
}

// Update implements tea.Model.
func (lt LayoutsTab) Update(msg tea.Msg) (LayoutsTab, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		lt.width = msg.Width
		lt.height = msg.Height
		lt.updateListSize()
		lt.ready = true
		return lt, nil

	case statusMsg:
		lt.statusText = msg.text
		return lt, tea.Tick(3*time.Second, func(time.Time) tea.Msg {
			return clearStatusMsg{}
		})

	case clearStatusMsg:
		lt.statusText = ""
		return lt, nil

	case refreshLayoutsMsg:
		lt.refreshFromDaemon()
		lt.rebuildItems()
		return lt, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "enter", "a":
			return lt.applySelected()
		case "d":
			return lt.setDefaultSelected()
		case "p":
			return lt.previewSelected()
		// Tile count shortcuts — these override the tab-switching keys
		// only when the layouts tab is active
		case "2":
			lt.tileCount = 2
			return lt, nil
		case "4":
			lt.tileCount = 4
			return lt, nil
		case "6":
			lt.tileCount = 6
			return lt, nil
		case "9":
			lt.tileCount = 9
			return lt, nil
		}
	}

	var cmd tea.Cmd
	lt.list, cmd = lt.list.Update(msg)
	return lt, cmd
}

func (lt *LayoutsTab) updateListSize() {
	sidebarWidth := lt.sidebarWidth()
	// Reserve 2 lines for status bar at bottom of the tab content
	listHeight := lt.height - 2
	if listHeight < 1 {
		listHeight = 1
	}
	lt.list.SetSize(sidebarWidth, listHeight)
}

func (lt LayoutsTab) sidebarWidth() int {
	// Sidebar takes ~35% of width, min 20, max 40
	sw := lt.width * 35 / 100
	if sw < 20 {
		sw = 20
	}
	if sw > 40 {
		sw = 40
	}
	return sw
}

func (lt LayoutsTab) selectedName() string {
	item, ok := lt.list.SelectedItem().(layoutItem)
	if !ok {
		return ""
	}
	return item.name
}

func (lt LayoutsTab) applySelected() (LayoutsTab, tea.Cmd) {
	name := lt.selectedName()
	if name == "" {
		return lt, nil
	}
	if lt.ipcClient == nil {
		lt.statusText = "daemon not connected"
		return lt, tea.Tick(3*time.Second, func(time.Time) tea.Msg {
			return clearStatusMsg{}
		})
	}
	if err := lt.ipcClient.ApplyLayout(name, true); err != nil {
		lt.statusText = fmt.Sprintf("error: %v", err)
	} else {
		lt.activeLayout = name
		lt.statusText = fmt.Sprintf("applied: %s", name)
		lt.rebuildItems()
	}
	return lt, tea.Tick(3*time.Second, func(time.Time) tea.Msg {
		return clearStatusMsg{}
	})
}

func (lt LayoutsTab) setDefaultSelected() (LayoutsTab, tea.Cmd) {
	name := lt.selectedName()
	if name == "" {
		return lt, nil
	}
	if lt.ipcClient == nil {
		lt.statusText = "daemon not connected"
		return lt, tea.Tick(3*time.Second, func(time.Time) tea.Msg {
			return clearStatusMsg{}
		})
	}
	if err := lt.ipcClient.SetDefaultLayout(name, false); err != nil {
		lt.statusText = fmt.Sprintf("error: %v", err)
	} else {
		lt.defaultLayout = name
		lt.statusText = fmt.Sprintf("default set: %s", name)
		lt.rebuildItems()
	}
	return lt, tea.Tick(3*time.Second, func(time.Time) tea.Msg {
		return clearStatusMsg{}
	})
}

func (lt LayoutsTab) previewSelected() (LayoutsTab, tea.Cmd) {
	name := lt.selectedName()
	if name == "" {
		return lt, nil
	}
	if lt.ipcClient == nil {
		lt.statusText = "daemon not connected"
		return lt, tea.Tick(3*time.Second, func(time.Time) tea.Msg {
			return clearStatusMsg{}
		})
	}
	if err := lt.ipcClient.PreviewLayout(name, 5); err != nil {
		lt.statusText = fmt.Sprintf("error: %v", err)
	} else {
		lt.statusText = fmt.Sprintf("previewing: %s (5s)", name)
	}
	return lt, tea.Tick(3*time.Second, func(time.Time) tea.Msg {
		return clearStatusMsg{}
	})
}

func (lt *LayoutsTab) refreshFromDaemon() {
	if lt.ipcClient == nil {
		return
	}
	data, err := lt.ipcClient.ListLayouts()
	if err != nil {
		return
	}
	lt.activeLayout = data.ActiveLayout
	lt.defaultLayout = data.DefaultLayout
}

func (lt *LayoutsTab) rebuildItems() {
	items := buildLayoutItems(lt.cfg, lt.activeLayout, lt.defaultLayout)
	lt.list.SetItems(items)
}

// View implements tea.Model.
func (lt LayoutsTab) View() string {
	if !lt.ready || lt.width == 0 || lt.height == 0 {
		return ""
	}

	sidebarWidth := lt.sidebarWidth()
	previewWidth := lt.width - sidebarWidth - 3 // 3 for separator + padding
	if previewWidth < 10 {
		previewWidth = 10
	}

	// Render sidebar (layout list)
	sidebar := lt.list.View()
	sidebarStyle := lipgloss.NewStyle().
		Width(sidebarWidth).
		Height(lt.height - 2) // reserve for status
	sidebar = sidebarStyle.Render(sidebar)

	// Render preview pane
	preview := lt.renderPreview(previewWidth)

	// Separator
	sep := lipgloss.NewStyle().
		Foreground(lipgloss.Color("238")).
		Render(strings.Repeat("│\n", lt.height-2))

	// Join columns
	columns := lipgloss.JoinHorizontal(lipgloss.Top, sidebar, " "+sep, preview)

	// Status / help bar for this tab
	status := lt.renderTabStatus()

	return lipgloss.JoinVertical(lipgloss.Left, columns, status)
}

func (lt LayoutsTab) renderPreview(previewWidth int) string {
	name := lt.selectedName()
	if name == "" || lt.cfg == nil {
		return ""
	}

	layout, ok := lt.cfg.Layouts[name]
	if !ok {
		return ""
	}

	// Title
	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("15")).
		Render(fmt.Sprintf(" %s  [%d tiles]", name, lt.tileCount))

	// Summary line
	summary := lipgloss.NewStyle().
		Foreground(lipgloss.Color("250")).
		Render(" " + summarizeLayout(&layout, lt.tileCount, 8))

	// ASCII preview
	previewHeight := lt.height - 6 // title + summary + status + padding
	if previewHeight < 5 {
		previewHeight = 5
	}
	asciiWidth := previewWidth - 2
	if asciiWidth < 5 {
		asciiWidth = 5
	}
	lines := renderASCIIPreview(&layout, lt.tileCount, asciiWidth, previewHeight)

	previewStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("247"))
	previewBlock := previewStyle.Render(strings.Join(lines, "\n"))

	return lipgloss.JoinVertical(lipgloss.Left, title, summary, "", previewBlock)
}

func (lt LayoutsTab) renderTabStatus() string {
	left := ""
	if lt.statusText != "" {
		left = lipgloss.NewStyle().
			Foreground(lipgloss.Color("42")).
			Render(lt.statusText)
	}

	right := lipgloss.NewStyle().
		Foreground(lipgloss.Color("241")).
		Render(fmt.Sprintf("tiles:%d  enter/a:apply  d:default  p:preview  2/4/6/9:tiles", lt.tileCount))

	gap := lt.width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
	}

	return lipgloss.NewStyle().
		Width(lt.width).
		Padding(0, 1).
		Render(left + strings.Repeat(" ", gap) + right)
}
