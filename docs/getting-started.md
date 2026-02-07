# Getting Started

termtile is a terminal window tiling manager for Linux/X11 that automatically arranges terminal windows into grid layouts.

## Requirements

- **OS**: Linux with X11/Xorg (Wayland is supported via XWayland).
- **Window Manager**: EWMH-compliant (GNOME, KDE, i3, bspwm, Openbox, etc.).
- **Dependencies**:
  - `tmux` (required for workspace agent mode and MCP server).
  - `rofi`, `dmenu`, `wofi`, or `fuzzel` (optional, for the command palette).
- **Build**: Go 1.24+ (if building from source).

## Installation

### From Source
```bash
go install github.com/1broseidon/termtile/cmd/termtile@latest
```

### Using the Install Script
The install script builds the binary, installs the default configuration, and sets up a systemd user service:
```bash
git clone https://github.com/1broseidon/termtile.git
cd termtile
./scripts/install.sh
```

### Manual Installation
```bash
git clone https://github.com/1broseidon/termtile.git
cd termtile
go build -o termtile ./cmd/termtile
sudo install -m 755 termtile /usr/local/bin/termtile

# Create default configuration
mkdir -p ~/.config/termtile
cp configs/termtile.yaml ~/.config/termtile/config.yaml
```

## Initial Setup

### Start the Daemon
termtile runs as a background daemon that listens for global hotkeys and IPC commands.

**Manual start:**
```bash
termtile daemon
```

**Systemd service (recommended):**
```bash
systemctl --user enable --now termtile
```

### Verify Installation
Check if the daemon is running and communicating correctly:
```bash
termtile status
```

## Your First Tile

1. Open several terminal windows (e.g., 3 or 4 windows).
2. Press the default tiling hotkey: `Super+Alt+T` (`Mod4-Mod1-t`).
3. Your terminals should immediately snap into an optimized grid layout on your current monitor.

## Next Steps

- Explore [Configuration](configuration.md) to customize hotkeys and behavior.
- Learn about different [Layouts](layouts.md) and how to switch between them.
- Check out [Workspaces](workspaces.md) for managing groups of terminals.
