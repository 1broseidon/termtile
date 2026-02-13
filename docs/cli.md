# CLI Reference

## Top-level Commands

| Command | Description |
|---|---|
| `termtile daemon` | Start daemon in foreground. |
| `termtile status` | Show daemon status. |
| `termtile undo` | Undo last tiling operation. |
| `termtile layout ...` | List/apply/default/preview layouts. |
| `termtile workspace ...` | Manage saved workspaces and project bindings. |
| `termtile terminal ...` | Add/remove/move/list/send/read terminals. |
| `termtile config ...` | Validate/print/explain config values. |
| `termtile palette` | Open command palette. |
| `termtile tui` | Open interactive TUI. |
| `termtile mcp ...` | MCP server and MCP session cleanup commands. |
| `termtile hook ...` | Hook helper commands used by hook-based agent output flow. |

## MCP Commands

### `termtile mcp serve`

Starts the MCP server on stdio (for Claude Code / other MCP clients).

```bash
termtile mcp serve
```

### `termtile mcp cleanup`

Lists `termtile-*` tmux sessions and marks each as tracked vs orphan.

```bash
termtile mcp cleanup
```

Use `--force` to kill only orphan sessions that are still alive:

```bash
termtile mcp cleanup --force
```

## Hook Commands

Hook commands operate on artifact directories under:

- `~/.local/share/termtile/artifacts/{workspace}/{slot}`
- or `$XDG_DATA_HOME/termtile/artifacts/{workspace}/{slot}`

### `termtile hook start`

Reads `context.md` for a slot and prints rendered hook output context.

```bash
termtile hook start --auto
termtile hook start --workspace my-ws --slot 2
```

### `termtile hook check`

Reads `checkpoint.json`, prints rendered context, then removes the checkpoint file.

```bash
termtile hook check --auto
termtile hook check --workspace my-ws --slot 2
```

### `termtile hook emit`

Writes slot `output.json` with completion payload.

Auto mode (used by native hooks):

```bash
termtile hook emit --auto
```

Manual mode:

```bash
termtile hook emit --workspace my-ws --slot 2 --output "done"
printf 'done' | termtile hook emit --workspace my-ws --slot 2
```

In `--auto` mode, slot/workspace are inferred from tmux session name `termtile-{workspace}-{slot}`.

In `--auto` mode, output is extracted from hook stdin context using:

1. configured `hook_response_field` (if set), then
2. transcript fallback (`transcript_path`) when needed.

## Layout Commands

| Command | Description |
|---|---|
| `termtile layout list [--json]` | List layouts. |
| `termtile layout apply [--tile] <layout>` | Set active layout. |
| `termtile layout default [--tile] <layout>` | Set default layout. |
| `termtile layout preview [--duration N] <layout>` | Temporary preview. |

## Config Commands

| Command | Description |
|---|---|
| `termtile config validate [--path PATH]` | Validate config. |
| `termtile config print [--path PATH] [--effective|--defaults]` | Print configuration. |
| `termtile config explain [--path PATH] <yaml.path>` | Show value source. |
