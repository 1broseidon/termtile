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
	if raw.TerminalAddHotkey != nil {
		cfg.TerminalAddHotkey = *raw.TerminalAddHotkey
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
	if raw.Display != nil {
		cfg.Display = *raw.Display
	}
	if raw.XAuthority != nil {
		cfg.XAuthority = *raw.XAuthority
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

	if raw.AgentMode != nil {
		if raw.AgentMode.ProtectSlotZero != nil {
			cfg.AgentMode.ProtectSlotZero = raw.AgentMode.ProtectSlotZero
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
	if raw.ProjectWorkspace != nil {
		projectCfg, err := buildEffectiveProjectWorkspaceConfig(*raw.ProjectWorkspace)
		if err != nil {
			return nil, nil, err
		}
		applyProjectWorkspaceOverrides(cfg, projectCfg)
		cfg.ProjectWorkspace = &projectCfg
		applyAgentDefaults(cfg.Agents)
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

func buildEffectiveProjectWorkspaceConfig(raw RawProjectWorkspaceConfig) (ProjectWorkspaceConfig, error) {
	out := DefaultProjectWorkspaceConfig()

	if raw.Version != nil {
		out.Version = *raw.Version
	}
	if out.Version != ProjectWorkspaceSchemaVersion {
		return ProjectWorkspaceConfig{}, &ValidationError{
			Path: "project_workspace.version",
			Err:  fmt.Errorf("version must be %d", ProjectWorkspaceSchemaVersion),
		}
	}

	if raw.Workspace != nil {
		out.Workspace = strings.TrimSpace(*raw.Workspace)
	}
	if out.Workspace == "" {
		return ProjectWorkspaceConfig{}, &ValidationError{
			Path: "project_workspace.workspace",
			Err:  fmt.Errorf("workspace is required"),
		}
	}

	if raw.Project != nil {
		if raw.Project.RootMarker != nil {
			out.Project.RootMarker = strings.TrimSpace(*raw.Project.RootMarker)
		}
		if raw.Project.CWDMode != nil {
			out.Project.CWDMode = ProjectCWDMode(strings.TrimSpace(*raw.Project.CWDMode))
		}
		if raw.Project.CWD != nil {
			out.Project.CWD = strings.TrimSpace(*raw.Project.CWD)
		}
	}
	if out.Project.RootMarker == "" {
		return ProjectWorkspaceConfig{}, &ValidationError{
			Path: "project_workspace.project.root_marker",
			Err:  fmt.Errorf("root_marker is required"),
		}
	}
	switch out.Project.CWDMode {
	case ProjectCWDModeProjectRoot, ProjectCWDModeWorkspaceSave, ProjectCWDModeExplicit:
	default:
		return ProjectWorkspaceConfig{}, &ValidationError{
			Path: "project_workspace.project.cwd_mode",
			Err:  fmt.Errorf("cwd_mode must be one of: project_root, workspace_saved, explicit"),
		}
	}
	if out.Project.CWDMode == ProjectCWDModeExplicit && out.Project.CWD == "" {
		return ProjectWorkspaceConfig{}, &ValidationError{
			Path: "project_workspace.project.cwd",
			Err:  fmt.Errorf("cwd is required when cwd_mode=explicit"),
		}
	}

	if raw.MCP != nil {
		if raw.MCP.Spawn != nil {
			if raw.MCP.Spawn.RequireExplicitWorkspace != nil {
				out.MCP.Spawn.RequireExplicitWorkspace = *raw.MCP.Spawn.RequireExplicitWorkspace
			}
			if raw.MCP.Spawn.ResolutionOrder != nil {
				out.MCP.Spawn.ResolutionOrder = append([]string(nil), raw.MCP.Spawn.ResolutionOrder...)
			}
		}
		if raw.MCP.Read != nil {
			if raw.MCP.Read.DefaultLines != nil {
				out.MCP.Read.DefaultLines = *raw.MCP.Read.DefaultLines
			}
			if raw.MCP.Read.MaxLines != nil {
				out.MCP.Read.MaxLines = *raw.MCP.Read.MaxLines
			}
			if raw.MCP.Read.SinceLastDefault != nil {
				out.MCP.Read.SinceLastDefault = *raw.MCP.Read.SinceLastDefault
			}
		}
	}
	if len(out.MCP.Spawn.ResolutionOrder) == 0 {
		return ProjectWorkspaceConfig{}, &ValidationError{
			Path: "project_workspace.mcp.spawn.resolution_order",
			Err:  fmt.Errorf("resolution_order must not be empty"),
		}
	}
	allowedResolutionOrder := map[string]struct{}{
		"explicit_arg":                      {},
		"source_workspace_hint":             {},
		"project_marker":                    {},
		"single_registered_agent_workspace": {},
		"error":                             {},
	}
	for _, step := range out.MCP.Spawn.ResolutionOrder {
		if _, ok := allowedResolutionOrder[step]; !ok {
			return ProjectWorkspaceConfig{}, &ValidationError{
				Path: "project_workspace.mcp.spawn.resolution_order",
				Err:  fmt.Errorf("unsupported resolution_order step %q", step),
			}
		}
	}
	if out.MCP.Read.DefaultLines <= 0 || out.MCP.Read.DefaultLines > 100 {
		return ProjectWorkspaceConfig{}, &ValidationError{
			Path: "project_workspace.mcp.read.default_lines",
			Err:  fmt.Errorf("default_lines must be between 1 and 100"),
		}
	}
	if out.MCP.Read.MaxLines <= 0 || out.MCP.Read.MaxLines > 100 {
		return ProjectWorkspaceConfig{}, &ValidationError{
			Path: "project_workspace.mcp.read.max_lines",
			Err:  fmt.Errorf("max_lines must be between 1 and 100"),
		}
	}
	if out.MCP.Read.DefaultLines > out.MCP.Read.MaxLines {
		return ProjectWorkspaceConfig{}, &ValidationError{
			Path: "project_workspace.mcp.read.default_lines",
			Err:  fmt.Errorf("default_lines must be <= max_lines"),
		}
	}

	if raw.Agents != nil {
		if raw.Agents.Defaults != nil {
			if raw.Agents.Defaults.SpawnMode != nil {
				out.Agents.Defaults.SpawnMode = strings.TrimSpace(*raw.Agents.Defaults.SpawnMode)
			}
			if raw.Agents.Defaults.Model != nil {
				out.Agents.Defaults.Model = strings.TrimSpace(*raw.Agents.Defaults.Model)
			}
			if raw.Agents.Defaults.Env != nil {
				out.Agents.Defaults.Env = mergeStringMap(out.Agents.Defaults.Env, raw.Agents.Defaults.Env)
			}
		}
		if raw.Agents.Overrides != nil {
			if out.Agents.Overrides == nil {
				out.Agents.Overrides = map[string]ProjectWorkspaceAgentOverride{}
			}
			for name, override := range raw.Agents.Overrides {
				current := out.Agents.Overrides[name]
				if override.SpawnMode != nil {
					current.SpawnMode = strings.TrimSpace(*override.SpawnMode)
				}
				if override.Model != nil {
					current.Model = strings.TrimSpace(*override.Model)
				}
				if override.Env != nil {
					current.Env = mergeStringMap(current.Env, override.Env)
				}
				out.Agents.Overrides[name] = current
			}
		}
	}
	if err := validateProjectAgentSpawnMode("project_workspace.agents.defaults.spawn_mode", out.Agents.Defaults.SpawnMode); err != nil {
		return ProjectWorkspaceConfig{}, err
	}
	for name, override := range out.Agents.Overrides {
		if err := validateProjectAgentSpawnMode("project_workspace.agents.overrides."+name+".spawn_mode", override.SpawnMode); err != nil {
			return ProjectWorkspaceConfig{}, err
		}
	}

	if raw.WorkspaceOverrides != nil {
		if raw.WorkspaceOverrides.Layout != nil {
			out.WorkspaceOverrides.Layout = strings.TrimSpace(*raw.WorkspaceOverrides.Layout)
		}
		if raw.WorkspaceOverrides.Terminal != nil {
			out.WorkspaceOverrides.Terminal = strings.TrimSpace(*raw.WorkspaceOverrides.Terminal)
		}
		if raw.WorkspaceOverrides.TerminalSpawnCommand != nil {
			out.WorkspaceOverrides.TerminalSpawnCommand = strings.TrimSpace(*raw.WorkspaceOverrides.TerminalSpawnCommand)
		}
	}

	if raw.Sync != nil {
		if raw.Sync.Mode != nil {
			out.Sync.Mode = ProjectSyncMode(strings.TrimSpace(*raw.Sync.Mode))
		}
		if raw.Sync.PullOnWorkspaceLoad != nil {
			out.Sync.PullOnWorkspaceLoad = *raw.Sync.PullOnWorkspaceLoad
		}
		if raw.Sync.PushOnWorkspaceSave != nil {
			out.Sync.PushOnWorkspaceSave = *raw.Sync.PushOnWorkspaceSave
		}
		if raw.Sync.Include != nil {
			out.Sync.Include = append([]string(nil), raw.Sync.Include...)
		}
	}
	switch out.Sync.Mode {
	case ProjectSyncModeLinked, ProjectSyncModeDetached:
	default:
		return ProjectWorkspaceConfig{}, &ValidationError{
			Path: "project_workspace.sync.mode",
			Err:  fmt.Errorf("mode must be one of: linked, detached"),
		}
	}
	if len(out.Sync.Include) == 0 {
		return ProjectWorkspaceConfig{}, &ValidationError{
			Path: "project_workspace.sync.include",
			Err:  fmt.Errorf("include must not be empty"),
		}
	}
	allowedSyncInclude := map[string]struct{}{
		"layout":     {},
		"terminals":  {},
		"agent_mode": {},
	}
	for _, field := range out.Sync.Include {
		if _, ok := allowedSyncInclude[field]; !ok {
			return ProjectWorkspaceConfig{}, &ValidationError{
				Path: "project_workspace.sync.include",
				Err:  fmt.Errorf("unsupported include field %q", field),
			}
		}
	}

	return out, nil
}

func applyProjectWorkspaceOverrides(cfg *Config, projectCfg ProjectWorkspaceConfig) {
	if cfg == nil {
		return
	}

	if layout := strings.TrimSpace(projectCfg.WorkspaceOverrides.Layout); layout != "" {
		cfg.DefaultLayout = layout
	}
	if terminal := strings.TrimSpace(projectCfg.WorkspaceOverrides.Terminal); terminal != "" {
		cfg.PreferredTerminal = terminal
	}
	if cmd := strings.TrimSpace(projectCfg.WorkspaceOverrides.TerminalSpawnCommand); cmd != "" {
		terminalClass := strings.TrimSpace(projectCfg.WorkspaceOverrides.Terminal)
		if terminalClass == "" {
			terminalClass = strings.TrimSpace(cfg.PreferredTerminal)
		}
		if terminalClass != "" {
			if cfg.TerminalSpawnCommands == nil {
				cfg.TerminalSpawnCommands = make(map[string]string)
			}
			cfg.TerminalSpawnCommands[terminalClass] = cmd
		}
	}

	for name, agentCfg := range cfg.Agents {
		if spawnMode := strings.TrimSpace(projectCfg.Agents.Defaults.SpawnMode); spawnMode != "" {
			agentCfg.SpawnMode = spawnMode
		}
		if model := strings.TrimSpace(projectCfg.Agents.Defaults.Model); model != "" {
			agentCfg.DefaultModel = model
		}
		agentCfg.Env = mergeStringMap(agentCfg.Env, projectCfg.Agents.Defaults.Env)
		cfg.Agents[name] = agentCfg
	}

	for name, override := range projectCfg.Agents.Overrides {
		agentCfg, ok := cfg.Agents[name]
		if !ok {
			continue
		}
		if spawnMode := strings.TrimSpace(override.SpawnMode); spawnMode != "" {
			agentCfg.SpawnMode = spawnMode
		}
		if model := strings.TrimSpace(override.Model); model != "" {
			agentCfg.DefaultModel = model
		}
		agentCfg.Env = mergeStringMap(agentCfg.Env, override.Env)
		cfg.Agents[name] = agentCfg
	}
}

func validateProjectAgentSpawnMode(path string, mode string) error {
	if strings.TrimSpace(mode) == "" {
		return nil
	}
	switch mode {
	case "pane", "window":
		return nil
	default:
		return &ValidationError{
			Path: path,
			Err:  fmt.Errorf("spawn_mode must be one of: pane, window"),
		}
	}
}

func mergeStringMap(base map[string]string, overlay map[string]string) map[string]string {
	if base == nil && overlay == nil {
		return nil
	}
	out := make(map[string]string)
	for key, value := range base {
		out[key] = value
	}
	for key, value := range overlay {
		out[key] = value
	}
	return out
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
