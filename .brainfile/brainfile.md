---
schema: https://brainfile.md/v1/board.json
title: termtile
agent:
  instructions:
    - Modify only the YAML frontmatter
    - Preserve all IDs
    - Keep ordering
    - Make minimal changes
columns:
  - id: todo
    title: To Do
    tasks:
      - id: task-118
        title: Set up GoReleaser for automated releases and package distribution
        description: |-
          Configure GoReleaser to automate binary builds and publishing on git tag push.

          SETUP:
          - Add .goreleaser.yaml at project root
          - GitHub Actions workflow (.github/workflows/release.yml) triggered on tag push (v*)
          - Build linux/amd64, linux/arm64, darwin/amd64, darwin/arm64
          - Generate checksums and changelog from commit messages
          - Publish to GitHub Releases with archives (tar.gz for linux/darwin)
          - Include README.md and LICENSE in archives

          OPTIONAL PACKAGE MANAGERS:
          - Homebrew tap: create 1broseidon/homebrew-tap repo formula (or add to .goreleaser.yaml brews section)
          - AUR: consider PKGBUILD for Arch users (can be manual/separate)
          - Snap/Flatpak: skip for now

          GORELEASER CONFIG NOTES:
          - Binary name: termtile
          - Main: ./cmd/termtile
          - Module: github.com/1broseidon/termtile
          - Strip debug symbols (ldflags -s -w)
          - Inject version via ldflags if applicable

          EXISTING: There is already a Makefile publish target and scripts/. Review what exists and integrate rather than duplicate.
        priority: high
        tags:
          - release
          - goreleaser
          - ci
          - packaging
  - id: in-progress
    title: In Progress
    tasks: []
  - id: done
    title: Done
    tasks:
      - id: task-88
        title: "QA all configured agents: spawn → idle detection → read → kill workflow"
        description: "Systematically test every configured agent through the full MCP lifecycle: spawn_agent → wait_for_idle → read_from_agent → kill_agent. Document which agents work end-to-end and which break (and where). Originally just Gemini idle detection, now expanded to full agent QA matrix."
        priority: high
        tags:
          - bug
          - mcp
          - idle-detection
          - gemini
        relatedFiles:
          - internal/mcp/server.go
          - internal/config/config.go
      - id: task-90
        title: "Docs site prep: refactor README + create organized docs/"
        description: "Clean up and refactor documentation in preparation for a docs website. Slim down README.md to a concise marketing-style landing page (honest, not cheesy). Move detailed documentation into organized docs/ directory as markdown files.\\n\\nWorkflow:\\n1. Claude haiku agent reviews codebase and reports all active, real features\\n2. Gemini agent uses that feature report to write the actual docs\\n\\nThe docs/ directory should cover: installation, configuration, CLI usage, TUI guide, MCP server/agent orchestration, tiling layouts, and workspace management."
        priority: medium
        tags:
          - docs
          - readme
          - refactor
        relatedFiles:
          - README.md
          - docs/
        subtasks:
          - id: task-90-1
            title: "Haiku: audit codebase and produce feature inventory report"
            completed: true
          - id: task-90-2
            title: "Gemini: write docs/ markdown files from feature report"
            completed: true
          - id: task-90-3
            title: "Gemini: rewrite README.md as concise marketing-style landing page"
            completed: true
          - id: task-90-4
            title: Review and commit final docs
            completed: true
      - id: task-91
        title: Dependency-aware agent spawning (depends_on)
        description: Add a `depends_on` parameter to the `spawn_agent` MCP tool. When specified, termtile holds the spawn until the dependency slots have gone idle. This lets any orchestrator — even a simple one — declare a multi-step agent workflow upfront without needing to manually sequence spawn calls.\n\nBehavior:\n- `depends_on` accepts a list of slot numbers (ints)\n- Before spawning, termtile polls each dependency slot using existing `checkIdle` logic\n- If all deps are idle, spawn proceeds immediately\n- If any dep is not idle, poll at interval (e.g. 2s) until all resolve or timeout\n- If a dependency slot doesn't exist or was killed, treat as an error (fail the spawn)\n- Timeout should be configurable with a sensible default (e.g. 300s)\n- When deps resolve, spawn proceeds exactly as it does today\n- The depends_on param is optional — omitting it preserves current behavior
        priority: high
        tags:
          - mcp
          - orchestration
          - pipeline
          - feature
        relatedFiles:
          - internal/mcp/server.go
          - internal/mcp/tools.go
          - internal/mcp/types.go
          - internal/mcp/spawn.go
        subtasks:
          - id: task-91-1
            title: Add depends_on and depends_on_timeout params to spawn_agent tool schema in tools.go
            completed: true
          - id: task-91-2
            title: Add SpawnAgentArgs.DependsOn ([]int) and DependsOnTimeout (int) to types.go
            completed: true
          - id: task-91-3
            title: Implement waitForDependencies(slots []int, timeout int) in server.go — polls checkIdle for each slot
            completed: true
          - id: task-91-4
            title: Wire dependency wait into handleSpawnAgent before the actual spawn logic
            completed: true
          - id: task-91-5
            title: "Handle error cases: slot not tracked, agent killed mid-wait, timeout exceeded"
            completed: true
          - id: task-91-6
            title: Add unit tests for waitForDependencies (mock checkIdle scenarios)
            completed: true
          - id: task-91-7
            title: "Integration test: spawn two agents where B depends_on A, verify B waits"
            completed: true
          - id: task-91-8
            title: Update spawn_agent tool description to document depends_on behavior
            completed: true
        contract:
          status: done
          deliverables:
            - type: file
              path: internal/mcp/types.go
              description: Add DependsOn and DependsOnTimeout fields to SpawnAgentArgs
            - type: file
              path: internal/mcp/tools.go
              description: Add depends_on params to spawn_agent schema and wire into handler
            - type: file
              path: internal/mcp/server.go
              description: Implement waitForDependencies method on Server
            - type: file
              path: internal/mcp/spawn.go
              description: Call waitForDependencies before spawn proceeds
            - type: test
              path: internal/mcp/deps_test.go
              description: Unit tests for dependency wait logic
          validation:
            commands:
              - go vet ./...
              - go test ./internal/mcp/ -run TestDep -v
              - go test ./internal/mcp/ -v
          constraints:
            - depends_on param must be optional — omitting preserves current behavior
            - Use existing checkIdle logic, do not duplicate idle detection
            - Poll interval 2s, default timeout 300s
            - Fail spawn with clear error if dependency slot not tracked or killed
            - Thread-safe — dependency wait must not hold the server mutex
      - id: task-92
        title: Artifact passing between agents
        description: "When an agent goes idle, termtile already captures its fenced output. Store this as a retrievable artifact per slot so downstream agents can consume it. This removes the need for orchestrators to wire file paths or manually read/inject outputs between pipeline steps.\\n\\nTwo mechanisms:\\n1. **Automatic injection**: When `spawn_agent` has `depends_on`, the captured output from dependency slots is available via template variables (e.g. `{{slot_1.output}}`) in the task prompt. Termtile substitutes these before sending.\\n2. **Explicit retrieval**: A new `get_artifact` MCP tool lets any orchestrator fetch a slot's captured output on demand.\\n\\nArtifact lifecycle:\\n- Created when `wait_for_idle` or `checkIdle` detects idle and captures fenced output\\n- Stored in memory per workspace/slot (not persisted to disk)\\n- Cleared when the slot is killed or reused\\n- Size cap to prevent memory issues (e.g. 1MB per artifact, truncate with warning)"
        priority: high
        tags:
          - mcp
          - orchestration
          - pipeline
          - feature
        relatedFiles:
          - internal/mcp/server.go
          - internal/mcp/tools.go
          - internal/mcp/types.go
          - internal/mcp/output.go
        subtasks:
          - id: task-92-1
            title: Add artifact store to Server struct — map[workspace]map[slot]string, mutex-protected
            completed: true
          - id: task-92-2
            title: Capture artifact in handleWaitForIdle after successful idle detection (fenced output)
            completed: true
          - id: task-92-3
            title: Also capture artifact in checkIdle path when called from dependency wait
            completed: true
          - id: task-92-4
            title: "Add get_artifact MCP tool — params: slot, workspace — returns stored output"
            completed: true
          - id: task-92-5
            title: "Implement template substitution in spawn task: replace {{slot_N.output}} with artifact content"
            completed: true
          - id: task-92-6
            title: "Handle missing artifacts: clear error if referenced slot has no output yet"
            completed: true
          - id: task-92-7
            title: "Size cap: truncate artifacts over 1MB, include warning in get_artifact response"
            completed: true
          - id: task-92-8
            title: Add unit tests for artifact store (store, retrieve, clear, size cap)
            completed: true
          - id: task-92-9
            title: Add unit tests for template substitution in task prompts
            completed: true
          - id: task-92-10
            title: Update spawn_agent and get_artifact tool descriptions in schema
            completed: true
        contract:
          status: delivered
          deliverables:
            - type: file
              path: internal/mcp/artifacts.go
              description: Artifact store implementation with thread-safe map and size cap
            - type: file
              path: internal/mcp/server.go
              description: Wire artifact capture into idle detection and dependency resolution
            - type: file
              path: internal/mcp/tools.go
              description: Add get_artifact tool schema and handler, add template substitution to spawn
            - type: file
              path: internal/mcp/types.go
              description: Add GetArtifactArgs struct
            - type: test
              path: internal/mcp/artifacts_test.go
              description: Unit tests for artifact store, retrieval, cap, and template substitution
          validation:
            commands:
              - go vet ./...
              - go test ./internal/mcp/ -run TestArtifact -v
              - go test ./internal/mcp/ -v
          constraints:
            - Artifacts stored in memory only — no disk persistence
            - 1MB size cap per artifact, truncate with warning
            - "Template syntax: {{slot_N.output}} where N is the slot number"
            - get_artifact must work independently of depends_on (explicit retrieval)
            - Clear artifacts on kill_agent to prevent stale data
            - Thread-safe — artifact reads/writes must not race with idle detection
      - id: task-93
        title: "Record termtile demo: parallel agent orchestration showcase"
        description: "Record a screen demo of termtile's agent orchestration features. User drives the initial prompt, Claude orchestrates the demo.\\n\\nSetup:\\n- Screen: 3440x1440, DISPLAY=:1\\n- Layout: master-stack-left (already configured)\\n- Record with ffmpeg x11grab, convert to 2x speed gif + keep original mp4\\n\\nDemo flow:\\n1. User says go → start ffmpeg recording (DISPLAY :1)\\n2. Spawn 3 agents in parallel: cursor-agent, gemini, codex — each with a real codebase task\\n3. Agents auto-tile into master-stack-left layout\\n4. Wait for all 3 to finish (wait_for_idle on each)\\n5. Summarize what each agent completed\\n6. Kill all agents — windows close\\n7. Stop ffmpeg recording\\n8. Convert mp4 to 2x speed gif at half resolution (1720px wide)\\n\\nAgent tasks:\\n- cursor-agent: Review internal/mcp/artifacts.go and summarize what it does\\n- gemini: Explain termtile's 3-tier idle detection from internal/mcp/server.go\\n- codex: List all MCP tools from internal/mcp/server.go with descriptions\\n\\nRecording commands:\\n```\\n# Start recording\\nffmpeg -video_size 3440x1440 -framerate 30 -f x11grab -i :1.0 -c:v libx264 -preset ultrafast -crf 18 termtile-demo.mp4\\n\\n# Convert to 2x gif\\nffmpeg -i termtile-demo.mp4 -vf \\\"setpts=0.5*PTS,fps=15,scale=1720:-1:flags=lanczos,split[s0][s1];[s0]palettegen=max_colors=128[p];[s1][p]paletteuse=dither=bayer\\\" termtile-demo.gif\\n```\\n\\nKey gotcha: DISPLAY is :1 not :0.0"
        priority: high
        tags:
          - demo
          - video
          - marketing
        relatedFiles:
          - internal/mcp/artifacts.go
          - internal/mcp/server.go
        subtasks:
          - id: task-93-1
            title: Start ffmpeg screen recording (DISPLAY :1, 3440x1440, 30fps)
            completed: true
          - id: task-93-2
            title: "Spawn 3 agents in parallel: cursor-agent, gemini, codex with codebase tasks"
            completed: true
          - id: task-93-3
            title: Wait for all agents to complete and summarize results
            completed: true
          - id: task-93-4
            title: Kill all agents
            completed: true
          - id: task-93-5
            title: Stop ffmpeg recording
            completed: true
          - id: task-93-6
            title: Convert mp4 to 2x speed gif (1720px wide)
            completed: true
      - id: task-94
        title: Fix workspace resolution bug + add move_terminal feature
        description: "When MCP server spawns terminal windows, resolveWorkspaceName() resolves based on the currently visible desktop. If the user switches desktops while agents work, new terminals spawn on the wrong desktop. Fix: after spawning, correct the window's desktop via EWMH. Also add move_terminal MCP tool and terminal move CLI command."
        priority: high
        tags:
          - bug
          - mcp
          - feature
          - x11
          - workspace
        relatedFiles:
          - internal/x11/desktop.go
          - internal/platform/backend_linux.go
          - internal/mcp/tools.go
          - internal/mcp/types.go
          - internal/mcp/server.go
          - internal/workspace/state.go
          - cmd/termtile/terminal.go
        subtasks:
          - id: task-94-1
            title: "X11: Add SetWindowDesktop + FindWindowByTitle"
            completed: true
          - id: task-94-2
            title: "Platform: Add standalone functions for window move"
            completed: true
          - id: task-94-3
            title: "Bug fix: Correct desktop in spawnWindow after spawn"
            completed: true
          - id: task-94-4
            title: "Workspace state: Add MoveTerminalBetweenWorkspaces"
            completed: true
          - id: task-94-5
            title: "MCP types + server: Add move_terminal tool"
            completed: true
          - id: task-94-6
            title: "MCP tools: Implement handleMoveTerminal handler"
            completed: true
          - id: task-94-7
            title: "CLI: Add terminal move subcommand"
            completed: true
          - id: task-94-8
            title: "Tests: workspace state + MCP server tests"
            completed: true
          - id: task-94-9
            title: "Verify: go vet + go test pass"
            completed: true
      - id: task-95
        title: "Audit CLI commands: remove dead references from help menus"
        description: "Review all termtile CLI subcommands and their help/usage text. Cross-reference the command router in main.go with actual implementations. Remove any help text, usage strings, or documentation references to commands that don't have working implementations. Check: main.go top-level commands, layout subcommands, config subcommands, terminal subcommands, workspace subcommands, palette subcommands, mcp subcommands."
        priority: medium
        tags:
          - cli
          - cleanup
        relatedFiles:
          - cmd/termtile/main.go
          - cmd/termtile/terminal.go
          - cmd/termtile/workspace.go
          - cmd/termtile/palette.go
          - cmd/termtile/mcp.go
        contract:
          status: done
          deliverables:
            - type: file
              path: cmd/termtile/main.go
              description: Remove dead command references from help and router
            - type: file
              path: cmd/termtile/terminal.go
              description: Remove dead terminal subcommand references if any
            - type: file
              path: cmd/termtile/workspace.go
              description: Remove dead workspace subcommand references if any
            - type: file
              path: cmd/termtile/palette.go
              description: Remove dead palette subcommand references if any
            - type: file
              path: cmd/termtile/mcp.go
              description: Remove dead mcp subcommand references if any
          validation:
            commands:
              - go vet ./...
              - go build ./cmd/termtile/
          constraints:
            - Only remove references to commands that truly have no implementation
            - Do not remove working commands
            - Do not add new functionality
            - Run go vet ./... after changes
      - id: task-96
        title: "Shortcut UX: add dedicated Super+Alt+N terminal-add hotkey"
        description: Implement a first-class hotkey for adding a terminal to the active workspace on the current desktop, using existing terminal-add behavior and limits. This is the fast-path complement to move mode.
        priority: high
        tags:
          - shortcuts
          - hotkeys
          - workspace
          - ux
        relatedFiles:
          - internal/config/config.go
          - internal/config/raw.go
          - internal/config/effective.go
          - internal/config/explain.go
          - cmd/termtile/main.go
          - docs/configuration.md
          - configs/termtile.yaml
        subtasks:
          - id: task-96-1
            title: Add config key and default for terminal-add hotkey (Mod4-Mod1-n)
            completed: true
          - id: task-96-2
            title: Register daemon callback for the hotkey and execute add-terminal action
            completed: true
          - id: task-96-3
            title: Ensure action behaves safely when no active workspace is present
            completed: true
          - id: task-96-4
            title: Document hotkey in docs/configuration.md and sample config
            completed: true
        contract:
          status: delivered
          deliverables:
            - type: file
              path: internal/config/config.go
              description: Add terminal_add_hotkey config field and default
            - type: file
              path: internal/config/raw.go
              description: Wire raw config overlay support for terminal_add_hotkey
            - type: file
              path: internal/config/effective.go
              description: Apply effective config merge for terminal_add_hotkey
            - type: file
              path: internal/config/explain.go
              description: Expose config explain support for terminal_add_hotkey
            - type: file
              path: cmd/termtile/main.go
              description: Register hotkey callback and run terminal add flow
            - type: file
              path: docs/configuration.md
              description: Document new hotkey in Hotkeys section
            - type: file
              path: configs/termtile.yaml
              description: Add commented sample entry for terminal_add_hotkey
          validation:
            commands:
              - go test ./internal/config/...
              - go test ./cmd/termtile/...
              - go test ./...
          constraints:
            - Default behavior must preserve existing hotkeys and not alter move mode semantics
            - If no active workspace is present, fail gracefully with logging and no daemon crash
            - Reuse existing terminal add path rather than implementing a duplicate add workflow
            - Keep hotkey registration style consistent with existing daemon hotkey registrations
        assignee: pi-shortcuts-96
      - id: task-97
        title: "Shortcut UX: extend Super+Alt+R mode with action keys (delete/add/append)"
        description: Evolve move mode into a unified terminal action mode. In selecting phase, support action keys like D (remove selected slot), N (insert terminal after selected slot), and A (append terminal). Keep Enter/arrow behavior for relocate flow intact.
        priority: high
        tags:
          - shortcuts
          - movemode
          - hotkeys
          - ux
        relatedFiles:
          - internal/movemode/state.go
          - internal/movemode/movemode.go
          - cmd/termtile/main.go
          - cmd/termtile/terminal.go
          - internal/workspace/state.go
          - internal/movemode/navigation_test.go
        subtasks:
          - id: task-97-1
            title: Add explicit action-phase state for destructive confirmation (delete confirm)
            completed: true
          - id: task-97-2
            title: Implement key dispatch for D/N/A in move-mode key handler
            completed: true
          - id: task-97-3
            title: Wire action execution to existing terminal add/remove logic
            completed: true
          - id: task-97-4
            title: Add regression tests for phase transitions and key handling
            completed: true
        contract:
          status: delivered
          deliverables:
            - type: file
              path: internal/movemode/state.go
              description: Add action/confirmation state for mode key flows
            - type: file
              path: internal/movemode/movemode.go
              description: Handle D/N/A keys and execute add/remove actions while preserving relocate flow
            - type: test
              path: internal/movemode/navigation_test.go
              description: Add tests for action keys and state transitions
          validation:
            commands:
              - go test ./internal/movemode/...
              - go test ./...
          constraints:
            - Existing relocate workflow (enter, arrows, confirm, escape) must remain backward-compatible
            - Delete must require explicit confirmation in-mode before executing removal
            - Action handlers must fail safely and keep mode stable (no panic, no stuck keyboard grab)
            - Do not duplicate terminal add/remove business logic; call existing CLI pathways from movemode
        assignee: pi-shortcuts-97
      - id: task-98
        title: "Shortcut UX: add on-screen move-mode hint overlay for available actions"
        description: Add a text hint overlay during Super+Alt+R mode so users can discover available shortcuts without memorization. Show phase-aware hints (select/move/confirm-delete) and key legend.
        priority: medium
        tags:
          - shortcuts
          - movemode
          - overlay
          - ux
        relatedFiles:
          - internal/movemode/overlay.go
          - internal/movemode/movemode.go
          - docs/daemon.md
          - docs/configuration.md
        subtasks:
          - id: task-98-1
            title: Add text-overlay primitive to overlay manager
            completed: true
          - id: task-98-2
            title: Render compact key hint legend while mode is active
            completed: true
          - id: task-98-3
            title: Show phase-specific hint text for selecting vs grabbed vs delete-confirm
            completed: true
          - id: task-98-4
            title: Document mode controls in docs
            completed: true
        contract:
          status: delivered
          deliverables:
            - type: file
              path: internal/movemode/overlay.go
              description: Implement text-hint overlay support alongside border overlays
            - type: file
              path: docs/daemon.md
              description: Document move-mode key legend and overlay behavior
            - type: file
              path: docs/configuration.md
              description: Document updated move-mode interaction model
          validation:
            commands:
              - go test ./internal/movemode/...
              - go test ./...
          constraints:
            - Overlay must not interfere with keyboard grab handling or move responsiveness
            - Hints must be visible but compact; avoid obscuring selected terminal borders
            - Fallback cleanly if text rendering cannot be initialized on a given environment
            - Do not remove or regress existing border overlays
        assignee: pi-shortcuts-98
      - id: task-101
        title: "Deterministic workspace resolver: project marker + explicit ambiguity errors"
        description: Integrate project marker workspace resolution for MCP spawn/read operations so behavior is deterministic across desktops and clients. Implement resolver chain and hard-fail ambiguity per schema v1.
        priority: high
        tags:
          - mcp
          - workspace
          - resolver
          - determinism
        assignee: pi-resolver
        relatedFiles:
          - internal/mcp/tools.go
          - internal/mcp/types.go
          - internal/mcp/workspace_resolution_test.go
          - internal/workspace/state.go
        contract:
          status: done
          deliverables:
            - type: file
              path: internal/mcp/tools.go
              description: Implement project-aware workspace resolver chain
            - type: test
              path: internal/mcp/workspace_resolution_test.go
              description: Add coverage for marker-based resolution and ambiguity handling
            - type: file
              path: internal/mcp/types.go
              description: Add optional source_workspace hint input where appropriate
          validation:
            commands:
              - go test ./internal/mcp/...
          constraints:
            - Use resolver precedence from .brainfile/plans/project-workspace-schema-v1.md
            - Explicit workspace argument must always win
            - Ambiguous omitted-workspace cases must return clear actionable errors
      - id: task-100
        title: "Project workspace schema v1: add config structs + loader for .termtile/workspace.yaml"
        description: Implement repo-local project workspace schema with merge-ready structures. Add loaders for .termtile/workspace.yaml and .termtile/local.yaml, with defaults and validation based on .brainfile/plans/project-workspace-schema-v1.md.
        priority: high
        tags:
          - project-config
          - mcp
          - workspace
          - schema
        assignee: pi-schema-core
        relatedFiles:
          - internal/config/config.go
          - internal/config/raw.go
          - internal/config/effective.go
          - internal/runtimepath/
          - docs/configuration.md
        contract:
          status: delivered
          deliverables:
            - type: file
              path: internal/config/raw.go
              description: Add raw schema types for project workspace config
            - type: file
              path: internal/config/config.go
              description: Add effective project workspace config types and defaults
            - type: file
              path: internal/config/effective.go
              description: Wire merge hooks for project-level config layer
            - type: test
              path: internal/config/config_test.go
              description: Add tests for project config parse/default/invalid cases
          validation:
            commands:
              - go test ./internal/config/...
          constraints:
            - Follow schema in .brainfile/plans/project-workspace-schema-v1.md
            - Do not break existing global config loading paths
            - Keep precedence plumbing extension-ready for resolver work
        subtasks:
          - id: task-100-1
            title: Add raw and effective project workspace schema types/defaults
            completed: true
          - id: task-100-2
            title: Implement project-aware loader merge for .termtile/workspace.yaml and .termtile/local.yaml
            completed: true
          - id: task-100-3
            title: Add tests for project parse/default/invalid cases and run go test ./internal/config/...
            completed: true
      - id: task-102
        title: "Project workflow commands: init/link/sync and docs"
        description: Add CLI entrypoints for project-local termtile workflows and document usage. Implement termtile project init/link/sync pull/push plus user docs.
        priority: medium
        tags:
          - cli
          - docs
          - workspace
          - project-config
        assignee: pi-project-cli
        relatedFiles:
          - cmd/termtile/main.go
          - cmd/termtile/workspace.go
          - docs/configuration.md
          - README.md
        contract:
          status: done
          deliverables:
            - type: file
              path: cmd/termtile/main.go
              description: Add project command group and subcommand wiring
            - type: file
              path: cmd/termtile/workspace.go
              description: Implement init/link/sync handlers
            - type: file
              path: docs/configuration.md
              description: Document .termtile/workspace.yaml and precedence
            - type: test
              path: cmd/termtile/
              description: Add/extend tests for project command parsing and behavior
          validation:
            commands:
              - go test ./cmd/termtile/...
              - go test ./...
          constraints:
            - Command behavior must align with .brainfile/plans/project-workspace-schema-v1.md
            - Do not regress existing workspace commands
            - Keep sync behavior explicit and safe (no silent destructive merge)
      - id: task-103
        title: "Release automation: interactive make publish with semver/tag push flow"
        description: Added scripts/publish.sh and Makefile publish targets. Workflow now supports interactive major/minor/patch/none selection when VERSION/BUMP not provided, explicit VERSION override, dry-run mode, tests toggle, dirty-tree guard, version file update (ServerVersion), commit, annotated tag, and remote push.
        priority: medium
        tags:
          - release
          - build
          - automation
          - cli
          - makefile
        assignee: codex
        relatedFiles:
          - Makefile
          - scripts/publish.sh
      - id: task-104
        title: "Brainfile sync guard: validate task state consistency before issue sync"
        description: Added a pre-sync validator for .brainfile/brainfile.md and wired it into GitHub Actions + sync script. Validation now fails fast on inconsistent task/contract state before any issue create/edit/close operations run.
        priority: high
        tags:
          - brainfile
          - github-actions
          - sync
          - validation
        assignee: codex
        relatedFiles:
          - .github/scripts/brainfile-validate.sh
          - .github/scripts/brainfile-sync.sh
          - .github/workflows/brainfile-sync.yml
          - Makefile
      - id: task-99
        title: "Super+Alt+R selector refactor: discover, fix, and deploy"
        description: Current Super+Alt+R selector UX is inconsistent and visually incorrect (outline sizing mismatch, overlay menu visibility issues, and phase-dependent rendering glitches). Run a focused discovery to identify root causes, then define an implementation and rollout plan that restores predictable behavior and consistent visuals.
        priority: high
        tags:
          - shortcuts
          - movemode
          - overlay
          - ux
          - refactor
        relatedFiles:
          - internal/movemode/movemode.go
          - internal/movemode/overlay.go
          - internal/movemode/state.go
          - docs/daemon.md
          - docs/configuration.md
        subtasks:
          - id: task-99-1
            title: Reproduce current selector UX issues and catalog exact failure modes
            completed: true
          - id: task-99-2
            title: Trace root causes in move-mode state, overlay rendering, and border geometry logic
            completed: true
          - id: task-99-3
            title: Propose refactor architecture and phased rollout plan
            completed: true
          - id: task-99-4
            title: Define implementation tasks/contracts for parallel delegation
            completed: true
        contract:
          status: done
          deliverables:
            - type: file
              path: internal/movemode/movemode.go
              description: Normalize move-mode render model and geometry pipeline for consistent overlays
            - type: file
              path: internal/movemode/overlay.go
              description: Stabilize hint overlay sizing/placement and phase-accurate legend content
            - type: test
              path: internal/movemode/overlay_test.go
              description: Add regression tests for hint placement/clamping and phase legend behavior
          validation:
            commands:
              - go test ./internal/movemode/...
              - go test ./...
          constraints:
            - Keep existing move/confirm key behavior backward-compatible
            - Overlay/hint refactor must not break keyboard grab lifecycle
            - Use a single geometry contract for terminal and slot preview overlays
            - Hint legend must match implemented actions (d/n/a in select phase)
        assignee: codex
      - id: task-105
        title: "Agent hooks: add output_mode config field to agent definitions"
        description: Add `output_mode` field to agent config supporting "hooks" (default), "tags", or "terminal" modes. The existing `response_fence` and `idle_pattern` fields become fallbacks for "tags" mode. This enables hook-based output detection as the primary mechanism.
        priority: high
        tags:
          - hooks
          - config
          - agent
          - feature
        relatedFiles:
          - internal/config/config.go
          - internal/config/raw.go
          - internal/config/effective.go
        contract:
          status: delivered
          deliverables:
            - type: file
              path: internal/config/config.go
              description: Add OutputMode field to AgentConfig struct with yaml tag
            - type: file
              path: internal/config/raw.go
              description: Add OutputMode to RawAgentConfig and merge logic
            - type: file
              path: internal/config/effective.go
              description: Add OutputMode to BuildEffectiveConfig with default hooks
          validation:
            commands:
              - go build ./...
              - go vet ./...
              - go test ./internal/config/...
          constraints:
            - Use string type for OutputMode (not enum) for YAML flexibility
            - Default must be 'hooks' when field is empty
            - Preserve backward compatibility - missing field = hooks mode
        assignee: codex-slot-2
      - id: task-106
        title: "Agent hooks: artifact directory management for hook output"
        description: Implement artifact directory creation and management at ~/.local/share/termtile/artifacts/{workspace}/{slot}/. Create directory on agent spawn, provide helper functions for reading artifacts, clean up on agent kill.
        priority: high
        tags:
          - hooks
          - artifacts
          - mcp
          - feature
        relatedFiles:
          - internal/mcp/artifacts.go
          - internal/mcp/spawn.go
          - internal/mcp/tools.go
        contract:
          status: done
          deliverables:
            - type: file
              path: internal/mcp/artifacts.go
              description: Artifact directory helpers (GetArtifactDir EnsureArtifactDir ReadArtifact CleanupArtifact)
            - type: file
              path: internal/mcp/spawn.go
              description: Call EnsureArtifactDir when spawning agent
            - type: file
              path: internal/mcp/tools.go
              description: Call CleanupArtifact in kill_agent handler
          validation:
            commands:
              - go build ./...
              - go vet ./...
              - go test ./internal/mcp/...
          constraints:
            - Use XDG_DATA_HOME or ~/.local/share/termtile/artifacts as base
            - Artifact file is output.json within slot directory
            - Create directory with 0755 permissions
            - Do not fail spawn if artifact dir creation fails (log warning)
        assignee: codex-slot-3
        subtasks:
          - id: task-106-1
            title: Add artifact directory helper functions in internal/mcp/artifacts.go
            completed: true
          - id: task-106-2
            title: Integrate EnsureArtifactDir into spawn flow with non-fatal warning
            completed: true
          - id: task-106-3
            title: Integrate CleanupArtifact into kill flow and run build/vet/tests
            completed: true
      - id: task-107
        title: "Agent hooks: termtile hook emit CLI subcommand"
        description: "Add `termtile hook emit` CLI subcommand that agents can call from their hooks to write structured output. Usage: `termtile hook emit --workspace NAME --slot N --output \"result...\"` or read from stdin. This is called by agent hook scripts to signal completion with output."
        priority: high
        tags:
          - hooks
          - cli
          - feature
        relatedFiles:
          - cmd/termtile/hook.go
          - cmd/termtile/main.go
        contract:
          status: done
          deliverables:
            - type: file
              path: cmd/termtile/hook.go
              description: Hook subcommand with emit action
            - type: file
              path: cmd/termtile/main.go
              description: Add hook case to command router
          validation:
            commands:
              - go build ./...
              - go vet ./...
          constraints:
            - Support --workspace and --slot and --output flags
            - Support reading output from stdin if --output not provided
            - Write JSON to artifact directory with status output timestamp fields
            - Exit 0 on success and non-zero with error message on failure
      - id: task-108
        title: "Agent hooks: modify read_from_agent to support hook-based output"
        description: Update read_from_agent MCP tool to check agent's output_mode config. When mode is "hooks", read from artifact file instead of tmux capture. Fall back to existing terminal scraping for "tags" or "terminal" modes.
        priority: high
        tags:
          - hooks
          - mcp
          - feature
        relatedFiles:
          - internal/mcp/tools.go
        contract:
          status: done
          deliverables:
            - type: file
              path: internal/mcp/tools.go
              description: Update handleReadFromAgent to check output_mode and read from artifact when hooks mode
          validation:
            commands:
              - go build ./...
              - go vet ./...
              - go test ./internal/mcp/...
          constraints:
            - Load agent config to check output_mode field
            - If output_mode is hooks then read artifact file and return parsed output
            - If artifact file missing or empty return appropriate message
            - If output_mode is tags or terminal use existing tmux capture logic
            - Preserve all existing parameters like lines pattern since_last clean
      - id: task-109
        title: "Agent hooks: auto-inject hook config for Claude Code on spawn"
        description: "When spawning Claude Code with output_mode: hooks, automatically inject TERMTILE_WORKSPACE and TERMTILE_SLOT env vars, generate a Stop hook config, and pass via --settings flag. The hook should call `termtile hook emit` with the workspace/slot from env vars."
        priority: high
        tags:
          - hooks
          - mcp
          - claude-code
          - spawn
        relatedFiles:
          - internal/mcp/tools.go
          - internal/mcp/hooks.go
        contract:
          status: ready
          deliverables:
            - type: file
              path: internal/mcp/hooks.go
              description: Generate Claude Code hook settings JSON for Stop event
            - type: file
              path: internal/mcp/tools.go
              description: Inject TERMTILE env vars and --settings flag when output_mode is hooks
          validation:
            commands:
              - go build ./...
              - go vet ./...
          constraints:
            - Only inject for agents where output_mode is hooks
            - Use --settings with inline JSON or temp file
            - Hook must call termtile hook emit with workspace and slot from env
            - Clean up temp settings file if created
      - id: task-113
        title: Remove in-memory ArtifactStore, use disk-only artifacts
        description: "Remove the in-memory ArtifactStore from server.go and artifacts.go. get_artifact should read output.json directly from disk (GetArtifactDir + output.json). All agents now write to the same file path — hook agents via termtile hook emit, hookless agents write directly. Remove ArtifactStore struct, NewArtifactStore, Set/Get/Clear methods. Keep the disk-based helpers: GetArtifactDir, EnsureArtifactDir, ReadArtifact, CleanupArtifact, CleanStaleOutput, artifactFilePath, artifactBaseDir. Update get_artifact tool handler to read from disk. Remove ArtifactStore from Server struct and initialization."
        priority: high
        tags:
          - mcp
          - refactor
        relatedFiles:
          - internal/mcp/artifacts.go
          - internal/mcp/artifacts_test.go
          - internal/mcp/server.go
          - internal/mcp/tools.go
        contract:
          status: done
          deliverables:
            - type: file
              path: internal/mcp/artifacts.go
              description: Remove ArtifactStore in-memory code, keep disk helpers
            - type: file
              path: internal/mcp/server.go
              description: Remove ArtifactStore from Server struct
            - type: file
              path: internal/mcp/tools.go
              description: Update get_artifact handler to read from disk
          validation:
            commands:
              - go vet ./...
              - go test ./...
          constraints:
            - Keep all disk-based artifact helpers (GetArtifactDir, EnsureArtifactDir, ReadArtifact, etc)
            - Keep substituteSlotOutputTemplates but have it read from disk via ReadArtifact instead of store.Get
            - Update or remove artifacts_test.go to match
            - Run go vet ./... and go test ./... before delivering
        subtasks:
          - id: task-113-1
            title: Remove ArtifactStore types and template substitution in artifacts.go
            completed: true
          - id: task-113-2
            title: Remove server-side in-memory artifact hooks and storage calls
            completed: true
          - id: task-113-3
            title: Switch get_artifact and slot transfer logic to disk artifacts
            completed: true
          - id: task-113-4
            title: Update artifacts tests and run vet/test validations
            completed: true
      - id: task-111
        title: Simplify wait_for_idle to poll for output.json
        description: Replace tmux idle pattern matching in wait_for_idle with file polling for output.json in the artifact dir. The file appearing signals completion. Keep timeout parameter. Remove idle_pattern logic from this tool handler. The tool should poll the artifact file path (GetArtifactDir + output.json) on an interval (e.g. 2s) until it exists or timeout.
        priority: high
        tags:
          - mcp
          - refactor
        relatedFiles:
          - internal/mcp/tools.go
          - internal/mcp/artifacts.go
        contract:
          status: done
          deliverables:
            - type: file
              path: internal/mcp/tools.go
              description: Rewrite wait_for_idle tool handler to poll for output.json file
          validation:
            commands:
              - go vet ./...
              - go test ./...
          constraints:
            - Do not remove idle_pattern from config - it may still be useful for other purposes
            - "Keep the timeout parameter and return is_idle: false on timeout"
            - Return the parsed output.json content in the output field on success
            - Run go vet ./... and go test ./... before delivering
      - id: task-112
        title: Simplify read_from_agent to raw tmux tail
        description: Strip read_from_agent down to a raw tmux capture-pane tail (50-100 lines, configurable via lines param). Remove artifact reading, fence parsing, and any structured output extraction. This tool becomes pure live monitoring — what's on screen right now. Remove the response_fence / fence-related parsing logic from the tool handler. Keep the clean parameter for stripping TUI chrome.
        priority: high
        tags:
          - mcp
          - refactor
        relatedFiles:
          - internal/mcp/tools.go
        contract:
          status: done
          deliverables:
            - type: file
              path: internal/mcp/tools.go
              description: Rewrite read_from_agent to simple tmux capture-pane tail
          validation:
            commands:
              - go vet ./...
              - go test ./...
          constraints:
            - Keep lines parameter (default 50, max 100)
            - Keep clean parameter for TUI chrome stripping
            - Remove fence parsing and artifact reading from this handler
            - Do not touch spawn or other tool handlers
            - Run go vet ./... and go test ./... before delivering
      - id: task-114
        title: "Docs rebuild pass 1: rewrite docs to match current hook-based architecture"
        description: |-
          First pass by codex. Read all source files to understand the current code reality, then rewrite docs/ to accurately reflect it.

          KEY CHANGES TO DOCUMENT:
          1. **Hook system is the primary output mechanism** — output_mode: hooks is the default. Agents get hooks injected at spawn (BeforeAgent for task context, AfterAgent for output capture). All agents write output.json to a disk artifact directory.
          2. **Three hook delivery types**: cli_flag (Claude — hooks JSON passed via --hooks flag), project_file (Gemini — hooks written to .gemini/settings.json), none (codex/cursor-agent — no native hooks, file-write instructions appended to task instead)
          3. **Artifact system is disk-based** — no more in-memory ArtifactStore. Artifacts live at ~/.local/share/termtile/artifacts/{workspace}/{slot}/output.json
          4. **wait_for_idle polls output.json** — no more tiered fence/pattern/process detection for wait_for_idle. It simply polls for output.json file appearance with status: complete.
          5. **read_from_agent is pure tmux tail** — bounded capture-pane tail for live monitoring only. No artifact reading, no fence parsing.
          6. **checkIdle still uses the old tiers** for list_agents is_idle field and depends_on polling (fence → pattern → process). Document this distinction.
          7. **New agent config fields**: output_mode, hook_delivery, hook_entry, hook_wrapper, hook_events, hook_settings_flag, hook_settings_dir, hook_settings_file, hook_output, hook_response_field, prompt_as_arg, prompt_flag, pipe_task, default_model, model_flag, models, ready_pattern, env, description
          8. **move_terminal tool** — moves terminals between workspaces (X11 window move + tmux session rename + registry update)
          9. **agent_meta.json** written at spawn so hook CLI can look up agent-specific config
          10. **fileWriteInstructions** — hookless agents get instructions appended to write output.json manually

          SOURCE FILES TO READ:
          - internal/mcp/tools.go (handleSpawnAgent, handleWaitForIdle, handleReadFromAgent, handleGetArtifact, handleKillAgent, handleMoveTerminal, checkIdle, fileWriteInstructions)
          - internal/mcp/hooks.go (resolveHooks, renderHookSettings, fileWriteInstructions)
          - internal/mcp/hook_files.go (injectProjectFileHooks, restoreProjectFileHooks, writeAgentMeta)
          - internal/mcp/artifacts.go (disk artifact helpers)
          - internal/mcp/server.go (Server struct, NewServer, registerTools)
          - internal/mcp/spawn.go (spawnAgentWithDependencies)
          - internal/mcp/types.go (all input/output types)
          - internal/config/config.go (AgentConfig struct with all fields)
          - cmd/termtile/hook.go (hook CLI subcommands)
          - cmd/termtile/main.go (CLI router)

          DOCS TO UPDATE:
          - docs/agent-orchestration.md — MAJOR rewrite needed. Replace fence-centric content with hook-based architecture.
          - docs/configuration.md — Add all new agent config fields, document hook schema, update agent example configs.
          - docs/cli.md — Add hook subcommands (termtile hook start/check/emit), mcp cleanup.

          STYLE: Keep docs concise, use tables for config fields, include realistic YAML examples showing claude + gemini + codex configs.
        priority: high
        tags:
          - docs
          - hooks
          - mcp
          - rewrite
        contract:
          status: delivered
          deliverables:
            - type: file
              path: docs/agent-orchestration.md
              description: Rewritten to document hook-based architecture, output.json pipeline, all tools including move_terminal
            - type: file
              path: docs/configuration.md
              description: Updated with all new agent config fields and hook schema
            - type: file
              path: docs/cli.md
              description: Updated with hook subcommands
          validation:
            commands:
              - grep -q 'output.json' docs/agent-orchestration.md
              - grep -q 'hook_delivery' docs/configuration.md
              - grep -q 'hook' docs/cli.md
          constraints:
            - Read source files before writing — do not guess at behavior
            - Keep existing doc structure where still accurate, rewrite sections that are wrong
            - Do not remove docs for features that still exist (fence detection still used by checkIdle for list_agents)
            - Use realistic config examples matching actual ~/.config/termtile/config.yaml
        subtasks:
          - id: task-114-1
            title: Read all listed MCP/config/CLI source files and capture behavior deltas for docs
            completed: true
          - id: task-114-2
            title: Rewrite docs/agent-orchestration.md for hook-first architecture and tool behavior
            completed: true
          - id: task-114-3
            title: Rewrite docs/configuration.md with full agent config + hook schema and examples
            completed: true
          - id: task-114-4
            title: Rewrite docs/cli.md with hook subcommands and current MCP CLI notes
            completed: true
          - id: task-114-5
            title: Run validation commands and write completion JSON artifact output
            completed: true
      - id: task-116
        title: Build VitePress docs site with landing page
        description: |-
          Set up a VitePress documentation site for termtile. The existing docs/*.md files become the content pages. Add a polished landing page with hero section and feature grid.

          SETUP:
          - Initialize VitePress in the docs/ directory (docs/.vitepress/)
          - package.json at project root with scripts: docs:dev, docs:build, docs:preview
          - Use default VitePress theme (no custom Vue components needed)
          - Configure for GitHub Pages deployment (base: '/termtile/')

          LANDING PAGE (docs/index.md):
          - Hero section: name "termtile", tagline "AI agent orchestration meets tiling window management", description of what it does
          - Hero image: reference /demo.gif (we'll add the actual file later, use a placeholder path)
          - Two action buttons: "Get Started" → /getting-started, "View on GitHub" → https://github.com/1broseidon/termtile
          - Features grid (3-4 cards):
            1. MCP Agent Orchestration — Spawn, monitor, and coordinate AI agents across terminal windows via Model Context Protocol
            2. Hook-Based Output Capture — Unified output.json pipeline works across Claude, Gemini, Codex and any future agent
            3. Tiling Window Manager — Automatic tiling layouts for terminal windows with hotkeys, workspaces, and live retiling
            4. Pipeline Dependencies — Chain agents with depends_on, pass artifacts between stages with template substitution

          SIDEBAR NAVIGATION:
          - Getting Started
          - Configuration
          - Layouts
          - Workspaces
          - AI Agent Orchestration
          - CLI Reference
          - TUI
          - Daemon

          VITEPRESS CONFIG (docs/.vitepress/config.ts):
          - title: termtile
          - description: Tiling window manager with AI agent orchestration
          - themeConfig: nav bar with GitHub link, sidebar matching above structure
          - Dark mode default
          - head: favicon (can be placeholder)

          FILES TO CREATE:
          - docs/.vitepress/config.ts
          - docs/index.md (landing page with hero + features)
          - package.json (at project root, vitepress as devDependency)
          - .gitignore addition: docs/.vitepress/cache, docs/.vitepress/dist

          DO NOT modify existing docs/*.md content files — those are being rewritten by another agent concurrently.
        priority: high
        tags:
          - docs
          - vitepress
          - landing-page
          - site
        contract:
          status: delivered
          deliverables:
            - type: file
              path: docs/.vitepress/config.ts
              description: VitePress config with nav, sidebar, theme
            - type: file
              path: docs/index.md
              description: Landing page with hero and features grid
            - type: file
              path: package.json
              description: Project root package.json with vitepress devDep and scripts
          validation:
            commands:
              - test -f docs/.vitepress/config.ts
              - test -f docs/index.md
              - test -f package.json
          constraints:
            - Do NOT modify any existing docs/*.md files
            - Use default VitePress theme only, no custom Vue components
            - Keep config minimal and clean
      - id: task-115
        title: "Docs rebuild pass 2: technical accuracy review by Claude agent"
        description: |-
          Second pass. Read every doc file that pass 1 updated, then cross-reference against actual source code to verify:
          - Every config field documented actually exists in config.go AgentConfig struct
          - Every CLI command documented matches cmd/termtile/main.go router
          - Every MCP tool description matches the actual handler behavior in tools.go
          - Code examples compile/parse correctly (YAML syntax, JSON syntax)
          - No stale references to removed features (ArtifactStore, in-memory artifacts, fence-only idle detection for wait_for_idle)
          - Flow descriptions match actual code paths (e.g. spawn → hook injection → task send sequence)

          Fix any inaccuracies found. Add missing details where source code reveals behavior not documented.
        priority: high
        tags:
          - docs
          - review
          - accuracy
        contract:
          status: delivered
          deliverables:
            - type: file
              path: docs/agent-orchestration.md
              description: Technically verified against source code
            - type: file
              path: docs/configuration.md
              description: All config fields verified against config.go
            - type: file
              path: docs/cli.md
              description: All commands verified against main.go
          validation:
            commands:
              - go vet ./...
          constraints:
            - Cross-reference every claim against source code
            - Do not invent features that don't exist in the code
            - Preserve pass 1 structure, only fix inaccuracies and add missing details
      - id: task-117
        title: Align termtile docs site structure and styling with otto docs pattern
        description: |-
          Make the termtile VitePress docs site follow the same structural pattern as ~/Projects/core/otto/docs while keeping termtile's own identity.

          STRUCTURAL CHANGES:
          1. Move package.json INTO docs/ directory (like otto). Delete the root package.json. Scripts become `npm run dev`, `npm run build`, `npm run preview` (no docs: prefix)
          2. Rename config.ts → config.mts (match otto)
          3. Add custom theme: docs/.vitepress/theme/index.ts importing custom.css (same pattern as otto)
          4. Add srcExclude for any non-doc directories if needed

          STYLING (custom.css):
          - Create a termtile-specific color theme — NOT teal like otto. Use a cool blue/indigo palette that evokes terminals/code:
            - Brand primary: indigo/electric blue (#6366f1 / #818cf8 range)
            - Dark mode variants
            - Hero name gradient (indigo → slate)
            - Hero image glow effect (like otto has)
            - Button hover states
            - Clean minimal hero text styling

          CONFIG (config.mts):
          - Add logo to themeConfig (use placeholder path /termtile-logo.png for now)
          - Add footer: message "Tiling window manager with AI agent orchestration"
          - Add nav items: Guide, Config, Agents, GitHub
          - Keep existing sidebar structure
          - Add head entries for favicon (placeholder paths like otto)
          - Remove the base: '/termtile/' for now (can add back for GH Pages deploy later)

          LANDING PAGE (index.md):
          - Add a "Quick Start" section below the features (like otto has) with install instructions:
            go install github.com/1broseidon/termtile/cmd/termtile@latest
          - Keep existing hero and features, just ensure style matches otto's clean look

          REFERENCE: ~/Projects/core/otto/docs/ for exact file structure and patterns. Match the structure, NOT the content/branding.

          DO NOT modify any existing docs content files (agent-orchestration.md, configuration.md, etc) — only VitePress scaffolding files.
        priority: high
        tags:
          - docs
          - vitepress
          - styling
          - site
        contract:
          status: delivered
          deliverables:
            - type: file
              path: docs/package.json
              description: Moved into docs/ with simple scripts
            - type: file
              path: docs/.vitepress/config.mts
              description: Renamed with logo, footer, nav, favicon heads
            - type: file
              path: docs/.vitepress/theme/index.ts
              description: Custom theme loader
            - type: file
              path: docs/.vitepress/theme/custom.css
              description: Indigo/blue brand colors with hero glow
            - type: file
              path: docs/index.md
              description: Updated with quick start section
          validation:
            commands:
              - test -f docs/package.json
              - test -f docs/.vitepress/config.mts
              - test -f docs/.vitepress/theme/custom.css
              - test ! -f package.json
          constraints:
            - Follow ~/Projects/core/otto/docs/ structure exactly
            - Do NOT use teal/green colors — use indigo/blue for termtile identity
            - Do NOT modify existing docs content files
            - Delete root package.json after creating docs/package.json
  - id: backlog
    title: Backlog
    tasks:
      - id: task-87
        title: "Daemon auto-sync phase 2: X11 DestroyNotify events + sync config options"
        description: "Remaining work from task-69 (daemon auto-sync). Core reconciler is working (phases 1-4,6-8 done). These are the nice-to-have improvements: (1) X11 window monitor for DestroyNotify events - reactive cleanup instead of polling, (2) sync configuration options in config.yaml - tune intervals, enable/disable features."
        priority: low
        tags:
          - feature
          - daemon
          - tmux
          - sync
          - phase-2
        subtasks:
          - id: task-87-1
            title: "Phase 2: Add Window Monitor for X11 DestroyNotify events"
            completed: false
          - id: task-87-2
            title: "Phase 5: Add sync configuration options to config.yaml"
            completed: false
      - id: task-51
        title: GNU Screen multiplexer implementation
        description: Implement ScreenMultiplexer that satisfies the Multiplexer interface. Include screenrc template with equivalent scroll UX to tmux config. This extends agent mode to work with GNU Screen users.
        priority: low
        tags:
          - agent-mode
          - multiplexer
          - backlog
          - screen
        relatedFiles:
          - internal/agent/screen.go
          - internal/agent/templates/screenrc.tmpl
      - id: task-89
        title: "Bubbletea TUI: unit tests + README docs"
        description: Add unit tests for the Bubbletea TUI and document usage in the README. The TUI was rewritten in the Bubbletea redesign (35fbf75) and currently has no test coverage.
        priority: medium
        tags:
          - tui
          - tests
          - docs
          - bubbletea
        relatedFiles:
          - internal/tui/preview.go
          - internal/tui/tui.go
          - internal/tui/app.go
          - internal/tui/tabs.go
          - README.md
        subtasks:
          - id: task-89-1
            title: Add unit tests for preview renderer (pure layout functions)
            completed: false
          - id: task-89-2
            title: Add tests for config reload/error display paths
            completed: false
          - id: task-89-3
            title: Write README section for termtile tui usage + keybinds
            completed: false
      - id: task-110
        title: Research hook support for codex, gemini, and cecli agents
        description: "Investigate native hook/lifecycle event support in codex CLI, gemini CLI, and cecli (aider-ce, docs at cecli.dev). Map each agent's native events to termtile's 3 abstract hook points (on_start, on_check, on_end). Document: what events exist, what stdin/stdout format they use, how to inject settings, and what the fallback strategy is for unsupported hooks."
        priority: medium
        tags:
          - hooks
          - research
          - agents
---
