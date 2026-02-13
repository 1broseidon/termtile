---
layout: home

hero:
  name: termtile
  text: AI agent orchestration meets tiling window management
  tagline: Spawn, tile, and coordinate AI agents across terminal windows — all from a single config file.
  image:
    src: /termtile-logo.png
    alt: termtile
  actions:
    - theme: brand
      text: Get Started
      link: /getting-started
    - theme: alt
      text: View on GitHub
      link: https://github.com/1broseidon/termtile

features:
  - icon: "\U0001F916"
    title: MCP Agent Orchestration
    details: Spawn, monitor, and coordinate AI agents across terminal windows via Model Context Protocol.
  - icon: "\U0001F3A3"
    title: Hook-Based Output Capture
    details: Unified output.json pipeline works across Claude, Gemini, Codex and any future agent.
  - icon: "\U0001F5BC\uFE0F"
    title: Tiling Window Manager
    details: Automatic tiling layouts for terminal windows with hotkeys, workspaces, and live retiling.
  - icon: "\U0001F517"
    title: Pipeline Dependencies
    details: Chain agents with depends_on, pass artifacts between stages with template substitution.
---

## Quick Start

```bash
# Install termtile
go install github.com/1broseidon/termtile/cmd/termtile@latest

# Start the daemon
termtile daemon start

# Tile your terminal windows
termtile tile --layout columns
```
