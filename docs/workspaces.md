# Workspaces

Workspaces allow you to manage groups of terminal windows as a single unit. You can save their state, including working directories and running commands, and restore them later.

## Active Workspaces

When you create or load a workspace, termtile tracks it in an active registry. Each virtual desktop can have one active workspace.

### Creation
```bash
# Create a workspace with 4 terminals
termtile workspace new --n 4 dev-env
```

### Modification
- **Add Terminal**: `termtile terminal add` adds a window to the current workspace and triggers a retile.
- **Remove Terminal**: `termtile terminal remove --slot 2` closes the window and re-indexes the remaining terminals.

## Persistence (Save & Load)

Saved workspaces are stored as JSON files in `~/.config/termtile/workspaces/`.

### Saving
termtile inspects your running terminals to capture:
- The terminal emulator class.
- The Current Working Directory (CWD) of the shell.
- The currently running command (optional).

```bash
termtile workspace save my-project
```

### Loading
When you load a workspace, termtile:
1. Minimizes or closes the previous workspace.
2. Spawns the required number of terminals using your configured templates.
3. Automatically applies the saved layout.

```bash
termtile workspace load my-project
```

## Workspace Features

### Agent Mode
Workspaces can be launched in "agent mode," which automatically creates a tmux session for every terminal window. This is required for using MCP tools or interacting with terminals via the CLI.

### Automatic Snapshots
Before loading a new workspace, termtile automatically saves your current state as a workspace named `_previous`, allowing you to undo a load operation easily.

## Limits

To prevent accidental resource exhaustion (e.g., spawning 100 terminals), you can set limits in your configuration:

```yaml
limits:
  max_terminals_per_workspace: 12
  max_workspaces: 10
  max_terminals_total: 30
```
