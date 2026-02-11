package config

import (
	"fmt"
	"strings"
)

// Explain returns the effective value at the given YAML-like path and its source.
//
// Supported paths include:
//
//	hotkey
//	terminal_add_hotkey
//	palette_hotkey
//	palette_backend
//	display
//	xauthority
//	preferred_terminal
//	terminal
//	limits.max_terminals_per_workspace
//	limits.max_workspaces
//	limits.max_terminals_total
//	terminal_spawn_commands
//	gap_size
//	screen_padding.top
//	default_layout
//	terminal_classes
//	terminal_sort
//	log_level
//	terminal_margins.<WM_CLASS>.top
//	layouts.<name>.mode
//	layouts.<name>.tile_region.type
//	layouts.<name>.fixed_grid.rows
func Explain(res *LoadResult, path string) (any, Source, error) {
	if res == nil || res.Config == nil {
		return nil, Source{}, fmt.Errorf("no config loaded")
	}
	if path == "" {
		return nil, Source{}, fmt.Errorf("path is empty")
	}

	value, err := lookupValue(res.Config, path)
	if err != nil {
		return nil, Source{}, err
	}

	// Exact-path file source wins.
	if src, ok := res.Sources[path]; ok {
		return value, src, nil
	}

	// Otherwise infer from category.
	if strings.HasPrefix(path, "layouts.") {
		name := layoutNameFromPath(path)
		base := ""
		if name != "" {
			base = res.LayoutBases[name]
		}
		return value, Source{Kind: SourceBuiltin, Name: base}, nil
	}

	return value, Source{Kind: SourceDefault, Name: "defaults"}, nil
}

func layoutNameFromPath(path string) string {
	parts := strings.Split(path, ".")
	if len(parts) < 2 {
		return ""
	}
	if parts[0] != "layouts" {
		return ""
	}
	return parts[1]
}

func lookupValue(cfg *Config, path string) (any, error) {
	parts := strings.Split(path, ".")
	switch parts[0] {
	case "hotkey":
		if len(parts) != 1 {
			return nil, fmt.Errorf("unknown path: %s", path)
		}
		return cfg.Hotkey, nil
	case "cycle_layout_hotkey":
		if len(parts) != 1 {
			return nil, fmt.Errorf("unknown path: %s", path)
		}
		return cfg.CycleLayoutHotkey, nil
	case "cycle_layout_reverse_hotkey":
		if len(parts) != 1 {
			return nil, fmt.Errorf("unknown path: %s", path)
		}
		return cfg.CycleLayoutReverseHotkey, nil
	case "undo_hotkey":
		if len(parts) != 1 {
			return nil, fmt.Errorf("unknown path: %s", path)
		}
		return cfg.UndoHotkey, nil
	case "terminal_add_hotkey":
		if len(parts) != 1 {
			return nil, fmt.Errorf("unknown path: %s", path)
		}
		return cfg.TerminalAddHotkey, nil
	case "palette_hotkey":
		if len(parts) != 1 {
			return nil, fmt.Errorf("unknown path: %s", path)
		}
		return cfg.PaletteHotkey, nil
	case "palette_backend":
		if len(parts) != 1 {
			return nil, fmt.Errorf("unknown path: %s", path)
		}
		return cfg.PaletteBackend, nil
	case "display":
		if len(parts) != 1 {
			return nil, fmt.Errorf("unknown path: %s", path)
		}
		return cfg.Display, nil
	case "xauthority":
		if len(parts) != 1 {
			return nil, fmt.Errorf("unknown path: %s", path)
		}
		return cfg.XAuthority, nil
	case "preferred_terminal":
		if len(parts) != 1 {
			return nil, fmt.Errorf("unknown path: %s", path)
		}
		return cfg.PreferredTerminal, nil
	case "terminal":
		if len(parts) != 1 {
			return nil, fmt.Errorf("unknown path: %s", path)
		}
		return cfg.ResolveTerminal(), nil
	case "terminal_spawn_commands":
		if len(parts) == 1 {
			return cfg.TerminalSpawnCommands, nil
		}
		if len(parts) != 2 {
			return nil, fmt.Errorf("unknown path: %s", path)
		}
		class := parts[1]
		cmd, ok := cfg.TerminalSpawnCommands[class]
		if !ok {
			return nil, fmt.Errorf("unknown terminal_spawn_commands entry %q", class)
		}
		return cmd, nil
	case "gap_size":
		if len(parts) != 1 {
			return nil, fmt.Errorf("unknown path: %s", path)
		}
		return cfg.GapSize, nil
	case "screen_padding":
		if len(parts) == 1 {
			return cfg.ScreenPadding, nil
		}
		if len(parts) != 2 {
			return nil, fmt.Errorf("unknown path: %s", path)
		}
		switch parts[1] {
		case "top":
			return cfg.ScreenPadding.Top, nil
		case "bottom":
			return cfg.ScreenPadding.Bottom, nil
		case "left":
			return cfg.ScreenPadding.Left, nil
		case "right":
			return cfg.ScreenPadding.Right, nil
		default:
			return nil, fmt.Errorf("unknown path: %s", path)
		}
	case "default_layout":
		if len(parts) != 1 {
			return nil, fmt.Errorf("unknown path: %s", path)
		}
		return cfg.DefaultLayout, nil
	case "terminal_classes":
		if len(parts) != 1 {
			return nil, fmt.Errorf("unknown path: %s", path)
		}
		return cfg.TerminalClasses, nil
	case "terminal_sort":
		if len(parts) != 1 {
			return nil, fmt.Errorf("unknown path: %s", path)
		}
		return cfg.TerminalSort, nil
	case "log_level":
		if len(parts) != 1 {
			return nil, fmt.Errorf("unknown path: %s", path)
		}
		return cfg.LogLevel, nil
	case "limits":
		if len(parts) == 1 {
			return cfg.Limits, nil
		}
		if len(parts) == 2 {
			switch parts[1] {
			case "max_terminals_per_workspace":
				return cfg.Limits.MaxTerminalsPerWorkspace, nil
			case "max_workspaces":
				return cfg.Limits.MaxWorkspaces, nil
			case "max_terminals_total":
				return cfg.Limits.MaxTerminalsTotal, nil
			case "workspace_overrides":
				return cfg.Limits.WorkspaceOverrides, nil
			default:
				return nil, fmt.Errorf("unknown path: %s", path)
			}
		}
		if len(parts) >= 3 && parts[1] == "workspace_overrides" {
			name := parts[2]
			override, ok := cfg.Limits.WorkspaceOverrides[name]
			if !ok {
				return nil, fmt.Errorf("unknown limits.workspace_overrides entry %q", name)
			}
			if len(parts) == 3 {
				return override, nil
			}
			if len(parts) == 4 && parts[3] == "max_terminals" {
				return override.MaxTerminals, nil
			}
		}
		return nil, fmt.Errorf("unknown path: %s", path)
	case "terminal_margins":
		if len(parts) == 1 {
			return cfg.TerminalMargins, nil
		}
		if len(parts) < 2 {
			return nil, fmt.Errorf("unknown path: %s", path)
		}
		class := parts[1]
		margins, ok := cfg.TerminalMargins[class]
		if !ok {
			return nil, fmt.Errorf("unknown terminal_margins entry %q", class)
		}
		if len(parts) == 2 {
			return margins, nil
		}
		if len(parts) != 3 {
			return nil, fmt.Errorf("unknown path: %s", path)
		}
		switch parts[2] {
		case "top":
			return margins.Top, nil
		case "bottom":
			return margins.Bottom, nil
		case "left":
			return margins.Left, nil
		case "right":
			return margins.Right, nil
		default:
			return nil, fmt.Errorf("unknown path: %s", path)
		}
	case "layouts":
		if len(parts) < 2 {
			return cfg.Layouts, nil
		}
		name := parts[1]
		layout, ok := cfg.Layouts[name]
		if !ok {
			return nil, fmt.Errorf("unknown layout %q", name)
		}
		if len(parts) == 2 {
			return layout, nil
		}
		if len(parts) < 3 {
			return nil, fmt.Errorf("unknown path: %s", path)
		}
		switch parts[2] {
		case "mode":
			if len(parts) != 3 {
				return nil, fmt.Errorf("unknown path: %s", path)
			}
			return layout.Mode, nil
		case "tile_region":
			if len(parts) == 3 {
				return layout.TileRegion, nil
			}
			if len(parts) != 4 {
				return nil, fmt.Errorf("unknown path: %s", path)
			}
			switch parts[3] {
			case "type":
				return layout.TileRegion.Type, nil
			case "x_percent":
				return layout.TileRegion.XPercent, nil
			case "y_percent":
				return layout.TileRegion.YPercent, nil
			case "width_percent":
				return layout.TileRegion.WidthPercent, nil
			case "height_percent":
				return layout.TileRegion.HeightPercent, nil
			default:
				return nil, fmt.Errorf("unknown path: %s", path)
			}
		case "fixed_grid":
			if len(parts) == 3 {
				return layout.FixedGrid, nil
			}
			if len(parts) != 4 {
				return nil, fmt.Errorf("unknown path: %s", path)
			}
			switch parts[3] {
			case "rows":
				return layout.FixedGrid.Rows, nil
			case "cols":
				return layout.FixedGrid.Cols, nil
			default:
				return nil, fmt.Errorf("unknown path: %s", path)
			}
		case "max_terminal_width":
			if len(parts) != 3 {
				return nil, fmt.Errorf("unknown path: %s", path)
			}
			return layout.MaxTerminalWidth, nil
		case "max_terminal_height":
			if len(parts) != 3 {
				return nil, fmt.Errorf("unknown path: %s", path)
			}
			return layout.MaxTerminalHeight, nil
		default:
			return nil, fmt.Errorf("unknown path: %s", path)
		}
	default:
		return nil, fmt.Errorf("unknown path: %s", path)
	}
}
