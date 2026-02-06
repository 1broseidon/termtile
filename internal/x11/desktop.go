package x11

import (
	"fmt"

	"github.com/BurntSushi/xgb/xproto"
	"github.com/BurntSushi/xgbutil/ewmh"
)

// GetCurrentDesktop returns the current virtual desktop number (0-indexed).
// Uses _NET_CURRENT_DESKTOP atom. Returns 0 with an error if detection fails.
func (c *Connection) GetCurrentDesktop() (int, error) {
	desktop, err := ewmh.CurrentDesktopGet(c.XUtil)
	if err != nil {
		return 0, fmt.Errorf("failed to get current desktop: %w", err)
	}
	return int(desktop), nil
}

// GetWindowDesktop returns the desktop number a window is on.
// Uses _NET_WM_DESKTOP atom. Returns -1 for "sticky" windows (visible on all desktops).
// Returns 0 with an error if detection fails.
func (c *Connection) GetWindowDesktop(windowID uint32) (int, error) {
	desktop, err := ewmh.WmDesktopGet(c.XUtil, xproto.Window(windowID))
	if err != nil {
		return 0, fmt.Errorf("failed to get window desktop: %w", err)
	}
	// 0xFFFFFFFF means the window is on all desktops (sticky)
	if desktop == 0xFFFFFFFF {
		return -1, nil
	}
	return int(desktop), nil
}

// GetDesktopCount returns the number of virtual desktops.
func (c *Connection) GetDesktopCount() (int, error) {
	count, err := ewmh.NumberOfDesktopsGet(c.XUtil)
	if err != nil {
		return 0, fmt.Errorf("failed to get desktop count: %w", err)
	}
	return int(count), nil
}

// GetCurrentDesktopStandalone returns the current virtual desktop number
// using a new X11 connection. This is useful when you don't have an existing
// connection available.
func GetCurrentDesktopStandalone() (int, error) {
	conn, err := NewConnection()
	if err != nil {
		return 0, fmt.Errorf("failed to connect to X11: %w", err)
	}
	defer conn.Close()

	return conn.GetCurrentDesktop()
}
