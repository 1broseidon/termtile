//go:build linux

package platform

import (
	"fmt"
	"sort"
	"strings"

	"github.com/1broseidon/termtile/internal/x11"
	"github.com/BurntSushi/xgb/xproto"
	"github.com/BurntSushi/xgbutil"
	"github.com/BurntSushi/xgbutil/ewmh"
	"github.com/BurntSushi/xgbutil/icccm"
)

// LinuxBackend wraps an existing X11 connection behind the platform Backend interface.
type LinuxBackend struct {
	conn *x11.Connection
}

var _ Backend = (*LinuxBackend)(nil)

// NewLinuxBackend creates a Linux platform backend from an existing X11 connection.
func NewLinuxBackend(conn *x11.Connection) *LinuxBackend {
	return &LinuxBackend{conn: conn}
}

// NewLinuxBackendFromDisplay creates a new Linux backend by opening a fresh X11 connection.
func NewLinuxBackendFromDisplay() (*LinuxBackend, error) {
	conn, err := x11.NewConnection()
	if err != nil {
		return nil, fmt.Errorf("failed to connect to X11: %w", err)
	}
	return &LinuxBackend{conn: conn}, nil
}

// Disconnect closes the underlying X11 connection.
func (b *LinuxBackend) Disconnect() {
	if b != nil && b.conn != nil {
		b.conn.Close()
	}
}

// EventLoop starts the X11 event loop (blocking).
func (b *LinuxBackend) EventLoop() {
	if b != nil && b.conn != nil {
		b.conn.EventLoop()
	}
}

// XUtil returns the underlying xgbutil connection for X11-specific operations.
func (b *LinuxBackend) XUtil() *xgbutil.XUtil {
	if b == nil || b.conn == nil {
		return nil
	}
	return b.conn.XUtil
}

// RootWindow returns the X11 root window ID.
func (b *LinuxBackend) RootWindow() xproto.Window {
	if b == nil || b.conn == nil {
		return 0
	}
	return b.conn.Root
}

// GetCurrentDesktopStandalone returns the current virtual desktop number
// using a new temporary X11 connection.
func GetCurrentDesktopStandalone() (int, error) {
	return x11.GetCurrentDesktopStandalone()
}

// Displays returns all active displays.
func (b *LinuxBackend) Displays() ([]Display, error) {
	conn, err := b.connection()
	if err != nil {
		return nil, err
	}

	monitors, err := conn.GetMonitors()
	if err != nil {
		return nil, err
	}

	displays := make([]Display, 0, len(monitors))
	for _, m := range monitors {
		d := displayFromMonitor(m)
		displays = append(displays, d)
	}

	sort.Slice(displays, func(i, j int) bool {
		return displays[i].ID < displays[j].ID
	})

	return displays, nil
}

// ActiveDisplay returns the currently active display.
func (b *LinuxBackend) ActiveDisplay() (Display, error) {
	conn, err := b.connection()
	if err != nil {
		return Display{}, err
	}

	active, err := conn.GetActiveMonitor()
	if err != nil {
		return Display{}, err
	}

	return displayFromMonitor(*active), nil
}

// ActiveWindow returns the currently active/focused window ID.
func (b *LinuxBackend) ActiveWindow() (WindowID, error) {
	conn, err := b.connection()
	if err != nil {
		return 0, err
	}

	wid, err := conn.GetActiveWindow()
	if err != nil {
		return 0, err
	}
	return WindowID(wid), nil
}

// ListWindowsOnDisplay lists normal windows whose centers are inside the display bounds.
func (b *LinuxBackend) ListWindowsOnDisplay(displayID int) ([]Window, error) {
	conn, err := b.connection()
	if err != nil {
		return nil, err
	}

	displays, err := b.Displays()
	if err != nil {
		return nil, err
	}

	var target *Display
	for i := range displays {
		if displays[i].ID == displayID {
			target = &displays[i]
			break
		}
	}
	if target == nil {
		return nil, fmt.Errorf("display with id %d not found", displayID)
	}

	clients, err := ewmh.ClientListGet(conn.XUtil)
	if err != nil {
		return nil, err
	}

	// Get current desktop for filtering.
	currentDesktop, desktopErr := ewmh.CurrentDesktopGet(conn.XUtil)
	hasCurrentDesktop := desktopErr == nil

	windows := make([]Window, 0, len(clients))
	for _, windowID := range clients {
		if !conn.IsNormalWindow(windowID) {
			continue
		}

		// Filter by current desktop.
		if hasCurrentDesktop {
			desktop, err := ewmh.WmDesktopGet(conn.XUtil, windowID)
			if err == nil && desktop != uint(0xFFFFFFFF) && desktop != currentDesktop {
				continue
			}
		}

		// Skip hidden/fullscreen windows.
		if b.shouldSkipByState(windowID) {
			continue
		}

		rect, ok := b.windowRect(windowID)
		if !ok {
			continue
		}

		if !containsPoint(target.Bounds, rect.X+rect.Width/2, rect.Y+rect.Height/2) {
			continue
		}

		pid := 0
		if p, err := ewmh.WmPidGet(conn.XUtil, windowID); err == nil {
			pid = int(p)
		}

		windows = append(windows, Window{
			ID:     WindowID(windowID),
			PID:    pid,
			AppID:  b.windowAppID(windowID),
			Title:  b.windowTitle(windowID),
			Bounds: rect,
		})
	}

	sort.Slice(windows, func(i, j int) bool {
		return windows[i].ID < windows[j].ID
	})

	return windows, nil
}

// MoveResize moves and resizes a window to the specified bounds.
func (b *LinuxBackend) MoveResize(windowID WindowID, bounds Rect) error {
	conn, err := b.connection()
	if err != nil {
		return err
	}

	return conn.MoveResizeWindow(
		xproto.Window(windowID),
		bounds.X,
		bounds.Y,
		bounds.Width,
		bounds.Height,
	)
}

// Minimize minimizes a window via WM_CHANGE_STATE.
func (b *LinuxBackend) Minimize(windowID WindowID) error {
	conn, err := b.connection()
	if err != nil {
		return err
	}

	reply, err := xproto.InternAtom(conn.XUtil.Conn(), false, uint16(len("WM_CHANGE_STATE")), "WM_CHANGE_STATE").Reply()
	if err != nil {
		return err
	}

	const iconicState = 3
	ev := xproto.ClientMessageEvent{
		Format: 32,
		Window: xproto.Window(windowID),
		Type:   reply.Atom,
		Data:   xproto.ClientMessageDataUnionData32New([]uint32{iconicState, 0, 0, 0, 0}),
	}

	return xproto.SendEvent(
		conn.XUtil.Conn(),
		false,
		conn.Root,
		xproto.EventMaskSubstructureRedirect|xproto.EventMaskSubstructureNotify,
		string(ev.Bytes()),
	).Check()
}

// Close requests graceful window close via WM_DELETE_WINDOW.
func (b *LinuxBackend) Close(windowID WindowID) error {
	conn, err := b.connection()
	if err != nil {
		return err
	}

	deleteReply, err := xproto.InternAtom(conn.XUtil.Conn(), false, uint16(len("WM_DELETE_WINDOW")), "WM_DELETE_WINDOW").Reply()
	if err != nil {
		return err
	}
	protocolsReply, err := xproto.InternAtom(conn.XUtil.Conn(), false, uint16(len("WM_PROTOCOLS")), "WM_PROTOCOLS").Reply()
	if err != nil {
		return err
	}

	ev := xproto.ClientMessageEvent{
		Format: 32,
		Window: xproto.Window(windowID),
		Type:   protocolsReply.Atom,
		Data:   xproto.ClientMessageDataUnionData32New([]uint32{uint32(deleteReply.Atom), 0, 0, 0, 0}),
	}

	return xproto.SendEventChecked(
		conn.XUtil.Conn(),
		false,
		xproto.Window(windowID),
		xproto.EventMaskNoEvent,
		string(ev.Bytes()),
	).Check()
}

func (b *LinuxBackend) shouldSkipByState(windowID xproto.Window) bool {
	states, err := ewmh.WmStateGet(b.conn.XUtil, windowID)
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

func (b *LinuxBackend) connection() (*x11.Connection, error) {
	if b == nil || b.conn == nil {
		return nil, fmt.Errorf("x11 backend connection is nil")
	}
	return b.conn, nil
}

func displayFromMonitor(m x11.Monitor) Display {
	bounds := Rect{
		X:      m.X,
		Y:      m.Y,
		Width:  m.Width,
		Height: m.Height,
	}
	return Display{
		ID:     m.ID,
		Name:   m.Name,
		Bounds: bounds,
		Usable: bounds,
	}
}

func containsPoint(r Rect, x, y int) bool {
	return x >= r.X && x < r.X+r.Width && y >= r.Y && y < r.Y+r.Height
}

func (b *LinuxBackend) windowRect(windowID xproto.Window) (Rect, bool) {
	conn := b.conn
	geom, err := xproto.GetGeometry(conn.XUtil.Conn(), xproto.Drawable(windowID)).Reply()
	if err != nil {
		return Rect{}, false
	}

	translate, err := xproto.TranslateCoordinates(
		conn.XUtil.Conn(),
		windowID,
		conn.Root,
		0, 0,
	).Reply()
	if err != nil {
		return Rect{}, false
	}

	return Rect{
		X:      int(translate.DstX),
		Y:      int(translate.DstY),
		Width:  int(geom.Width),
		Height: int(geom.Height),
	}, true
}

func (b *LinuxBackend) windowAppID(windowID xproto.Window) string {
	wmClass, err := icccm.WmClassGet(b.conn.XUtil, windowID)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(wmClass.Class)
}

func (b *LinuxBackend) windowTitle(windowID xproto.Window) string {
	title, err := ewmh.WmNameGet(b.conn.XUtil, windowID)
	if err == nil {
		title = strings.TrimSpace(title)
		if title != "" {
			return title
		}
	}

	title, err = icccm.WmNameGet(b.conn.XUtil, windowID)
	if err == nil {
		title = strings.TrimSpace(title)
		if title != "" {
			return title
		}
	}

	return ""
}
