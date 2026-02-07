package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"text/tabwriter"

	"github.com/1broseidon/termtile/internal/config"
	"github.com/1broseidon/termtile/internal/mcp"
	"github.com/1broseidon/termtile/internal/workspace"
)

func printMCPUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage: termtile mcp <command>")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Commands:")
	fmt.Fprintln(w, "  serve    Start the MCP server (stdio transport)")
	fmt.Fprintln(w, "  cleanup  List and optionally kill orphaned termtile tmux sessions")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Run 'termtile mcp <command> --help' for command-specific options.")
}

func runMCP(args []string) int {
	if len(args) == 0 {
		printMCPUsage(os.Stderr)
		return 2
	}

	switch args[0] {
	case "serve":
		return runMCPServe(args[1:])
	case "cleanup":
		return runMCPCleanup(args[1:])
	case "help", "-h", "--help":
		printMCPUsage(os.Stdout)
		return 0
	default:
		fmt.Fprintf(os.Stderr, "Unknown mcp command: %s\n\n", args[0])
		printMCPUsage(os.Stderr)
		return 2
	}
}

func runMCPServe(args []string) int {
	if len(args) > 0 && (args[0] == "help" || args[0] == "-h" || args[0] == "--help") {
		fmt.Fprintln(os.Stdout, "Usage: termtile mcp serve")
		fmt.Fprintln(os.Stdout, "")
		fmt.Fprintln(os.Stdout, "Start the MCP server on stdio. Designed to be invoked by MCP clients")
		fmt.Fprintln(os.Stdout, "such as Claude Code or Claude Desktop.")
		fmt.Fprintln(os.Stdout, "")
		fmt.Fprintln(os.Stdout, "Example (Claude Code):")
		fmt.Fprintln(os.Stdout, "  claude mcp add termtile -- termtile mcp serve")
		return 0
	}

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	server, err := mcp.NewServer(cfg)
	if err != nil {
		log.Fatalf("Failed to create MCP server: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		cancel()
	}()

	if err := server.Run(ctx); err != nil {
		log.Fatalf("MCP server error: %v", err)
	}
	return 0
}

type mcpCleanupSession struct {
	name      string
	workspace string
	slot      int
	slotValid bool
	alive     bool
	tracked   bool
}

func runMCPCleanup(args []string) int {
	fs := flag.NewFlagSet("mcp cleanup", flag.ExitOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: termtile mcp cleanup [--force]")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "List termtile tmux sessions and identify tracked vs orphan sessions.")
		fmt.Fprintln(os.Stderr, "Use --force to kill only orphan sessions.")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Flags:")
		fs.PrintDefaults()
	}
	force := fs.Bool("force", false, "Kill all discovered orphan termtile sessions")
	_ = fs.Parse(args)
	if fs.NArg() > 0 {
		fmt.Fprintf(os.Stderr, "cleanup does not accept positional arguments: %s\n", strings.Join(fs.Args(), " "))
		fs.Usage()
		return 2
	}

	out, err := exec.Command("tmux", "list-sessions", "-F", "#{session_name}").Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
			fmt.Fprintln(os.Stdout, "No termtile tmux sessions found.")
			return 0
		}
		fmt.Fprintf(os.Stderr, "failed to list tmux sessions: %v\n", err)
		return 1
	}

	var sessions []mcpCleanupSession
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		sessionName := strings.TrimSpace(line)
		if sessionName == "" || !strings.HasPrefix(sessionName, "termtile-") {
			continue
		}

		wsName, slot, slotValid := parseTermtileSessionName(sessionName)
		alive := exec.Command("tmux", "has-session", "-t", sessionName).Run() == nil
		tracked := workspace.HasSessionInRegistry(sessionName)
		sessions = append(sessions, mcpCleanupSession{
			name:      sessionName,
			workspace: wsName,
			slot:      slot,
			slotValid: slotValid,
			alive:     alive,
			tracked:   tracked,
		})
	}

	if len(sessions) == 0 {
		fmt.Fprintln(os.Stdout, "No termtile tmux sessions found.")
		return 0
	}

	fmt.Fprintln(os.Stdout, "Discovered termtile tmux sessions:")
	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "SESSION\tWORKSPACE\tSLOT\tSTATUS\tALIVE")
	orphanCount := 0
	for _, session := range sessions {
		slotText := "?"
		if session.slotValid {
			slotText = strconv.Itoa(session.slot)
		}
		aliveText := "no"
		if session.alive {
			aliveText = "yes"
		}
		status := "orphan"
		if session.tracked {
			status = "tracked"
		} else {
			orphanCount++
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", session.name, session.workspace, slotText, status, aliveText)
	}
	_ = tw.Flush()

	if !*force {
		fmt.Fprintln(os.Stdout, "")
		fmt.Fprintln(os.Stdout, "Run 'termtile mcp cleanup --force' to kill orphan sessions only.")
		return 0
	}

	if orphanCount == 0 {
		fmt.Fprintln(os.Stdout, "")
		fmt.Fprintln(os.Stdout, "No orphan sessions to kill.")
		return 0
	}

	killed := 0
	for _, session := range sessions {
		if session.tracked || !session.alive {
			continue
		}
		if err := exec.Command("tmux", "kill-session", "-t", session.name).Run(); err != nil {
			fmt.Fprintf(os.Stderr, "failed to kill session %q: %v\n", session.name, err)
			return 1
		}
		killed++
	}

	fmt.Fprintln(os.Stdout, "")
	fmt.Fprintf(os.Stdout, "Killed %d orphan termtile session(s).\n", killed)
	return 0
}

func parseTermtileSessionName(sessionName string) (workspace string, slot int, ok bool) {
	trimmed := strings.TrimPrefix(sessionName, "termtile-")
	cut := strings.LastIndex(trimmed, "-")
	if cut == -1 {
		return trimmed, 0, false
	}

	workspace = trimmed[:cut]
	slotText := trimmed[cut+1:]
	parsedSlot, err := strconv.Atoi(slotText)
	if err != nil {
		return workspace, 0, false
	}
	return workspace, parsedSlot, true
}
