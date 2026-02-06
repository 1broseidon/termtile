package x11

import (
	"github.com/BurntSushi/xgb/xproto"
	"github.com/BurntSushi/xgbutil/ewmh"
	"github.com/BurntSushi/xgbutil/xwindow"
)

// MoveResizeWindow moves and resizes a window to the specified geometry
func (c *Connection) MoveResizeWindow(windowID xproto.Window, x, y, width, height int) error {
	// First, check if window is maximized and unmaximize it
	if err := c.unmaximizeWindow(windowID); err != nil {
		// Log but don't fail - some windows might not support this
	}

	// Create xwindow wrapper
	win := xwindow.New(c.XUtil, windowID)

	// Use EWMH MoveResize for better WM compatibility
	err := ewmh.MoveresizeWindow(
		c.XUtil,
		windowID,
		x, y, width, height,
	)

	if err != nil {
		// Fallback to direct window manipulation
		win.MoveResize(x, y, width, height)
		return nil
	}

	return nil
}

// unmaximizeWindow removes maximized state from a window
func (c *Connection) unmaximizeWindow(windowID xproto.Window) error {
	// Get current window states
	states, err := ewmh.WmStateGet(c.XUtil, windowID)
	if err != nil {
		return err
	}

	// Check if window is maximized
	hasMaxH := false
	hasMaxV := false

	for _, state := range states {
		if state == "_NET_WM_STATE_MAXIMIZED_HORZ" {
			hasMaxH = true
		}
		if state == "_NET_WM_STATE_MAXIMIZED_VERT" {
			hasMaxV = true
		}
	}

	// Remove maximized states if present
	if hasMaxH || hasMaxV {
		// Request state removal
		if hasMaxH {
			ewmh.WmStateReq(c.XUtil, windowID, 0, "_NET_WM_STATE_MAXIMIZED_HORZ")
		}
		if hasMaxV {
			ewmh.WmStateReq(c.XUtil, windowID, 0, "_NET_WM_STATE_MAXIMIZED_VERT")
		}
	}

	return nil
}

// GetFrameExtents returns the window decoration sizes (if available)
func (c *Connection) GetFrameExtents(windowID xproto.Window) (left, right, top, bottom int, err error) {
	extents, err := ewmh.FrameExtentsGet(c.XUtil, windowID)
	if err != nil {
		// No frame extents available, return zeros
		return 0, 0, 0, 0, nil
	}

	return int(extents.Left), int(extents.Right), int(extents.Top), int(extents.Bottom), nil
}

// IsNormalWindow checks if a window is a normal application window
func (c *Connection) IsNormalWindow(windowID xproto.Window) bool {
	types, err := ewmh.WmWindowTypeGet(c.XUtil, windowID)
	if err != nil {
		// If we can't determine type, assume it's normal
		return true
	}

	// Check for normal window type
	for _, t := range types {
		if t == "_NET_WM_WINDOW_TYPE_NORMAL" {
			return true
		}
		// Reject desktop, dock, splash, etc.
		if t == "_NET_WM_WINDOW_TYPE_DESKTOP" ||
			t == "_NET_WM_WINDOW_TYPE_DOCK" ||
			t == "_NET_WM_WINDOW_TYPE_SPLASH" ||
			t == "_NET_WM_WINDOW_TYPE_NOTIFICATION" {
			return false
		}
	}

	// If no specific type is set, assume it's normal
	return len(types) == 0
}

func (c *Connection) GetActiveWindow() (xproto.Window, error) {
	return ewmh.ActiveWindowGet(c.XUtil)
}
