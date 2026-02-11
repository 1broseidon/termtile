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
      - id: task-75
        title: "macOS backend: Accessibility API + CoreGraphics window management"
        description: Implement internal/platform/backend_darwin.go behind //go:build darwin. Use CGWindowListCopyWindowInfo for window enumeration, AXUIElement for move/resize/minimize, CGDisplay for monitor enumeration, NSScreen.visibleFrame for usable work area. Requires cgo with -framework ApplicationServices -framework CoreGraphics.
        priority: high
        tags:
          - macos
          - platform
          - darwin
          - phase-2
        subtasks:
          - id: task-75-1
            title: "Create cgo bridge: AXUIElement wrappers for window position/size get/set"
            completed: false
          - id: task-75-2
            title: Implement CGWindowListCopyWindowInfo for window enumeration (PID, bounds, title, onscreen)
            completed: false
          - id: task-75-3
            title: Implement CGDisplay + NSScreen for monitor enumeration with usable area (excludes Dock/menu bar)
            completed: false
          - id: task-75-4
            title: Implement MoveResize via AXUIElementSetAttributeValue (kAXPositionAttribute, kAXSizeAttribute)
            completed: false
          - id: task-75-5
            title: Add terminal detection via bundle ID matching (com.googlecode.iterm2, com.apple.Terminal, etc)
            completed: false
          - id: task-75-6
            title: Handle Accessibility permission check and user-facing error message
            completed: false
      - id: task-76
        title: "macOS hotkeys: Carbon HotKey API or golang.design/x/hotkey"
        description: Port global hotkey support to macOS. Replace X11 XGrabKey with Carbon RegisterEventHotKey via cgo, or use golang.design/x/hotkey cross-platform library. Must handle main thread requirement on macOS. Replace X11 event loop with CFRunLoop for the daemon on darwin.
        priority: medium
        tags:
          - macos
          - hotkeys
          - darwin
          - phase-2
      - id: task-77
        title: "macOS terminal config: bundle ID matching and terminal_apps config"
        description: Add terminal_apps config field for macOS bundle IDs alongside existing terminal_classes for Linux. Update terminal detector to use bundle IDs on darwin (com.googlecode.iterm2, com.github.wez.wezterm, com.apple.Terminal, dev.warp.Warp-Stable). Add to config.go, raw.go, effective.go with yaml tags.
        priority: medium
        tags:
          - macos
          - config
          - terminal
          - phase-2
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
            completed: false
          - id: task-99-2
            title: Trace root causes in move-mode state, overlay rendering, and border geometry logic
            completed: false
          - id: task-99-3
            title: Propose refactor architecture and phased rollout plan
            completed: false
          - id: task-99-4
            title: Define implementation tasks/contracts for parallel delegation
            completed: false
        contract:
          status: ready
          deliverables:
            - type: file
              path: .brainfile/plans/super-alt-r-selector-refactor-discovery.md
              description: Discovery report with reproducible issues, root causes, and refactor blueprint
            - type: file
              path: .brainfile/plans/super-alt-r-selector-refactor-discovery.md
              description: Task breakdown for follow-on implementation/deploy contracts
          validation:
            commands:
              - test -f .brainfile/plans/super-alt-r-selector-refactor-discovery.md
          constraints:
            - "Discovery-first pass: do not modify production code in this task"
            - Report must include exact file/function touch points for each root cause
            - Include rollout/testing strategy to validate visual parity and interaction correctness
            - Output summary must be consumable for immediate parallel task delegation
        assignee: ""
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
---
