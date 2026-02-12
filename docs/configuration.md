# Configuration

termtile uses a YAML-based configuration system that supports inheritance, include directives, and live reloading.

## Configuration File

The default configuration file is located at `~/.config/termtile/config.yaml`.

## Project Workspace Configuration (v1)

termtile also supports project-local workspace settings:

- `.termtile/workspace.yaml` (committed): project workspace binding and defaults
- `.termtile/local.yaml` (gitignored): local pull/push sync snapshot
- `~/.config/termtile/workspaces/<workspace>.json`: canonical workspace snapshot

Initialize a project file:

```bash
termtile workspace init --workspace my-workspace
```

Update only the project workspace binding:

```bash
termtile workspace link --workspace my-workspace
```

Sync selected fields between project-local and canonical snapshots:

```bash
termtile workspace sync pull
termtile workspace sync push
```

Default synced fields are:

- `layout`
- `terminals`
- `agent_mode`

### Precedence

Resolver and config precedence for workspace selection/overrides:

1. CLI/tool explicit args
2. Request-scoped hints (for example `source_workspace`)
3. `.termtile/local.yaml`
4. `.termtile/workspace.yaml`
5. `~/.config/termtile/config.yaml`
6. Built-in defaults

### Include Directives
You can split your configuration into multiple files using the `include` key. This supports single files or entire directories.
```yaml
include:
  - config.d/hotkeys.yaml
  - layouts/
```
- Relative paths are resolved from the current file.
- Directories include all `.yaml` and `.yml` files in alphabetical order.
- Cyclic includes are automatically detected and prevented.

## Global Options

| Option | Type | Default | Description |
|---|---|---|---|
| `hotkey` | string | `Mod4-Mod1-t` | Global hotkey to trigger tiling. |
| `gap_size` | int | `0` | Gap between tiled windows in pixels. |
| `screen_padding` | object | `{top:0, bottom:0, left:0, right:0}` | Padding around the screen edges (see Margins below). |
| `default_layout` | string | (first layout) | Layout applied on daemon startup. |
| `preferred_terminal` | string | (auto-detected) | Preferred terminal class to use when spawning. |
| `terminal_sort` | string | `position` | Order of windows: `position`, `window_id`, `client_list`, `active_first`. |
| `log_level` | string | `info` | Simple log level: `debug`, `info`, `warning`, `error`. See `logging` section for advanced options. |
| `display` | string | (inherited) | Optional X11 display override for window-mode agent spawns (e.g. `:1`). |
| `xauthority` | string | (inherited) | Optional Xauthority path override used with `display` for window-mode spawns. |

## Hotkeys

Hotkeys use the format `[Modifier]-[Key]`.
Modifiers: `Mod4` (Super), `Mod1` (Alt), `Control`, `Shift`.

```yaml
hotkey: "Mod4-Mod1-t"                          # Main tiling hotkey
cycle_layout_hotkey: "Mod4-Mod1-bracketright"   # Cycle to next layout
cycle_layout_reverse_hotkey: ""                 # Cycle to previous layout (optional)
undo_hotkey: "Mod4-Mod1-u"                      # Undo last layout change
move_mode_hotkey: "Mod4-Mod1-r"                 # Enter move/relocate mode (default: Mod4-Mod1-r)
terminal_add_hotkey: "Mod4-Mod1-n"              # Add new terminal to active workspace (default: Mod4-Mod1-n)
palette_hotkey: "Mod4-Mod1-g"                   # Open command palette (default: Mod4-Mod1-g)
```

### Move Mode Interaction

`move_mode_hotkey` enters a phase-based interaction model and shows a compact on-screen key legend:

- `select`: cycle terminals (`Arrow keys`), grab (`Enter`), request delete (`d`), insert after (`n`), append (`a`), cancel (`Esc`)
- `move`: after grabbing, pick a target slot (`Arrow keys`) and confirm move/swap (`Enter`)
- `confirm-delete`: confirm deletion (`Enter`) or cancel and return to select (`Esc`)


### Move Mode Timeout

```yaml
move_mode_timeout: 10  # Timeout in seconds (default: 10)
```

## Command Palette

The command palette provides a quick launcher for common actions.

```yaml
palette_hotkey: "Mod4-Mod1-g"      # Hotkey to open palette (default: Mod4-Mod1-g)
palette_backend: "auto"             # Backend: auto, rofi, fuzzel, dmenu, wofi (default: auto)
palette_fuzzy_matching: false       # Enable fuzzy matching (default: false)
```

The palette automatically detects available backends (`rofi`, `fuzzel`, `dmenu`, `wofi`) when set to `auto`.

## Terminal Detection

termtile identifies terminals by their X11 `WM_CLASS`.

```yaml
terminal_classes:
  - class: Alacritty
    default: true
  - class: Gnome-terminal
```

### Spawn Commands
Define how to launch new terminals. This is a map of terminal class to command template.
```yaml
terminal_spawn_commands:
  Alacritty: "alacritty --working-directory {{dir}} -e {{cmd}}"
  kitty: "kitty --directory {{dir}} {{cmd}}"
```

Available template variables: `{{dir}}` (working directory), `{{cmd}}` (command to execute).

### Per-Terminal Margins
Adjust for internal padding of specific terminals.
```yaml
terminal_margins:
  "Gnome-terminal":
    top: -5
    bottom: -5
    left: -5
    right: -5
```

## Layouts

Layouts can be custom-defined or inherit from built-in presets.

```yaml
layouts:
  my-grid:
    inherits: "builtin:grid"
    gap_size: 16
  wide-left:
    mode: "auto"
    tile_region:
      type: "custom"
      width_percent: 60
      height_percent: 100
```

See the [Layouts Documentation](layouts.md) for more details.

## Agent Mode

Configure how termtile manages terminal multiplexers (tmux/screen) for agent orchestration.

```yaml
agent_mode:
  multiplexer: "auto"                    # Which multiplexer to use: auto, tmux, screen (default: auto)
  manage_multiplexer_config: true        # Let termtile manage multiplexer config (default: true)
  protect_slot_zero: true                # Protect slot 0 from being killed (default: true)
```

- **`multiplexer`**: Specifies which terminal multiplexer to use. `auto` will detect and prefer tmux over screen.
- **`manage_multiplexer_config`**: When `true`, termtile generates an optimized multiplexer config file (e.g., `~/.config/termtile/tmux.conf`). Set to `false` to use your own tmux/screen config entirely.
- **`protect_slot_zero`**: When `true`, prevents slot 0 from being killed via MCP tools, providing a safe anchor slot.

## Logging

Configure structured logging for agent actions and system events.

```yaml
logging:
  enabled: false                         # Enable structured logging (default: false)
  level: "info"                          # Log level: debug, info, warn, error (default: info)
  file: "~/.local/share/termtile/agent-actions.log"  # Log file path
  max_size_mb: 10                        # Maximum log file size before rotation (default: 10)
  max_files: 3                           # Number of rotated files to keep (default: 3)
  include_content: false                 # Include full command/output content in logs (default: false)
  preview_length: 100                    # Length of content preview when include_content is false (default: 100)
```

**Note**: The simple `log_level` field is still supported for basic logging, but the `logging` section provides more comprehensive control.


## Agent Orchestration

Configure how AI agents are managed via MCP.

```yaml
agents:
  claude:
    command: "claude"
    idle_pattern: "❯"
    spawn_mode: "window"
    response_fence: true
```

## Limits

Prevent resource exhaustion.

```yaml
limits:
  max_terminals_per_workspace: 10
  max_workspaces: 5
  max_terminals_total: 20
```

## Config Management CLI

| Command | Description |
|---|---|
| `termtile config validate` | Check config for syntax and schema errors. |
| `termtile config print --effective` | Show the final merged configuration. |
| `termtile config explain <key>` | Show value and exactly which file/line it came from. |
| `termtile tui` | Interactive TUI for editing common settings. |
