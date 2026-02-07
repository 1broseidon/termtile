# CLI Reference

## General Commands

| Command | Description |
|---|---|
| `termtile daemon` | Starts the background daemon. |
| `termtile status` | Shows daemon health, active layout, and terminal count. |
| `termtile undo` | Restores window positions from before the last tile. |
| `termtile tui` | Launches the interactive configuration TUI. |
| `termtile palette` | Opens the fuzzy-search command palette (rofi/dmenu). |

## Layout Management

`termtile layout <subcommand>`

| Subcommand | Flags | Description |
|---|---|---|
| `list` | `--json` | List all available layouts. |
| `apply <name>` | `--tile` | Set the active layout. |
| `default <name>` | `--tile` | Set the default layout in config. |
| `preview <name>`| `--duration`| Temporarily apply a layout. |

## Workspace Management

`termtile workspace <subcommand>`

| Subcommand | Description |
|---|---|
| `new <name>` | Create a new workspace and spawn N terminals. |
| `save <name>` | Capture the current window state to a file. |
| `load <name>` | Restore a saved workspace. |
| `list` | List all saved workspaces. |
| `close <name>` | Close all windows in a workspace. |
| `rename <old> <new>` | Rename a workspace and its tmux sessions. |
| `delete <name>` | Delete a workspace save file. |

## Terminal Control

`termtile terminal <subcommand>`

| Subcommand | Description |
|---|---|
| `add` | Add a new terminal to the active workspace. |
| `remove` | Remove a terminal by slot number. |
| `list` | List terminals in the current workspace. |
| `status` | Show tmux session status for all slots. |
| `send` | Send text input to a specific slot. |
| `read` | Read output/scrollback from a slot. |

## Configuration

`termtile config <subcommand>`

| Subcommand | Description |
|---|---|
| `validate` | Validate the configuration file. |
| `print` | Print effective config or defaults. |
| `explain <path>` | Explain the source of a config value. |

## AI Agent (MCP)

`termtile mcp <subcommand>`

| Subcommand | Description |
|---|---|
| `serve` | Start the MCP server (Stdio). |
| `cleanup` | Kill orphaned agent tmux sessions. |
