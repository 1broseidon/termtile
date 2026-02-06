package palette

import (
	"fmt"
	"os/exec"
	"strings"
)

// Item is a single selectable entry in a palette menu.
type Item struct {
	Label     string // Display text
	Action    string // Action identifier returned on selection
	Icon      string // Icon name (e.g., "firefox", "folder") for rofi -show-icons
	Info      string // Hidden data returned on selection (for action IDs)
	Meta      string // Hidden search keywords (rofi meta field)
	IsHeader  bool   // Non-selectable section header (bold)
	IsDivider bool   // Non-selectable divider line (dim)
	IsActive  bool   // Highlighted as current/active (rofi active row)
	IsUrgent  bool   // Highlighted as urgent (rofi urgent row)
}

// SelectResult contains the result of a palette selection.
type SelectResult struct {
	Item     Item
	ExitCode int // 0=normal, 10=kb-custom-1 (Alt+Return), 11=kb-custom-2 (Alt+d)
}

// Capabilities describes what features a backend supports.
type Capabilities struct {
	Icons         bool // Supports icon display
	Markup        bool // Supports pango markup in labels
	NonSelectable bool // Supports non-selectable rows (headers)
	CustomKeys    bool // Supports kb-custom-N keybindings
	IndexOutput   bool // Can output selection index (not just text)
	MessageBar    bool // Supports message/prompt bar
	RowStates     bool // Supports active/urgent row highlighting
}

// Backend shows a palette to the user and returns the selected item.
type Backend interface {
	// Show displays the palette and returns the selected item.
	// prompt: the prompt text shown to the user
	// items: the list of items to display
	// message: optional context message (shown in rofi message bar)
	// Returns: selected item result with exit code, or error
	Show(prompt string, items []Item, message string) (SelectResult, error)

	// Capabilities returns the features supported by this backend.
	Capabilities() Capabilities
}

// AutoDetect selects the first available backend in priority order.
func AutoDetect() (Backend, error) {
	name, err := DetectBackend()
	if err != nil {
		return nil, err
	}
	return NewBackend(name)
}

// NewBackend creates a backend by name.
//
// Supported names: auto, rofi, fuzzel, wofi, dmenu.
func NewBackend(name string) (Backend, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "", "auto":
		return AutoDetect()
	case "rofi":
		if _, err := exec.LookPath("rofi"); err != nil {
			return nil, fmt.Errorf("palette backend %q not found in PATH", "rofi")
		}
		return NewRofiBackend(), nil
	case "fuzzel":
		if _, err := exec.LookPath("fuzzel"); err != nil {
			return nil, fmt.Errorf("palette backend %q not found in PATH", "fuzzel")
		}
		return NewFuzzelBackend(), nil
	case "wofi":
		if _, err := exec.LookPath("wofi"); err != nil {
			return nil, fmt.Errorf("palette backend %q not found in PATH", "wofi")
		}
		return NewWofiBackend(), nil
	case "dmenu":
		if _, err := exec.LookPath("dmenu"); err != nil {
			return nil, fmt.Errorf("palette backend %q not found in PATH", "dmenu")
		}
		return NewDmenuBackend(), nil
	default:
		return nil, fmt.Errorf("unknown palette backend: %q (expected: auto, rofi, fuzzel, wofi, dmenu)", name)
	}
}
