# Codex Agent Instructions

## Scope

These instructions apply to the entire repository.

## Task Management (Brainfile)

- Use Brainfile MCP tools for task operations (`list`, `search`, `get`, `add`, `move`, `patch`, `subtasks`, `contract_*`).
- Create subtasks immediately for complex tasks.
- Include description, priority, tags, and related files when creating tasks.
- Move tasks to `In Progress` before execution and to `Done` after completion.
- Tasks delegated to spawned agents must have contracts attached first.
- Plans and research notes belong in `.brainfile/plans/`.

## Agent Orchestration (Termtile MCP)

- Use Termtile MCP tools for agent lifecycle: `spawn_agent`, `send_to_agent`, `read_from_agent`, `wait_for_idle`, `list_agents`, `kill_agent`.
- Do not use `termtile terminal ...` CLI commands for MCP-managed agent orchestration.
- Prefer `window: true` when delegating visible parallel work.
- Always set or resolve the correct workspace explicitly when context is ambiguous.
- Delegate one task per fresh spawned agent and kill the agent after task completion.

## Delegation Contract

- Delegate with a task-specific pickup prompt, e.g. `Pick up task-NN from .brainfile/brainfile.md`.
- Require final delegate response inside `[termtile-response]...[/termtile-response]`.
- Require delegates to run listed validation commands before marking a task delivered.
- Partition file ownership across parallel delegates to avoid collisions.

## Monitoring Pattern (Low Context)

- Default monitoring loop:
  1. `list_agents` for slot state.
  2. `wait_for_idle` for completion checks.
  3. `read_from_agent` only when needed.
- Treat `read_from_agent` as a narrow tail view, not a transcript dump.
- Use `read_from_agent` with `clean: true` and a small line window (`lines: 50` default, `100` max).
- Avoid repeated large reads; space checks and prefer state-based polling.

## Steering Behavior

- If a delegate stalls, send one targeted `send_to_agent` corrective message, then return to passive polling.
- Common corrective nudges:
  - approval prompt loops: instruct no-approval path and sandbox-safe test env.
  - wrapper mismatch: restate `[termtile-response]` requirement.
  - ownership drift: restate task/file boundaries.
- Keep steering minimal and specific; do not micromanage every step.

## Validation & Completion

- Before declaring completion, ensure:
  - Contract status is `delivered`.
  - Subtasks are complete.
  - Task is moved to `Done`.
  - Delegate agent is terminated if no longer needed.
- Run project validation from orchestrator side when practical (`go test ./...`) using sandbox-safe cache settings when required.

## Project Conventions

- Go module: `github.com/1broseidon/termtile`.
- CLI commands in `cmd/termtile/`, usually one file per subcommand.
- Config uses strict YAML decoding: add new fields across `internal/config/config.go`, `internal/config/raw.go`, and `internal/config/effective.go` (plus explain/docs as needed).
- Keep changes focused and minimal; do not fix unrelated issues during task execution.
