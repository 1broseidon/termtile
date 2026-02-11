package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/1broseidon/termtile/internal/agent"
	"github.com/1broseidon/termtile/internal/config"
	"github.com/1broseidon/termtile/internal/ipc"
	"github.com/1broseidon/termtile/internal/platform"
	"github.com/1broseidon/termtile/internal/terminals"
	"github.com/1broseidon/termtile/internal/workspace"
	"github.com/BurntSushi/xgb/xproto"
	"github.com/BurntSushi/xgbutil"
	"github.com/BurntSushi/xgbutil/ewmh"
	"github.com/BurntSushi/xgbutil/icccm"
	"gopkg.in/yaml.v3"
)

// logWorkspaceAction logs a workspace action using the shared terminal logger.
func logWorkspaceAction(action agent.ActionType, wsName string, slot int, details map[string]interface{}) {
	if logger := getTerminalLogger(); logger != nil {
		logger.Log(action, wsName, slot, details)
	}
}

// x11Accessor is an optional interface for backends that expose X11 internals.
type x11Accessor interface {
	XUtil() *xgbutil.XUtil
	RootWindow() xproto.Window
}

type platformTerminalLister struct {
	backend  platform.Backend
	detector *terminals.Detector
	xu       *xgbutil.XUtil // optional, for PID/title lookups
}

func (l *platformTerminalLister) ListTerminals() ([]workspace.TerminalWindow, error) {
	display, err := l.backend.ActiveDisplay()
	if err != nil {
		return nil, err
	}

	terms, err := l.detector.FindTerminals(l.backend, display.ID, display.Bounds)
	if err != nil {
		return nil, err
	}

	out := make([]workspace.TerminalWindow, 0, len(terms))
	for _, t := range terms {
		pid := 0
		if l.xu != nil {
			if p, err := ewmh.WmPidGet(l.xu, xproto.Window(t.WindowID)); err == nil {
				pid = int(p)
			}
		}
		out = append(out, workspace.TerminalWindow{
			WindowID: uint32(t.WindowID),
			WMClass:  t.Class,
			X:        t.X,
			Y:        t.Y,
			PID:      pid,
		})
	}

	return out, nil
}

func (l *platformTerminalLister) ActiveWindowID() (uint32, error) {
	win, err := l.backend.ActiveWindow()
	return uint32(win), err
}

func (l *platformTerminalLister) WindowTitle(windowID uint32) (string, error) {
	if l.xu == nil {
		return "", fmt.Errorf("no X11 connection for title lookup")
	}
	win := xproto.Window(windowID)

	title, err := ewmh.WmNameGet(l.xu, win)
	if err == nil {
		title = strings.TrimSpace(title)
		if title != "" {
			return title, nil
		}
	}

	title, err2 := icccm.WmNameGet(l.xu, win)
	if err2 == nil {
		title = strings.TrimSpace(title)
		if title != "" {
			return title, nil
		}
	}

	if err != nil {
		return "", err
	}
	return "", err2
}

type platformWindowMinimizer struct {
	backend platform.Backend
}

func (m *platformWindowMinimizer) MinimizeWindow(windowID uint32) error {
	return m.backend.Minimize(platform.WindowID(windowID))
}

type ipcLayoutApplier struct {
	client *ipc.Client
}

func (a *ipcLayoutApplier) ApplyLayout(layoutName string, tileNow bool) error {
	return a.client.ApplyLayout(layoutName, tileNow)
}

func (a *ipcLayoutApplier) ApplyLayoutWithOrder(layoutName string, windowOrder []uint32) error {
	return a.client.ApplyLayoutWithOrder(layoutName, windowOrder)
}

// newTerminalLister creates a terminal lister from a platform backend.
func newTerminalLister(backend platform.Backend, cfg *config.Config) *platformTerminalLister {
	var xu *xgbutil.XUtil
	if accessor, ok := backend.(x11Accessor); ok {
		xu = accessor.XUtil()
	}
	return &platformTerminalLister{
		backend:  backend,
		detector: terminals.NewDetector(cfg.TerminalClassNames()),
		xu:       xu,
	}
}

func runWorkspace(args []string) int {
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		fmt.Fprintln(os.Stderr, "Usage:")
		fmt.Fprintln(os.Stderr, "  termtile workspace new [flags] <name>     Create and launch a new workspace")
		fmt.Fprintln(os.Stderr, "  termtile workspace save [flags] <name>    Save current terminal state")
		fmt.Fprintln(os.Stderr, "  termtile workspace load [flags] <name>    Load a saved workspace")
		fmt.Fprintln(os.Stderr, "  termtile workspace close <name>           Close active workspace")
		fmt.Fprintln(os.Stderr, "  termtile workspace list                   List saved workspaces")
		fmt.Fprintln(os.Stderr, "  termtile workspace delete <name>          Delete a saved workspace")
		fmt.Fprintln(os.Stderr, "  termtile workspace rename <old> <new>     Rename a workspace")
		fmt.Fprintln(os.Stderr, "  termtile workspace init --workspace <name> Initialize project workspace config")
		fmt.Fprintln(os.Stderr, "  termtile workspace link --workspace <name> Link project to a canonical workspace")
		fmt.Fprintln(os.Stderr, "  termtile workspace sync pull|push          Sync project view pull/push")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Run 'termtile workspace <command> --help' for command-specific options.")
		return 2
	}

	switch args[0] {
	case "list":
		names, err := workspace.List()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		for _, name := range names {
			fmt.Printf("- %s\n", name)
		}
		return 0

	case "new":
		fs := flag.NewFlagSet("new", flag.ContinueOnError)
		fs.SetOutput(os.Stderr)
		fs.Usage = func() {
			fmt.Fprintln(os.Stderr, "Usage: termtile workspace new [flags] <name>")
			fmt.Fprintln(os.Stderr, "")
			fmt.Fprintln(os.Stderr, "Create and launch a new workspace with fresh terminal windows.")
			fmt.Fprintln(os.Stderr, "")
			fmt.Fprintln(os.Stderr, "Flags:")
			fs.PrintDefaults()
			fmt.Fprintln(os.Stderr, "")
			fmt.Fprintln(os.Stderr, "Examples:")
			fmt.Fprintln(os.Stderr, "  termtile workspace new myproject              # 3 terminals in current directory")
			fmt.Fprintln(os.Stderr, "  termtile workspace new -n 4 dev               # 4 terminals")
			fmt.Fprintln(os.Stderr, "  termtile workspace new -n 2 --cwd ~/code api  # 2 terminals in ~/code")
			fmt.Fprintln(os.Stderr, "  termtile workspace new --agent-mode agents    # With tmux sessions for agent control")
		}
		path := fs.String("path", "", "Config file path")
		numTerminals := fs.Int("n", 3, "Number of terminal windows to create")
		cwd := fs.String("cwd", "", "Working directory for all terminals (default: current directory)")
		layout := fs.String("layout", "", "Layout to use (default: active or config default)")
		agentMode := fs.Bool("agent-mode", false, "Create tmux sessions for inter-terminal agent control")
		terminalClass := fs.String("terminal", "", "Terminal class to use (default: resolved from config and system defaults)")
		ignoreLimits := fs.Bool("ignore-limits", false, "Ignore configured workspace limits")
		timeout := fs.Int("timeout", 10, "Spawn synchronization timeout in seconds")

		if err := fs.Parse(args[1:]); err != nil {
			if err == flag.ErrHelp {
				return 0
			}
			return 2
		}
		if fs.NArg() < 1 {
			fmt.Fprintln(os.Stderr, "workspace new requires <name>")
			fs.Usage()
			return 2
		}
		name := fs.Arg(0)

		// Load config
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

		if !*ignoreLimits {
			activeWs, err := workspace.GetActiveWorkspace()
			if err != nil || activeWs.Name == "" {
				if err := workspace.CheckCanCreateWorkspace(res.Config); err != nil {
					fmt.Fprintln(os.Stderr, "cannot create workspace:", err)
					return 1
				}
			}
			if err := workspace.CheckCanCreateTerminals(name, *numTerminals, res.Config); err != nil {
				fmt.Fprintln(os.Stderr, "cannot create workspace:", err)
				return 1
			}
		}

		// Determine working directory
		workDir := *cwd
		if workDir == "" {
			workDir, err = os.Getwd()
			if err != nil {
				fmt.Fprintln(os.Stderr, "failed to get current directory:", err)
				return 1
			}
		}

		// Determine layout
		layoutName := *layout
		if layoutName == "" {
			// Try to get active layout from daemon
			if status, err := ipc.NewClient().GetStatus(); err == nil && status.ActiveLayout != "" {
				layoutName = status.ActiveLayout
			} else {
				layoutName = res.Config.DefaultLayout
			}
		}

		// Determine terminal class
		termClass := *terminalClass
		if termClass == "" {
			termClass = res.Config.ResolveTerminal()
			if termClass == "" {
				fmt.Fprintln(os.Stderr, "no terminal classes configured; set terminal_classes in config or use --terminal")
				return 1
			}
		}

		// Build workspace config
		ws := &workspace.WorkspaceConfig{
			Name:      name,
			Layout:    layoutName,
			AgentMode: *agentMode,
			Terminals: make([]workspace.TerminalConfig, *numTerminals),
		}
		for i := 0; i < *numTerminals; i++ {
			ws.Terminals[i] = workspace.TerminalConfig{
				WMClass:   termClass,
				Cwd:       workDir,
				SlotIndex: i,
			}
		}

		// Connect to display
		backend, err := platform.NewLinuxBackendFromDisplay()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		defer backend.Disconnect()

		lister := newTerminalLister(backend, res.Config)

		applier := &ipcLayoutApplier{client: ipc.NewClient()}
		if err := applier.client.Ping(); err != nil {
			fmt.Fprintln(os.Stderr, "daemon not running:", err)
			return 1
		}

		minimizer := &platformWindowMinimizer{backend: backend}

		// Get current layout for auto-save
		autoSaveLayout := res.Config.DefaultLayout
		if status, err := applier.client.GetStatus(); err == nil && status.ActiveLayout != "" {
			autoSaveLayout = status.ActiveLayout
		}

		// Load the workspace (spawns terminals, tiles, etc.)
		if err := workspace.Load(ws, res.Config.TerminalSpawnCommands, lister, minimizer, applier, workspace.LoadOptions{
			Timeout:              time.Duration(*timeout) * time.Second,
			AutoSaveLayout:       autoSaveLayout,
			AutoSaveTerminalSort: res.Config.TerminalSort,
			AppConfig:            res.Config,
		}); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}

		// Save the workspace config
		if err := workspace.Write(ws); err != nil {
			fmt.Fprintln(os.Stderr, "warning: workspace created but failed to save:", err)
		}

		// Collect agent slots for agent-mode workspaces
		var agentSlots []int
		if ws.AgentMode {
			for _, t := range ws.Terminals {
				agentSlots = append(agentSlots, t.SlotIndex)
			}
		}

		// Record active workspace on current desktop with agent slots
		if err := workspace.SetActiveWorkspace(ws.Name, len(ws.Terminals), ws.AgentMode, -1, agentSlots); err != nil {
			fmt.Fprintln(os.Stderr, "warning:", err)
		}

		// Log workspace creation
		logWorkspaceAction(agent.ActionWorkspaceNew, name, -1, map[string]interface{}{
			"terminals": *numTerminals,
		})

		fmt.Printf("Created workspace %q with %d terminals\n", name, *numTerminals)
		return 0

	case "delete":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "workspace delete requires <name>")
			return 2
		}
		if err := workspace.Delete(args[1]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		return 0

	case "save":
		fs := flag.NewFlagSet("save", flag.ContinueOnError)
		fs.SetOutput(os.Stderr)
		path := fs.String("path", "", "Config file path (default: ~/.config/termtile/config.yaml)")
		includeCmd := fs.Bool("cmd", false, "Also capture /proc/PID/cmdline (best-effort)")
		agentMode := fs.Bool("agent-mode", false, "Spawn this workspace inside tmux sessions for inter-terminal agent control")
		if err := fs.Parse(args[1:]); err != nil {
			return 2
		}
		if fs.NArg() < 1 {
			fmt.Fprintln(os.Stderr, "workspace save requires <name>")
			return 2
		}
		name := fs.Arg(0)

		// Check if there's a workspace on the current desktop
		activeWs, err := workspace.GetActiveWorkspace()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		if activeWs.Name == "" {
			fmt.Fprintln(os.Stderr, "no workspace on current desktop")
			return 1
		}

		var res *config.LoadResult
		if *path == "" {
			res, err = config.LoadWithSources()
		} else {
			res, err = config.LoadFromPath(*path)
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}

		layout := res.Config.DefaultLayout
		if status, err := ipc.NewClient().GetStatus(); err == nil && status.ActiveLayout != "" {
			layout = status.ActiveLayout
		}

		backend, err := platform.NewLinuxBackendFromDisplay()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		defer backend.Disconnect()

		lister := newTerminalLister(backend, res.Config)

		ws, err := workspace.Save(name, layout, res.Config.TerminalSort, *includeCmd, lister)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		// Preserve agent mode from active workspace state, or use explicit flag
		ws.AgentMode = *agentMode || activeWs.AgentMode
		if err := workspace.Write(ws); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		return 0

	case "load":
		fs := flag.NewFlagSet("load", flag.ContinueOnError)
		fs.SetOutput(os.Stderr)
		path := fs.String("path", "", "Config file path (default: ~/.config/termtile/config.yaml)")
		timeoutSeconds := fs.Int("timeout", 10, "Spawn synchronization timeout in seconds")
		rerun := fs.Bool("rerun", false, "If your spawn template includes {{cmd}}, substitute the saved cmdline")
		noReplace := fs.Bool("no-replace", false, "Add new terminals without minimizing existing ones or auto-saving to _previous")
		ignoreLimits := fs.Bool("ignore-limits", false, "Ignore configured workspace limits")
		if err := fs.Parse(args[1:]); err != nil {
			return 2
		}
		if fs.NArg() < 1 {
			fmt.Fprintln(os.Stderr, "workspace load requires <name>")
			return 2
		}
		name := fs.Arg(0)

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

		ws, err := workspace.Read(name)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}

		if !*ignoreLimits {
			activeWs, err := workspace.GetActiveWorkspace()
			if err != nil || activeWs.Name == "" {
				if err := workspace.CheckCanCreateWorkspace(res.Config); err != nil {
					fmt.Fprintln(os.Stderr, "cannot load workspace:", err)
					return 1
				}
			}
			if err := workspace.CheckCanCreateTerminals(ws.Name, len(ws.Terminals), res.Config); err != nil {
				fmt.Fprintln(os.Stderr, "cannot load workspace:", err)
				return 1
			}
		}

		backend, err := platform.NewLinuxBackendFromDisplay()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		defer backend.Disconnect()

		lister := newTerminalLister(backend, res.Config)

		applier := &ipcLayoutApplier{client: ipc.NewClient()}
		if err := applier.client.Ping(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}

		var minimizer workspace.WindowMinimizer
		if !*noReplace {
			minimizer = &platformWindowMinimizer{backend: backend}
		}

		autoSaveLayout := ""
		autoSaveTerminalSort := ""
		if !*noReplace && ws.Name != "_previous" {
			autoSaveLayout = res.Config.DefaultLayout
			if status, err := applier.client.GetStatus(); err == nil && status.ActiveLayout != "" {
				autoSaveLayout = status.ActiveLayout
			}
			autoSaveTerminalSort = res.Config.TerminalSort
		}

		if err := workspace.Load(ws, res.Config.TerminalSpawnCommands, lister, minimizer, applier, workspace.LoadOptions{
			Timeout:      time.Duration(*timeoutSeconds) * time.Second,
			RerunCommand: *rerun,
			NoReplace:    *noReplace,

			AutoSaveLayout:       autoSaveLayout,
			AutoSaveTerminalSort: autoSaveTerminalSort,
		}); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}

		// Collect agent slots for agent-mode workspaces
		var agentSlots []int
		if ws.AgentMode {
			for _, t := range ws.Terminals {
				agentSlots = append(agentSlots, t.SlotIndex)
			}
		}

		// Record active workspace on current desktop with agent slots
		if err := workspace.SetActiveWorkspace(ws.Name, len(ws.Terminals), ws.AgentMode, -1, agentSlots); err != nil {
			fmt.Fprintln(os.Stderr, "warning:", err)
		}

		return 0

	case "close":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "workspace close requires <name>")
			return 2
		}
		name := args[1]

		// Verify this is the active workspace on the current desktop
		activeWs, err := workspace.GetActiveWorkspace()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		if activeWs.Name == "" {
			fmt.Fprintln(os.Stderr, "no workspace on current desktop")
			return 1
		}
		if activeWs.Name != name {
			fmt.Fprintf(os.Stderr, "workspace %q is not on the current desktop (current desktop has: %q)\n", name, activeWs.Name)
			return 1
		}

		// Load config to get terminal classes
		res, err := config.LoadWithSources()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}

		backend, err := platform.NewLinuxBackendFromDisplay()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		defer backend.Disconnect()

		lister := newTerminalLister(backend, res.Config)

		// Close all terminal windows
		if err := workspace.CloseTerminals(lister); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}

		// Clear workspace state on current desktop
		if err := workspace.ClearWorkspace(-1); err != nil {
			fmt.Fprintln(os.Stderr, "warning:", err)
		}

		// Log workspace close
		logWorkspaceAction(agent.ActionWorkspaceClose, name, -1, map[string]interface{}{
			"terminals": activeWs.TerminalCount,
		})

		return 0

	case "rename":
		return runWorkspaceRename(args[1:])
	case "init":
		return runProjectInit(args[1:])
	case "link":
		return runProjectLink(args[1:])
	case "sync":
		return runProjectSync(args[1:])

	default:
		fmt.Fprintf(os.Stderr, "Unknown workspace subcommand: %s\n", args[0])
		return 2
	}
}

const (
	projectDirName          = ".termtile"
	projectWorkspaceCfgFile = "workspace.yaml"
	projectLocalCfgFile     = "local.yaml"
)

var projectSyncAllowedFields = map[string]struct{}{
	"layout":     {},
	"terminals":  {},
	"agent_mode": {},
}

type projectWorkspaceConfig struct {
	Version            int                       `yaml:"version"`
	Workspace          string                    `yaml:"workspace"`
	Project            projectSettings           `yaml:"project"`
	MCP                projectMCPSettings        `yaml:"mcp"`
	Agents             projectAgentsSettings     `yaml:"agents"`
	WorkspaceOverrides projectWorkspaceOverrides `yaml:"workspace_overrides"`
	Sync               projectSyncSettings       `yaml:"sync"`
}

type projectSettings struct {
	RootMarker string  `yaml:"root_marker"`
	CwdMode    string  `yaml:"cwd_mode"`
	Cwd        *string `yaml:"cwd"`
}

type projectMCPSettings struct {
	Spawn projectMCPSpawn `yaml:"spawn"`
	Read  projectMCPRead  `yaml:"read"`
}

type projectMCPSpawn struct {
	RequireExplicitWorkspace bool     `yaml:"require_explicit_workspace"`
	ResolutionOrder          []string `yaml:"resolution_order"`
}

type projectMCPRead struct {
	DefaultLines     int  `yaml:"default_lines"`
	MaxLines         int  `yaml:"max_lines"`
	SinceLastDefault bool `yaml:"since_last_default"`
}

type projectAgentsSettings struct {
	Defaults  projectAgentDefaults             `yaml:"defaults"`
	Overrides map[string]projectAgentOverrides `yaml:"overrides"`
}

type projectAgentDefaults struct {
	SpawnMode string            `yaml:"spawn_mode"`
	Model     *string           `yaml:"model"`
	Env       map[string]string `yaml:"env"`
}

type projectAgentOverrides struct {
	Model *string           `yaml:"model"`
	Env   map[string]string `yaml:"env"`
}

type projectWorkspaceOverrides struct {
	Layout               any `yaml:"layout"`
	Terminal             any `yaml:"terminal"`
	TerminalSpawnCommand any `yaml:"terminal_spawn_command"`
}

type projectSyncSettings struct {
	Mode                string   `yaml:"mode"`
	PullOnWorkspaceLoad bool     `yaml:"pull_on_workspace_load"`
	PushOnWorkspaceSave bool     `yaml:"push_on_workspace_save"`
	Include             []string `yaml:"include"`
}

type projectLocalConfig struct {
	Version   int                      `yaml:"version"`
	Workspace string                   `yaml:"workspace"`
	Snapshot  projectWorkspaceSnapshot `yaml:"snapshot"`
}

type projectWorkspaceSnapshot struct {
	Layout    *string                     `yaml:"layout,omitempty"`
	Terminals *[]workspace.TerminalConfig `yaml:"terminals,omitempty"`
	AgentMode *bool                       `yaml:"agent_mode,omitempty"`
}

func runProjectInit(args []string) int {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	workspaceName := fs.String("workspace", "", "Canonical workspace name to link to")
	force := fs.Bool("force", false, "Overwrite existing .termtile/workspace.yaml")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: termtile workspace init --workspace <name> [--force]")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Create .termtile/workspace.yaml with schema v1 defaults.")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Flags:")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "workspace init takes no positional arguments")
		fs.Usage()
		return 2
	}
	if strings.TrimSpace(*workspaceName) == "" {
		fmt.Fprintln(os.Stderr, "workspace init requires --workspace")
		fs.Usage()
		return 2
	}
	if err := workspace.ValidateWorkspaceName(*workspaceName); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}

	root, err := resolveProjectRootForInit()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	projectDir := filepath.Join(root, projectDirName)
	projectCfgPath := filepath.Join(projectDir, projectWorkspaceCfgFile)

	if _, err := os.Stat(projectCfgPath); err == nil && !*force {
		fmt.Fprintf(os.Stderr, "%s already exists (use --force to overwrite)\n", projectCfgPath)
		return 1
	}

	if err := os.MkdirAll(projectDir, 0755); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	cfg := defaultProjectWorkspaceConfig(*workspaceName)
	if err := writeProjectWorkspaceConfig(projectCfgPath, cfg); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if err := ensureProjectGitignore(projectDir); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	fmt.Printf("Initialized %s\n", projectCfgPath)
	return 0
}

func runProjectLink(args []string) int {
	fs := flag.NewFlagSet("link", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	workspaceName := fs.String("workspace", "", "Canonical workspace name to link to")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: termtile workspace link --workspace <name>")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Update only the workspace binding in .termtile/workspace.yaml.")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Flags:")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "workspace link takes no positional arguments")
		fs.Usage()
		return 2
	}
	if strings.TrimSpace(*workspaceName) == "" {
		fmt.Fprintln(os.Stderr, "workspace link requires --workspace")
		fs.Usage()
		return 2
	}
	if err := workspace.ValidateWorkspaceName(*workspaceName); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}

	cfgPath, err := findProjectWorkspaceConfigPath()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	cfg, err := readProjectWorkspaceConfig(cfgPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	cfg.Workspace = *workspaceName
	if err := writeProjectWorkspaceConfig(cfgPath, cfg); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	fmt.Printf("Linked project to workspace %q\n", *workspaceName)
	return 0
}

func runProjectSync(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "workspace sync requires pull or push")
		return 2
	}
	if args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		fmt.Fprintln(os.Stdout, "Usage: termtile workspace sync pull|push")
		return 0
	}
	if len(args) > 1 {
		fmt.Fprintln(os.Stderr, "workspace sync takes only one positional argument: pull|push")
		return 2
	}

	cfgPath, err := findProjectWorkspaceConfigPath()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	projectCfg, err := readProjectWorkspaceConfig(cfgPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if strings.TrimSpace(projectCfg.Workspace) == "" {
		fmt.Fprintf(os.Stderr, "%s is missing workspace binding\n", cfgPath)
		return 1
	}

	include, err := normalizeProjectSyncInclude(projectCfg.Sync.Include)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	projectDir := filepath.Dir(cfgPath)
	localCfgPath := filepath.Join(projectDir, projectLocalCfgFile)

	switch args[0] {
	case "pull":
		if err := projectSyncPull(projectCfg.Workspace, include, localCfgPath); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		fmt.Printf("Pulled %s from workspace %q into %s\n", strings.Join(include, ", "), projectCfg.Workspace, localCfgPath)
		return 0
	case "push":
		if err := projectSyncPush(projectCfg.Workspace, include, localCfgPath); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		fmt.Printf("Pushed %s from %s to workspace %q\n", strings.Join(include, ", "), localCfgPath, projectCfg.Workspace)
		return 0
	default:
		fmt.Fprintf(os.Stderr, "unknown sync mode %q (expected pull or push)\n", args[0])
		return 2
	}
}

func normalizeProjectSyncInclude(include []string) ([]string, error) {
	if len(include) == 0 {
		return []string{"layout", "terminals", "agent_mode"}, nil
	}
	out := make([]string, 0, len(include))
	seen := make(map[string]struct{}, len(include))
	for _, v := range include {
		key := strings.TrimSpace(v)
		if key == "" {
			continue
		}
		if _, ok := projectSyncAllowedFields[key]; !ok {
			return nil, fmt.Errorf("unsupported sync.include field %q", key)
		}
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, key)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("sync.include is empty")
	}
	return out, nil
}

func projectSyncPull(workspaceName string, include []string, localCfgPath string) error {
	ws, err := workspace.Read(workspaceName)
	if err != nil {
		return err
	}
	localCfg, err := readProjectLocalConfig(localCfgPath)
	if err != nil {
		return err
	}
	localCfg.Version = 1
	localCfg.Workspace = workspaceName

	for _, field := range include {
		switch field {
		case "layout":
			layout := ws.Layout
			localCfg.Snapshot.Layout = &layout
		case "terminals":
			terms := make([]workspace.TerminalConfig, len(ws.Terminals))
			copy(terms, ws.Terminals)
			localCfg.Snapshot.Terminals = &terms
		case "agent_mode":
			agentMode := ws.AgentMode
			localCfg.Snapshot.AgentMode = &agentMode
		}
	}

	return writeProjectLocalConfig(localCfgPath, localCfg)
}

func projectSyncPush(workspaceName string, include []string, localCfgPath string) error {
	localCfg, err := readProjectLocalConfig(localCfgPath)
	if err != nil {
		return err
	}
	ws, err := workspace.Read(workspaceName)
	if err != nil {
		return err
	}

	for _, field := range include {
		switch field {
		case "layout":
			if localCfg.Snapshot.Layout == nil {
				return fmt.Errorf("%s: snapshot.layout is required for push", localCfgPath)
			}
			ws.Layout = *localCfg.Snapshot.Layout
		case "terminals":
			if localCfg.Snapshot.Terminals == nil {
				return fmt.Errorf("%s: snapshot.terminals is required for push", localCfgPath)
			}
			terms := make([]workspace.TerminalConfig, len(*localCfg.Snapshot.Terminals))
			copy(terms, *localCfg.Snapshot.Terminals)
			ws.Terminals = terms
		case "agent_mode":
			if localCfg.Snapshot.AgentMode == nil {
				return fmt.Errorf("%s: snapshot.agent_mode is required for push", localCfgPath)
			}
			ws.AgentMode = *localCfg.Snapshot.AgentMode
		}
	}

	return workspace.Write(ws)
}

func resolveProjectRootForInit() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return findProjectRootFrom(cwd), nil
}

func findProjectWorkspaceConfigPath() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	root := findProjectRootFrom(cwd)
	path := filepath.Join(root, projectDirName, projectWorkspaceCfgFile)
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("project config not found: %s (run 'termtile workspace init --workspace <name>')", path)
		}
		return "", err
	}
	return path, nil
}

func findProjectRootFrom(start string) string {
	dir := start
	for {
		if exists(filepath.Join(dir, projectDirName, projectWorkspaceCfgFile)) {
			return dir
		}
		if exists(filepath.Join(dir, ".git")) {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return start
		}
		dir = parent
	}
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func defaultProjectWorkspaceConfig(workspaceName string) *projectWorkspaceConfig {
	return &projectWorkspaceConfig{
		Version:   1,
		Workspace: workspaceName,
		Project: projectSettings{
			RootMarker: ".git",
			CwdMode:    "project_root",
			Cwd:        nil,
		},
		MCP: projectMCPSettings{
			Spawn: projectMCPSpawn{
				RequireExplicitWorkspace: false,
				ResolutionOrder: []string{
					"explicit_arg",
					"source_workspace_hint",
					"project_marker",
					"single_registered_agent_workspace",
					"error",
				},
			},
			Read: projectMCPRead{
				DefaultLines:     50,
				MaxLines:         100,
				SinceLastDefault: false,
			},
		},
		Agents: projectAgentsSettings{
			Defaults: projectAgentDefaults{
				SpawnMode: "window",
				Model:     nil,
				Env:       map[string]string{},
			},
			Overrides: map[string]projectAgentOverrides{
				"codex": {
					Model: stringPtr("gpt-5"),
					Env: map[string]string{
						"TERM": "xterm-256color",
					},
				},
				"claude": {
					Model: nil,
					Env:   map[string]string{},
				},
			},
		},
		WorkspaceOverrides: projectWorkspaceOverrides{
			Layout:               nil,
			Terminal:             nil,
			TerminalSpawnCommand: nil,
		},
		Sync: projectSyncSettings{
			Mode:                "linked",
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

func stringPtr(v string) *string {
	return &v
}

func ensureProjectGitignore(projectDir string) error {
	path := filepath.Join(projectDir, ".gitignore")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return os.WriteFile(path, []byte(projectLocalCfgFile+"\n"), 0644)
		}
		return err
	}

	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	for _, line := range lines {
		if strings.TrimSpace(line) == projectLocalCfgFile {
			return nil
		}
	}

	var b strings.Builder
	b.WriteString(strings.TrimRight(string(data), "\n"))
	if b.Len() > 0 {
		b.WriteByte('\n')
	}
	b.WriteString(projectLocalCfgFile)
	b.WriteByte('\n')
	return os.WriteFile(path, []byte(b.String()), 0644)
}

func readProjectWorkspaceConfig(path string) (*projectWorkspaceConfig, error) {
	cfg := &projectWorkspaceConfig{}
	if err := decodeStrictYAMLFile(path, cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

func writeProjectWorkspaceConfig(path string, cfg *projectWorkspaceConfig) error {
	if cfg == nil {
		return fmt.Errorf("project workspace config is nil")
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func readProjectLocalConfig(path string) (*projectLocalConfig, error) {
	cfg := &projectLocalConfig{Version: 1}
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return nil, err
	}
	if err := decodeStrictYAMLFile(path, cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

func writeProjectLocalConfig(path string, cfg *projectLocalConfig) error {
	if cfg == nil {
		return fmt.Errorf("project local config is nil")
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func decodeStrictYAMLFile(path string, out any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(out); err != nil && err != io.EOF {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}

// closeWindowViaBackend closes a window using the platform backend.
func closeWindowViaBackend(backend platform.Backend, windowID uint32) error {
	return backend.Close(platform.WindowID(windowID))
}

// spawnTerminalWithCommand spawns a terminal with an optional command override.
func spawnTerminalWithCommand(term workspace.TerminalConfig, templates map[string]string, cmdOverride string) error {
	class := strings.TrimSpace(term.WMClass)
	if class == "" {
		return fmt.Errorf("terminal WMClass is empty")
	}

	template, ok := lookupSpawnTemplate(templates, class)
	if !ok {
		return fmt.Errorf("no spawn template configured for terminal class %q (set terminal_spawn_commands.%s)", class, class)
	}
	if cmdOverride != "" && !strings.Contains(template, "{{cmd}}") {
		return fmt.Errorf("spawn template for %q must include {{cmd}} for agent-mode workspaces (set terminal_spawn_commands.%s)", class, class)
	}

	cwd := strings.TrimSpace(term.Cwd)
	if cwd == "" {
		home, _ := os.UserHomeDir()
		cwd = home
	}

	argv, err := renderCommandTemplate(template, cwd, cmdOverride)
	if err != nil {
		return fmt.Errorf("failed to render spawn template for %q: %w", class, err)
	}
	if len(argv) == 0 {
		return fmt.Errorf("spawn template for %q produced empty command", class)
	}

	cmd := exec.Command(argv[0], argv[1:]...)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to spawn %q: %w", class, err)
	}
	return nil
}

// lookupSpawnTemplate looks up spawn template for a terminal class.
func lookupSpawnTemplate(templates map[string]string, class string) (string, bool) {
	if templates == nil {
		return "", false
	}
	if v, ok := templates[class]; ok {
		return v, true
	}
	if v, ok := templates[strings.ToLower(class)]; ok {
		return v, true
	}
	lower := strings.ToLower(class)
	for k, v := range templates {
		if strings.ToLower(k) == lower {
			return v, true
		}
	}
	return "", false
}

// renderCommandTemplate renders a spawn command template with directory and command.
func renderCommandTemplate(template, dir, cmd string) ([]string, error) {
	argv, err := splitCommand(template)
	if err != nil {
		return nil, err
	}

	argvOut := make([]string, 0, len(argv))
	for _, arg := range argv {
		hadCmdPlaceholder := strings.Contains(arg, "{{cmd}}")
		arg = strings.ReplaceAll(arg, "{{dir}}", dir)
		if cmd != "" {
			arg = strings.ReplaceAll(arg, "{{cmd}}", cmd)
		} else {
			arg = strings.ReplaceAll(arg, "{{cmd}}", "")
		}
		arg = strings.TrimSpace(arg)
		if arg == "" {
			if hadCmdPlaceholder && cmd == "" && len(argvOut) > 0 {
				prev := argvOut[len(argvOut)-1]
				if strings.HasPrefix(prev, "-") {
					argvOut = argvOut[:len(argvOut)-1]
				}
			}
			continue
		}
		if hadCmdPlaceholder && cmd != "" {
			parts, err := splitCommand(arg)
			if err == nil && len(parts) > 0 {
				argvOut = append(argvOut, parts...)
				continue
			}
		}
		argvOut = append(argvOut, arg)
	}

	return argvOut, nil
}

// splitCommand splits a shell command string into arguments.
func splitCommand(s string) ([]string, error) {
	var out []string
	var buf strings.Builder
	inSingle := false
	inDouble := false
	escaped := false

	flush := func() {
		if buf.Len() == 0 {
			return
		}
		out = append(out, buf.String())
		buf.Reset()
	}

	for _, r := range s {
		if escaped {
			buf.WriteRune(r)
			escaped = false
			continue
		}
		if !inSingle && r == '\\' {
			escaped = true
			continue
		}
		if !inDouble && r == '\'' {
			inSingle = !inSingle
			continue
		}
		if !inSingle && r == '"' {
			inDouble = !inDouble
			continue
		}
		if !inSingle && !inDouble {
			if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
				flush()
				continue
			}
		}
		buf.WriteRune(r)
	}

	if escaped {
		return nil, fmt.Errorf("unfinished escape in command template")
	}
	if inSingle || inDouble {
		return nil, fmt.Errorf("unterminated quote in command template")
	}

	flush()
	return out, nil
}

// shellJoin joins arguments into a shell command string.
func shellJoin(argv []string) string {
	if len(argv) == 0 {
		return ""
	}
	parts := make([]string, 0, len(argv))
	for _, a := range argv {
		parts = append(parts, shellQuote(a))
	}
	return strings.Join(parts, " ")
}

// shellQuote quotes a string for shell usage.
func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	if !strings.ContainsAny(s, " \t\r\n'\"\\$`(){}[]*?!;|&<>") {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// waitForNewTerminal waits for a single new terminal to appear.
func waitForNewTerminal(lister *platformTerminalLister, existing map[uint32]struct{}, timeout time.Duration) ([]uint32, error) {
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(150 * time.Millisecond)
	defer ticker.Stop()

	var newIDs []uint32
	for {
		windows, err := lister.ListTerminals()
		if err == nil {
			for _, w := range windows {
				if _, ok := existing[w.WindowID]; ok {
					continue
				}
				found := false
				for _, id := range newIDs {
					if id == w.WindowID {
						found = true
						break
					}
				}
				if !found {
					newIDs = append(newIDs, w.WindowID)
				}
			}
			if len(newIDs) >= 1 {
				return newIDs, nil
			}
		}

		if time.Now().After(deadline) {
			return newIDs, fmt.Errorf("timeout waiting for new terminal (%d seen after %s)", len(newIDs), timeout)
		}
		<-ticker.C
	}
}

func runWorkspaceRename(args []string) int {
	fs := flag.NewFlagSet("rename", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: termtile workspace rename <old-name> <new-name>")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Renames workspace and all associated tmux sessions.")
	}
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}

	if fs.NArg() != 2 {
		fs.Usage()
		return 2
	}

	oldName := fs.Arg(0)
	newName := fs.Arg(1)

	// Validate new name
	if err := workspace.ValidateWorkspaceName(newName); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	// Check old workspace exists
	oldPath := workspace.ConfigPath(oldName)
	if _, err := os.Stat(oldPath); os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "workspace %q not found\n", oldName)
		return 1
	}

	// Check new name doesn't exist
	newPath := workspace.ConfigPath(newName)
	if _, err := os.Stat(newPath); err == nil {
		fmt.Fprintf(os.Stderr, "workspace %q already exists\n", newName)
		return 1
	}

	// Load config
	cfg, err := workspace.Read(oldName)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	// Rename live tmux sessions first (can fail, easier to rollback)
	tmux := agent.NewTmuxMultiplexer()
	for i, term := range cfg.Terminals {
		oldSession := agent.SessionName(oldName, term.SlotIndex)
		newSession := agent.SessionName(newName, term.SlotIndex)

		if exists, _ := tmux.HasSession(oldSession); exists {
			if err := tmux.RenameSession(oldSession, newSession); err != nil {
				fmt.Fprintf(os.Stderr, "warning: failed to rename tmux session %s: %v\n", oldSession, err)
			}
		}
		cfg.Terminals[i].SessionName = newSession
	}

	// Update config name
	cfg.Name = newName

	// Write new config file
	if err := workspace.Write(cfg); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	// Delete old config file
	if err := os.Remove(oldPath); err != nil {
		fmt.Fprintf(os.Stderr, "warning: failed to remove old config: %v\n", err)
	}

	// Update runtime state if this workspace is active
	allWs, _ := workspace.GetAllWorkspaces()
	for desktop, ws := range allWs {
		if ws.Name == oldName {
			ws.Name = newName
			workspace.SetActiveWorkspace(ws.Name, ws.TerminalCount, ws.AgentMode, desktop, ws.AgentSlots)
			break
		}
	}

	fmt.Printf("Renamed workspace %q to %q\n", oldName, newName)
	return 0
}
