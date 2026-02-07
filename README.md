# termtile

A terminal window tiling manager and agent orchestrator for Linux/X11.

![termtile agent orchestration demo](termtile-demo-github.gif)

termtile automatically arranges your terminal emulator windows into clean, grid-based layouts. It combines robust window management with workspace persistence and a Model Context Protocol (MCP) server for orchestrating AI coding agents.

## Key Features

- **Instant Tiling**: Snap all open terminals on your current monitor into a grid with one hotkey (`Super+Alt+T`). It automatically detects active monitors and respects your desktop's work area (panels and docks).
- **Smart Layouts**: Automatically calculates optimal grids based on window count (e.g., 2 terminals -> 1x2, 4 -> 2x2). Supports vertical stacks, horizontal rows, fixed grids, and custom regions.
- **Workspace Management**: Save and restore groups of terminals for different projects. Workspaces track current working directories, window positions, and integrated `tmux` sessions.
- **Agent Orchestration**: A built-in MCP server designed for AI-native workflows. Spawn, monitor, and pipe data to AI agents (Claude, Aider, etc.) directly in your tiled terminals.
- **Desktop Native**: Built specifically for X11 with EWMH compliance. Integrates with systemd for daemon management and supports external launchers like `rofi`, `dmenu`, or `wofi` for command palettes.
- **Interactive TUI**: A terminal UI for browsing, previewing, and selecting layouts without needing to remember complex command flags.

## Quick Install

### From Source
```bash
go install github.com/1broseidon/termtile/cmd/termtile@latest
```

### Via Install Script
The install script builds the binary, installs default configuration files, and sets up a systemd user service:
```bash
git clone https://github.com/1broseidon/termtile.git
cd termtile
./scripts/install.sh
```

## Basic Usage

### Running the Daemon
termtile requires a background daemon to listen for global hotkeys and manage terminal state.
```bash
# Start manually for testing
termtile daemon

# Or run as a systemd user service (recommended)
systemctl --user enable --now termtile
```

### Tiling & Layouts
Once the daemon is running, use hotkeys or the CLI to organize your windows:
```bash
# Apply a specific layout and tile immediately
termtile layout apply columns --tile

# Preview a layout for 3 seconds before reverting
termtile layout preview master-stack --duration 3

# Open the interactive layout browser
termtile tui
```

### Working with Agents
Use the MCP server to orchestrate AI agents from Claude Code or any MCP client:
```bash
# Start the MCP server (stdio transport)
termtile mcp serve

# Create a workspace with tmux sessions for agent control
termtile workspace new --agent-mode -n 3 dev-env
```

## Documentation

For detailed guides and configuration options, see the `docs/` directory:

- [Getting Started](docs/getting-started.md) — Detailed installation and first steps.
- [Configuration](docs/configuration.md) — Customizing hotkeys, gaps, and terminal detection.
- [Layouts](docs/layouts.md) — Understanding grid modes and custom regions.
- [Workspaces](docs/workspaces.md) — Managing groups of terminals and state.
- [Agent Orchestration](docs/agent-orchestration.md) — Using the MCP server with AI agents.
- [CLI Reference](docs/cli.md) — Full list of commands and flags.
- [TUI Guide](docs/tui.md) — Using the interactive layout browser.
- [Daemon Mode](docs/daemon.md) — Background execution and systemd integration.

## Requirements

- Linux with X11/Xorg (Wayland support via XWayland).
- EWMH-compliant window manager (GNOME, KDE, i3, bspwm, etc.).
- `tmux` (required for workspaces and agent features).
- `rofi`, `dmenu`, or `wofi` (optional, for the command palette).

## License

MIT License. See [LICENSE](LICENSE) for details.
