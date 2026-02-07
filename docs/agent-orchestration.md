# AI Agent Orchestration

termtile includes a built-in Model Context Protocol (MCP) server that allows AI agents to control terminal windows, execute commands, and coordinate with other agents.

## MCP Server

The server runs on `stdio` and can be integrated with Claude Desktop, Claude Code, or any MCP-compliant client.

**Launch command:**
```bash
termtile mcp serve
```

## Available Tools

| Tool | Purpose |
|---|---|
| `spawn_agent` | Launch an agent (Claude, Aider, etc.) in a new terminal window or pane. |
| `send_to_agent` | Send text input to an agent's terminal session. |
| `read_from_agent` | Capture output from the terminal with optional pattern matching. |
| `wait_for_idle` | Block until the agent has finished processing (detected via patterns/process). |
| `list_agents` | See all active agents and their current status. |
| `kill_agent` | Terminate an agent and close its window. |

## Agent Lifecycle

### Spawn Modes
- **Window Mode** (Default): Spawns a dedicated terminal window. This ensures shell init files (`.zshrc`, `.bashrc`) are sourced, providing the agent with your full environment and tools.
- **Pane Mode**: Splits an existing tmux window to create a smaller pane.

### Response Fencing
When `response_fence: true` is enabled in config, termtile instructs agents to wrap their output in special tags:
`[termtile-response]` ... `[/termtile-response]`

This allows for:
- Reliable extraction of the agent's actual response.
- Accurate idle detection (the agent is idle when the closing tag appears).
- Filtering out background noise or shell prompts.

## Idle Detection Tiers

termtile uses a three-tier system to determine if an agent is "done":

1. **Fence Detection**: If fencing is enabled, it waits for the `[/termtile-response]` tag.
2. **Content Pattern**: If an `idle_pattern` (like `❯`) is defined, it scans the last few lines of output.
3. **Process-based**: Fallback that checks if the agent's process (via `pgrep`) has any active children.

## Configuration

Agents are defined in your `config.yaml` under the `agents:` section:

```yaml
agents:
  claude:
    command: "claude"
    args: ["--dangerously-skip-permissions"]
    idle_pattern: "❯"
    spawn_mode: "window"
    response_fence: true
    description: "Claude Code CLI agent"
```
