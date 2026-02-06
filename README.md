# termtile

A terminal window tiling manager for Linux/X11. termtile automatically
arranges terminal emulator windows into grid layouts with a single hotkey.
It also provides workspace management, inter-terminal communication via tmux,
and an MCP server for orchestrating AI coding agents.

## Features

**Window Tiling**
- Multiple layout modes: auto grid, fixed grid, vertical stack, horizontal
- Tile regions: full screen, half screen (left/right/top/bottom), or custom areas
- Dynamic grid calculation (2 terminals -> 1x2, 4 -> 2x2, 6 -> 2x3)
- Named layouts with inheritance from built-in presets
- Multi-monitor support with independent workspaces per monitor
- Panel and dock aware via EWMH work area detection

**Workspace Management**
- Create, save, load, and close named workspaces
- Spawn N terminals into a tiled layout with one command
- Agent mode: tmux-backed sessions for programmatic terminal control
- Add and remove terminals from running workspaces with automatic retiling

**AI Agent Orchestration (MCP Server)**
- Model Context Protocol server for AI agent management
- Spawn, monitor, and communicate with AI coding agents (Claude, Codex, Aider, Gemini)
- Read agent output, wait for patterns or idle state, orchestrate multi-agent workflows
- Configurable agent registry with per-agent settings

**Desktop Integration**
- Global hotkeys via X11 (configurable key combinations)
- Command palette via rofi, dmenu, or wofi
- Background daemon with systemd user service support
- Interactive TUI for layout browsing and preview
- YAML configuration with include directives and hot reload

## Demo

Press `Super+Alt+T` and all terminals on the current monitor snap into a grid:

```
Before:              After (4 terminals):
+-----+              +----+----+
|  .  |              | 1  | 2  |
| . . |      ->      +----+----+
|   . |              | 3  | 4  |
+-----+              +----+----+
```

## Requirements

- Linux with X11/Xorg (Wayland works via XWayland)
- EWMH-compliant window manager (GNOME, KDE, i3, bspwm, Openbox, etc.)
- Go 1.24+ (for building from source)
- tmux (for workspace agent mode and MCP server features)
- rofi, dmenu, or wofi (optional, for command palette)

## Installation

### From Source

```bash
go install github.com/1broseidon/termtile/cmd/termtile@latest
```

### Install Script

The install script builds the binary, installs the default configuration, and
sets up a systemd user service:

```bash
git clone https://github.com/1broseidon/termtile.git
cd termtile
./scripts/install.sh
```

### Manual Build

```bash
git clone https://github.com/1broseidon/termtile.git
cd termtile
go build -o termtile ./cmd/termtile
sudo install -m 755 termtile /usr/local/bin/termtile
```

After installing, create the default configuration:

```bash
mkdir -p ~/.config/termtile
cp configs/termtile.yaml ~/.config/termtile/config.yaml
```

## Quick Start

Start the daemon:

```bash
termtile daemon
```

Or run as a systemd user service:

```bash
systemctl --user enable --now termtile
```

Open a few terminal windows and press `Super+Alt+T`. All terminals on the
active monitor will tile into a grid.

## Usage

### Tiling

The daemon listens for a global hotkey and tiles all terminal windows on the
active monitor into the current layout.

```bash
# Start the daemon (foreground)
termtile daemon

# Check daemon status
termtile status

# Undo the last tiling operation
termtile undo
```

### Layouts

termtile ships with built-in layouts: `auto_full`, `vertical_full`,
`horizontal_full`, `left_half_auto`, `right_half_auto`, `top_half_auto`,
`bottom_half_auto`, and `fixed_2x2_full`.

```bash
# List all available layouts
termtile layout list

# Apply a layout to the running daemon
termtile layout apply left_half_auto

# Apply and tile immediately
termtile layout apply --tile vertical_full

# Preview a layout for 3 seconds, then revert
termtile layout preview --duration 3 fixed_2x2_full

# Set the persistent default layout
termtile layout default horizontal_full
```

Custom layouts are defined in configuration and can inherit from built-in
presets. See the Configuration section below.

### Command Palette

Open a fuzzy-searchable layout switcher (requires rofi, dmenu, or wofi):

```bash
termtile palette
```

Or press `Super+Alt+G` (configurable). The palette auto-detects which launcher
is available.

### Interactive TUI

Browse and preview layouts in the terminal:

```bash
termtile tui
```

### Workspaces

Workspaces group terminal windows together for a specific task.

```bash
# Create a workspace with 4 terminals
termtile workspace new -n 4 dev

# Create a workspace with agent-mode tmux sessions
termtile workspace new -n 3 --agent-mode --cwd ~/project agents

# Save the current terminal layout
termtile workspace save dev

# Load a saved workspace
termtile workspace load dev

# List saved workspaces
termtile workspace list

# Close all terminals in a workspace
termtile workspace close dev

# Rename a workspace
termtile workspace rename dev development
```

### Terminal Management

Add, remove, and interact with terminals in a running workspace.

```bash
# Add a terminal to the current workspace and retile
termtile terminal add

# Add a terminal at a specific slot position
termtile terminal add --slot 2 --cwd ~/project

# Remove the last terminal
termtile terminal remove --last

# List terminals in the current workspace
termtile terminal list

# Show terminal and tmux session status
termtile terminal status
```

In agent-mode workspaces, you can send input to and read output from
individual terminal slots:

```bash
# Send a command to slot 0
termtile terminal send --slot 0 "ls -la"

# Read the last 50 lines from slot 1
termtile terminal read --slot 1 --lines 50

# Wait for specific output to appear in slot 0
termtile terminal read --slot 0 --wait-for "Done" --timeout 30
```

### MCP Server

termtile includes an MCP (Model Context Protocol) server for AI agent
orchestration. The server manages agent lifecycles through tmux sessions,
providing tools to spawn, monitor, and communicate with AI coding agents.

#### Setup

Register termtile as an MCP server with your AI tool:

```bash
# Claude Code
claude mcp add termtile -- termtile mcp serve

# Claude Desktop: add to claude_desktop_config.json
{
  "mcpServers": {
    "termtile": {
      "command": "termtile",
      "args": ["mcp", "serve"]
    }
  }
}
```

#### MCP Tools

The server exposes these tools to MCP clients:

| Tool | Description |
|------|-------------|
| `spawn_agent` | Launch an agent in a new terminal slot |
| `send_to_agent` | Send text input to an agent's terminal |
| `read_from_agent` | Read output from an agent's terminal, optionally wait for a pattern |
| `wait_for_idle` | Wait until an agent finishes processing |
| `list_agents` | List all running agents and their status |
| `kill_agent` | Terminate an agent's tmux session |

#### Agent Configuration

Define agents in `~/.config/termtile/config.yaml`:

```yaml
agents:
  claude:
    command: claude
    args: ["--dangerously-skip-permissions"]
    description: "Claude Code CLI agent"
    prompt_as_arg: true
    idle_pattern: ">"
    spawn_mode: window
  codex:
    command: codex
    args: ["--full-auto"]
    description: "OpenAI Codex CLI agent"
    prompt_as_arg: true
    idle_pattern: ">"
  aider:
    command: aider
    args: ["--yes"]
    description: "Aider AI coding assistant"
    prompt_as_arg: false
```

Configuration fields:

- `command`: Executable name or path
- `args`: CLI arguments passed on launch
- `description`: Human-readable label
- `prompt_as_arg`: Pass the task as a CLI argument (true) or send via tmux
  keys after startup (false)
- `idle_pattern`: Text pattern that indicates the agent is ready for input
- `spawn_mode`: `pane` (tmux pane, default) or `window` (separate terminal
  window)
- `response_fence`: When true, agents are instructed to wrap output in
  delimiters for clean extraction
- `env`: Additional environment variables

### Configuration

Config file location: `~/.config/termtile/config.yaml`

```yaml
# Load additional config fragments from a directory.
include:
  - config.d

# Global hotkey to trigger tiling (Mod4 = Super, Mod1 = Alt).
hotkey: "Mod4-Mod1-t"

# Gap between tiled windows in pixels.
gap_size: 8

# Extra padding around the usable work area.
screen_padding:
  top: 0
  bottom: 0
  left: 0
  right: 0

# Default layout applied on startup.
default_layout: "auto_full"

# Terminal sort order: position, window_id, client_list, active_first.
terminal_sort: "position"

# Log level: debug, info, warning, error.
log_level: "info"
```

#### Custom Layouts

Define custom layouts that inherit from built-in presets:

```yaml
layouts:
  # Terminals on the left half, vertical stack
  left_stack:
    inherits: "builtin:vertical_full"
    tile_region:
      type: "left-half"

  # Fixed 3x3 monitoring grid
  monitor_grid:
    inherits: "builtin:auto_full"
    mode: "fixed"
    fixed_grid:
      rows: 3
      cols: 3

  # Custom region: left 60% of the screen
  wide_left:
    mode: "auto"
    tile_region:
      type: "custom"
      x_percent: 0
      y_percent: 0
      width_percent: 60
      height_percent: 100
    max_terminal_width: 1200
    max_terminal_height: 0
```

Layout modes: `auto` (dynamic grid), `fixed` (rows x cols), `vertical`
(single column), `horizontal` (single row).

Tile regions: `full`, `left-half`, `right-half`, `top-half`, `bottom-half`,
`custom`.

#### Terminal Detection

termtile detects terminals by X11 WM_CLASS. Most common terminals are
recognized automatically. To add a terminal that is not detected:

```bash
# Find your terminal's WM_CLASS
xprop WM_CLASS
# Click on the terminal window, then add the Class value to config:
```

```yaml
terminal_classes:
  - class: MyTerminal
    default: true
  - class: Alacritty
```

#### Per-Terminal Margin Adjustments

Compensate for terminals with unusual internal padding:

```yaml
terminal_margins:
  "Gnome-terminal":
    top: -5
    bottom: -5
    left: -5
    right: -5
```

Positive values shrink the window; negative values expand it.

#### Workspace Limits

Prevent runaway terminal spawning in automated workflows:

```yaml
limits:
  max_terminals_per_workspace: 10
  max_workspaces: 5
  max_terminals_total: 20
  workspace_overrides:
    my-agents:
      max_terminals: 5
```

#### Config Debugging

```bash
termtile config validate
termtile config print --effective
termtile config print --defaults
termtile config explain layouts.auto_full.mode
```

#### Hotkey Syntax

Format: `[Modifier]-[Key]`

| Modifier | Key |
|----------|-----|
| `Mod4` | Super/Windows |
| `Mod1` | Alt |
| `Control` | Ctrl |
| `Shift` | Shift |

Examples: `Mod4-Mod1-t` (Super+Alt+T), `Mod4-t` (Super+T),
`Control-Mod1-t` (Ctrl+Alt+T).

## Auto Grid Layouts

In auto mode, termtile calculates the optimal grid:

| Terminals | Grid | Layout |
|-----------|------|--------|
| 1 | 1x1 | Fullscreen |
| 2 | 1x2 | Side by side |
| 3 | 2x2 | 3 in grid |
| 4 | 2x2 | Perfect grid |
| 5 | 2x3 | 5 in grid |
| 6 | 2x3 | Perfect grid |
| 9 | 3x3 | Perfect grid |

Algorithm: `cols = ceil(sqrt(n)), rows = ceil(n / cols)`

## How It Works

1. **Hotkey Registration** -- XGrabKey registers global hotkeys with the X
   server.
2. **Monitor Detection** -- XRandR identifies connected monitors and their
   geometry.
3. **Terminal Discovery** -- All windows are queried and filtered by WM_CLASS.
4. **Layout Calculation** -- The selected layout mode computes grid positions.
5. **Window Manipulation** -- EWMH protocol moves and resizes windows.
6. **IPC Server** -- A Unix socket server accepts commands from the CLI and
   palette.
7. **Hot Reload** -- Configuration changes are reloaded on SIGHUP without
   restarting the daemon.

## Troubleshooting

### Hotkey Not Working

Check if another application has grabbed the key combination. Verify the daemon
is running:

```bash
systemctl --user status termtile
```

### Terminals Not Detected

Find your terminal's WM_CLASS with `xprop WM_CLASS`, click on the terminal,
and add the Class value to `terminal_classes` in config. Check logs for
detection messages:

```bash
journalctl --user -u termtile -f
```

### Windows Not Moving

Verify EWMH support:

```bash
xprop -root _NET_SUPPORTED | grep MOVERESIZE
```

### Service Fails to Start

Check that DISPLAY is set in the service environment:

```bash
systemctl --user show termtile | grep Environment
```

Edit `~/.config/termtile/env` or the service unit if needed.

## Development

### Project Structure

```
termtile/
├── cmd/termtile/        # CLI entry point and subcommands
├── internal/
│   ├── agent/           # tmux multiplexer for agent sessions
│   ├── config/          # YAML configuration loading and merging
│   ├── hotkeys/         # Global X11 hotkey handling
│   ├── ipc/             # Unix socket IPC protocol
│   ├── mcp/             # MCP server for AI agent orchestration
│   ├── movemode/        # Interactive window repositioning
│   ├── palette/         # Command palette (rofi/dmenu/wofi)
│   ├── terminals/       # Terminal detection via WM_CLASS
│   ├── tiling/          # Layout algorithms and workspace state
│   ├── tui/             # Terminal UI for layout browsing
│   └── x11/             # X11 connection, monitors, EWMH
├── configs/             # Default configuration file
└── scripts/             # Installation and service scripts
```

### Building

```bash
go build -o termtile ./cmd/termtile
```

### Running Tests

```bash
go test ./...
```

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

## License

MIT License. See [LICENSE](LICENSE) for details.

## Credits

Built with:
- [xgb](https://github.com/BurntSushi/xgb) -- X11 Go bindings
- [xgbutil](https://github.com/BurntSushi/xgbutil) -- X11 utilities
- [go-sdk](https://github.com/modelcontextprotocol/go-sdk) -- Model Context Protocol SDK
