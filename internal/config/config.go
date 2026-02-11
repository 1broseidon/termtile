package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Margins represents margin adjustments for a terminal.
type Margins struct {
	Top    int `yaml:"top"`
	Bottom int `yaml:"bottom"`
	Left   int `yaml:"left"`
	Right  int `yaml:"right"`
}

// LayoutMode defines how terminals are arranged.
type LayoutMode string

const (
	LayoutModeAuto        LayoutMode = "auto"         // Dynamic grid based on count.
	LayoutModeFixed       LayoutMode = "fixed"        // Specific rows × cols.
	LayoutModeVertical    LayoutMode = "vertical"     // Single column stack.
	LayoutModeHorizontal  LayoutMode = "horizontal"   // Single row side-by-side.
	LayoutModeMasterStack LayoutMode = "master-stack" // Master pane left, stack grid right.
)

// RegionType defines tile region presets.
type RegionType string

const (
	RegionFull       RegionType = "full"
	RegionLeftHalf   RegionType = "left-half"
	RegionRightHalf  RegionType = "right-half"
	RegionTopHalf    RegionType = "top-half"
	RegionBottomHalf RegionType = "bottom-half"
	RegionCustom     RegionType = "custom"
)

// TileRegion defines where to tile windows.
type TileRegion struct {
	Type          RegionType `yaml:"type"`
	XPercent      int        `yaml:"x_percent"`      // 0-100
	YPercent      int        `yaml:"y_percent"`      // 0-100
	WidthPercent  int        `yaml:"width_percent"`  // 0-100
	HeightPercent int        `yaml:"height_percent"` // 0-100
}

// FixedGrid defines specific grid dimensions.
type FixedGrid struct {
	Rows int `yaml:"rows"`
	Cols int `yaml:"cols"`
}

// MasterStack defines the master-stack layout parameters.
type MasterStack struct {
	MasterWidthPercent int `yaml:"master_width_percent"` // Width of master pane as percentage (10-90)
	MaxStackRows       int `yaml:"max_stack_rows"`       // Maximum rows in the stack grid (>= 1)
	MaxStackCols       int `yaml:"max_stack_cols"`       // Maximum columns in the stack grid (>= 1)
}

// Layout defines a tiling configuration.
type Layout struct {
	Mode              LayoutMode  `yaml:"mode"`
	TileRegion        TileRegion  `yaml:"tile_region"`
	FixedGrid         FixedGrid   `yaml:"fixed_grid,omitempty"`
	MasterStack       MasterStack `yaml:"master_stack,omitempty"`
	MaxTerminalWidth  int         `yaml:"max_terminal_width"`  // 0 = unlimited
	MaxTerminalHeight int         `yaml:"max_terminal_height"` // 0 = unlimited
	FlexibleLastRow   bool        `yaml:"flexible_last_row"`   // Last row windows expand to fill width (auto mode only)
}

// AgentMode configures the agent/multiplexer integration
type AgentMode struct {
	// Multiplexer specifies which terminal multiplexer to use
	// Values: "auto" (default), "tmux", "screen"
	// "auto" will detect and prefer tmux > screen
	Multiplexer string `yaml:"multiplexer"`

	// ManageMultiplexerConfig controls whether termtile generates
	// an optimized multiplexer config file (e.g., ~/.config/termtile/tmux.conf)
	// Default: true
	// Set to false if you want to use your own tmux/screen config entirely
	ManageMultiplexerConfig *bool `yaml:"manage_multiplexer_config"`

	// ProtectSlotZero prevents slot 0 from being killed in agent-mode
	// workspaces, since slot 0 is typically the orchestrating agent.
	// Default: true
	ProtectSlotZero *bool `yaml:"protect_slot_zero"`
}

const (
	DefaultMaxTerminalsPerWorkspace = 10
	DefaultMaxWorkspaces            = 5
	DefaultMaxTerminalsTotal        = 20
)

type WorkspaceLimit struct {
	MaxTerminals int `yaml:"max_terminals,omitempty"`
}

type Limits struct {
	MaxTerminalsPerWorkspace int                       `yaml:"max_terminals_per_workspace,omitempty"`
	MaxWorkspaces            int                       `yaml:"max_workspaces,omitempty"`
	MaxTerminalsTotal        int                       `yaml:"max_terminals_total,omitempty"`
	WorkspaceOverrides       map[string]WorkspaceLimit `yaml:"workspace_overrides,omitempty"`
}

// LoggingConfig configures agent action logging.
type LoggingConfig struct {
	// Enabled turns agent action logging on/off
	Enabled bool `yaml:"enabled,omitempty"`
	// Level controls logging verbosity: debug, info, warn, error
	Level string `yaml:"level,omitempty"`
	// File is the log file path (default: ~/.local/share/termtile/agent-actions.log)
	File string `yaml:"file,omitempty"`
	// MaxSizeMB is the maximum log file size before rotation (default: 10)
	MaxSizeMB int `yaml:"max_size_mb,omitempty"`
	// MaxFiles is the number of rotated files to keep (default: 3)
	MaxFiles int `yaml:"max_files,omitempty"`
	// IncludeContent logs full send content (security risk, default: false)
	IncludeContent bool `yaml:"include_content,omitempty"`
	// PreviewLength is the number of characters to preview in log (default: 50)
	PreviewLength int `yaml:"preview_length,omitempty"`
}

// GetManageMultiplexerConfig returns the effective value, defaulting to true
func (a *AgentMode) GetManageMultiplexerConfig() bool {
	if a == nil || a.ManageMultiplexerConfig == nil {
		return true
	}
	return *a.ManageMultiplexerConfig
}

// GetProtectSlotZero returns the effective value, defaulting to true.
// When true, slot 0 cannot be killed in agent-mode workspaces (it is
// typically the orchestrating agent).
func (a *AgentMode) GetProtectSlotZero() bool {
	if a == nil || a.ProtectSlotZero == nil {
		return true
	}
	return *a.ProtectSlotZero
}

// AgentConfig describes a CLI agent that can be spawned via MCP.
type AgentConfig struct {
	Command       string            `yaml:"command"`
	Args          []string          `yaml:"args,omitempty"`
	ReadyPattern  string            `yaml:"ready_pattern,omitempty"`
	IdlePattern   string            `yaml:"idle_pattern,omitempty"`
	Description   string            `yaml:"description,omitempty"`
	Env           map[string]string `yaml:"env,omitempty"`
	PromptAsArg   bool              `yaml:"prompt_as_arg,omitempty"`
	SpawnMode     string            `yaml:"spawn_mode,omitempty"`     // "pane" (default) or "window"
	ResponseFence bool              `yaml:"response_fence,omitempty"` // prepend task with fence instructions for structured output parsing
	PipeTask      bool              `yaml:"pipe_task,omitempty"`      // pipe task via stdin instead of appending as arg or sending via send-keys
	Models        []string          `yaml:"models,omitempty"`
	DefaultModel  string            `yaml:"default_model,omitempty"`
	ModelFlag     string            `yaml:"model_flag,omitempty"`
}

type ProjectCWDMode string

const (
	ProjectCWDModeProjectRoot   ProjectCWDMode = "project_root"
	ProjectCWDModeWorkspaceSave ProjectCWDMode = "workspace_saved"
	ProjectCWDModeExplicit      ProjectCWDMode = "explicit"
)

type ProjectSyncMode string

const (
	ProjectSyncModeLinked   ProjectSyncMode = "linked"
	ProjectSyncModeDetached ProjectSyncMode = "detached"
)

const ProjectWorkspaceSchemaVersion = 1

type ProjectWorkspaceProject struct {
	RootMarker string         `yaml:"root_marker"`
	CWDMode    ProjectCWDMode `yaml:"cwd_mode"`
	CWD        string         `yaml:"cwd,omitempty"`
}

type ProjectWorkspaceMCPSpawn struct {
	RequireExplicitWorkspace bool     `yaml:"require_explicit_workspace"`
	ResolutionOrder          []string `yaml:"resolution_order"`
}

type ProjectWorkspaceMCPRead struct {
	DefaultLines     int  `yaml:"default_lines"`
	MaxLines         int  `yaml:"max_lines"`
	SinceLastDefault bool `yaml:"since_last_default"`
}

type ProjectWorkspaceMCP struct {
	Spawn ProjectWorkspaceMCPSpawn `yaml:"spawn"`
	Read  ProjectWorkspaceMCPRead  `yaml:"read"`
}

type ProjectWorkspaceAgentDefaults struct {
	SpawnMode string            `yaml:"spawn_mode,omitempty"`
	Model     string            `yaml:"model,omitempty"`
	Env       map[string]string `yaml:"env,omitempty"`
}

type ProjectWorkspaceAgentOverride struct {
	SpawnMode string            `yaml:"spawn_mode,omitempty"`
	Model     string            `yaml:"model,omitempty"`
	Env       map[string]string `yaml:"env,omitempty"`
}

type ProjectWorkspaceAgents struct {
	Defaults  ProjectWorkspaceAgentDefaults            `yaml:"defaults"`
	Overrides map[string]ProjectWorkspaceAgentOverride `yaml:"overrides,omitempty"`
}

type ProjectWorkspaceOverrides struct {
	Layout               string `yaml:"layout,omitempty"`
	Terminal             string `yaml:"terminal,omitempty"`
	TerminalSpawnCommand string `yaml:"terminal_spawn_command,omitempty"`
}

type ProjectWorkspaceSync struct {
	Mode                ProjectSyncMode `yaml:"mode"`
	PullOnWorkspaceLoad bool            `yaml:"pull_on_workspace_load"`
	PushOnWorkspaceSave bool            `yaml:"push_on_workspace_save"`
	Include             []string        `yaml:"include"`
}

// ProjectWorkspaceConfig stores effective merged project workspace settings
// from .termtile/workspace.yaml and .termtile/local.yaml.
type ProjectWorkspaceConfig struct {
	Version            int                       `yaml:"version"`
	Workspace          string                    `yaml:"workspace,omitempty"`
	Project            ProjectWorkspaceProject   `yaml:"project"`
	MCP                ProjectWorkspaceMCP       `yaml:"mcp"`
	Agents             ProjectWorkspaceAgents    `yaml:"agents"`
	WorkspaceOverrides ProjectWorkspaceOverrides `yaml:"workspace_overrides"`
	Sync               ProjectWorkspaceSync      `yaml:"sync"`
}

func DefaultProjectWorkspaceConfig() ProjectWorkspaceConfig {
	return ProjectWorkspaceConfig{
		Version:   ProjectWorkspaceSchemaVersion,
		Workspace: "",
		Project: ProjectWorkspaceProject{
			RootMarker: ".git",
			CWDMode:    ProjectCWDModeProjectRoot,
		},
		MCP: ProjectWorkspaceMCP{
			Spawn: ProjectWorkspaceMCPSpawn{
				RequireExplicitWorkspace: false,
				ResolutionOrder: []string{
					"explicit_arg",
					"source_workspace_hint",
					"project_marker",
					"single_registered_agent_workspace",
					"error",
				},
			},
			Read: ProjectWorkspaceMCPRead{
				DefaultLines:     50,
				MaxLines:         100,
				SinceLastDefault: false,
			},
		},
		Agents: ProjectWorkspaceAgents{
			Defaults: ProjectWorkspaceAgentDefaults{
				SpawnMode: "window",
				Env:       map[string]string{},
			},
			Overrides: map[string]ProjectWorkspaceAgentOverride{},
		},
		Sync: ProjectWorkspaceSync{
			Mode:                ProjectSyncModeLinked,
			PullOnWorkspaceLoad: true,
			PushOnWorkspaceSave: false,
			Include: []string{
				"layout",
				"terminals",
				"agent_mode",
			},
		},
	}
}

// Config holds the application configuration.
type Config struct {
	Hotkey                   string                  `yaml:"hotkey"`
	CycleLayoutHotkey        string                  `yaml:"cycle_layout_hotkey"`
	CycleLayoutReverseHotkey string                  `yaml:"cycle_layout_reverse_hotkey"`
	UndoHotkey               string                  `yaml:"undo_hotkey"`
	MoveModeHotkey           string                  `yaml:"move_mode_hotkey"`
	TerminalAddHotkey        string                  `yaml:"terminal_add_hotkey"`
	MoveModeTimeout          int                     `yaml:"move_mode_timeout"`
	PaletteHotkey            string                  `yaml:"palette_hotkey"`
	PaletteBackend           string                  `yaml:"palette_backend"`
	PaletteFuzzyMatching     bool                    `yaml:"palette_fuzzy_matching"`
	Display                  string                  `yaml:"display,omitempty"`
	XAuthority               string                  `yaml:"xauthority,omitempty"`
	PreferredTerminal        string                  `yaml:"preferred_terminal,omitempty"`
	TerminalSpawnCommands    map[string]string       `yaml:"terminal_spawn_commands"`
	GapSize                  int                     `yaml:"gap_size"`
	ScreenPadding            Margins                 `yaml:"screen_padding"`
	DefaultLayout            string                  `yaml:"default_layout"`
	Layouts                  map[string]Layout       `yaml:"layouts"`
	TerminalClasses          TerminalClassList       `yaml:"terminal_classes"`
	TerminalSort             string                  `yaml:"terminal_sort"`
	LogLevel                 string                  `yaml:"log_level"`
	TerminalMargins          map[string]Margins      `yaml:"terminal_margins"`
	AgentMode                AgentMode               `yaml:"agent_mode"`
	Limits                   Limits                  `yaml:"limits,omitempty"`
	Logging                  LoggingConfig           `yaml:"logging,omitempty"`
	Agents                   map[string]AgentConfig  `yaml:"agents,omitempty"`
	ProjectWorkspace         *ProjectWorkspaceConfig `yaml:"-"`
}

func DefaultConfig() *Config {
	return &Config{
		Hotkey:            "Mod4-Mod1-t",
		MoveModeHotkey:    "Mod4-Mod1-r", // Super+Alt+R for "relocate"
		TerminalAddHotkey: "Mod4-Mod1-n", // Super+Alt+N for new terminal in active workspace
		MoveModeTimeout:   10,            // 10 seconds default timeout
		PaletteHotkey:     "Mod4-Mod1-g", // Super+Alt+G for palette
		PaletteBackend:    "auto",
		// Disabled by default to preserve existing match behavior.
		PaletteFuzzyMatching: false,
		TerminalSpawnCommands: map[string]string{
			"kitty":                 "kitty --directory {{dir}} {{cmd}}",
			"Alacritty":             "alacritty --working-directory {{dir}} -e {{cmd}}",
			"com.mitchellh.ghostty": "ghostty --working-directory={{dir}} -e {{cmd}}",
			"ghostty":               "ghostty --working-directory={{dir}} -e {{cmd}}",
			"wezterm":               "wezterm start --cwd {{dir}} -- {{cmd}}",
			"Gnome-terminal":        "gnome-terminal --working-directory={{dir}} -- {{cmd}}",
			"gnome-terminal-server": "gnome-terminal --working-directory={{dir}} -- {{cmd}}",
			"konsole":               "konsole --workdir {{dir}} -e {{cmd}}",
		},
		GapSize: 8,
		ScreenPadding: Margins{
			Top:    0,
			Bottom: 0,
			Left:   0,
			Right:  0,
		},
		DefaultLayout:   DefaultBuiltinLayout,
		Layouts:         BuiltinLayouts(),
		TerminalClasses: defaultTerminalClasses(),
		TerminalSort:    "position",
		LogLevel:        "info",
		TerminalMargins: make(map[string]Margins),
		AgentMode: AgentMode{
			Multiplexer: "auto", // Auto-detect: tmux > screen
			// ManageMultiplexerConfig defaults to true via getter
		},
		Limits: Limits{
			MaxTerminalsPerWorkspace: DefaultMaxTerminalsPerWorkspace,
			MaxWorkspaces:            DefaultMaxWorkspaces,
			MaxTerminalsTotal:        DefaultMaxTerminalsTotal,
		},
		Agents: map[string]AgentConfig{
			"claude": {
				Command:       "claude",
				Args:          []string{"--dangerously-skip-permissions"},
				Description:   "Claude Code CLI agent",
				SpawnMode:     "window",
				PromptAsArg:   true,
				IdlePattern:   "\u276f", // ❯ (U+276F) Claude Code input prompt
				ResponseFence: true,
				Models:        []string{"sonnet", "haiku", "opus"},
			},
			"codex": {
				Command:       "codex",
				Args:          []string{"--full-auto", "--no-alt-screen", "-c", "notice.model_migrations={}", "-c", "notice.hide_rate_limit_switch_prompt=true"},
				Description:   "OpenAI Codex CLI agent",
				SpawnMode:     "window",
				PromptAsArg:   true,
				IdlePattern:   "\u203a", // › (U+203A) Codex input prompt
				ResponseFence: true,
				Models:        []string{"gpt-5.2-codex", "gpt-5.3-codex", "gpt-5.1-codex-max", "gpt-5.2", "gpt-5.1-codex-mini"},
			},
			"gemini": {
				Command:       "gemini",
				Args:          []string{},
				Description:   "Google Gemini CLI",
				SpawnMode:     "window",
				PromptAsArg:   true,
				IdlePattern:   ">", // Gemini input prompt
				ResponseFence: true,
			},
			"cursor-agent": {
				Command:       "cursor-agent",
				Args:          []string{},
				Description:   "Cursor AI agent CLI",
				SpawnMode:     "window",
				PromptAsArg:   true,
				IdlePattern:   "\u2192", // → (U+2192) Cursor agent input prompt
				ResponseFence: true,
			},
			"cecli": {
				Command:       "cecli",
				Args:          []string{"--no-tui", "--yes-always", "--no-auto-commits", "--no-check-update", "--no-show-model-warnings"},
				Description:   "Cecli (aider fork) coding agent",
				SpawnMode:     "window",
				PipeTask:      true,
				ResponseFence: true,
				Models:        []string{"anthropic/claude-sonnet-4-5", "anthropic/claude-opus-4-6", "openai/gpt-5.2", "deepseek/deepseek-chat"},
				DefaultModel:  "anthropic/claude-sonnet-4-5",
				ModelFlag:     "--model",
			},
			"pi": {
				Command:       "pi",
				Args:          []string{"--no-session"},
				Description:   "Pi coding agent (multi-provider)",
				SpawnMode:     "window",
				PromptAsArg:   true,
				IdlePattern:   "\u2500", // ─ (U+2500) Box drawing horizontal, part of pi's input divider
				ResponseFence: true,
				Models:        []string{"gemini-3-pro-high", "gemini-3-flash", "claude-sonnet-4-5", "claude-opus-4-6", "claude-haiku-4-5"},
				DefaultModel:  "gemini-3-pro-high",
			},
		},
	}
}

// GetMaxTerminalsForWorkspace returns the max terminals allowed for a workspace name.
func (c *Config) GetMaxTerminalsForWorkspace(name string) int {
	if c == nil {
		return DefaultMaxTerminalsPerWorkspace
	}
	if override, ok := c.Limits.WorkspaceOverrides[name]; ok && override.MaxTerminals > 0 {
		return override.MaxTerminals
	}
	if c.Limits.MaxTerminalsPerWorkspace > 0 {
		return c.Limits.MaxTerminalsPerWorkspace
	}
	return DefaultMaxTerminalsPerWorkspace
}

func (c *Config) GetMaxWorkspaces() int {
	if c == nil || c.Limits.MaxWorkspaces <= 0 {
		return DefaultMaxWorkspaces
	}
	return c.Limits.MaxWorkspaces
}

func (c *Config) GetMaxTerminalsTotal() int {
	if c == nil || c.Limits.MaxTerminalsTotal <= 0 {
		return DefaultMaxTerminalsTotal
	}
	return c.Limits.MaxTerminalsTotal
}

// GetLoggingConfig returns the logging configuration with defaults applied.
func (c *Config) GetLoggingConfig() LoggingConfig {
	if c == nil {
		return LoggingConfig{}
	}
	cfg := c.Logging
	if cfg.File == "" {
		home, err := os.UserHomeDir()
		if err != nil || home == "" {
			home = os.Getenv("HOME")
		}
		if home == "" {
			// Last resort fallback - use current directory
			home = "."
		}
		cfg.File = filepath.Join(home, ".local/share/termtile/agent-actions.log")
	}
	if cfg.MaxSizeMB == 0 {
		cfg.MaxSizeMB = 10
	}
	if cfg.MaxFiles == 0 {
		cfg.MaxFiles = 3
	}
	if cfg.PreviewLength == 0 {
		cfg.PreviewLength = 50
	}
	if cfg.Level == "" {
		cfg.Level = "info"
	}
	return cfg
}

// Save writes the configuration to the standard location.
//
// Note: this marshals the effective config and will not preserve comments or
// include/inherits structure from the original YAML.
func (c *Config) Save() error {
	if err := c.Validate(); err != nil {
		return err
	}

	path, err := DefaultConfigPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	save := *c
	save.Layouts = layoutsForSave(c.Layouts)
	save.TerminalSpawnCommands = spawnCommandsForSave(c.TerminalSpawnCommands)

	data, err := yaml.Marshal(&save)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}
	return nil
}

func layoutsForSave(layouts map[string]Layout) map[string]Layout {
	builtin := BuiltinLayouts()
	out := make(map[string]Layout)
	for name, layout := range layouts {
		if base, ok := builtin[name]; ok && base == layout {
			continue
		}
		out[name] = layout
	}
	return out
}

func spawnCommandsForSave(commands map[string]string) map[string]string {
	if len(commands) == 0 {
		return nil
	}
	defaults := DefaultConfig().TerminalSpawnCommands
	out := make(map[string]string)
	for class, cmd := range commands {
		if def, ok := defaults[class]; ok && def == cmd {
			continue
		}
		out[class] = cmd
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// GetMargins returns the margin configuration for a given terminal class.
func (c *Config) GetMargins(terminalClass string) Margins {
	if margins, ok := c.TerminalMargins[terminalClass]; ok {
		return margins
	}
	// Return zero margins if not configured.
	return Margins{}
}

// GetLayout retrieves a layout by name with validation.
func (c *Config) GetLayout(name string) (*Layout, error) {
	layout, ok := c.Layouts[name]
	if !ok {
		return nil, fmt.Errorf("layout %q not found", name)
	}

	if err := validateLayout(&layout); err != nil {
		return nil, fmt.Errorf("invalid layout %q: %w", name, err)
	}

	return &layout, nil
}

// GetDefaultLayout retrieves the default layout.
func (c *Config) GetDefaultLayout() (*Layout, error) {
	return c.GetLayout(c.DefaultLayout)
}

// Validate performs strict validation of the effective configuration.
func (c *Config) Validate() error {
	if c.Hotkey == "" {
		return &ValidationError{Path: "hotkey", Err: fmt.Errorf("hotkey is required")}
	}
	switch c.PaletteBackend {
	case "auto", "rofi", "fuzzel", "dmenu", "wofi":
	default:
		return &ValidationError{Path: "palette_backend", Err: fmt.Errorf("palette_backend must be one of: auto, rofi, fuzzel, dmenu, wofi")}
	}
	if c.TerminalSpawnCommands == nil {
		return &ValidationError{Path: "terminal_spawn_commands", Err: fmt.Errorf("terminal_spawn_commands must not be null")}
	}
	for class, cmd := range c.TerminalSpawnCommands {
		if strings.TrimSpace(class) == "" {
			return &ValidationError{Path: "terminal_spawn_commands", Err: fmt.Errorf("terminal_spawn_commands contains an empty class name")}
		}
		if strings.TrimSpace(cmd) == "" {
			return &ValidationError{Path: "terminal_spawn_commands." + class, Err: fmt.Errorf("spawn command must not be empty")}
		}
	}
	if c.GapSize < 0 {
		return &ValidationError{Path: "gap_size", Err: fmt.Errorf("gap_size must be >= 0")}
	}
	if c.ScreenPadding.Top < 0 || c.ScreenPadding.Bottom < 0 || c.ScreenPadding.Left < 0 || c.ScreenPadding.Right < 0 {
		return &ValidationError{Path: "screen_padding", Err: fmt.Errorf("screen_padding values must be >= 0")}
	}
	if len(c.TerminalClasses) == 0 {
		return &ValidationError{Path: "terminal_classes", Err: fmt.Errorf("terminal_classes must not be empty")}
	}
	if c.TerminalMargins == nil {
		return &ValidationError{Path: "terminal_margins", Err: fmt.Errorf("terminal_margins must not be null")}
	}
	if c.LogLevel != "debug" && c.LogLevel != "info" && c.LogLevel != "warning" && c.LogLevel != "error" {
		return &ValidationError{Path: "log_level", Err: fmt.Errorf("log_level must be one of: debug, info, warning, error")}
	}
	switch c.TerminalSort {
	case "position", "window_id", "client_list", "active_first":
	default:
		return &ValidationError{Path: "terminal_sort", Err: fmt.Errorf("terminal_sort must be one of: position, window_id, client_list, active_first")}
	}
	if c.Limits.MaxTerminalsPerWorkspace < 0 {
		return &ValidationError{Path: "limits.max_terminals_per_workspace", Err: fmt.Errorf("max_terminals_per_workspace must be >= 0")}
	}
	if c.Limits.MaxWorkspaces < 0 {
		return &ValidationError{Path: "limits.max_workspaces", Err: fmt.Errorf("max_workspaces must be >= 0")}
	}
	if c.Limits.MaxTerminalsTotal < 0 {
		return &ValidationError{Path: "limits.max_terminals_total", Err: fmt.Errorf("max_terminals_total must be >= 0")}
	}
	for name, override := range c.Limits.WorkspaceOverrides {
		if override.MaxTerminals < 0 {
			return &ValidationError{Path: "limits.workspace_overrides." + name + ".max_terminals", Err: fmt.Errorf("max_terminals must be >= 0")}
		}
	}

	if len(c.Layouts) == 0 {
		return &ValidationError{Path: "layouts", Err: fmt.Errorf("layouts must not be empty")}
	}
	if c.DefaultLayout == "" {
		return &ValidationError{Path: "default_layout", Err: fmt.Errorf("default_layout is required")}
	}
	if _, ok := c.Layouts[c.DefaultLayout]; !ok {
		return &ValidationError{Path: "default_layout", Err: fmt.Errorf("default_layout %q not found in layouts", c.DefaultLayout)}
	}

	for name, layout := range c.Layouts {
		layout := layout
		if err := validateLayout(&layout); err != nil {
			return &ValidationError{Path: "layouts." + name, Err: err}
		}
	}

	if warnings := c.validationWarnings(); len(warnings) > 0 {
		for _, w := range warnings {
			fmt.Fprintln(os.Stderr, "warning:", w)
		}
	}

	return nil
}

func (c *Config) validationWarnings() []string {
	if c == nil {
		return nil
	}

	var warnings []string

	if strings.TrimSpace(c.PreferredTerminal) != "" {
		if _, ok := c.matchTerminalClass(c.PreferredTerminal); !ok {
			warnings = append(warnings, fmt.Sprintf("preferred_terminal %q is not in terminal_classes; it will be ignored", c.PreferredTerminal))
		}
	}

	defaultCount := 0
	for _, tc := range c.TerminalClasses {
		if tc.Default {
			defaultCount++
		}
	}
	if defaultCount > 1 {
		warnings = append(warnings, fmt.Sprintf("terminal_classes has %d entries with default: true; the first one wins", defaultCount))
	}

	return warnings
}

// validateLayout checks if a layout configuration is valid.
func validateLayout(layout *Layout) error {
	switch layout.Mode {
	case LayoutModeAuto, LayoutModeFixed, LayoutModeVertical, LayoutModeHorizontal, LayoutModeMasterStack:
	default:
		return fmt.Errorf("invalid mode %q", layout.Mode)
	}

	if layout.Mode == LayoutModeFixed {
		if layout.FixedGrid.Rows <= 0 || layout.FixedGrid.Cols <= 0 {
			return fmt.Errorf("fixed mode requires rows and cols to be positive")
		}
	}

	if layout.Mode == LayoutModeMasterStack {
		if layout.MasterStack.MasterWidthPercent < 10 || layout.MasterStack.MasterWidthPercent > 90 {
			return fmt.Errorf("master_stack.master_width_percent must be between 10 and 90")
		}
		if layout.MasterStack.MaxStackRows < 1 {
			return fmt.Errorf("master_stack.max_stack_rows must be >= 1")
		}
		if layout.MasterStack.MaxStackCols < 1 {
			return fmt.Errorf("master_stack.max_stack_cols must be >= 1")
		}
	}

	if layout.MaxTerminalWidth < 0 || layout.MaxTerminalHeight < 0 {
		return fmt.Errorf("max_terminal_width/height must be >= 0")
	}

	switch layout.TileRegion.Type {
	case RegionFull, RegionLeftHalf, RegionRightHalf, RegionTopHalf, RegionBottomHalf:
		// ok
	case RegionCustom:
		if layout.TileRegion.XPercent < 0 || layout.TileRegion.XPercent > 100 {
			return fmt.Errorf("x_percent must be between 0 and 100")
		}
		if layout.TileRegion.YPercent < 0 || layout.TileRegion.YPercent > 100 {
			return fmt.Errorf("y_percent must be between 0 and 100")
		}
		if layout.TileRegion.WidthPercent <= 0 || layout.TileRegion.WidthPercent > 100 {
			return fmt.Errorf("width_percent must be between 1 and 100")
		}
		if layout.TileRegion.HeightPercent <= 0 || layout.TileRegion.HeightPercent > 100 {
			return fmt.Errorf("height_percent must be between 1 and 100")
		}
		if layout.TileRegion.XPercent+layout.TileRegion.WidthPercent > 100 {
			return fmt.Errorf("x_percent + width_percent must be <= 100")
		}
		if layout.TileRegion.YPercent+layout.TileRegion.HeightPercent > 100 {
			return fmt.Errorf("y_percent + height_percent must be <= 100")
		}
	default:
		return fmt.Errorf("invalid region type %q", layout.TileRegion.Type)
	}

	return nil
}

func defaultTerminalClasses() TerminalClassList {
	return TerminalClassList{
		{Class: "Alacritty"},
		{Class: "kitty"},
		{Class: "com.mitchellh.ghostty"},
		{Class: "ghostty"},
		{Class: "Gnome-terminal"},
		{Class: "gnome-terminal-server"},
		{Class: "Tilix"},
		{Class: "com.gexperts.Tilix"},
		{Class: "XTerm"},
		{Class: "UXTerm"},
		{Class: "konsole"},
		{Class: "terminator"},
		{Class: "Terminator"},
		{Class: "URxvt"},
		{Class: "urxvt"},
		{Class: "st"},
		{Class: "st-256color"},
		{Class: "wezterm"},
	}
}

func firstLayoutName(layouts map[string]Layout) string {
	if len(layouts) == 0 {
		return ""
	}
	names := make([]string, 0, len(layouts))
	for name := range layouts {
		names = append(names, name)
	}
	sort.Strings(names)
	return names[0]
}
