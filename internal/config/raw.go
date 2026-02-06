package config

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// IncludeList supports either:
//
//	include: "/path/to/file.yaml"
//
// or:
//
//	include:
//	  - "/path/to/file.yaml"
//	  - "/path/to/dir"
type IncludeList []string

func (l *IncludeList) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case 0:
		// Not present.
		*l = nil
		return nil
	case yaml.ScalarNode:
		if value.Tag != "!!str" {
			return fmt.Errorf("include must be a string or list of strings")
		}
		*l = []string{value.Value}
		return nil
	case yaml.SequenceNode:
		out := make([]string, 0, len(value.Content))
		for _, item := range value.Content {
			if item.Kind != yaml.ScalarNode || item.Tag != "!!str" {
				return fmt.Errorf("include entries must be strings")
			}
			out = append(out, item.Value)
		}
		*l = out
		return nil
	default:
		return fmt.Errorf("include must be a string or list of strings")
	}
}

type RawMargins struct {
	Top    *int `yaml:"top"`
	Bottom *int `yaml:"bottom"`
	Left   *int `yaml:"left"`
	Right  *int `yaml:"right"`
}

type RawFixedGrid struct {
	Rows *int `yaml:"rows"`
	Cols *int `yaml:"cols"`
}

type RawTileRegion struct {
	Type          *RegionType `yaml:"type"`
	XPercent      *int        `yaml:"x_percent"`
	YPercent      *int        `yaml:"y_percent"`
	WidthPercent  *int        `yaml:"width_percent"`
	HeightPercent *int        `yaml:"height_percent"`
}

type RawLayout struct {
	Inherits          *string        `yaml:"inherits"`
	Mode              *LayoutMode    `yaml:"mode"`
	TileRegion        *RawTileRegion `yaml:"tile_region"`
	FixedGrid         *RawFixedGrid  `yaml:"fixed_grid"`
	MaxTerminalWidth  *int           `yaml:"max_terminal_width"`
	MaxTerminalHeight *int           `yaml:"max_terminal_height"`
	FlexibleLastRow   *bool          `yaml:"flexible_last_row"`
}

type RawWorkspaceLimit struct {
	MaxTerminals *int `yaml:"max_terminals"`
}

type RawLimits struct {
	MaxTerminalsPerWorkspace *int                         `yaml:"max_terminals_per_workspace"`
	MaxWorkspaces            *int                         `yaml:"max_workspaces"`
	MaxTerminalsTotal        *int                         `yaml:"max_terminals_total"`
	WorkspaceOverrides       map[string]RawWorkspaceLimit `yaml:"workspace_overrides"`
}

type RawLoggingConfig struct {
	Enabled        *bool   `yaml:"enabled"`
	Level          *string `yaml:"level"`
	File           *string `yaml:"file"`
	MaxSizeMB      *int    `yaml:"max_size_mb"`
	MaxFiles       *int    `yaml:"max_files"`
	IncludeContent *bool   `yaml:"include_content"`
	PreviewLength  *int    `yaml:"preview_length"`
}

type RawConfig struct {
	Include                  IncludeList           `yaml:"include"`
	Hotkey                   *string               `yaml:"hotkey"`
	CycleLayoutHotkey        *string               `yaml:"cycle_layout_hotkey"`
	CycleLayoutReverseHotkey *string               `yaml:"cycle_layout_reverse_hotkey"`
	UndoHotkey               *string               `yaml:"undo_hotkey"`
	PaletteHotkey            *string               `yaml:"palette_hotkey"`
	PaletteBackend           *string               `yaml:"palette_backend"`
	PreferredTerminal        *string               `yaml:"preferred_terminal"`
	TerminalSpawnCommands    map[string]string     `yaml:"terminal_spawn_commands"`
	GapSize                  *int                  `yaml:"gap_size"`
	ScreenPadding            *RawMargins           `yaml:"screen_padding"`
	DefaultLayout            *string               `yaml:"default_layout"`
	Layouts                  map[string]RawLayout  `yaml:"layouts"`
	TerminalClasses          TerminalClassList     `yaml:"terminal_classes"`
	TerminalSort             *string               `yaml:"terminal_sort"`
	LogLevel                 *string               `yaml:"log_level"`
	TerminalMargins          map[string]RawMargins `yaml:"terminal_margins"`
	Limits                   *RawLimits                `yaml:"limits"`
	Logging                  *RawLoggingConfig         `yaml:"logging"`
	Agents                   map[string]AgentConfig    `yaml:"agents"`
}

func (c RawConfig) merge(overlay RawConfig) RawConfig {
	out := c

	if overlay.Hotkey != nil {
		out.Hotkey = overlay.Hotkey
	}
	if overlay.CycleLayoutHotkey != nil {
		out.CycleLayoutHotkey = overlay.CycleLayoutHotkey
	}
	if overlay.CycleLayoutReverseHotkey != nil {
		out.CycleLayoutReverseHotkey = overlay.CycleLayoutReverseHotkey
	}
	if overlay.UndoHotkey != nil {
		out.UndoHotkey = overlay.UndoHotkey
	}
	if overlay.PaletteHotkey != nil {
		out.PaletteHotkey = overlay.PaletteHotkey
	}
	if overlay.PaletteBackend != nil {
		out.PaletteBackend = overlay.PaletteBackend
	}
	if overlay.PreferredTerminal != nil {
		out.PreferredTerminal = overlay.PreferredTerminal
	}
	if overlay.TerminalSpawnCommands != nil {
		if out.TerminalSpawnCommands == nil {
			out.TerminalSpawnCommands = make(map[string]string, len(overlay.TerminalSpawnCommands))
		}
		for class, cmd := range overlay.TerminalSpawnCommands {
			out.TerminalSpawnCommands[class] = cmd
		}
	}
	if overlay.GapSize != nil {
		out.GapSize = overlay.GapSize
	}
	if overlay.ScreenPadding != nil {
		if out.ScreenPadding == nil {
			out.ScreenPadding = &RawMargins{}
		}
		if overlay.ScreenPadding.Top != nil {
			out.ScreenPadding.Top = overlay.ScreenPadding.Top
		}
		if overlay.ScreenPadding.Bottom != nil {
			out.ScreenPadding.Bottom = overlay.ScreenPadding.Bottom
		}
		if overlay.ScreenPadding.Left != nil {
			out.ScreenPadding.Left = overlay.ScreenPadding.Left
		}
		if overlay.ScreenPadding.Right != nil {
			out.ScreenPadding.Right = overlay.ScreenPadding.Right
		}
	}
	if overlay.DefaultLayout != nil {
		out.DefaultLayout = overlay.DefaultLayout
	}

	if overlay.Layouts != nil {
		if out.Layouts == nil {
			out.Layouts = make(map[string]RawLayout, len(overlay.Layouts))
		}
		for name, layout := range overlay.Layouts {
			base, ok := out.Layouts[name]
			if !ok {
				out.Layouts[name] = layout
				continue
			}
			out.Layouts[name] = mergeRawLayout(base, layout)
		}
	}

	if overlay.TerminalClasses != nil {
		out.TerminalClasses = overlay.TerminalClasses
	}
	if overlay.TerminalSort != nil {
		out.TerminalSort = overlay.TerminalSort
	}
	if overlay.LogLevel != nil {
		out.LogLevel = overlay.LogLevel
	}

	if overlay.TerminalMargins != nil {
		if out.TerminalMargins == nil {
			out.TerminalMargins = make(map[string]RawMargins, len(overlay.TerminalMargins))
		}
		for class, margins := range overlay.TerminalMargins {
			base, ok := out.TerminalMargins[class]
			if !ok {
				out.TerminalMargins[class] = margins
				continue
			}
			out.TerminalMargins[class] = mergeRawMargins(base, margins)
		}
	}

	if overlay.Limits != nil {
		if out.Limits == nil {
			out.Limits = &RawLimits{}
		}
		if overlay.Limits.MaxTerminalsPerWorkspace != nil {
			out.Limits.MaxTerminalsPerWorkspace = overlay.Limits.MaxTerminalsPerWorkspace
		}
		if overlay.Limits.MaxWorkspaces != nil {
			out.Limits.MaxWorkspaces = overlay.Limits.MaxWorkspaces
		}
		if overlay.Limits.MaxTerminalsTotal != nil {
			out.Limits.MaxTerminalsTotal = overlay.Limits.MaxTerminalsTotal
		}
		if overlay.Limits.WorkspaceOverrides != nil {
			if out.Limits.WorkspaceOverrides == nil {
				out.Limits.WorkspaceOverrides = make(map[string]RawWorkspaceLimit, len(overlay.Limits.WorkspaceOverrides))
			}
			for name, limit := range overlay.Limits.WorkspaceOverrides {
				base, ok := out.Limits.WorkspaceOverrides[name]
				if !ok {
					out.Limits.WorkspaceOverrides[name] = limit
					continue
				}
				out.Limits.WorkspaceOverrides[name] = mergeRawWorkspaceLimit(base, limit)
			}
		}
	}

	if overlay.Logging != nil {
		if out.Logging == nil {
			out.Logging = &RawLoggingConfig{}
		}
		if overlay.Logging.Enabled != nil {
			out.Logging.Enabled = overlay.Logging.Enabled
		}
		if overlay.Logging.Level != nil {
			out.Logging.Level = overlay.Logging.Level
		}
		if overlay.Logging.File != nil {
			out.Logging.File = overlay.Logging.File
		}
		if overlay.Logging.MaxSizeMB != nil {
			out.Logging.MaxSizeMB = overlay.Logging.MaxSizeMB
		}
		if overlay.Logging.MaxFiles != nil {
			out.Logging.MaxFiles = overlay.Logging.MaxFiles
		}
		if overlay.Logging.IncludeContent != nil {
			out.Logging.IncludeContent = overlay.Logging.IncludeContent
		}
		if overlay.Logging.PreviewLength != nil {
			out.Logging.PreviewLength = overlay.Logging.PreviewLength
		}
	}

	if overlay.Agents != nil {
		if out.Agents == nil {
			out.Agents = make(map[string]AgentConfig, len(overlay.Agents))
		}
		for name, agent := range overlay.Agents {
			out.Agents[name] = agent
		}
	}

	return out
}

func mergeRawMargins(base RawMargins, overlay RawMargins) RawMargins {
	out := base
	if overlay.Top != nil {
		out.Top = overlay.Top
	}
	if overlay.Bottom != nil {
		out.Bottom = overlay.Bottom
	}
	if overlay.Left != nil {
		out.Left = overlay.Left
	}
	if overlay.Right != nil {
		out.Right = overlay.Right
	}
	return out
}

func mergeRawWorkspaceLimit(base RawWorkspaceLimit, overlay RawWorkspaceLimit) RawWorkspaceLimit {
	out := base
	if overlay.MaxTerminals != nil {
		out.MaxTerminals = overlay.MaxTerminals
	}
	return out
}

func mergeRawTileRegion(base RawTileRegion, overlay RawTileRegion) RawTileRegion {
	out := base
	if overlay.Type != nil {
		out.Type = overlay.Type
	}
	if overlay.XPercent != nil {
		out.XPercent = overlay.XPercent
	}
	if overlay.YPercent != nil {
		out.YPercent = overlay.YPercent
	}
	if overlay.WidthPercent != nil {
		out.WidthPercent = overlay.WidthPercent
	}
	if overlay.HeightPercent != nil {
		out.HeightPercent = overlay.HeightPercent
	}
	return out
}

func mergeRawFixedGrid(base RawFixedGrid, overlay RawFixedGrid) RawFixedGrid {
	out := base
	if overlay.Rows != nil {
		out.Rows = overlay.Rows
	}
	if overlay.Cols != nil {
		out.Cols = overlay.Cols
	}
	return out
}

func mergeRawLayout(base RawLayout, overlay RawLayout) RawLayout {
	out := base
	if overlay.Inherits != nil {
		out.Inherits = overlay.Inherits
	}
	if overlay.Mode != nil {
		out.Mode = overlay.Mode
	}
	if overlay.TileRegion != nil {
		if out.TileRegion == nil {
			out.TileRegion = &RawTileRegion{}
		}
		merged := mergeRawTileRegion(*out.TileRegion, *overlay.TileRegion)
		out.TileRegion = &merged
	}
	if overlay.FixedGrid != nil {
		if out.FixedGrid == nil {
			out.FixedGrid = &RawFixedGrid{}
		}
		merged := mergeRawFixedGrid(*out.FixedGrid, *overlay.FixedGrid)
		out.FixedGrid = &merged
	}
	if overlay.MaxTerminalWidth != nil {
		out.MaxTerminalWidth = overlay.MaxTerminalWidth
	}
	if overlay.MaxTerminalHeight != nil {
		out.MaxTerminalHeight = overlay.MaxTerminalHeight
	}
	if overlay.FlexibleLastRow != nil {
		out.FlexibleLastRow = overlay.FlexibleLastRow
	}
	return out
}
