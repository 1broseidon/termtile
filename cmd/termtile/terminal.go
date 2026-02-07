package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/1broseidon/termtile/internal/agent"
	"github.com/1broseidon/termtile/internal/config"
	"github.com/1broseidon/termtile/internal/ipc"
	"github.com/1broseidon/termtile/internal/platform"
	"github.com/1broseidon/termtile/internal/workspace"
)

var (
	terminalLoggerOnce sync.Once
	terminalLogger     *agent.Logger
)

// getTerminalLogger returns the shared terminal logger, initializing it if needed.
func getTerminalLogger() *agent.Logger {
	terminalLoggerOnce.Do(func() {
		res, err := config.LoadWithSources()
		if err != nil {
			return
		}
		logCfg := res.Config.GetLoggingConfig()
		if !logCfg.Enabled {
			return
		}
		terminalLogger, err = agent.NewLogger(agent.LogConfig{
			Enabled:        logCfg.Enabled,
			Level:          agent.ParseLogLevel(logCfg.Level),
			FilePath:       logCfg.File,
			MaxSizeMB:      logCfg.MaxSizeMB,
			MaxFiles:       logCfg.MaxFiles,
			IncludeContent: logCfg.IncludeContent,
			PreviewLength:  logCfg.PreviewLength,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to initialize terminal logger: %v\n", err)
		}
	})
	return terminalLogger
}

// TerminalWorkspaceStatus holds status info for a workspace with terminal sessions
type TerminalWorkspaceStatus struct {
	Name          string               `json:"name"`
	Desktop       int                  `json:"desktop"`
	TerminalCount int                  `json:"terminal_count"`
	OpenedAt      time.Time            `json:"opened_at"`
	Slots         []TerminalSlotStatus `json:"slots"`
}

// TerminalSlotStatus holds status info for a single terminal slot
type TerminalSlotStatus struct {
	Slot           int    `json:"slot"`
	SessionName    string `json:"session_name"`
	Exists         bool   `json:"exists"`
	CurrentCommand string `json:"current_command,omitempty"`
	IsIdle         bool   `json:"is_idle"`
}

func printTerminalUsage(w *os.File) {
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  termtile terminal add [flags]              Add terminal to workspace")
	fmt.Fprintln(w, "  termtile terminal remove [flags]           Remove terminal from workspace")
	fmt.Fprintln(w, "  termtile terminal send --slot N <text>     Send input to terminal session")
	fmt.Fprintln(w, "  termtile terminal read --slot N [flags]    Read output from terminal session")
	fmt.Fprintln(w, "  termtile terminal status [--json]          Show terminal/session status")
	fmt.Fprintln(w, "  termtile terminal list                     List current terminals")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Run 'termtile terminal <command> --help' for command-specific options.")
}

func runTerminal(args []string) int {
	if len(args) == 0 {
		printTerminalUsage(os.Stderr)
		return 2
	}
	if args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		printTerminalUsage(os.Stdout)
		return 0
	}

	switch args[0] {
	case "add":
		return runTerminalAdd(args[1:])
	case "remove":
		return runTerminalRemove(args[1:])
	case "send":
		return runTerminalSend(args[1:])
	case "read":
		return runTerminalRead(args[1:])
	case "status":
		return runTerminalStatus(args[1:])
	case "list":
		return runTerminalList(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "Unknown terminal command: %s\n\n", args[0])
		printTerminalUsage(os.Stderr)
		return 2
	}
}

// getTerminalWorkspaceInfo retrieves workspace info for terminal commands.
func getTerminalWorkspaceInfo() *agent.WorkspaceInfo {
	wsInfo, err := workspace.GetActiveWorkspace()
	if err != nil || wsInfo.Name == "" {
		return nil
	}
	return &agent.WorkspaceInfo{
		Name:       wsInfo.Name,
		AgentMode:  wsInfo.AgentMode,
		AgentSlots: wsInfo.AgentSlots,
	}
}

func runTerminalSend(args []string) int {
	fs := flag.NewFlagSet("send", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: termtile terminal send --slot N [--workspace NAME] <text>")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Send input to a tmux-backed terminal slot.")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Flags:")
		fs.PrintDefaults()
	}
	slot := fs.Int("slot", -1, "Target workspace slot index")
	workspaceName := fs.String("workspace", "", "Target workspace name (default: current desktop's workspace)")
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if fs.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "send requires <text>")
		fs.Usage()
		return 2
	}

	if err := agent.RequireTmux(); err != nil {
		fmt.Fprintln(os.Stderr, "tmux not available (required for terminal send/read):", err)
		return 1
	}

	// Get workspace info from current desktop for auto-detection
	wsInfo := getTerminalWorkspaceInfo()

	session, err := agent.ResolveSession(*workspaceName, *slot, wsInfo)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}

	ok, err := agent.HasSession(session)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if !ok {
		fmt.Fprintf(os.Stderr, "tmux session %q not found (load a workspace with agent-mode first)\n", session)
		return 1
	}

	text := strings.Join(fs.Args(), " ")
	if err := agent.SendKeys(session, text); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	// Log the send action
	if logger := getTerminalLogger(); logger != nil {
		wsName := *workspaceName
		if wsName == "" && wsInfo != nil {
			wsName = wsInfo.Name
		}
		details := map[string]interface{}{
			"len": len(text),
		}
		// Get preview length from config
		res, err := config.LoadWithSources()
		previewLen := 50
		if err == nil {
			logCfg := res.Config.GetLoggingConfig()
			previewLen = logCfg.PreviewLength
			if logCfg.IncludeContent {
				details["content"] = text
			} else {
				details["preview"] = agent.Truncate(text, previewLen)
			}
		} else {
			details["preview"] = agent.Truncate(text, previewLen)
		}
		logger.Log(agent.ActionSend, wsName, *slot, details)
	}

	return 0
}

func runTerminalRead(args []string) int {
	fs := flag.NewFlagSet("read", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage:")
		fmt.Fprintln(os.Stderr, "  termtile terminal read --slot N [--workspace NAME] [--lines M]")
		fmt.Fprintln(os.Stderr, "  termtile terminal read --slot N [--workspace NAME] --wait-for <pattern> [--timeout S] [--lines M]")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Read output from a tmux-backed terminal slot.")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Flags:")
		fs.PrintDefaults()
	}
	slot := fs.Int("slot", -1, "Target workspace slot index")
	workspaceName := fs.String("workspace", "", "Target workspace name (default: current desktop's workspace)")
	lines := fs.Int("lines", 200, "Number of lines to capture from the pane (approx; uses tmux -S -N)")
	waitFor := fs.String("wait-for", "", "Wait until output contains this substring")
	timeoutSeconds := fs.Int("timeout", 10, "Wait timeout in seconds (used with --wait-for)")
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}

	if err := agent.RequireTmux(); err != nil {
		fmt.Fprintln(os.Stderr, "tmux not available (required for terminal send/read):", err)
		return 1
	}

	// Get workspace info from current desktop for auto-detection
	wsInfo := getTerminalWorkspaceInfo()

	session, err := agent.ResolveSession(*workspaceName, *slot, wsInfo)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}

	ok, err := agent.HasSession(session)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if !ok {
		fmt.Fprintf(os.Stderr, "tmux session %q not found (load a workspace with agent-mode first)\n", session)
		return 1
	}

	// Helper to log read action
	logRead := func() {
		if logger := getTerminalLogger(); logger != nil {
			wsName := *workspaceName
			if wsName == "" && wsInfo != nil {
				wsName = wsInfo.Name
			}
			logger.Log(agent.ActionRead, wsName, *slot, map[string]interface{}{
				"lines": *lines,
			})
		}
	}

	if strings.TrimSpace(*waitFor) != "" {
		out, err := agent.WaitFor(session, *waitFor, time.Duration(*timeoutSeconds)*time.Second, *lines)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			if strings.TrimSpace(out) != "" {
				fmt.Fprint(os.Stdout, out)
			}
			return 1
		}
		fmt.Fprint(os.Stdout, out)
		logRead()
		return 0
	}

	out, err := agent.CapturePane(session, *lines)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Fprint(os.Stdout, out)
	logRead()
	return 0
}

func runTerminalStatus(args []string) int {
	fs := flag.NewFlagSet("status", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: termtile terminal status [--json] [--workspace NAME]")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Show status of all workspaces with tmux sessions.")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Flags:")
		fs.PrintDefaults()
	}
	jsonOut := fs.Bool("json", false, "Output as JSON")
	workspaceName := fs.String("workspace", "", "Filter to specific workspace")
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}

	// Check tmux availability
	if err := agent.RequireTmux(); err != nil {
		fmt.Fprintln(os.Stderr, "tmux not available:", err)
		return 1
	}

	// Get all workspaces
	allWs, err := workspace.GetAllWorkspaces()
	if err != nil {
		fmt.Fprintln(os.Stderr, "failed to get workspaces:", err)
		return 1
	}

	// Filter and build status for agent-mode workspaces
	var results []TerminalWorkspaceStatus
	for desktop, ws := range allWs {
		if !ws.AgentMode {
			continue
		}
		if *workspaceName != "" && ws.Name != *workspaceName {
			continue
		}

		status := TerminalWorkspaceStatus{
			Name:          ws.Name,
			Desktop:       desktop,
			TerminalCount: ws.TerminalCount,
			OpenedAt:      ws.OpenedAt,
			Slots:         make([]TerminalSlotStatus, 0, len(ws.AgentSlots)),
		}

		for _, slot := range ws.AgentSlots {
			session := agent.SessionName(ws.Name, slot)
			slotStatus := TerminalSlotStatus{
				Slot:        slot,
				SessionName: session,
			}

			// Query tmux session status
			sessionStatus, err := agent.GetSessionStatus(session)
			if err == nil {
				slotStatus.Exists = sessionStatus.Exists
				slotStatus.CurrentCommand = sessionStatus.CurrentCommand
				slotStatus.IsIdle = sessionStatus.IsIdle
			}

			status.Slots = append(status.Slots, slotStatus)
		}

		results = append(results, status)
	}

	// Sort by desktop number
	sort.Slice(results, func(i, j int) bool {
		return results[i].Desktop < results[j].Desktop
	})

	// Output
	if *jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(results); err != nil {
			fmt.Fprintln(os.Stderr, "failed to encode JSON:", err)
			return 1
		}
		return 0
	}

	// Human-readable output
	if len(results) == 0 {
		fmt.Println("No workspaces with tmux sessions found")
		return 0
	}

	for i, ws := range results {
		fmt.Printf("Workspace: %s (Desktop %d)\n", ws.Name, ws.Desktop)
		fmt.Printf("  Terminals: %d\n", ws.TerminalCount)
		fmt.Printf("  Slots:\n")
		for _, slot := range ws.Slots {
			status := "not running"
			if slot.Exists {
				if slot.IsIdle {
					status = "idle"
				} else {
					status = fmt.Sprintf("running (%s)", slot.CurrentCommand)
				}
			}
			fmt.Printf("    [%d] %s: %s\n", slot.Slot, slot.SessionName, status)
		}
		if i < len(results)-1 {
			fmt.Println()
		}
	}

	return 0
}

func runTerminalList(args []string) int {
	fs := flag.NewFlagSet("list", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: termtile terminal list [--json]")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "List terminals in the current workspace.")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Flags:")
		fs.PrintDefaults()
	}
	jsonOut := fs.Bool("json", false, "Output as JSON")
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}

	wsInfo, err := workspace.GetActiveWorkspace()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if wsInfo.Name == "" {
		fmt.Fprintln(os.Stderr, "No active workspace on current desktop")
		return 1
	}

	if *jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.Encode(map[string]interface{}{
			"workspace":      wsInfo.Name,
			"desktop":        wsInfo.Desktop,
			"terminal_count": wsInfo.TerminalCount,
			"agent_mode":     wsInfo.AgentMode,
			"agent_slots":    wsInfo.AgentSlots,
		})
		return 0
	}

	fmt.Printf("Workspace: %s (Desktop %d)\n", wsInfo.Name, wsInfo.Desktop)
	fmt.Printf("Terminals: %d\n", wsInfo.TerminalCount)
	if wsInfo.AgentMode {
		fmt.Printf("Agent Mode: yes\n")
		fmt.Printf("Slots: %v\n", wsInfo.AgentSlots)
	}
	return 0
}

func runTerminalAdd(args []string) int {
	fs := flag.NewFlagSet("add", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: termtile terminal add [flags]")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Spawn a new terminal into an existing workspace and re-tile all windows.")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Flags:")
		fs.PrintDefaults()
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Examples:")
		fmt.Fprintln(os.Stderr, "  termtile terminal add               # Add terminal to current workspace")
		fmt.Fprintln(os.Stderr, "  termtile terminal add --cwd ~/code  # New terminal in ~/code")
		fmt.Fprintln(os.Stderr, "  termtile terminal add --no-agent    # Skip tmux session on agent-mode workspace")
		fmt.Fprintln(os.Stderr, "  termtile terminal add --slot 1      # Insert at slot 1, shift others up")
	}
	path := fs.String("path", "", "Config file path")
	workspaceName := fs.String("workspace", "", "Target workspace name (default: workspace on current desktop)")
	cwd := fs.String("cwd", "", "Working directory for new terminal (default: inherit from workspace)")
	noAgent := fs.Bool("no-agent", false, "Skip tmux session creation even if workspace is agent-mode")
	ignoreLimits := fs.Bool("ignore-limits", false, "Ignore configured workspace limits")
	timeout := fs.Int("timeout", 10, "Spawn synchronization timeout in seconds")
	slotPos := fs.Int("slot", -1, "Insert at specific slot position (shifts existing slots up)")

	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}

	// IMPORTANT: Capture desktop immediately to avoid race conditions
	// if user switches desktops while command is running
	capturedDesktop, desktopErr := platform.GetCurrentDesktopStandalone()
	if desktopErr != nil {
		fmt.Fprintf(os.Stderr, "failed to detect current desktop: %v\n", desktopErr)
		return 1
	}

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

	// Get workspace info from captured desktop (or --workspace override)
	var wsInfo workspace.WorkspaceInfo
	if *workspaceName != "" {
		// Find workspace by name across all desktops
		ws, err := workspace.GetWorkspaceByName(*workspaceName)
		if err != nil {
			fmt.Fprintf(os.Stderr, "workspace %q not found on any desktop\n", *workspaceName)
			return 1
		}
		wsInfo = ws

		// Validate workspace is on captured desktop
		if wsInfo.Desktop != capturedDesktop {
			fmt.Fprintf(os.Stderr, "error: workspace %q is on desktop %d, but you were on desktop %d\n",
				wsInfo.Name, wsInfo.Desktop, capturedDesktop)
			fmt.Fprintf(os.Stderr, "hint: switch to desktop %d first\n", wsInfo.Desktop)
			return 1
		}
	} else {
		// Use captured desktop to avoid race conditions
		var ok bool
		wsInfo, ok = workspace.GetWorkspaceByDesktop(capturedDesktop)
		if !ok || wsInfo.Name == "" {
			fmt.Fprintf(os.Stderr, "no workspace on desktop %d\n", capturedDesktop)
			return 1
		}
	}

	if !*ignoreLimits {
		if err := workspace.CheckCanAddTerminal(wsInfo.Name, wsInfo.TerminalCount, res.Config); err != nil {
			fmt.Fprintln(os.Stderr, "cannot add terminal:", err)
			return 1
		}
	}

	// Load the saved workspace config to get terminal class
	savedWs, err := workspace.Read(wsInfo.Name)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to read workspace config: %v\n", err)
		return 1
	}

	// Determine terminal class from workspace
	var termClass string
	if len(savedWs.Terminals) > 0 {
		termClass = savedWs.Terminals[0].WMClass
	}
	if termClass == "" {
		termClass = res.Config.ResolveTerminal()
		if termClass == "" {
			fmt.Fprintln(os.Stderr, "no terminal class configured; set terminal_classes in config")
			return 1
		}
	}

	// Determine working directory
	workDir := *cwd
	if workDir == "" {
		// Inherit from workspace (use first terminal's cwd or current directory)
		if len(savedWs.Terminals) > 0 && savedWs.Terminals[0].Cwd != "" {
			workDir = savedWs.Terminals[0].Cwd
		} else {
			workDir, err = os.Getwd()
			if err != nil {
				fmt.Fprintln(os.Stderr, "failed to get current directory:", err)
				return 1
			}
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

	// Get existing terminals before spawning
	before, err := lister.ListTerminals()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	existing := make(map[uint32]struct{}, len(before))
	for _, w := range before {
		existing[w.WindowID] = struct{}{}
	}

	// Determine slot position - either insert at specific position or append
	newSlot := wsInfo.TerminalCount
	insertMode := false

	if *slotPos >= 0 {
		// Validate slot position
		if *slotPos > wsInfo.TerminalCount {
			fmt.Fprintf(os.Stderr, "slot %d out of range (0-%d)\n", *slotPos, wsInfo.TerminalCount)
			return 1
		}
		newSlot = *slotPos
		insertMode = *slotPos < wsInfo.TerminalCount
	}

	// Determine if we should create tmux session
	createTmux := wsInfo.AgentMode && !*noAgent

	// If inserting at a position (not appending), shift existing sessions up
	if insertMode && createTmux {
		tmux := agent.NewTmuxMultiplexer()

		// Shift existing sessions UP (from end to insertSlot, to avoid collisions)
		// If inserting at slot 1 with 3 terminals:
		//   slot 2 → slot 3
		//   slot 1 → slot 2
		//   (then new terminal takes slot 1)
		for i := wsInfo.TerminalCount - 1; i >= newSlot; i-- {
			oldSession := agent.SessionName(wsInfo.Name, i)
			newSession := agent.SessionName(wsInfo.Name, i+1)

			if exists, _ := tmux.HasSession(oldSession); exists {
				if err := tmux.RenameSession(oldSession, newSession); err != nil {
					fmt.Fprintf(os.Stderr, "failed to shift session %s to %s: %v\n", oldSession, newSession, err)
					return 1
				}
			}
		}
	}

	// Build spawn command
	var cmdOverride string
	if createTmux {
		appCfg := res.Config
		configMgr, err := agent.NewConfigManager(appCfg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to initialize multiplexer: %v\n", err)
			return 1
		}

		session := agent.SessionName(wsInfo.Name, newSlot)
		sessionCmd := configMgr.SessionCommand(session)

		// Build command with cwd
		baseArgs, err := splitCommand(sessionCmd)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to parse multiplexer command: %v\n", err)
			return 1
		}
		muxArgs := append(baseArgs, "-c", workDir)
		cmdOverride = shellJoin(muxArgs)
	}

	// Spawn the terminal
	termConfig := workspace.TerminalConfig{
		WMClass:   termClass,
		Cwd:       workDir,
		SlotIndex: newSlot,
	}
	if err := spawnTerminalWithCommand(termConfig, res.Config.TerminalSpawnCommands, cmdOverride); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	// Wait for the new terminal to appear
	newWindowIDs, err := waitForNewTerminal(lister, existing, time.Duration(*timeout)*time.Second)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if len(newWindowIDs) == 0 {
		fmt.Fprintln(os.Stderr, "terminal spawned but window not detected")
		return 1
	}

	// Update workspace state
	if insertMode {
		err = workspace.InsertTerminalAtSlot(wsInfo.Desktop, newSlot, createTmux)
	} else {
		_, err = workspace.AddTerminalToWorkspace(wsInfo.Desktop, createTmux)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: failed to update workspace state: %v\n", err)
	}

	// Get current layout from daemon
	layoutName := savedWs.Layout
	if status, err := applier.client.GetStatus(); err == nil && status.ActiveLayout != "" {
		layoutName = status.ActiveLayout
	}

	// Re-tile all terminals
	if insertMode && len(newWindowIDs) > 0 {
		// For insert mode, we need to specify the window order
		// Sort existing windows by their visual position (Y then X)
		sort.Slice(before, func(i, j int) bool {
			if before[i].Y != before[j].Y {
				return before[i].Y < before[j].Y
			}
			return before[i].X < before[j].X
		})

		// Build window order with new window inserted at the correct position
		windowOrder := make([]uint32, 0, len(before)+1)
		for i, w := range before {
			if i == newSlot {
				windowOrder = append(windowOrder, newWindowIDs[0])
			}
			windowOrder = append(windowOrder, w.WindowID)
		}
		// If inserting at the end (shouldn't happen in insertMode, but handle it)
		if newSlot >= len(before) {
			windowOrder = append(windowOrder, newWindowIDs[0])
		}

		if err := applier.ApplyLayoutWithOrder(layoutName, windowOrder); err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to re-tile: %v\n", err)
		}
	} else {
		if err := applier.ApplyLayout(layoutName, true); err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to re-tile: %v\n", err)
		}
	}

	// Log add-terminal action
	logTerminalAction(agent.ActionAddTerminal, wsInfo.Name, newSlot, nil)

	fmt.Printf("Added terminal (slot %d) to workspace %q\n", newSlot, wsInfo.Name)
	return 0
}

func runTerminalRemove(args []string) int {
	fs := flag.NewFlagSet("remove", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: termtile terminal remove [flags]")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Close a terminal from an existing workspace and re-tile remaining windows.")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Flags:")
		fs.PrintDefaults()
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Examples:")
		fmt.Fprintln(os.Stderr, "  termtile terminal remove --slot 2     # Remove terminal at slot 2")
		fmt.Fprintln(os.Stderr, "  termtile terminal remove --last       # Remove the highest numbered slot")
		fmt.Fprintln(os.Stderr, "  termtile terminal remove --slot 1 --force  # Force remove even if busy")
	}
	path := fs.String("path", "", "Config file path")
	workspaceName := fs.String("workspace", "", "Target workspace name (default: workspace on current desktop)")
	slot := fs.Int("slot", -1, "Slot index to remove")
	last := fs.Bool("last", false, "Remove the last/highest slot")
	force := fs.Bool("force", false, "Skip confirmation for non-empty tmux sessions")

	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}

	// Validate flags
	if *slot < 0 && !*last {
		fmt.Fprintln(os.Stderr, "either --slot N or --last is required")
		fs.Usage()
		return 2
	}
	if *slot >= 0 && *last {
		fmt.Fprintln(os.Stderr, "--slot and --last are mutually exclusive")
		return 2
	}

	// IMPORTANT: Capture desktop immediately to avoid race conditions
	// if user switches desktops while command is running
	capturedDesktop, desktopErr := platform.GetCurrentDesktopStandalone()
	if desktopErr != nil {
		fmt.Fprintf(os.Stderr, "failed to detect current desktop: %v\n", desktopErr)
		return 1
	}

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

	// Get workspace info from captured desktop (or --workspace override)
	var wsInfo workspace.WorkspaceInfo
	if *workspaceName != "" {
		// Find workspace by name across all desktops
		ws, err := workspace.GetWorkspaceByName(*workspaceName)
		if err != nil {
			fmt.Fprintf(os.Stderr, "workspace %q not found on any desktop\n", *workspaceName)
			return 1
		}
		wsInfo = ws

		// Validate workspace is on captured desktop
		if wsInfo.Desktop != capturedDesktop {
			fmt.Fprintf(os.Stderr, "error: workspace %q is on desktop %d, but you were on desktop %d\n",
				wsInfo.Name, wsInfo.Desktop, capturedDesktop)
			fmt.Fprintf(os.Stderr, "hint: switch to desktop %d first\n", wsInfo.Desktop)
			return 1
		}
	} else {
		// Use captured desktop to avoid race conditions
		var ok bool
		wsInfo, ok = workspace.GetWorkspaceByDesktop(capturedDesktop)
		if !ok || wsInfo.Name == "" {
			fmt.Fprintf(os.Stderr, "no workspace on desktop %d\n", capturedDesktop)
			return 1
		}
	}

	// Determine slot to remove
	targetSlot := *slot
	if *last {
		targetSlot = wsInfo.TerminalCount - 1
	}

	if targetSlot < 0 || targetSlot >= wsInfo.TerminalCount {
		fmt.Fprintf(os.Stderr, "slot %d out of range (workspace has %d terminals)\n", targetSlot, wsInfo.TerminalCount)
		return 1
	}

	// Check if slot has active tmux session
	session := agent.SessionName(wsInfo.Name, targetSlot)
	hasSession, _ := agent.HasSession(session)

	if hasSession && !*force {
		// Check if session is busy
		status, err := agent.GetSessionStatus(session)
		if err == nil && status.Exists && !status.IsIdle {
			fmt.Fprintf(os.Stderr, "slot %d has running process (%s); use --force to remove anyway\n",
				targetSlot, status.CurrentCommand)
			return 1
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

	// Get current terminals
	windows, err := lister.ListTerminals()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	if targetSlot >= len(windows) {
		fmt.Fprintf(os.Stderr, "slot %d not found in current terminal list\n", targetSlot)
		return 1
	}

	// For agent-mode terminals, killing tmux will close the window automatically
	// For non-agent terminals, we need to close the window via the backend
	if hasSession {
		// Kill tmux session - this will close the terminal window automatically
		if err := exec.Command("tmux", "kill-session", "-t", session).Run(); err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to kill tmux session: %v\n", err)
		}
		// Give the window time to close
		time.Sleep(200 * time.Millisecond)
	} else {
		// No tmux session - close the window via platform backend
		targetWindow := windows[targetSlot]
		if err := closeWindowViaBackend(backend, targetWindow.WindowID); err != nil {
			fmt.Fprintf(os.Stderr, "failed to close window: %v\n", err)
			return 1
		}
	}

	// Shift remaining tmux sessions DOWN to keep IDs matching visual positions
	// If removing slot 2 from [0,1,2,3,4], shift: 3→2, 4→3
	if wsInfo.AgentMode && targetSlot < wsInfo.TerminalCount-1 {
		tmux := agent.NewTmuxMultiplexer()
		for i := targetSlot + 1; i < wsInfo.TerminalCount; i++ {
			oldSession := agent.SessionName(wsInfo.Name, i)
			newSession := agent.SessionName(wsInfo.Name, i-1)

			if exists, _ := tmux.HasSession(oldSession); exists {
				if err := tmux.RenameSession(oldSession, newSession); err != nil {
					fmt.Fprintf(os.Stderr, "warning: failed to shift session %s to %s: %v\n", oldSession, newSession, err)
				}
			}
		}
	}

	// Update workspace state
	if err := workspace.RemoveTerminalFromWorkspace(wsInfo.Desktop, targetSlot); err != nil {
		fmt.Fprintf(os.Stderr, "warning: failed to update workspace state: %v\n", err)
	}

	// Re-tile remaining terminals
	applier := &ipcLayoutApplier{client: ipc.NewClient()}
	savedWs, _ := workspace.Read(wsInfo.Name)
	layoutName := savedWs.Layout
	if status, err := applier.client.GetStatus(); err == nil && status.ActiveLayout != "" {
		layoutName = status.ActiveLayout
	}

	// Small delay to let window close
	time.Sleep(100 * time.Millisecond)

	if err := applier.ApplyLayout(layoutName, true); err != nil {
		fmt.Fprintf(os.Stderr, "warning: failed to re-tile: %v\n", err)
	}

	// Log remove-terminal action
	logTerminalAction(agent.ActionRemoveTerminal, wsInfo.Name, targetSlot, nil)

	fmt.Printf("Removed terminal (slot %d) from workspace %q\n", targetSlot, wsInfo.Name)
	return 0
}

// logTerminalAction logs a terminal action if logging is enabled.
func logTerminalAction(action agent.ActionType, workspace string, slot int, details map[string]interface{}) {
	if logger := getTerminalLogger(); logger != nil {
		logger.Log(action, workspace, slot, details)
	}
}
