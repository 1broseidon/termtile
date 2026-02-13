# AI Agent Orchestration

termtile includes a built-in Model Context Protocol (MCP) server for spawning and coordinating CLI agents in tmux-backed slots.

## MCP Server

Start the server on stdio:

```bash
termtile mcp serve
```

### Connecting Your AI Client

**Claude Code:**

```bash
claude mcp add termtile -- termtile mcp serve
```

**Claude Desktop / generic MCP clients:**

```json
{
  "mcpServers": {
    "termtile": {
      "command": "termtile",
      "args": ["mcp", "serve"]
    }
  }
}
```

If window spawns fail from a non-desktop context, pass `DISPLAY` (and optionally `XAUTHORITY`) in the MCP server environment.

## Hook-First Architecture

`output_mode: hooks` is the effective default path.

At spawn time, termtile resolves three abstract hooks:

- `on_start` (default command: `termtile hook start --auto`)
- `on_check` (default command: `termtile hook check --auto`)
- `on_end` (default command: `termtile hook emit --auto`)

Final agent output is written to disk as `output.json` and consumed by MCP tools.

## Hook Delivery Types

| `hook_delivery` | Behavior |
|---|---|
| `cli_flag` | Injects rendered hook JSON directly into the agent CLI command (for example via `--settings`). |
| `project_file` | Writes rendered hook config into a project settings file (for example `.gemini/settings.json`) and restores the original file on cleanup. |
| `none` | No native hook injection. termtile appends explicit file-write instructions so the agent writes `output.json` itself. |

Notes:

- `project_file` mode writes `context.md` into the artifact directory and sends a generic `start` prompt so the start hook can inject the real task context.
- `agent_meta.json` is written at spawn so hook commands can resolve the spawned agent type/config.

## Artifact Storage (Disk)

Artifacts are filesystem-backed, not in-memory.

Default location:

- `~/.local/share/termtile/artifacts/{workspace}/{slot}/output.json`

If `XDG_DATA_HOME` is set:

- `$XDG_DATA_HOME/termtile/artifacts/{workspace}/{slot}/output.json`

Common files per slot:

- `output.json` (final result payload)
- `context.md` (task context for start hook)
- `checkpoint.json` (optional one-shot steering consumed by `hook check`)
- `agent_meta.json` (agent type metadata)
- `hook_state.json` + backup file (project-file hook injection rollback state)

## MCP Tools

| Tool | Current behavior |
|---|---|
| `spawn_agent` | Spawns pane/window agent session, sets up artifact dir, injects hooks (or file-write instructions), supports `depends_on` waiting and `{‍{slot_N.output}‍}` substitution from dependency artifacts. |
| `send_to_agent` | Sends text + Enter to tmux target (optionally wraps with response fence when configured). |
| `read_from_agent` | Pure tmux capture-pane tail (bounded lines, optional clean/since_last/pattern wait). No artifact parsing. |
| `wait_for_idle` | Polls slot `output.json` until a ready payload appears (`status: complete` and non-empty `output`), or timeout. |
| `get_artifact` | Reads and parses slot `output.json` from disk; returns payload output field. |
| `list_agents` | Lists tracked slots and computes `is_idle` using `checkIdle` tiers (fence/pattern/process). |
| `kill_agent` | Restores project-file hooks, stops pipe-pane, kills tmux target, removes tracking, and cleans slot artifact dir. |
| `move_terminal` | Moves terminal between workspaces (X11 desktop move for window mode, workspace registry update, tmux session rename, artifact directory move, tracking update). |

## Idle Detection: Important Distinction

Two different mechanisms are active:

1. `wait_for_idle` uses **artifact polling only** (`output.json`).
2. `checkIdle` (used by `list_agents` and `depends_on` waiting) still uses legacy tiers:
   - fence close-tag detection (pipe file first, capture-pane fallback)
   - configured `idle_pattern`
   - process-child fallback (`pgrep -P`)

This distinction is intentional and currently part of the runtime behavior.

## Pipelines With `depends_on`

`spawn_agent` can wait for dependency slots and template their outputs into a task:

```text
{{ "{{slot_3.output}}" }}
```

Substitution reads the `output` field from dependency slot artifacts. Placeholders for non-dependency slots are left unchanged.

Example:

```text
spawn_agent(agent_type="claude", task="audit repo") -> slot 1
spawn_agent(agent_type="gemini", task="summarize: {{ "{{slot_1.output}}" }}", depends_on=[1]) -> slot 2
```

## Agent Mode Guardrail

If `agent_mode.protect_slot_zero: true` (default), `kill_agent` refuses to kill slot `0` in agent-mode workspaces.
