package terminals

import (
	"strings"
	"sync"

	"github.com/1broseidon/termtile/internal/x11"
	"github.com/BurntSushi/xgb/xproto"
	"github.com/BurntSushi/xgbutil/ewmh"
	"github.com/BurntSushi/xgbutil/icccm"
)

// TerminalWindow represents a detected terminal window
type TerminalWindow struct {
	WindowID xproto.Window
	Class    string
	X        int
	Y        int
	Width    int
	Height   int
}

// Detector identifies terminal windows on the display
type Detector struct {
	mu              sync.RWMutex
	terminalClasses map[string]bool
}

// NewDetector creates a new terminal detector with the given terminal class list
func NewDetector(terminalClasses []string) *Detector {
	classMap := make(map[string]bool)
	for _, class := range terminalClasses {
		// Store both original and lowercase for case-insensitive matching
		classMap[class] = true
		classMap[strings.ToLower(class)] = true
	}

	return &Detector{
		terminalClasses: classMap,
	}
}

// UpdateTerminalClasses updates the terminal classes for detection
func (d *Detector) UpdateTerminalClasses(terminalClasses []string) {
	classMap := make(map[string]bool)
	for _, class := range terminalClasses {
		classMap[class] = true
		classMap[strings.ToLower(class)] = true
	}

	d.mu.Lock()
	defer d.mu.Unlock()
	d.terminalClasses = classMap
}

// FindTerminals finds all terminal windows on the specified monitor
func (d *Detector) FindTerminals(conn *x11.Connection, monitor *x11.Monitor) ([]TerminalWindow, error) {
	// Get all client windows
	clients, err := ewmh.ClientListGet(conn.XUtil)
	if err != nil {
		return nil, err
	}

	// Only include windows on the current desktop (plus sticky windows).
	currentDesktop, err := ewmh.CurrentDesktopGet(conn.XUtil)
	hasCurrentDesktop := err == nil

	var terminals []TerminalWindow

	for _, windowID := range clients {
		// Skip docks/desktops/notifications, etc.
		if !conn.IsNormalWindow(windowID) {
			continue
		}

		// Filter to the current desktop/workspace.
		if hasCurrentDesktop && !isOnCurrentDesktop(conn, windowID, currentDesktop) {
			continue
		}

		// Skip minimized/hidden/fullscreen windows.
		if shouldSkipByState(conn, windowID) {
			continue
		}

		// Get WM_CLASS property
		wmClass, err := icccm.WmClassGet(conn.XUtil, windowID)
		if err != nil {
			continue
		}

		// Check if this is a terminal
		if !d.isTerminalClass(wmClass.Class) {
			continue
		}

		winX, winY, winW, winH, ok := getWindowRect(conn, windowID)
		if !ok {
			continue
		}

		// Check if window is on this monitor
		winCenterX := winX + winW/2
		winCenterY := winY + winH/2
		if winCenterX < monitor.X || winCenterX >= monitor.X+monitor.Width ||
			winCenterY < monitor.Y || winCenterY >= monitor.Y+monitor.Height {
			continue
		}

		terminals = append(terminals, TerminalWindow{
			WindowID: windowID,
			Class:    wmClass.Class,
			X:        winX,
			Y:        winY,
			Width:    winW,
			Height:   winH,
		})
	}

	return terminals, nil
}

// isTerminalClass checks if the given WM_CLASS matches a known terminal
func (d *Detector) isTerminalClass(class string) bool {
	d.mu.RLock()
	defer d.mu.RUnlock()

	// Check exact match first
	if d.terminalClasses[class] {
		return true
	}
	// Check lowercase match
	return d.terminalClasses[strings.ToLower(class)]
}

func getWindowRect(conn *x11.Connection, windowID xproto.Window) (x, y, width, height int, ok bool) {
	geom, err := xproto.GetGeometry(conn.XUtil.Conn(), xproto.Drawable(windowID)).Reply()
	if err != nil {
		return 0, 0, 0, 0, false
	}

	translate, err := xproto.TranslateCoordinates(
		conn.XUtil.Conn(),
		windowID,
		conn.Root,
		0, 0,
	).Reply()
	if err != nil {
		return 0, 0, 0, 0, false
	}

	return int(translate.DstX), int(translate.DstY), int(geom.Width), int(geom.Height), true
}

// isMinimized checks if a window is minimized/hidden
func isOnCurrentDesktop(conn *x11.Connection, windowID xproto.Window, currentDesktop uint) bool {
	desktop, err := ewmh.WmDesktopGet(conn.XUtil, windowID)
	if err != nil {
		// If we can't determine, be permissive.
		return true
	}

	// 0xFFFFFFFF means "sticky" (visible on all desktops).
	if desktop == uint(0xFFFFFFFF) {
		return true
	}

	return desktop == currentDesktop
}

func shouldSkipByState(conn *x11.Connection, windowID xproto.Window) bool {
	states, err := ewmh.WmStateGet(conn.XUtil, windowID)
	if err != nil {
		return false
	}

	for _, state := range states {
		switch state {
		case "_NET_WM_STATE_HIDDEN", "_NET_WM_STATE_FULLSCREEN":
			return true
		}
	}
	return false
}
