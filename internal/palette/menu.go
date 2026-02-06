package palette

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// MenuItem represents an item in the menu hierarchy.
type MenuItem struct {
	Label     string     // Display label (can include emoji prefix)
	Action    string     // Action identifier (empty for parent items)
	Icon      string     // Icon name for display
	Meta      string     // Hidden search keywords
	IsHeader  bool       // Non-selectable section header (bold)
	IsDivider bool       // Non-selectable divider line (dim)
	IsActive  bool       // Highlight as current/active
	Submenu   []MenuItem // Child items (empty for leaf items)
}

// IsParent returns true if this item has a submenu.
func (m MenuItem) IsParent() bool {
	return len(m.Submenu) > 0
}

// MenuResult contains the result of a menu selection.
type MenuResult struct {
	Action   string // The action identifier of the selected item
	ExitCode int    // Exit code (0=normal, 10=Alt+Return, 11=Alt+d)
}

// Menu handles hierarchical menu navigation using a palette backend.
type Menu struct {
	backend Backend
	root    []MenuItem
	message string // Context message to display
}

// NewMenu creates a new hierarchical menu with the given backend and root items.
func NewMenu(backend Backend, items []MenuItem) *Menu {
	return &Menu{
		backend: backend,
		root:    items,
	}
}

// SetMessage sets a context message to display in the palette (rofi message bar).
func (m *Menu) SetMessage(msg string) {
	m.message = msg
}

// Show displays the menu and handles navigation through submenus.
// Returns the action string of the selected leaf item, or ErrCancelled if user exits.
func (m *Menu) Show() (MenuResult, error) {
	return m.showLevel(m.root, nil)
}

// showLevel displays a menu level and handles navigation.
// breadcrumb is used to track the path for back navigation.
func (m *Menu) showLevel(items []MenuItem, breadcrumb []string) (MenuResult, error) {
	if len(items) == 0 {
		return MenuResult{}, fmt.Errorf("menu: no items to show")
	}

	const submenuPrefix = "__submenu__:"

	for {
		// Build palette items with back option if we're in a submenu
		paletteItems := make([]Item, 0, len(items)+1)

		if len(breadcrumb) > 0 {
			paletteItems = append(paletteItems, Item{
				Label:  "← Back",
				Action: "__back__",
				Icon:   "go-previous",
			})
		}

		for i, item := range items {
			label := item.Label
			icon := item.Icon
			action := item.Action
			if item.IsParent() {
				label += " →"
				if icon == "" {
					icon = "folder"
				}
				action = fmt.Sprintf("%s%d", submenuPrefix, i)
			} else if strings.TrimSpace(action) == "" {
				action = "noop"
			}
			paletteItems = append(paletteItems, Item{
				Label:     label,
				Action:    action,
				Icon:      icon,
				Meta:      item.Meta,
				IsHeader:  item.IsHeader,
				IsDivider: item.IsDivider,
				IsActive:  item.IsActive,
			})
		}

		// Build prompt from breadcrumb
		prompt := "termtile"
		if len(breadcrumb) > 0 {
			prompt = breadcrumb[len(breadcrumb)-1]
		}

		result, err := m.backend.Show(prompt, paletteItems, m.message)
		if err != nil {
			if errors.Is(err, ErrCancelled) {
				// In a submenu, cancel goes back one level.
				if len(breadcrumb) > 0 {
					return MenuResult{}, ErrCancelled
				}
				return MenuResult{}, ErrCancelled
			}
			return MenuResult{}, err
		}

		// Some backends can't enforce non-selectable rows. Treat selecting a header/divider as "no-op" and re-show.
		if result.Item.IsHeader || result.Item.IsDivider {
			continue
		}

		// Handle back navigation
		if result.Item.Action == "__back__" {
			return MenuResult{}, ErrCancelled
		}

		// Submenu navigation
		if strings.HasPrefix(result.Item.Action, submenuPrefix) {
			idxStr := strings.TrimPrefix(result.Item.Action, submenuPrefix)
			idx, err := strconv.Atoi(idxStr)
			if err != nil || idx < 0 || idx >= len(items) || !items[idx].IsParent() {
				continue
			}

			newBreadcrumb := append(breadcrumb, items[idx].Label)
			subResult, err := m.showLevel(items[idx].Submenu, newBreadcrumb)
			if errors.Is(err, ErrCancelled) {
				// User went back, show current level again
				continue
			}
			return subResult, err
		}

		// Leaf item - return the action with exit code
		return MenuResult{
			Action:   result.Item.Action,
			ExitCode: result.ExitCode,
		}, nil
	}
}
