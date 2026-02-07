# Daemon & Architecture

The `termtile daemon` is the core of the system. It manages global state, handles window management events, and provides an IPC interface for the CLI and TUI.

## The Daemon Process

The daemon should generally be run as a systemd user service.

```bash
# Start manually
termtile daemon

# Reload configuration (sends SIGHUP)
killall -HUP termtile
```

### Key Responsibilities
- **Hotkey Listening**: Registers global X11 hotkeys.
- **Window Tracking**: Monitors which windows belong to which workspace.
- **Layout Engine**: Calculates and applies window geometries.
- **IPC Server**: Listens on a Unix socket for commands.

## IPC Protocol

The daemon exposes a JSON-based protocol over a Unix socket located at `$XDG_RUNTIME_DIR/termtile.sock`.

All CLI commands (like `termtile layout apply`) communicate with the daemon via this socket. This ensures that the daemon is always the single source of truth for the tiling state.

## State Reconciliation

termtile includes a **Reconciler** that runs periodically (every 10 seconds) to detect "state drift."

If you manually close a terminal window or if a window manager event is missed, the reconciler:
1. Compares the internal registry with actual X11 windows.
2. Removes dead slots.
3. Cleans up orphaned tmux sessions.
4. Triggers a retile if necessary to fill gaps.

## X11 Integration

### Dock & Panel Awareness
termtile reads `_NET_WM_STRUT` properties from your desktop panels (GNOME Shell, Polybar, etc.). This ensures that tiled windows never overlap your taskbar or status icons.

### Move Mode
When Move Mode is activated (default `Mod4-Mod1-m`), the daemon "grabs" the keyboard. You can use arrow keys to swap the focused terminal's position in the grid and then press `Enter` to confirm or `Esc` to cancel.

### Multi-Monitor Support
termtile is monitor-aware. It identifies monitors via XRandR and manages an independent tiling state for each one. Tiling operations only affect windows on the currently active monitor.
