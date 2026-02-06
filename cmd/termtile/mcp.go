package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/1broseidon/termtile/internal/config"
	"github.com/1broseidon/termtile/internal/mcp"
)

func printMCPUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage: termtile mcp <command>")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Commands:")
	fmt.Fprintln(w, "  serve    Start the MCP server (stdio transport)")
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
