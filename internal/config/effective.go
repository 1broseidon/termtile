package config

import (
	"fmt"
	"sort"
	"strings"
)

const (
	DefaultBuiltinLayout = "grid"
)

type ValidationError struct {
	Path   string
	Source Source
	Err    error
}

func (e *ValidationError) Error() string {
	if e == nil {
		return "<nil>"
	}
	if e.Source.Kind == SourceFile && e.Source.File != "" && e.Source.Line > 0 {
		return fmt.Sprintf("%s:%d:%d: %s: %v", e.Source.File, e.Source.Line, e.Source.Column, e.Path, e.Err)
	}
	if e.Path != "" {
		return fmt.Sprintf("%s: %v", e.Path, e.Err)
	}
	return e.Err.Error()
}

func BuildEffectiveConfig(raw RawConfig) (*Config, map[string]string, error) {
	cfg := DefaultConfig()

	if raw.Hotkey != nil {
		cfg.Hotkey = *raw.Hotkey
	}
	if raw.CycleLayoutHotkey != nil {
		cfg.CycleLayoutHotkey = *raw.CycleLayoutHotkey
	}
	if raw.CycleLayoutReverseHotkey != nil {
		cfg.CycleLayoutReverseHotkey = *raw.CycleLayoutReverseHotkey
	}
	if raw.UndoHotkey != nil {
		cfg.UndoHotkey = *raw.UndoHotkey
	}
	if raw.PaletteHotkey != nil {
		cfg.PaletteHotkey = *raw.PaletteHotkey
	}
	if raw.PaletteBackend != nil {
		cfg.PaletteBackend = *raw.PaletteBackend
	}
	if raw.PaletteFuzzyMatching != nil {
		cfg.PaletteFuzzyMatching = *raw.PaletteFuzzyMatching
	}
	if raw.PreferredTerminal != nil {
		cfg.PreferredTerminal = *raw.PreferredTerminal
	}
	if raw.TerminalSpawnCommands != nil {
		if cfg.TerminalSpawnCommands == nil {
			cfg.TerminalSpawnCommands = make(map[string]string, len(raw.TerminalSpawnCommands))
		}
		for class, cmd := range raw.TerminalSpawnCommands {
			cfg.TerminalSpawnCommands[class] = cmd
		}
	}
	if raw.GapSize != nil {
		cfg.GapSize = *raw.GapSize
	}
	if raw.ScreenPadding != nil {
		if raw.ScreenPadding.Top != nil {
			cfg.ScreenPadding.Top = *raw.ScreenPadding.Top
		}
		if raw.ScreenPadding.Bottom != nil {
			cfg.ScreenPadding.Bottom = *raw.ScreenPadding.Bottom
		}
		if raw.ScreenPadding.Left != nil {
			cfg.ScreenPadding.Left = *raw.ScreenPadding.Left
		}
		if raw.ScreenPadding.Right != nil {
			cfg.ScreenPadding.Right = *raw.ScreenPadding.Right
		}
	}
	if raw.TerminalClasses != nil {
		cfg.TerminalClasses = raw.TerminalClasses
	}
	if raw.TerminalSort != nil {
		cfg.TerminalSort = *raw.TerminalSort
	}
	if raw.LogLevel != nil {
		cfg.LogLevel = *raw.LogLevel
	}
	if raw.TerminalMargins != nil {
		for class, margins := range raw.TerminalMargins {
			cfg.TerminalMargins[class] = Margins{
				Top:    derefInt(margins.Top, 0),
				Bottom: derefInt(margins.Bottom, 0),
				Left:   derefInt(margins.Left, 0),
				Right:  derefInt(margins.Right, 0),
			}
		}
	}

	if raw.Limits != nil {
		if raw.Limits.MaxTerminalsPerWorkspace != nil {
			cfg.Limits.MaxTerminalsPerWorkspace = *raw.Limits.MaxTerminalsPerWorkspace
		}
		if raw.Limits.MaxWorkspaces != nil {
			cfg.Limits.MaxWorkspaces = *raw.Limits.MaxWorkspaces
		}
		if raw.Limits.MaxTerminalsTotal != nil {
			cfg.Limits.MaxTerminalsTotal = *raw.Limits.MaxTerminalsTotal
		}
		if raw.Limits.WorkspaceOverrides != nil {
			if cfg.Limits.WorkspaceOverrides == nil {
				cfg.Limits.WorkspaceOverrides = make(map[string]WorkspaceLimit, len(raw.Limits.WorkspaceOverrides))
			}
			for name, limit := range raw.Limits.WorkspaceOverrides {
				cfg.Limits.WorkspaceOverrides[name] = WorkspaceLimit{
					MaxTerminals: derefInt(limit.MaxTerminals, 0),
				}
			}
		}
	}
	applyLimitDefaults(&cfg.Limits)

	if raw.Logging != nil {
		if raw.Logging.Enabled != nil {
			cfg.Logging.Enabled = *raw.Logging.Enabled
		}
		if raw.Logging.Level != nil {
			cfg.Logging.Level = *raw.Logging.Level
		}
		if raw.Logging.File != nil {
			cfg.Logging.File = *raw.Logging.File
		}
		if raw.Logging.MaxSizeMB != nil {
			cfg.Logging.MaxSizeMB = *raw.Logging.MaxSizeMB
		}
		if raw.Logging.MaxFiles != nil {
			cfg.Logging.MaxFiles = *raw.Logging.MaxFiles
		}
		if raw.Logging.IncludeContent != nil {
			cfg.Logging.IncludeContent = *raw.Logging.IncludeContent
		}
		if raw.Logging.PreviewLength != nil {
			cfg.Logging.PreviewLength = *raw.Logging.PreviewLength
		}
	}

	if raw.Agents != nil {
		if cfg.Agents == nil {
			cfg.Agents = make(map[string]AgentConfig, len(raw.Agents))
		}
		for name, agentCfg := range raw.Agents {
			if base, ok := cfg.Agents[name]; ok {
				// Merge: carry forward default fields the user didn't set.
				if agentCfg.IdlePattern == "" {
					agentCfg.IdlePattern = base.IdlePattern
				}
				if !agentCfg.ResponseFence {
					agentCfg.ResponseFence = base.ResponseFence
				}
				if agentCfg.SpawnMode == "" {
					agentCfg.SpawnMode = base.SpawnMode
				}
				if agentCfg.ReadyPattern == "" {
					agentCfg.ReadyPattern = base.ReadyPattern
				}
				if len(agentCfg.Models) == 0 {
					agentCfg.Models = base.Models
				}
				if agentCfg.DefaultModel == "" {
					agentCfg.DefaultModel = base.DefaultModel
				}
				if agentCfg.ModelFlag == "" {
					agentCfg.ModelFlag = base.ModelFlag
				}
			}
			cfg.Agents[name] = agentCfg
		}
	}
	applyAgentDefaults(cfg.Agents)

	layoutBases, err := applyLayouts(cfg, raw)
	if err != nil {
		return nil, nil, err
	}

	if raw.DefaultLayout != nil {
		cfg.DefaultLayout = *raw.DefaultLayout
	}
	if cfg.DefaultLayout == "" {
		cfg.DefaultLayout = DefaultBuiltinLayout
	}
	if _, err := cfg.GetLayout(cfg.DefaultLayout); err != nil {
		return nil, nil, &ValidationError{Path: "default_layout", Err: err}
	}

	return cfg, layoutBases, nil
}

func applyLimitDefaults(limits *Limits) {
	if limits == nil {
		return
	}
	if limits.MaxTerminalsPerWorkspace == 0 {
		limits.MaxTerminalsPerWorkspace = DefaultMaxTerminalsPerWorkspace
	}
	if limits.MaxWorkspaces == 0 {
		limits.MaxWorkspaces = DefaultMaxWorkspaces
	}
	if limits.MaxTerminalsTotal == 0 {
		limits.MaxTerminalsTotal = DefaultMaxTerminalsTotal
	}
}

func applyAgentDefaults(agents map[string]AgentConfig) {
	for name, agentCfg := range agents {
		if strings.TrimSpace(agentCfg.ModelFlag) == "" {
			agentCfg.ModelFlag = "--model"
		}
		agents[name] = agentCfg
	}
}

func applyLayouts(cfg *Config, raw RawConfig) (map[string]string, error) {
	builtin := BuiltinLayouts()

	// Start with built-ins.
	cfg.Layouts = make(map[string]Layout, len(builtin))
	for name, layout := range builtin {
		cfg.Layouts[name] = layout
	}

	layoutBases := make(map[string]string)
	for name := range cfg.Layouts {
		layoutBases[name] = name
	}

	// Apply user layout patches.
	for name, patch := range raw.Layouts {
		baseName, baseLayout, err := selectLayoutBase(name, patch, builtin)
		if err != nil {
			return nil, err
		}

		merged, err := mergeLayoutPatch(baseLayout, patch)
		if err != nil {
			return nil, err
		}
		if err := validateLayout(&merged); err != nil {
			return nil, &ValidationError{Path: "layouts." + name, Err: err}
		}

		cfg.Layouts[name] = merged
		layoutBases[name] = baseName
	}

	// Ensure deterministic iteration for callers that sort keys.
	_ = sortedKeys(cfg.Layouts)

	return layoutBases, nil
}

func selectLayoutBase(name string, patch RawLayout, builtin map[string]Layout) (string, Layout, error) {
	ref := ""
	if patch.Inherits != nil {
		ref = strings.TrimSpace(*patch.Inherits)
	}

	baseName := DefaultBuiltinLayout
	if _, ok := builtin[name]; ok {
		baseName = name
	}

	if ref != "" {
		const prefix = "builtin:"
		if !strings.HasPrefix(ref, prefix) {
			return "", Layout{}, &ValidationError{
				Path: "layouts." + name + ".inherits",
				Err:  fmt.Errorf("inherits must be %q-prefixed (builtin-only), got %q", prefix, ref),
			}
		}
		baseName = strings.TrimSpace(strings.TrimPrefix(ref, prefix))
	}

	baseLayout, ok := builtin[baseName]
	if !ok {
		return "", Layout{}, &ValidationError{
			Path: "layouts." + name + ".inherits",
			Err:  fmt.Errorf("unknown builtin layout %q", baseName),
		}
	}

	return baseName, baseLayout, nil
}

func mergeLayoutPatch(base Layout, patch RawLayout) (Layout, error) {
	out := base

	if patch.Mode != nil {
		out.Mode = *patch.Mode
	}
	if patch.TileRegion != nil {
		if patch.TileRegion.Type != nil {
			out.TileRegion.Type = *patch.TileRegion.Type
		}
		if patch.TileRegion.XPercent != nil {
			out.TileRegion.XPercent = *patch.TileRegion.XPercent
		}
		if patch.TileRegion.YPercent != nil {
			out.TileRegion.YPercent = *patch.TileRegion.YPercent
		}
		if patch.TileRegion.WidthPercent != nil {
			out.TileRegion.WidthPercent = *patch.TileRegion.WidthPercent
		}
		if patch.TileRegion.HeightPercent != nil {
			out.TileRegion.HeightPercent = *patch.TileRegion.HeightPercent
		}

		if out.TileRegion.Type == RegionCustom {
			// Only default fields that the user didn't set.
			if patch.TileRegion.XPercent == nil && out.TileRegion.XPercent == 0 {
				out.TileRegion.XPercent = 0
			}
			if patch.TileRegion.YPercent == nil && out.TileRegion.YPercent == 0 {
				out.TileRegion.YPercent = 0
			}
			if patch.TileRegion.WidthPercent == nil && out.TileRegion.WidthPercent == 0 {
				out.TileRegion.WidthPercent = 100
			}
			if patch.TileRegion.HeightPercent == nil && out.TileRegion.HeightPercent == 0 {
				out.TileRegion.HeightPercent = 100
			}
		}
	}
	if patch.FixedGrid != nil {
		if patch.FixedGrid.Rows != nil {
			out.FixedGrid.Rows = *patch.FixedGrid.Rows
		}
		if patch.FixedGrid.Cols != nil {
			out.FixedGrid.Cols = *patch.FixedGrid.Cols
		}
	}
	if patch.MasterStack != nil {
		if patch.MasterStack.MasterWidthPercent != nil {
			out.MasterStack.MasterWidthPercent = *patch.MasterStack.MasterWidthPercent
		}
		if patch.MasterStack.MaxStackRows != nil {
			out.MasterStack.MaxStackRows = *patch.MasterStack.MaxStackRows
		}
		if patch.MasterStack.MaxStackCols != nil {
			out.MasterStack.MaxStackCols = *patch.MasterStack.MaxStackCols
		}
	}
	if patch.MaxTerminalWidth != nil {
		out.MaxTerminalWidth = *patch.MaxTerminalWidth
	}
	if patch.MaxTerminalHeight != nil {
		out.MaxTerminalHeight = *patch.MaxTerminalHeight
	}
	if patch.FlexibleLastRow != nil {
		out.FlexibleLastRow = *patch.FlexibleLastRow
	}

	return out, nil
}

func derefInt(p *int, def int) int {
	if p == nil {
		return def
	}
	return *p
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
