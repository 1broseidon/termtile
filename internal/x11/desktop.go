package x11

import (
	"fmt"
	"strings"

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

// SetWindowDesktop moves a window to the specified virtual desktop.
// Sends a _NET_WM_DESKTOP client message to the root window per EWMH spec.
// We build the message manually because the xgbutil ewmh.WmDesktopReq
// helper panics on this library version (uint vs int type assertion).
func (c *Connection) SetWindowDesktop(windowID uint32, desktop int) error {
	atomReply, err := xproto.InternAtom(c.XUtil.Conn(), false,
		uint16(len("_NET_WM_DESKTOP")), "_NET_WM_DESKTOP").Reply()
	if err != nil {
		return fmt.Errorf("failed to intern _NET_WM_DESKTOP: %w", err)
	}

	const sourceIndication = 2 // pager/direct action
	ev := xproto.ClientMessageEvent{
		Format: 32,
		Window: xproto.Window(windowID),
		Type:   atomReply.Atom,
		Data:   xproto.ClientMessageDataUnionData32New([]uint32{uint32(desktop), sourceIndication, 0, 0, 0}),
	}

	return xproto.SendEventChecked(
		c.XUtil.Conn(),
		false,
		c.Root,
		xproto.EventMaskSubstructureRedirect|xproto.EventMaskSubstructureNotify,
		string(ev.Bytes()),
	).Check()
}

// FocusWindow activates and raises a window using _NET_ACTIVE_WINDOW.
// Sends a client message to the root window per EWMH spec.
// We build the message manually (same as SetWindowDesktop) because the
// xgbutil ewmh helpers panic on this library version.
func (c *Connection) FocusWindow(windowID uint32) error {
	atomReply, err := xproto.InternAtom(c.XUtil.Conn(), false,
		uint16(len("_NET_ACTIVE_WINDOW")), "_NET_ACTIVE_WINDOW").Reply()
	if err != nil {
		return fmt.Errorf("failed to intern _NET_ACTIVE_WINDOW: %w", err)
	}

	const sourceIndication = 2 // pager/direct action
	ev := xproto.ClientMessageEvent{
		Format: 32,
		Window: xproto.Window(windowID),
		Type:   atomReply.Atom,
		Data:   xproto.ClientMessageDataUnionData32New([]uint32{sourceIndication, 0, 0, 0, 0}),
	}

	return xproto.SendEventChecked(
		c.XUtil.Conn(),
		false,
		c.Root,
		xproto.EventMaskSubstructureRedirect|xproto.EventMaskSubstructureNotify,
		string(ev.Bytes()),
	).Check()
}

// FocusWindowStandalone activates and raises a window using a new temporary
// X11 connection.
func FocusWindowStandalone(windowID uint32) error {
	conn, err := NewConnection()
	if err != nil {
		return fmt.Errorf("failed to connect to X11: %w", err)
	}
	defer conn.Close()

	return conn.FocusWindow(windowID)
}

// FindWindowByTitle searches the EWMH client list for a window whose
// _NET_WM_NAME contains the given substring. Returns the first match.
func (c *Connection) FindWindowByTitle(substring string) (uint32, error) {
	clients, err := ewmh.ClientListGet(c.XUtil)
	if err != nil {
		return 0, fmt.Errorf("failed to get client list: %w", err)
	}
	for _, win := range clients {
		name, err := ewmh.WmNameGet(c.XUtil, win)
		if err != nil {
			continue
		}
		if containsSubstring(name, substring) {
			return uint32(win), nil
		}
	}
	return 0, fmt.Errorf("no window found with title containing %q", substring)
}

// containsSubstring checks if s contains substr (case-sensitive).
func containsSubstring(s, substr string) bool {
	return len(substr) > 0 && len(s) >= len(substr) && strings.Contains(s, substr)
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

// SetWindowDesktopStandalone moves a window to the specified virtual desktop
// using a new temporary X11 connection.
func SetWindowDesktopStandalone(windowID uint32, desktop int) error {
	conn, err := NewConnection()
	if err != nil {
		return fmt.Errorf("failed to connect to X11: %w", err)
	}
	defer conn.Close()

	return conn.SetWindowDesktop(windowID, desktop)
}

// FindWindowByTitleStandalone searches for a window by title substring
// using a new temporary X11 connection.
func FindWindowByTitleStandalone(substring string) (uint32, error) {
	conn, err := NewConnection()
	if err != nil {
		return 0, fmt.Errorf("failed to connect to X11: %w", err)
	}
	defer conn.Close()

	return conn.FindWindowByTitle(substring)
}
