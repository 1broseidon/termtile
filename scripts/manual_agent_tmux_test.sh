#!/usr/bin/env bash
set -euo pipefail

cat <<'EOF'
Manual tmux agent-mode test
==========================

Prereqs
-------
- X11 session
- A terminal class supported by your config's `terminal_spawn_commands` with `{{cmd}}` present
- tmux installed
- termtile daemon running (workspace load uses IPC to apply layout)

Steps
-----
1) Build:
     go build -o termtile ./cmd/termtile

2) Ensure tmux is available:
     which tmux && tmux -V

3) Open 2+ terminal windows on your active monitor.

4) Save an agent-mode workspace snapshot (captures WM_CLASS ordering and CWD best-effort):
     ./termtile workspace save --agent-mode agent-test

5) Load it (this spawns new terminals; they should open inside tmux sessions):
     ./termtile workspace load agent-test

6) Verify tmux sessions exist:
     tmux list-sessions | rg 'termtile-agent-test-' || tmux list-sessions

7) Send a command to slot 0:
     ./termtile agent send --slot 0 'echo hello-from-slot-0'

8) Read recent output from slot 0:
     ./termtile agent read --slot 0 --lines 50 | tail -n 20

9) Wait for a pattern (send first, then wait):
     ./termtile agent send --slot 1 'echo ready && sleep 1 && echo done'
     ./termtile agent read --slot 1 --wait-for done --timeout 5 --lines 200

Notes
-----
- Slot numbering is the saved workspace SlotIndex (0..N-1).
- `termtile agent send/read` default to the last loaded agent-mode workspace; use `--workspace NAME` to override.
- If you see: "spawn template ... must include {{cmd}}", add `{{cmd}}` to that terminal's spawn template.
EOF
