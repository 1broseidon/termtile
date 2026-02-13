# Configuration

termtile is configured with YAML at:

- `~/.config/termtile/config.yaml`

## Project Workspace Files (v1)

Project-local workspace config is stored in:

- `.termtile/workspace.yaml` (shared)
- `.termtile/local.yaml` (local overrides)

Initialize or update project linkage:

```bash
termtile workspace init --workspace my-workspace
termtile workspace link --workspace my-workspace
```

Sync between project-local and canonical snapshots:

```bash
termtile workspace sync pull
termtile workspace sync push
```

### Precedence

1. CLI/tool explicit args
2. Request-scoped hints (`source_workspace`)
3. `.termtile/local.yaml`
4. `.termtile/workspace.yaml`
5. `~/.config/termtile/config.yaml`
6. Built-in defaults

## Agent Configuration (MCP)

Agents are configured under `agents:`. These fields control spawn behavior, task delivery, hook integration, model selection, and output capture.

### Agent fields

| Field | Type | Default / behavior |
|---|---|---|
| `command` | string | Required executable name/path. |
| `args` | list[string] | Extra CLI args. |
| `description` | string | Optional description for tooling/UI. |
| `env` | map[string]string | Extra env vars for spawned process. |
| `spawn_mode` | `pane` \| `window` | Defaults to `pane` unless overridden by request or config. |
| `ready_pattern` | string | If set, used to wait for ready prompt before sending task. |
| `idle_pattern` | string | Used by `checkIdle` content-based idle detection (list/dependency checks). |
| `output_mode` | `hooks` \| `tags` \| `terminal` | Effective default is `hooks` when empty. |
| `hooks.on_start` | string | Hook command for session start context injection. |
| `hooks.on_check` | string | Hook command for mid-run steering/checkpoint ingestion. |
| `hooks.on_end` | string | Hook command for final output capture. |
| `hook_delivery` | `cli_flag` \| `project_file` \| `none` | Native hook delivery mechanism. |
| `hook_settings_flag` | string | CLI flag used when `hook_delivery: cli_flag` (for example `--settings`). |
| `hook_settings_dir` | string | Settings directory for `project_file` injection (for example `.gemini`). |
| `hook_settings_file` | string | Settings filename for `project_file` injection (for example `settings.json`). |
| `hook_events` | map[string]string | Abstract hook names (`on_start`, `on_check`, `on_end`) to native event names. |
| `hook_entry` | map[string]any | Template for one event entry; supports substitutions. |
| `hook_wrapper` | map[string]any | Top-level hook config template; `"{{events}}"` is replaced by rendered events map. |
| `hook_output` | map[string]any | Template returned by `termtile hook start/check`; supports `"{{context}}"`. |
| `hook_response_field` | string | Field to read from hook context stdin in `hook emit --auto` before transcript fallback. |
| `response_fence` | bool | Legacy fence wrapping/counting support (still used by `checkIdle` tiers). |
| `prompt_as_arg` | bool | If true, task is passed as CLI argument (optionally via `prompt_flag`). |
| `prompt_flag` | string | Flag used with `prompt_as_arg` (for example `-i`). |
| `pipe_task` | bool | If true, task is piped via stdin (`printf ... | command`). |
| `models` | list[string] | Allowed/known model list for this agent. |
| `default_model` | string | Model selected when request does not provide one. |
| `model_flag` | string | Flag used to pass selected model (defaults to `--model` when empty). |

### Hook template substitutions

`hook_entry` replacements:

- `{{command}}`: resolved hook command string
- `{{event}}`: native event name from `hook_events`
- `{{name}}`: abstract hook name (`on_start`/`on_check`/`on_end`)

`hook_wrapper` replacement:

- `{{events}}`: rendered event map

`hook_output` replacement:

- `{{context}}`: context/checkpoint payload emitted by `termtile hook start/check`

### Realistic multi-agent example

```yaml
agents:
  claude:
    command: "claude"
    args: ["--dangerously-skip-permissions"]
    description: "Claude Code CLI agent"
    spawn_mode: "window"
    output_mode: "hooks"
    prompt_as_arg: true
    idle_pattern: "❯"
    response_fence: true
    models: ["sonnet", "haiku", "opus"]
    hook_delivery: "cli_flag"
    hook_settings_flag: "--settings"
    hook_events:
      on_start: "SessionStart"
      on_check: "PostToolUse"
      on_end: "Stop"
    hook_entry:
      hooks:
        - type: "command"
          command: "{{command}}"
    hook_wrapper:
      hooks: "{{events}}"
    hook_output:
      hookSpecificOutput:
        additionalContext: "{{context}}"

  gemini:
    command: "gemini"
    args: ["--approval-mode", "auto_edit"]
    description: "Google Gemini CLI"
    spawn_mode: "window"
    output_mode: "hooks"
    prompt_as_arg: true
    prompt_flag: "-i"
    idle_pattern: ">"
    response_fence: true
    hook_delivery: "project_file"
    hook_settings_dir: ".gemini"
    hook_settings_file: "settings.json"
    hook_response_field: "prompt_response"
    hook_events:
      on_start: "BeforeAgent"
      on_check: "AfterTool"
      on_end: "AfterAgent"
    hook_entry:
      matcher: "*"
      hooks:
        - type: "command"
          command: "{{command}}"
    hook_wrapper:
      hooks: "{{events}}"
    hook_output:
      decision: "allow"
      hookSpecificOutput:
        additionalContext: "{{context}}"

  codex:
    command: "codex"
    args:
      - "--dangerously-bypass-approvals-and-sandbox"
      - "--no-alt-screen"
      - "-c"
      - "notice.model_migrations={}"
      - "-c"
      - "notice.hide_rate_limit_switch_prompt=true"
    description: "OpenAI Codex CLI agent"
    spawn_mode: "window"
    output_mode: "hooks"
    prompt_as_arg: true
    idle_pattern: "›"
    response_fence: true
    hook_delivery: "none"    # fileWriteInstructions appended to task
    models: ["gpt-5.2-codex", "gpt-5.3-codex", "gpt-5.1-codex-max", "gpt-5.2", "gpt-5.1-codex-mini"]
    default_model: "gpt-5.2-codex"
    model_flag: "--model"
```

## Include Directives

Split your configuration into multiple files:

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
| `screen_padding` | object | `{top:0, bottom:0, left:0, right:0}` | Padding around the screen edges. |
| `default_layout` | string | (first layout) | Layout applied on daemon startup. |
| `preferred_terminal` | string | (auto-detected) | Preferred terminal class for spawning. |
| `terminal_sort` | string | `position` | Window order: `position`, `window_id`, `client_list`, `active_first`. |
| `log_level` | string | `info` | Simple log level: `debug`, `info`, `warning`, `error`. |
| `display` | string | (inherited) | X11 display override for window-mode agent spawns. |
| `xauthority` | string | (inherited) | Xauthority path override for window-mode spawns. |

## Hotkeys

```yaml
hotkey: "Mod4-Mod1-t"
cycle_layout_hotkey: "Mod4-Mod1-bracketright"
cycle_layout_reverse_hotkey: ""
undo_hotkey: "Mod4-Mod1-u"
move_mode_hotkey: "Mod4-Mod1-r"
terminal_add_hotkey: "Mod4-Mod1-n"
palette_hotkey: "Mod4-Mod1-g"
```

Modifiers: `Mod4` (Super), `Mod1` (Alt), `Control`, `Shift`.

### Move Mode

`move_mode_hotkey` enters a phase-based interaction with on-screen key legend:

- **select**: cycle terminals (`Arrow keys`), grab (`Enter`), delete (`d`), insert (`n`), append (`a`), cancel (`Esc`)
- **move**: pick target slot (`Arrow keys`), confirm (`Enter`)
- **confirm-delete**: confirm (`Enter`) or cancel (`Esc`)

```yaml
move_mode_timeout: 10  # seconds (default: 10)
```

## Command Palette

```yaml
palette_hotkey: "Mod4-Mod1-g"
palette_backend: "auto"          # auto, rofi, fuzzel, dmenu, wofi
palette_fuzzy_matching: false
```

## Terminal Detection

```yaml
terminal_classes:
  - class: Alacritty
    default: true
  - class: Gnome-terminal
```

### Spawn Commands

```yaml
terminal_spawn_commands:
  Alacritty: "alacritty --working-directory {{dir}} -e {{cmd}}"
  kitty: "kitty --directory {{dir}} {{cmd}}"
```

### Per-Terminal Margins

```yaml
terminal_margins:
  "Gnome-terminal":
    top: -5
    bottom: -5
    left: -5
    right: -5
```

## Layouts

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

See the [Layouts Documentation](layouts.md) for details.

## Agent Mode

```yaml
agent_mode:
  multiplexer: "auto"
  manage_multiplexer_config: true
  protect_slot_zero: true
```

- `protect_slot_zero: true` blocks `kill_agent` for slot `0` in agent-mode workspaces.

## Logging

```yaml
logging:
  enabled: false
  level: "info"
  file: "~/.local/share/termtile/agent-actions.log"
  max_size_mb: 10
  max_files: 3
  include_content: false
  preview_length: 50
```

## Limits

```yaml
limits:
  max_terminals_per_workspace: 10
  max_workspaces: 5
  max_terminals_total: 20
```

## Config CLI

| Command | Description |
|---|---|
| `termtile config validate` | Validate config and schema. |
| `termtile config print --effective` | Print merged effective config. |
| `termtile config explain <yaml.path>` | Show resolved value and source location. |
