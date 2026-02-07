package main

import (
	"errors"
	"flag"
	"fmt"
	"html"
	"os"
	"sort"
	"strings"

	"github.com/1broseidon/termtile/internal/config"
	"github.com/1broseidon/termtile/internal/ipc"
	"github.com/1broseidon/termtile/internal/palette"
	"github.com/1broseidon/termtile/internal/platform"
	"github.com/1broseidon/termtile/internal/workspace"
)

func runPalette(args []string) int {
	fs := flag.NewFlagSet("palette", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	path := fs.String("path", "", "Config file path (default: ~/.config/termtile/config.yaml)")
	tileNow := fs.Bool("tile", true, "Tile immediately after applying layout selection")

	if len(args) > 0 && (args[0] == "help" || args[0] == "-h" || args[0] == "--help") {
		fmt.Fprintln(os.Stderr, "Usage: termtile palette [--path PATH] [--tile]")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Show a command palette for termtile actions.")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Menu options:")
		fmt.Fprintln(os.Stderr, "  Workspaces - Load, save, or close workspace sessions")
		fmt.Fprintln(os.Stderr, "  Layouts    - Switch between tiling layouts")
		fmt.Fprintln(os.Stderr, "  Settings   - Quick settings toggles")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Keybindings (rofi only):")
		fmt.Fprintln(os.Stderr, "  Enter      - Select item")
		fmt.Fprintln(os.Stderr, "  Alt+Enter  - Secondary action (edit/open)")
		fmt.Fprintln(os.Stderr, "  Alt+d      - Delete action")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Backends: rofi, dmenu, wofi, fuzzel (configured via palette_backend, default: auto).")
		return 0
	}

	if err := fs.Parse(args); err != nil {
		return 2
	}

	var res *config.LoadResult
	var err error
	if *path == "" {
		res, err = config.LoadWithSources()
	} else {
		res, err = config.LoadFromPath(*path)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	backend, err := palette.NewBackend(res.Config.PaletteBackend)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if setter, ok := backend.(interface{ SetFuzzyMatching(bool) }); ok {
		setter.SetFuzzyMatching(res.Config.PaletteFuzzyMatching)
	}

	// Build context information for the message bar
	message := buildPaletteMessage(buildContextMessage(res.Config))

	// Build the hierarchical menu
	menu := palette.NewMenu(backend, buildRootMenu(res.Config))
	menu.SetMessage(message)

	result, err := menu.Show()
	if err != nil {
		if errors.Is(err, palette.ErrCancelled) {
			return 0
		}
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	// Execute the selected action based on exit code
	return executeAction(result.Action, result.ExitCode, *tileNow)
}

func buildContextMessage(cfg *config.Config) string {
	var parts []string

	// Current desktop
	if desktop, err := platform.GetCurrentDesktopStandalone(); err == nil {
		parts = append(parts, fmt.Sprintf("Desktop %d", desktop))
	}

	// Current workspace
	if ws, err := workspace.GetActiveWorkspace(); err == nil && ws.Name != "" {
		wsInfo := ws.Name
		if ws.AgentMode {
			wsInfo += " (agent)"
		}
		parts = append(parts, wsInfo)
	}

	// Terminal count from status
	client := ipc.NewClient()
	if status, err := client.GetStatus(); err == nil {
		parts = append(parts, fmt.Sprintf("%d terminals", status.TerminalCount))
		if status.ActiveLayout != "" {
			parts = append(parts, status.ActiveLayout)
		}
	}

	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, " • ")
}

func buildPaletteMessage(contextLine string) string {
	const hints = "<span size='small'>Alt+Enter: secondary action | Alt+d: delete</span>"

	contextLine = strings.TrimSpace(contextLine)
	if contextLine == "" {
		return hints
	}

	return fmt.Sprintf("%s\n%s", html.EscapeString(contextLine), hints)
}

func buildRootMenu(cfg *config.Config) []palette.MenuItem {
	return []palette.MenuItem{
		{
			Label:   "Workspaces",
			Icon:    "folder-open",
			Submenu: buildWorkspacesMenu(),
		},
		{
			Label:   "Layouts",
			Icon:    "view-grid-symbolic",
			Submenu: buildLayoutsMenu(cfg),
		},
		{
			Label:   "Settings",
			Icon:    "preferences-system",
			Submenu: buildSettingsMenu(),
		},
	}
}

func buildLayoutsMenu(cfg *config.Config) []palette.MenuItem {
	layoutNames := make([]string, 0, len(cfg.Layouts))
	for name := range cfg.Layouts {
		layoutNames = append(layoutNames, name)
	}
	sort.Strings(layoutNames)

	// Get current layout for highlighting
	currentLayout := ""
	client := ipc.NewClient()
	if status, err := client.GetStatus(); err == nil {
		currentLayout = status.ActiveLayout
	}

	items := make([]palette.MenuItem, 0, len(layoutNames))
	for _, name := range layoutNames {
		icon := getLayoutIcon(name)
		items = append(items, palette.MenuItem{
			Label:    name,
			Action:   "layout:" + name,
			Icon:     icon,
			IsActive: name == currentLayout,
			Meta:     "layout tile grid arrange " + name,
		})
	}

	if len(items) == 0 {
		items = append(items, palette.MenuItem{
			Label:    "(no layouts configured)",
			Action:   "noop",
			Icon:     "dialog-warning",
			IsHeader: true,
		})
	}

	return items
}

func getLayoutIcon(name string) string {
	// Map common layout names to appropriate icons
	nameLower := strings.ToLower(name)
	switch {
	case strings.Contains(nameLower, "grid"):
		return "view-grid-symbolic"
	case strings.Contains(nameLower, "stack") || strings.Contains(nameLower, "master"):
		return "view-dual-symbolic"
	case strings.Contains(nameLower, "full") || strings.Contains(nameLower, "max"):
		return "view-fullscreen-symbolic"
	case strings.Contains(nameLower, "column") || strings.Contains(nameLower, "vertical"):
		return "view-column-symbolic"
	case strings.Contains(nameLower, "row") || strings.Contains(nameLower, "horizontal"):
		return "view-continuous-symbolic"
	case strings.Contains(nameLower, "float"):
		return "view-restore-symbolic"
	default:
		return "view-app-grid-symbolic"
	}
}

func buildWorkspacesMenu() []palette.MenuItem {
	var items []palette.MenuItem

	// Get current desktop for context
	currentDesktop := 0
	if d, err := platform.GetCurrentDesktopStandalone(); err == nil {
		currentDesktop = d
	} else {
		items = append(items, palette.MenuItem{
			Label:    "(failed to query current desktop)",
			Action:   "noop",
			Icon:     "dialog-warning",
			IsHeader: true,
		})
	}

	// Get all open workspaces
	openWorkspaces, err := workspace.GetAllWorkspaces()
	if err != nil {
		items = append(items, palette.MenuItem{
			Label:    "(failed to load open workspaces)",
			Action:   "noop",
			Icon:     "dialog-warning",
			IsHeader: true,
		})
	}

	// Get current desktop's workspace (if any)
	currentWs, currentWsErr := workspace.GetActiveWorkspace()
	if currentWsErr != nil {
		items = append(items, palette.MenuItem{
			Label:    "(failed to query active workspace)",
			Action:   "noop",
			Icon:     "dialog-warning",
			IsHeader: true,
		})
	}

	// If workspace on current desktop, show context menu for it
	if currentWs.Name != "" {
		// Show current desktop's workspace as primary context
		label := fmt.Sprintf("[Desktop %d] %s", currentDesktop, currentWs.Name)
		if currentWs.AgentMode {
			label += " (agent)"
		}
		items = append(items, palette.MenuItem{
			Label:    label,
			Action:   "noop",
			Icon:     "folder-open",
			IsActive: true,
			IsHeader: true,
		})

		// Check for unsaved changes
		hasChanges := workspace.HasUnsavedChanges(currentWs.Name)
		if hasChanges {
			// State differs from saved - offer save options
			items = append(items,
				palette.MenuItem{
					Label:  "  Save & close",
					Action: "workspace:save-close:" + currentWs.Name,
					Icon:   "document-save",
					Meta:   "save close workspace session",
				},
				palette.MenuItem{
					Label:  "  Close without saving",
					Action: "workspace:close:" + currentWs.Name,
					Icon:   "window-close",
					Meta:   "close discard workspace",
				},
			)
		} else {
			// State matches saved - just offer close
			items = append(items, palette.MenuItem{
				Label:  "  Close workspace",
				Action: "workspace:close:" + currentWs.Name,
				Icon:   "window-close",
				Meta:   "close workspace session",
			})
		}

		items = append(items, palette.MenuItem{
			Label:     "────────────────",
			Action:    "noop",
			IsDivider: true,
		})
	}

	// Show other open workspaces (on other desktops) - sorted by desktop number
	var otherDesktops []int
	for desktop := range openWorkspaces {
		if desktop != currentDesktop {
			otherDesktops = append(otherDesktops, desktop)
		}
	}
	sort.Ints(otherDesktops)

	for _, desktop := range otherDesktops {
		ws := openWorkspaces[desktop]
		label := fmt.Sprintf("[Desktop %d] %s", desktop, ws.Name)
		if ws.AgentMode {
			label += " (agent)"
		}
		label += " (switch to manage)"
		items = append(items, palette.MenuItem{
			Label:    label,
			Action:   "noop", // Can't manage workspace on different desktop from here
			Icon:     "folder",
			Meta:     fmt.Sprintf("desktop %d workspace %s", desktop, ws.Name),
			IsHeader: true,
		})
	}

	// Separator if we showed any open workspaces
	if len(openWorkspaces) > 0 && len(items) > 0 {
		// Check if we already have a separator at the end
		if !strings.HasPrefix(items[len(items)-1].Label, "──") {
			items = append(items, palette.MenuItem{
				Label:     "────────────────",
				Action:    "noop",
				IsDivider: true,
			})
		}
	}

	// List saved workspaces available to load
	workspaces, err := workspace.List()
	if err != nil {
		items = append(items, palette.MenuItem{
			Label:    "(failed to list saved workspaces)",
			Action:   "noop",
			Icon:     "dialog-warning",
			IsHeader: true,
		})
	} else {
		for _, name := range workspaces {
			// Skip internal workspaces
			if name == "_previous" {
				continue
			}

			// Check if this workspace is already open on any desktop
			isOpen := false
			for _, ws := range openWorkspaces {
				if ws.Name == name {
					isOpen = true
					break
				}
			}
			if isOpen {
				continue // Already shown above
			}

			items = append(items, palette.MenuItem{
				Label:  name,
				Action: "workspace:load:" + name,
				Icon:   "folder",
				Meta:   "load open workspace session " + name,
			})
		}
	}

	if len(items) == 0 {
		return []palette.MenuItem{
			{
				Label:    "(no saved workspaces)",
				Action:   "noop",
				Icon:     "dialog-information",
				IsHeader: true,
			},
		}
	}

	return items
}

func buildSettingsMenu() []palette.MenuItem {
	return []palette.MenuItem{
		{
			Label:  "Reload config",
			Action: "reload",
			Icon:   "view-refresh",
			Meta:   "reload refresh config configuration",
		},
		{
			Label:  "Undo last tile",
			Action: "undo",
			Icon:   "edit-undo",
			Meta:   "undo restore previous",
		},
		{
			Label:  "Show status",
			Action: "status",
			Icon:   "dialog-information",
			Meta:   "status info information",
		},
	}
}

func executeAction(action string, exitCode int, tileNow bool) int {
	// Handle secondary actions (Alt+Return = 10, Alt+d = 11)
	switch exitCode {
	case palette.ExitCustom1: // Alt+Return - secondary action
		return executeSecondaryAction(action)
	case palette.ExitCustom2: // Alt+d - delete action
		return executeDeleteAction(action)
	}

	// Normal selection (exitCode 0)
	return executePrimaryAction(action, tileNow)
}

func executePrimaryAction(action string, tileNow bool) int {
	switch {
	case action == "noop":
		return 0

	case strings.HasPrefix(action, "layout:"):
		layoutName := strings.TrimPrefix(action, "layout:")
		client := ipc.NewClient()
		if err := client.ApplyLayout(layoutName, tileNow); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		return 0

	case strings.HasPrefix(action, "workspace:load:"):
		wsName := strings.TrimPrefix(action, "workspace:load:")
		return runWorkspace([]string{"load", wsName})

	case strings.HasPrefix(action, "workspace:close:"):
		wsName := strings.TrimPrefix(action, "workspace:close:")
		return runWorkspace([]string{"close", wsName})

	case strings.HasPrefix(action, "workspace:save-close:"):
		wsName := strings.TrimPrefix(action, "workspace:save-close:")
		// Save first, then close
		if ret := runWorkspace([]string{"save", wsName}); ret != 0 {
			return ret
		}
		return runWorkspace([]string{"close", wsName})

	case action == "reload":
		client := ipc.NewClient()
		if err := client.Reload(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		fmt.Println("Config reloaded")
		return 0

	case action == "undo":
		return runUndo(nil)

	case action == "status":
		client := ipc.NewClient()
		status, err := client.GetStatus()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		fmt.Printf("Layout: %s\nTerminals: %d\nUptime: %ds\n",
			status.ActiveLayout, status.TerminalCount, status.UptimeSeconds)
		return 0

	default:
		fmt.Fprintf(os.Stderr, "palette: unknown action %q\n", action)
		return 1
	}
}

func executeSecondaryAction(action string) int {
	// Alt+Return secondary actions
	switch {
	case strings.HasPrefix(action, "workspace:load:"):
		// Secondary action on workspace: open in new desktop (future)
		wsName := strings.TrimPrefix(action, "workspace:load:")
		fmt.Printf("Secondary action on workspace %q (not yet implemented)\n", wsName)
		return 0

	case strings.HasPrefix(action, "layout:"):
		// Secondary action on layout: edit layout config (future)
		layoutName := strings.TrimPrefix(action, "layout:")
		fmt.Printf("Edit layout %q (not yet implemented)\n", layoutName)
		return 0

	default:
		// Fall back to primary action
		return executePrimaryAction(action, true)
	}
}

func executeDeleteAction(action string) int {
	// Alt+d delete actions
	switch {
	case strings.HasPrefix(action, "workspace:load:"):
		// Delete workspace
		wsName := strings.TrimPrefix(action, "workspace:load:")
		return runWorkspace([]string{"delete", wsName})

	default:
		fmt.Fprintf(os.Stderr, "palette: delete not supported for action %q\n", action)
		return 1
	}
}
