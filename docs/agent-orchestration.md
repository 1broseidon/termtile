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
| `get_artifact` | Fetch the last captured output artifact for a slot. |

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

## Pipeline Dependencies (`depends_on`)

The `spawn_agent` tool supports complex multi-step workflows through the `depends_on` parameter. This allows you to chain agents together, ensuring that downstream agents only start once their predecessors have completed their tasks.

- **`depends_on`**: A list of slot numbers that this agent depends on. The spawn operation will be held in a queue until all dependency slots go idle.
- **`depends_on_timeout`**: The maximum time (in seconds, default 300) to wait for dependencies to clear. If the timeout is reached, the spawn fails.

If a dependency slot is missing or the process in that slot is killed before completion, the dependent spawn will fail. This enables multi-step agent workflows where agent B waits for agent A to finish.

## Artifact Passing

When an agent goes idle, termtile automatically captures its fenced output (the content between `[termtile-response]` tags) as an in-memory artifact.

- **Storage**: Artifacts are capped at 1MB per slot.
- **Persistence**: Artifacts persist until the slot is reused for a new agent or the workspace is closed.

### Using Artifacts

There are two primary ways to consume captured artifacts:

1. **`get_artifact` Tool**: Explicitly fetch a specific slot's captured output.
   - Params: `slot`, `workspace`.
2. **Template Substitution**: Use the `{{slot_N.output}}` placeholder in the `task` parameter of `spawn_agent`.

When `depends_on` is set, termtile automatically replaces these placeholders with the actual output from the dependency slot before launching the agent.

### Example Pipeline

The following example demonstrates Agent A performing an audit and Agent B summarizing the results of that audit:

```
spawn_agent(type: "claude", task: "audit the codebase") → slot 1
spawn_agent(type: "gemini", task: "summarize: {{slot_1.output}}", depends_on: [1]) → slot 2
```
