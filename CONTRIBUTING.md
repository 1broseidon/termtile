# Contributing to termtile

Thank you for your interest in contributing to termtile. This document provides
guidelines and information to help you get started.

## Getting Started

1. Fork the repository on GitHub.
2. Clone your fork locally:
   ```bash
   git clone https://github.com/<your-username>/termtile.git
   cd termtile
   ```
3. Create a branch for your work:
   ```bash
   git checkout -b feature/your-feature-name
   ```

## Development Setup

### Requirements

- Go 1.24 or later
- Linux with X11/Xorg
- EWMH-compliant window manager
- tmux (for workspace and MCP agent features)

### Building

```bash
go build -o termtile ./cmd/termtile
```

### Running

```bash
# Run the daemon in the foreground
./termtile daemon

# Or run specific subcommands
./termtile config validate
./termtile layout list
```

### Testing

```bash
go test ./...
```

## Project Structure

```
termtile/
├── cmd/termtile/        # CLI entry point and subcommand files
├── internal/
│   ├── agent/           # tmux multiplexer for agent sessions
│   ├── config/          # YAML configuration loading and merging
│   ├── hotkeys/         # Global X11 hotkey handling
│   ├── ipc/             # IPC protocol (Unix socket)
│   ├── mcp/             # MCP server for AI agent orchestration
│   ├── movemode/        # Interactive window repositioning
│   ├── palette/         # Command palette (rofi/dmenu/wofi)
│   ├── terminals/       # Terminal detection via WM_CLASS
│   ├── tiling/          # Layout algorithms and workspace state
│   ├── tui/             # Terminal UI for layout browsing
│   └── x11/             # X11 connection, monitors, EWMH
├── configs/             # Default configuration file
└── scripts/             # Installation and service scripts
```

## How to Contribute

### Reporting Bugs

- Check existing issues to avoid duplicates.
- Include your OS, window manager, Go version, and termtile version.
- Provide steps to reproduce the issue.
- Include relevant log output (`journalctl --user -u termtile` or daemon
  stderr).

### Suggesting Features

- Open an issue describing the feature and its use case.
- Explain why existing functionality does not meet the need.

### Submitting Changes

1. Keep changes focused. One pull request per feature or bug fix.
2. Follow existing code style and conventions:
   - Each CLI subcommand gets its own file in `cmd/termtile/`.
   - Use `flag.NewFlagSet` for subcommand flags.
   - Return `int` exit codes from run functions.
   - Use `internal/` packages for non-exported logic.
3. Add or update tests where applicable.
4. Update documentation if your change affects user-facing behavior.
5. Ensure the project builds cleanly: `go build ./...`
6. Ensure tests pass: `go test ./...`

### Commit Messages

- Use clear, descriptive commit messages.
- Start with a short summary line (50 characters or less).
- Use the imperative mood ("Add feature" not "Added feature").

### Pull Requests

- Provide a clear description of what the PR does and why.
- Reference any related issues.
- Keep the diff as small as reasonably possible.

## Configuration Changes

If your change adds new configuration fields:

1. Add the field to the `Config` struct and `DefaultConfig()` in
   `internal/config/config.go`.
2. Add the field to `RawConfig` and its merge logic in
   `internal/config/raw.go`.
3. Add the field to `BuildEffectiveConfig` in
   `internal/config/effective.go`.
4. Add YAML tags (strict decoding is enabled via `KnownFields(true)`).
5. Update `configs/termtile.yaml` with a commented example.

## Code of Conduct

Be respectful and constructive in all interactions. We are building something
useful together.

## License

By contributing, you agree that your contributions will be licensed under the
MIT License.
