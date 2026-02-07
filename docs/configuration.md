# Configuration

termtile uses a YAML-based configuration system that supports inheritance, include directives, and live reloading.

## Configuration File

The default configuration file is located at `~/.config/termtile/config.yaml`.

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

## Hotkeys

Hotkeys use the format `[Modifier]-[Key]`.
Modifiers: `Mod4` (Super), `Mod1` (Alt), `Control`, `Shift`.

```yaml
hotkey: "Mod4-Mod1-t"
cycle_layout_hotkey: "Mod4-Mod1-bracketright"
undo_hotkey: "Mod4-Mod1-u"
move_mode_hotkey: "Mod4-Mod1-m"
palette_hotkey: "Mod4-Mod1-g"
```

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
