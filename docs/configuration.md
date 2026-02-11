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

| Option | Type | Description |
|---|---|---|
| `hotkey` | string | Global hotkey to trigger tiling. |
| `gap_size` | int | Gap between tiled windows in pixels. |
| `default_layout` | string | Layout applied on daemon startup. |
| `terminal_sort` | string | Order of windows: `position`, `window_id`, `client_list`, `active_first`. |
| `log_level` | string | `debug`, `info`, `warning`, `error`. |
| `display` | string | Optional X11 display override for window-mode agent spawns (e.g. `:1`). |
| `xauthority` | string | Optional Xauthority path override used with `display` for window-mode spawns. |

## Hotkeys

Hotkeys use the format `[Modifier]-[Key]`.
Modifiers: `Mod4` (Super), `Mod1` (Alt), `Control`, `Shift`.

```yaml
hotkey: "Mod4-Mod1-t"
cycle_layout_hotkey: "Mod4-Mod1-bracketright"
undo_hotkey: "Mod4-Mod1-u"
move_mode_hotkey: "Mod4-Mod1-r"
terminal_add_hotkey: "Mod4-Mod1-n"
palette_hotkey: "Mod4-Mod1-g"
```

### Move Mode Interaction

`move_mode_hotkey` enters a phase-based interaction model and shows a compact on-screen key legend:

- `select`: cycle terminals (`Arrow keys`), grab (`Enter`), request delete (`d`), insert after (`n`), append (`a`), cancel (`Esc`)
- `move`: after grabbing, pick a target slot (`Arrow keys`) and confirm move/swap (`Enter`)
- `confirm-delete`: confirm deletion (`Enter`) or cancel and return to select (`Esc`)

If text rendering is unavailable in the current environment, Move Mode keeps the border overlays and continues without the text panel.

## Terminal Detection

termtile identifies terminals by their X11 `WM_CLASS`.

```yaml
terminal_classes:
  - class: Alacritty
    default: true
  - class: Gnome-terminal
```

### Spawn Commands
Define how to launch new terminals.
```yaml
terminal_spawn_commands:
  - class: Alacritty
    command: "alacritty --working-directory {{dir}} -e {{cmd}}"
```

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
