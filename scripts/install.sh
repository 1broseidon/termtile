#!/bin/bash
set -e

echo "=== termtile Installation Script ==="

# Check if running on Linux
if [[ "$OSTYPE" != "linux-gnu"* ]]; then
    echo "Error: This script only supports Linux"
    exit 1
fi

# Check if Go is installed
if ! command -v go &> /dev/null; then
    echo "Error: Go is not installed. Please install Go 1.21 or later."
    exit 1
fi

# Get script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"

echo "Project root: $PROJECT_ROOT"

# Build the daemon
echo ""
echo "Building termtile daemon..."
cd "$PROJECT_ROOT"
go build -o termtile ./cmd/termtile

# Install daemon binary
INSTALL_DIR="${TERMTILE_INSTALL_DIR:-"$HOME/.local/bin"}"
INSTALL_PATH="$INSTALL_DIR/termtile"
echo ""
echo "Installing daemon to $INSTALL_PATH..."
mkdir -p "$INSTALL_DIR"
install -m 755 termtile "$INSTALL_PATH"

# Create config directory
CONFIG_DIR="$HOME/.config/termtile"
echo ""
echo "Creating config directory: $CONFIG_DIR"
mkdir -p "$CONFIG_DIR"
mkdir -p "$CONFIG_DIR/config.d"

# Copy config file if it doesn't exist
if [ ! -f "$CONFIG_DIR/config.yaml" ]; then
    echo "Copying default configuration..."
    cp "$PROJECT_ROOT/configs/termtile.yaml" "$CONFIG_DIR/config.yaml"
else
    echo "Config file already exists, skipping..."
fi

# Write environment file for systemd user service (optional)
ENV_FILE="$CONFIG_DIR/env"
if [ -n "$DISPLAY" ]; then
    echo "Writing systemd environment file: $ENV_FILE"
    echo "DISPLAY=$DISPLAY" > "$ENV_FILE"
    if [ -n "$XAUTHORITY" ]; then
        echo "XAUTHORITY=$XAUTHORITY" >> "$ENV_FILE"
    fi
    chmod 600 "$ENV_FILE"
else
    echo "Note: DISPLAY is not set; not writing $ENV_FILE"
    echo "If the service fails to start, create $ENV_FILE with DISPLAY and XAUTHORITY."
fi

# Install systemd user service
SYSTEMD_USER_DIR="$HOME/.config/systemd/user"
echo ""
echo "Installing systemd user service..."
mkdir -p "$SYSTEMD_USER_DIR"

# Prefer systemd %h specifier for home-relative paths
SERVICE_BIN="$INSTALL_PATH"
if [[ "$SERVICE_BIN" == "$HOME/"* ]]; then
    SERVICE_BIN="%h/${SERVICE_BIN#$HOME/}"
fi

SERVICE_SRC="$PROJECT_ROOT/scripts/termtile.service"
SERVICE_DST="$SYSTEMD_USER_DIR/termtile.service"
sed "s|^ExecStart=.*|ExecStart=$SERVICE_BIN daemon|" "$SERVICE_SRC" > "$SERVICE_DST"

# Reload systemd
echo "Reloading systemd user daemon..."
systemctl --user daemon-reload

# Enable and start service
echo ""
if [[ -t 0 ]]; then
    read -p "Do you want to enable termtile to start on login? (y/n) " -n 1 -r
    echo
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        systemctl --user enable termtile.service
        echo "Service enabled."

        read -p "Do you want to start termtile now? (y/n) " -n 1 -r
        echo
        if [[ $REPLY =~ ^[Yy]$ ]]; then
            systemctl --user start termtile.service
            echo "Service started."
            echo ""
            echo "Check service status with: systemctl --user status termtile"
            echo "View logs with: journalctl --user -u termtile -f"
        fi
    else
        echo "Service not enabled."
        echo "You can start it manually with: systemctl --user start termtile"
    fi
else
    echo "Non-interactive shell detected; skipping enable/start prompts."
    echo "Enable on login with: systemctl --user enable termtile"
    echo "Start now with:      systemctl --user start termtile"
fi

echo ""
echo "=== Installation Complete ==="
echo ""
echo "Configuration file: $CONFIG_DIR/config.yaml"
echo "Tiling hotkey:  Super+Alt+T (Mod4-Mod1-t)"
echo "Palette hotkey: Super+Alt+G (Mod4-Mod1-g) (requires rofi/dmenu/wofi)"
echo ""
echo "Useful commands:"
echo "  Start service:   systemctl --user start termtile"
echo "  Stop service:    systemctl --user stop termtile"
echo "  Enable on boot:  systemctl --user enable termtile"
echo "  View logs:       journalctl --user -u termtile -f"
echo "  Open TUI:        $INSTALL_PATH tui"
echo "  Command palette: $INSTALL_PATH palette (requires rofi/dmenu/wofi)"
echo ""
