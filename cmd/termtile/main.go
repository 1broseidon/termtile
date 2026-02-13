package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"sort"
	"syscall"
	"time"

	"github.com/1broseidon/termtile/internal/agent"
	"github.com/1broseidon/termtile/internal/config"
	"github.com/1broseidon/termtile/internal/daemon"
	"github.com/1broseidon/termtile/internal/hotkeys"
	"github.com/1broseidon/termtile/internal/ipc"
	"github.com/1broseidon/termtile/internal/movemode"
	"github.com/1broseidon/termtile/internal/platform"
	"github.com/1broseidon/termtile/internal/terminals"
	"github.com/1broseidon/termtile/internal/tiling"
	"github.com/1broseidon/termtile/internal/tui"
	"github.com/1broseidon/termtile/internal/workspace"
	"gopkg.in/yaml.v3"
)

func main() {
	if len(os.Args) < 2 {
		printMainUsage(os.Stdout)
		os.Exit(0)
	}

	switch os.Args[1] {
	case "daemon":
		if len(os.Args) > 2 && (os.Args[2] == "help" || os.Args[2] == "-h" || os.Args[2] == "--help") {
			fmt.Fprintln(os.Stdout, "Usage: termtile daemon")
			os.Exit(0)
		}
		if len(os.Args) > 2 {
			fmt.Fprintln(os.Stderr, "daemon takes no arguments")
			fmt.Fprintln(os.Stderr, "")
			fmt.Fprintln(os.Stderr, "Usage: termtile daemon")
			os.Exit(2)
		}
		runDaemon()
	case "status":
		os.Exit(runStatus(os.Args[2:]))
	case "undo":
		os.Exit(runUndo(os.Args[2:]))
	case "layout":
		os.Exit(runLayout(os.Args[2:]))
	case "terminal":
		os.Exit(runTerminal(os.Args[2:]))
	case "config":
		os.Exit(runConfig(os.Args[2:]))
	case "workspace":
		os.Exit(runWorkspace(os.Args[2:]))
	case "palette":
		os.Exit(runPalette(os.Args[2:]))
	case "tui":
		os.Exit(runTUI(os.Args[2:]))
	case "mcp":
		os.Exit(runMCP(os.Args[2:]))
	case "hook":
		os.Exit(runHook(os.Args[2:]))
	case "help", "-h", "--help":
		printMainUsage(os.Stdout)
		os.Exit(0)
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n", os.Args[1])
		printMainUsage(os.Stderr)
		os.Exit(2)
	}
}

func printMainUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage: termtile <command> [options]")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Commands:")
	fmt.Fprintln(w, "  daemon              Start the termtile daemon (foreground)")
	fmt.Fprintln(w, "  status              Show daemon status")
	fmt.Fprintln(w, "  undo                Undo last tiling operation")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "  layout list         List available layouts")
	fmt.Fprintln(w, "  layout apply        Apply a layout")
	fmt.Fprintln(w, "  layout default      Set default layout")
	fmt.Fprintln(w, "  layout preview      Preview a layout temporarily")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "  workspace new       Create a new workspace")
	fmt.Fprintln(w, "  workspace save      Save current terminal state")
	fmt.Fprintln(w, "  workspace load      Load a saved workspace")
	fmt.Fprintln(w, "  workspace close     Close active workspace")
	fmt.Fprintln(w, "  workspace list      List saved workspaces")
	fmt.Fprintln(w, "  workspace delete    Delete a workspace")
	fmt.Fprintln(w, "  workspace rename    Rename a workspace")
	fmt.Fprintln(w, "  workspace init      Initialize project workspace config")
	fmt.Fprintln(w, "  workspace link      Link project to a canonical workspace")
	fmt.Fprintln(w, "  workspace sync      Sync project view pull/push")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "  terminal add        Add terminal to workspace")
	fmt.Fprintln(w, "  terminal remove     Remove terminal from workspace")
	fmt.Fprintln(w, "  terminal move       Move terminal to another workspace")
	fmt.Fprintln(w, "  terminal send       Send input to terminal slot")
	fmt.Fprintln(w, "  terminal read       Read output from terminal slot")
	fmt.Fprintln(w, "  terminal status     Show terminal/session status")
	fmt.Fprintln(w, "  terminal list       List current terminals")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "  config validate     Validate configuration")
	fmt.Fprintln(w, "  config print        Print configuration")
	fmt.Fprintln(w, "  config explain      Explain a config value")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "  palette             Open command palette")
	fmt.Fprintln(w, "  tui                 Open interactive TUI")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "  mcp serve           Start MCP server (stdio transport)")
	fmt.Fprintln(w, "  mcp cleanup         List/clean orphaned termtile tmux sessions")
	fmt.Fprintln(w, "  hook emit           Write hook output artifact for a workspace slot")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Run 'termtile <command> --help' for command-specific options.")
}

func runStatus(args []string) int {
	fs := flag.NewFlagSet("status", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: termtile status")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Show daemon status via IPC.")
	}
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "status takes no arguments")
		fs.Usage()
		return 2
	}

	client := ipc.NewClient()
	status, err := client.GetStatus()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Printf("daemon_running: %v\n", status.DaemonRunning)
	fmt.Printf("active_layout:  %s\n", status.ActiveLayout)
	fmt.Printf("terminal_count: %d\n", status.TerminalCount)
	fmt.Printf("uptime_seconds: %d\n", status.UptimeSeconds)
	return 0
}

func runUndo(args []string) int {
	fs := flag.NewFlagSet("undo", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: termtile undo")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Undo the last tiling operation on the active monitor.")
	}
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "undo takes no arguments")
		fs.Usage()
		return 2
	}

	client := ipc.NewClient()
	if err := client.Undo(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}

func printLayoutUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  termtile layout list [--json]")
	fmt.Fprintln(w, "  termtile layout apply [--tile] <layout>")
	fmt.Fprintln(w, "  termtile layout default [--tile] <layout>")
	fmt.Fprintln(w, "  termtile layout preview [--duration N] <layout>")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Run 'termtile layout <command> --help' for command-specific options.")
}

func runLayout(args []string) int {
	if len(args) == 0 {
		printLayoutUsage(os.Stderr)
		return 2
	}
	if args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		printLayoutUsage(os.Stdout)
		return 0
	}

	client := ipc.NewClient()

	switch args[0] {
	case "list":
		fs := flag.NewFlagSet("list", flag.ContinueOnError)
		fs.SetOutput(os.Stderr)
		fs.Usage = func() {
			fmt.Fprintln(os.Stderr, "Usage: termtile layout list [--json]")
			fmt.Fprintln(os.Stderr, "")
			fmt.Fprintln(os.Stderr, "List available layouts (and current selection when the daemon is running).")
			fmt.Fprintln(os.Stderr, "")
			fmt.Fprintln(os.Stderr, "Flags:")
			fs.PrintDefaults()
		}
		jsonOut := fs.Bool("json", false, "Output full layout details as JSON")
		if err := fs.Parse(args[1:]); err != nil {
			if err == flag.ErrHelp {
				return 0
			}
			return 2
		}
		if fs.NArg() != 0 {
			fmt.Fprintln(os.Stderr, "layout list takes no arguments")
			fs.Usage()
			return 2
		}

		if *jsonOut {
			return layoutListJSON()
		}

		data, err := client.ListLayouts()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		fmt.Printf("default_layout: %s\n", data.DefaultLayout)
		fmt.Printf("active_layout:  %s\n", data.ActiveLayout)
		for _, name := range data.Layouts {
			fmt.Printf("- %s\n", name)
		}
		return 0

	case "apply":
		fs := flag.NewFlagSet("apply", flag.ContinueOnError)
		fs.SetOutput(os.Stderr)
		fs.Usage = func() {
			fmt.Fprintln(os.Stderr, "Usage: termtile layout apply [--tile] <layout>")
			fmt.Fprintln(os.Stderr, "")
			fmt.Fprintln(os.Stderr, "Set the daemon's active layout (optionally tiling immediately).")
			fmt.Fprintln(os.Stderr, "")
			fmt.Fprintln(os.Stderr, "Flags:")
			fs.PrintDefaults()
		}
		tileNow := fs.Bool("tile", false, "Tile immediately")
		if err := fs.Parse(args[1:]); err != nil {
			if err == flag.ErrHelp {
				return 0
			}
			return 2
		}
		if fs.NArg() < 1 {
			fmt.Fprintln(os.Stderr, "layout apply requires <layout>")
			fs.Usage()
			return 2
		}
		if err := client.ApplyLayout(fs.Arg(0), *tileNow); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		return 0

	case "default":
		fs := flag.NewFlagSet("default", flag.ContinueOnError)
		fs.SetOutput(os.Stderr)
		fs.Usage = func() {
			fmt.Fprintln(os.Stderr, "Usage: termtile layout default [--tile] <layout>")
			fmt.Fprintln(os.Stderr, "")
			fmt.Fprintln(os.Stderr, "Set default_layout in config (optionally tiling immediately).")
			fmt.Fprintln(os.Stderr, "")
			fmt.Fprintln(os.Stderr, "Flags:")
			fs.PrintDefaults()
		}
		tileNow := fs.Bool("tile", false, "Tile immediately")
		if err := fs.Parse(args[1:]); err != nil {
			if err == flag.ErrHelp {
				return 0
			}
			return 2
		}
		if fs.NArg() < 1 {
			fmt.Fprintln(os.Stderr, "layout default requires <layout>")
			fs.Usage()
			return 2
		}
		if err := client.SetDefaultLayout(fs.Arg(0), *tileNow); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		return 0

	case "preview":
		fs := flag.NewFlagSet("preview", flag.ContinueOnError)
		fs.SetOutput(os.Stderr)
		fs.Usage = func() {
			fmt.Fprintln(os.Stderr, "Usage: termtile layout preview [--duration N] <layout>")
			fmt.Fprintln(os.Stderr, "")
			fmt.Fprintln(os.Stderr, "Temporarily apply a layout and restore after a duration.")
			fmt.Fprintln(os.Stderr, "")
			fmt.Fprintln(os.Stderr, "Flags:")
			fs.PrintDefaults()
		}
		durationSeconds := fs.Int("duration", 3, "Preview duration in seconds")
		if err := fs.Parse(args[1:]); err != nil {
			if err == flag.ErrHelp {
				return 0
			}
			return 2
		}
		if fs.NArg() < 1 {
			fmt.Fprintln(os.Stderr, "layout preview requires <layout>")
			fs.Usage()
			return 2
		}
		if err := client.PreviewLayout(fs.Arg(0), *durationSeconds); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		return 0

	default:
		fmt.Fprintf(os.Stderr, "Unknown layout command: %s\n\n", args[0])
		printLayoutUsage(os.Stderr)
		return 2
	}
}

type layoutJSON struct {
	Name              string         `json:"name"`
	Mode              string         `json:"mode"`
	TileRegion        tileRegionJSON `json:"tile_region"`
	FixedGrid         fixedGridJSON  `json:"fixed_grid,omitempty"`
	MaxTerminalWidth  int            `json:"max_terminal_width"`
	MaxTerminalHeight int            `json:"max_terminal_height"`
	FlexibleLastRow   bool           `json:"flexible_last_row"`
}

type tileRegionJSON struct {
	Type          string `json:"type"`
	XPercent      int    `json:"x_percent,omitempty"`
	YPercent      int    `json:"y_percent,omitempty"`
	WidthPercent  int    `json:"width_percent,omitempty"`
	HeightPercent int    `json:"height_percent,omitempty"`
}

type fixedGridJSON struct {
	Rows int `json:"rows"`
	Cols int `json:"cols"`
}

func layoutListJSON() int {
	res, err := config.LoadWithSources()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	names := make([]string, 0, len(res.Config.Layouts))
	for name := range res.Config.Layouts {
		names = append(names, name)
	}
	sort.Strings(names)

	layouts := make([]layoutJSON, 0, len(names))
	for _, name := range names {
		l := res.Config.Layouts[name]
		entry := layoutJSON{
			Name:              name,
			Mode:              string(l.Mode),
			MaxTerminalWidth:  l.MaxTerminalWidth,
			MaxTerminalHeight: l.MaxTerminalHeight,
			FlexibleLastRow:   l.FlexibleLastRow,
			TileRegion: tileRegionJSON{
				Type:          string(l.TileRegion.Type),
				XPercent:      l.TileRegion.XPercent,
				YPercent:      l.TileRegion.YPercent,
				WidthPercent:  l.TileRegion.WidthPercent,
				HeightPercent: l.TileRegion.HeightPercent,
			},
		}
		if l.Mode == config.LayoutModeFixed {
			entry.FixedGrid = fixedGridJSON{Rows: l.FixedGrid.Rows, Cols: l.FixedGrid.Cols}
		}
		layouts = append(layouts, entry)
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(layouts); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}

func runConfig(args []string) int {
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		fmt.Fprintln(os.Stderr, "Usage:")
		fmt.Fprintln(os.Stderr, "  termtile config validate [--path PATH]")
		fmt.Fprintln(os.Stderr, "  termtile config print [--path PATH] [--effective|--defaults]")
		fmt.Fprintln(os.Stderr, "  termtile config explain [--path PATH] <yaml.path>")
		return 2
	}

	switch args[0] {
	case "validate":
		fs := flag.NewFlagSet("validate", flag.ContinueOnError)
		fs.SetOutput(os.Stderr)
		path := fs.String("path", "", "Config file path (default: ~/.config/termtile/config.yaml)")
		if err := fs.Parse(args[1:]); err != nil {
			return 2
		}

		var err error
		if *path == "" {
			_, err = config.LoadWithSources()
		} else {
			_, err = config.LoadFromPath(*path)
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		fmt.Println("config: ok")
		return 0

	case "print":
		fs := flag.NewFlagSet("print", flag.ContinueOnError)
		fs.SetOutput(os.Stderr)
		path := fs.String("path", "", "Config file path (default: ~/.config/termtile/config.yaml)")
		printDefaults := fs.Bool("defaults", false, "Print built-in defaults (no files)")
		printEffective := fs.Bool("effective", false, "Print effective config (default)")
		if err := fs.Parse(args[1:]); err != nil {
			return 2
		}

		if *printDefaults {
			cfg := config.DefaultConfig()
			if term := cfg.ResolveTerminal(); term != "" {
				fmt.Printf("# resolved_terminal: %s\n", term)
			}
			data, err := yaml.Marshal(cfg)
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				return 1
			}
			fmt.Print(string(data))
			return 0
		}

		_ = printEffective // default
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
		if term := res.Config.ResolveTerminal(); term != "" {
			fmt.Printf("# resolved_terminal: %s\n", term)
		}
		data, err := yaml.Marshal(res.Config)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		fmt.Print(string(data))
		return 0

	case "explain":
		fs := flag.NewFlagSet("explain", flag.ContinueOnError)
		fs.SetOutput(os.Stderr)
		path := fs.String("path", "", "Config file path (default: ~/.config/termtile/config.yaml)")
		if err := fs.Parse(args[1:]); err != nil {
			return 2
		}
		if fs.NArg() < 1 {
			fmt.Fprintln(os.Stderr, "explain requires <yaml.path>")
			return 2
		}
		queryPath := fs.Arg(0)

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

		value, src, err := config.Explain(res, queryPath)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}

		out, err := yaml.Marshal(value)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}

		fmt.Printf("path: %s\n", queryPath)
		fmt.Printf("source: %s\n", formatSource(src))
		fmt.Printf("value:\n%s", string(out))
		return 0

	default:
		fmt.Fprintf(os.Stderr, "Unknown config subcommand: %s\n", args[0])
		return 2
	}
}

func runTUI(args []string) int {
	fs := flag.NewFlagSet("tui", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	path := fs.String("path", "", "Config file path (default: ~/.config/termtile/config.yaml)")

	if len(args) > 0 && (args[0] == "help" || args[0] == "-h" || args[0] == "--help") {
		fmt.Fprintln(os.Stderr, "Usage: termtile tui [--path PATH]")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Interactive TUI for browsing layouts and controlling daemon layouts.")
		fmt.Fprintln(os.Stderr, "Works as an offline browser when the daemon is not running.")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Keybindings:")
		fmt.Fprintln(os.Stderr, "  j/k, ↑/↓  Navigate layouts")
		fmt.Fprintln(os.Stderr, "  Enter, a  Apply selected layout immediately (daemon)")
		fmt.Fprintln(os.Stderr, "  d         Set selected layout as default and tile now (daemon)")
		fmt.Fprintln(os.Stderr, "  p         Preview selected layout for 5 seconds (daemon)")
		fmt.Fprintln(os.Stderr, "  2/4/6/9   Set tile count for preview")
		fmt.Fprintln(os.Stderr, "  e         Edit config in $EDITOR")
		fmt.Fprintln(os.Stderr, "  r         Reload config (and daemon when running)")
		fmt.Fprintln(os.Stderr, "  q, Esc    Quit")
		fmt.Fprintln(os.Stderr, "  Ctrl+C    Quit")
		return 0
	}

	if err := fs.Parse(args); err != nil {
		return 2
	}

	t := tui.New(*path)
	if err := t.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	return 0
}

func formatSource(src config.Source) string {
	switch src.Kind {
	case config.SourceFile:
		if src.File == "" {
			return "file"
		}
		if src.Line > 0 {
			return fmt.Sprintf("file:%s:%d:%d", src.File, src.Line, src.Column)
		}
		return "file:" + src.File
	case config.SourceBuiltin:
		if src.Name != "" {
			return "builtin:" + src.Name
		}
		return "builtin"
	case config.SourceDefault:
		if src.Name != "" {
			return "default:" + src.Name
		}
		return "default"
	default:
		return string(src.Kind)
	}
}

// handleMoveComplete is called after a window move/swap operation completes.
// It renames tmux sessions to match the new slot positions.
func handleMoveComplete(result movemode.MoveResult) {
	// Get current workspace info
	wsInfo, err := workspace.GetActiveWorkspace()
	if err != nil || wsInfo.Name == "" {
		return // No active workspace, nothing to do
	}

	if !wsInfo.AgentMode {
		return // Not agent mode, nothing to rename
	}

	tmux := agent.NewTmuxMultiplexer()

	// Build session names
	sourceSession := agent.SessionName(wsInfo.Name, result.SourceSlot)
	targetSession := agent.SessionName(wsInfo.Name, result.TargetSlot)

	// Check which sessions exist
	sourceExists, _ := tmux.HasSession(sourceSession)
	targetExists, _ := tmux.HasSession(targetSession)

	if result.IsSwap {
		// Both terminals swapped positions - need to rename both sessions
		// Use a temporary name to avoid collision
		if sourceExists && targetExists {
			tempSession := agent.SessionName(wsInfo.Name, -9999) // Temporary name

			// Rename source -> temp
			if err := tmux.RenameSession(sourceSession, tempSession); err != nil {
				log.Printf("Move callback: failed to rename %s to temp: %v", sourceSession, err)
				return
			}

			// Rename target -> source
			if err := tmux.RenameSession(targetSession, sourceSession); err != nil {
				log.Printf("Move callback: failed to rename %s to %s: %v", targetSession, sourceSession, err)
				// Try to restore
				_ = tmux.RenameSession(tempSession, sourceSession)
				return
			}

			// Rename temp -> target
			if err := tmux.RenameSession(tempSession, targetSession); err != nil {
				log.Printf("Move callback: failed to rename temp to %s: %v", targetSession, err)
				return
			}

			log.Printf("Move callback: swapped sessions %s <-> %s", sourceSession, targetSession)
		} else if sourceExists {
			// Only source exists, move it to target
			if err := tmux.RenameSession(sourceSession, targetSession); err != nil {
				log.Printf("Move callback: failed to rename %s to %s: %v", sourceSession, targetSession, err)
			} else {
				log.Printf("Move callback: renamed %s -> %s", sourceSession, targetSession)
			}
		} else if targetExists {
			// Only target exists, move it to source
			if err := tmux.RenameSession(targetSession, sourceSession); err != nil {
				log.Printf("Move callback: failed to rename %s to %s: %v", targetSession, sourceSession, err)
			} else {
				log.Printf("Move callback: renamed %s -> %s", targetSession, sourceSession)
			}
		}
	} else {
		// Move to empty slot - just rename the session
		if sourceExists {
			if err := tmux.RenameSession(sourceSession, targetSession); err != nil {
				log.Printf("Move callback: failed to rename %s to %s: %v", sourceSession, targetSession, err)
			} else {
				log.Printf("Move callback: renamed %s -> %s", sourceSession, targetSession)
			}
		}
	}

	// Update workspace config file if it exists
	wsCfg, err := workspace.Read(wsInfo.Name)
	if err == nil && wsCfg != nil {
		// Update session names in config
		for i := range wsCfg.Terminals {
			if wsCfg.Terminals[i].SlotIndex == result.SourceSlot {
				wsCfg.Terminals[i].SessionName = targetSession
			} else if result.IsSwap && wsCfg.Terminals[i].SlotIndex == result.TargetSlot {
				wsCfg.Terminals[i].SessionName = sourceSession
			}
		}

		// Also swap slot indices if this was a swap
		if result.IsSwap {
			for i := range wsCfg.Terminals {
				if wsCfg.Terminals[i].SlotIndex == result.SourceSlot {
					wsCfg.Terminals[i].SlotIndex = result.TargetSlot
				} else if wsCfg.Terminals[i].SlotIndex == result.TargetSlot {
					wsCfg.Terminals[i].SlotIndex = result.SourceSlot
				}
			}
		} else {
			// Move to empty - just update the slot index
			for i := range wsCfg.Terminals {
				if wsCfg.Terminals[i].SlotIndex == result.SourceSlot {
					wsCfg.Terminals[i].SlotIndex = result.TargetSlot
					break
				}
			}
		}

		if err := workspace.Write(wsCfg); err != nil {
			log.Printf("Move callback: failed to update workspace config: %v", err)
		}
	}
}

func runDaemon() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}
	log.Printf("Configuration loaded (hotkey: %s, gap: %dpx)", cfg.Hotkey, cfg.GapSize)

	// Connect to display server
	backend, err := platform.NewLinuxBackendFromDisplay()
	if err != nil {
		log.Fatalf("Failed to connect to display: %v", err)
	}
	defer backend.Disconnect()

	log.Println("termtile daemon started successfully")

	// Create terminal detector
	detector := terminals.NewDetector(cfg.TerminalClassNames())
	log.Printf("Terminal detector initialized with %d terminal classes", len(cfg.TerminalClasses))

	// Create tiler
	tiler := tiling.NewTiler(backend, detector, cfg)
	log.Println("Tiler initialized")

	// Setup hotkey handler
	hotkeyHandler := hotkeys.NewHandler(backend, tiler)
	if err := hotkeyHandler.Register(cfg.Hotkey); err != nil {
		log.Fatalf("Failed to register hotkey: %v", err)
	}

	// Create and register move mode
	moveModeCtrl := movemode.NewMode(backend, detector, cfg, tiler)
	hotkeyHandler.SetMoveMode(moveModeCtrl)

	// Wire up callback to rename tmux sessions after window moves
	moveModeCtrl.OnMoveComplete = func(result movemode.MoveResult) {
		handleMoveComplete(result)
	}

	// Register move mode hotkey if configured
	if cfg.MoveModeHotkey != "" {
		if err := hotkeyHandler.RegisterMoveMode(cfg.MoveModeHotkey); err != nil {
			log.Printf("Warning: Failed to register move mode hotkey: %v", err)
		} else {
			log.Printf("Move mode hotkey registered: %s", cfg.MoveModeHotkey)
		}
	}

	// Register terminal-add hotkey if configured.
	if cfg.TerminalAddHotkey != "" {
		if err := hotkeyHandler.RegisterFunc(cfg.TerminalAddHotkey, func() {
			wsInfo, err := workspace.GetActiveWorkspace()
			if err != nil {
				log.Printf("Terminal-add hotkey: failed to resolve active workspace: %v", err)
				return
			}
			if wsInfo.Name == "" {
				log.Printf("Terminal-add hotkey: no active workspace on current desktop")
				return
			}

			exe, err := os.Executable()
			if err != nil {
				log.Printf("Terminal-add hotkey: failed to find executable: %v", err)
				return
			}

			cmd := exec.Command(exe, "terminal", "add")
			cmd.Stderr = os.Stderr
			if err := cmd.Start(); err != nil {
				log.Printf("Terminal-add hotkey: failed to launch terminal add command: %v", err)
				return
			}
			go func() {
				if err := cmd.Wait(); err != nil {
					log.Printf("Terminal-add hotkey: terminal add command failed: %v", err)
				}
			}()
		}); err != nil {
			log.Printf("Warning: Failed to register terminal add hotkey: %v", err)
		} else {
			log.Printf("Terminal add hotkey registered: %s", cfg.TerminalAddHotkey)
		}
	}

	// Register palette hotkey if configured
	if cfg.PaletteHotkey != "" {
		if err := hotkeyHandler.RegisterFunc(cfg.PaletteHotkey, func() {
			exe, err := os.Executable()
			if err != nil {
				log.Printf("Palette: failed to find executable: %v", err)
				return
			}
			cmd := exec.Command(exe, "palette")
			cmd.Stderr = os.Stderr
			if err := cmd.Start(); err != nil {
				log.Printf("Palette: failed to launch: %v", err)
				return
			}
			go cmd.Wait()
		}); err != nil {
			log.Printf("Warning: Failed to register palette hotkey: %v", err)
		} else {
			log.Printf("Palette hotkey registered: %s", cfg.PaletteHotkey)
		}
	}

	// Optional: Cycle layouts without editing config.
	if cfg.CycleLayoutHotkey != "" {
		if err := hotkeyHandler.RegisterFunc(cfg.CycleLayoutHotkey, func() {
			name, err := tiler.CycleActiveLayout(1)
			if err != nil {
				log.Printf("Failed to cycle layout: %v", err)
				return
			}
			log.Printf("Switched to layout: %s", name)
			if err := tiler.TileCurrentMonitor(); err != nil {
				log.Printf("Tiling failed: %v", err)
			}
		}); err != nil {
			log.Printf("Warning: Failed to register cycle_layout_hotkey: %v", err)
		}
	}
	if cfg.CycleLayoutReverseHotkey != "" {
		if err := hotkeyHandler.RegisterFunc(cfg.CycleLayoutReverseHotkey, func() {
			name, err := tiler.CycleActiveLayout(-1)
			if err != nil {
				log.Printf("Failed to cycle layout: %v", err)
				return
			}
			log.Printf("Switched to layout: %s", name)
			if err := tiler.TileCurrentMonitor(); err != nil {
				log.Printf("Tiling failed: %v", err)
			}
		}); err != nil {
			log.Printf("Warning: Failed to register cycle_layout_reverse_hotkey: %v", err)
		}
	}

	// Optional: Restore previous terminal geometry.
	if cfg.UndoHotkey != "" {
		if err := hotkeyHandler.RegisterFunc(cfg.UndoHotkey, func() {
			if err := tiler.UndoCurrentMonitor(); err != nil {
				log.Printf("Undo failed: %v", err)
			}
		}); err != nil {
			log.Printf("Warning: Failed to register undo_hotkey: %v", err)
		}
	}

	// Create config reload channel
	reloadChan := make(chan struct{}, 1)

	// Start IPC server
	ipcServer, err := ipc.NewServer(cfg, tiler, backend, reloadChan)
	if err != nil {
		log.Fatalf("Failed to create IPC server: %v", err)
	}
	if err := ipcServer.Start(); err != nil {
		log.Fatalf("Failed to start IPC server: %v", err)
	}
	defer ipcServer.Stop()

	// Setup state synchronizer and reconciler
	syncLogger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	stateSynchronizer := daemon.NewStateSynchronizer(syncLogger)

	// Create window lister function for reconciler
	windowLister := daemon.WindowListerFromBackend(backend)

	reconciler := daemon.NewReconciler(daemon.ReconcilerConfig{
		Interval:        10 * time.Second,
		CleanupOrphaned: true,
		Logger:          syncLogger,
	}, stateSynchronizer, windowLister)

	// Run an immediate reconciliation pass on startup to clean stale
	// workspace entries from a previous daemon lifecycle.
	reconciler.ReconcileNow()

	// Start reconciler in background
	reconcilerCtx, reconcilerCancel := context.WithCancel(context.Background())
	defer reconcilerCancel()
	go reconciler.Run(reconcilerCtx)

	// Setup signal handlers
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)

	// Handle signals and config reloads
	go func() {
		for {
			select {
			case sig := <-sigCh:
				switch sig {
				case syscall.SIGHUP:
					log.Println("Received SIGHUP, reloading config...")
					newCfg, err := config.Load()
					if err != nil {
						log.Printf("Config reload failed: %v", err)
						continue
					}

					// Update config in IPC server
					ipcServer.UpdateConfig(newCfg)

					// Update tiler config
					tiler.UpdateConfig(newCfg)

					// Update detector terminal classes
					detector.UpdateTerminalClasses(newCfg.TerminalClassNames())

					// Update move mode config
					moveModeCtrl.UpdateConfig(newCfg)

					log.Println("Config reloaded successfully")

				case os.Interrupt, syscall.SIGTERM:
					log.Println("Shutting down termtile daemon...")
					reconcilerCancel()
					ipcServer.Stop()
					os.Exit(0)
				}

			case <-reloadChan:
				// Config was reloaded via IPC, update components
				newCfg := ipcServer.GetConfig()
				tiler.UpdateConfig(newCfg)
				detector.UpdateTerminalClasses(newCfg.TerminalClassNames())
				moveModeCtrl.UpdateConfig(newCfg)
			}
		}
	}()

	// Start event loop (blocking)
	log.Println("Entering event loop...")
	backend.EventLoop()
}
