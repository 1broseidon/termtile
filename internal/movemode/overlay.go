package movemode

import (
	"fmt"

	"github.com/1broseidon/termtile/internal/tiling"
	"github.com/1broseidon/termtile/internal/x11"
	"github.com/BurntSushi/xgb/xproto"
)

// Border colors
const (
	ColorSelection = 0x3498db // Blue - window selection
	ColorGrabbed   = 0x27ae60 // Green - grabbed window
	ColorTarget    = 0x7f8c8d // Gray - target slot / empty slot preview
	ColorInactive  = 0x95a5a6 // Light gray - non-selected terminals
)

// Border thickness in pixels
const BorderThickness = 4

// BorderOverlay represents a rectangular border made of 4 thin windows
type BorderOverlay struct {
	Top     xproto.Window
	Bottom  xproto.Window
	Left    xproto.Window
	Right   xproto.Window
	created bool
	mapped  bool
}

// OverlayManager manages overlay windows for move mode
type OverlayManager struct {
	conn *x11.Connection

	terminalBorders []*BorderOverlay // Borders around terminal windows (decorated rects)
	slotBorders     []*BorderOverlay // Borders around every grid slot (preview)
}

// NewOverlayManager creates a new overlay manager
func NewOverlayManager(conn *x11.Connection) *OverlayManager {
	return &OverlayManager{
		conn:            conn,
		terminalBorders: nil,
		slotBorders:     nil,
	}
}

// Render draws borders for all terminals and all grid slots.
//
// Slots are rendered first and terminals after, so terminal borders appear on top.
func (m *OverlayManager) Render(terminalRects []tiling.Rect, terminalColors []uint32, slotRects []tiling.Rect, slotColors []uint32) error {
	if len(terminalRects) != len(terminalColors) {
		return fmt.Errorf("terminal rect/color length mismatch")
	}
	if len(slotRects) != len(slotColors) {
		return fmt.Errorf("slot rect/color length mismatch")
	}

	if err := m.ensureBorders(&m.slotBorders, len(slotRects)); err != nil {
		return err
	}
	if err := m.ensureBorders(&m.terminalBorders, len(terminalRects)); err != nil {
		return err
	}

	for i := range slotRects {
		if err := m.showBorder(m.slotBorders[i], slotRects[i], slotColors[i]); err != nil {
			return err
		}
	}
	for i := range terminalRects {
		if err := m.showBorder(m.terminalBorders[i], terminalRects[i], terminalColors[i]); err != nil {
			return err
		}
	}
	return nil
}

// HideAll hides all overlays without destroying them.
func (m *OverlayManager) HideAll() {
	for _, border := range m.terminalBorders {
		m.hideBorder(border)
	}
	for _, border := range m.slotBorders {
		m.hideBorder(border)
	}
}

// Cleanup destroys all overlay windows
func (m *OverlayManager) Cleanup() {
	for _, border := range m.terminalBorders {
		m.destroyBorder(border)
	}
	for _, border := range m.slotBorders {
		m.destroyBorder(border)
	}

	m.terminalBorders = nil
	m.slotBorders = nil
}

func (m *OverlayManager) ensureBorders(borders *[]*BorderOverlay, count int) error {
	if count <= len(*borders) {
		for i := count; i < len(*borders); i++ {
			m.hideBorder((*borders)[i])
		}
		return nil
	}

	for len(*borders) < count {
		border := &BorderOverlay{}
		if err := m.createBorderWindows(border); err != nil {
			return err
		}
		*borders = append(*borders, border)
	}
	return nil
}

// showBorder creates or updates a border around the given rectangle
func (m *OverlayManager) showBorder(border *BorderOverlay, rect tiling.Rect, color uint32) error {
	// Ensure border windows exist
	if !border.created {
		if err := m.createBorderWindows(border); err != nil {
			return err
		}
	}

	// Update positions and colors
	x, y := rect.X, rect.Y
	w, h := rect.Width, rect.Height
	t := BorderThickness

	// Top bar: full width, at top
	m.updateWindow(border.Top, x, y, w, t, color)

	// Bottom bar: full width, at bottom
	m.updateWindow(border.Bottom, x, y+h-t, w, t, color)

	// Left bar: between top and bottom bars
	m.updateWindow(border.Left, x, y+t, t, h-2*t, color)

	// Right bar: between top and bottom bars
	m.updateWindow(border.Right, x+w-t, y+t, t, h-2*t, color)

	// Map all windows
	xproto.MapWindow(m.conn.XUtil.Conn(), border.Top)
	xproto.MapWindow(m.conn.XUtil.Conn(), border.Bottom)
	xproto.MapWindow(m.conn.XUtil.Conn(), border.Left)
	xproto.MapWindow(m.conn.XUtil.Conn(), border.Right)

	border.mapped = true
	return nil
}

// hideBorder unmaps the border windows (but doesn't destroy them)
func (m *OverlayManager) hideBorder(border *BorderOverlay) {
	if !border.mapped {
		return
	}

	xproto.UnmapWindow(m.conn.XUtil.Conn(), border.Top)
	xproto.UnmapWindow(m.conn.XUtil.Conn(), border.Bottom)
	xproto.UnmapWindow(m.conn.XUtil.Conn(), border.Left)
	xproto.UnmapWindow(m.conn.XUtil.Conn(), border.Right)

	border.mapped = false
}

// destroyBorder destroys the border windows
func (m *OverlayManager) destroyBorder(border *BorderOverlay) {
	if border.Top != 0 {
		xproto.DestroyWindow(m.conn.XUtil.Conn(), border.Top)
	}
	if border.Bottom != 0 {
		xproto.DestroyWindow(m.conn.XUtil.Conn(), border.Bottom)
	}
	if border.Left != 0 {
		xproto.DestroyWindow(m.conn.XUtil.Conn(), border.Left)
	}
	if border.Right != 0 {
		xproto.DestroyWindow(m.conn.XUtil.Conn(), border.Right)
	}

	border.Top = 0
	border.Bottom = 0
	border.Left = 0
	border.Right = 0
	border.created = false
	border.mapped = false
}

// createBorderWindows creates the 4 border windows
func (m *OverlayManager) createBorderWindows(border *BorderOverlay) error {
	var err error

	border.Top, err = m.createOverrideRedirectWindow()
	if err != nil {
		return err
	}

	border.Bottom, err = m.createOverrideRedirectWindow()
	if err != nil {
		return err
	}

	border.Left, err = m.createOverrideRedirectWindow()
	if err != nil {
		return err
	}

	border.Right, err = m.createOverrideRedirectWindow()
	if err != nil {
		return err
	}

	border.created = true
	return nil
}

// createOverrideRedirectWindow creates a single override-redirect window
func (m *OverlayManager) createOverrideRedirectWindow() (xproto.Window, error) {
	conn := m.conn.XUtil.Conn()
	screen := m.conn.XUtil.Screen()

	wid, err := xproto.NewWindowId(conn)
	if err != nil {
		return 0, err
	}

	// Create window with override_redirect=true
	// This makes it bypass the window manager
	err = xproto.CreateWindowChecked(
		conn,
		screen.RootDepth,
		wid,
		m.conn.Root,
		0, 0, // x, y (will be updated later)
		1, 1, // width, height (will be updated later)
		0, // border_width
		xproto.WindowClassInputOutput,
		screen.RootVisual,
		xproto.CwOverrideRedirect|xproto.CwBackPixel,
		// Value list order follows the bit positions of the mask (low → high).
		// CwBackPixel comes before CwOverrideRedirect, so it must be first.
		[]uint32{0, 1}, // back_pixel=black, override_redirect=true
	).Check()

	if err != nil {
		return 0, err
	}

	return wid, nil
}

// updateWindow moves, resizes, and recolors a window
func (m *OverlayManager) updateWindow(wid xproto.Window, x, y, width, height int, color uint32) {
	conn := m.conn.XUtil.Conn()

	// Ensure minimum dimensions
	if width < 1 {
		width = 1
	}
	if height < 1 {
		height = 1
	}

	// Configure window geometry
	xproto.ConfigureWindow(
		conn,
		wid,
		xproto.ConfigWindowX|xproto.ConfigWindowY|xproto.ConfigWindowWidth|xproto.ConfigWindowHeight|xproto.ConfigWindowStackMode,
		[]uint32{
			uint32(x),
			uint32(y),
			uint32(width),
			uint32(height),
			xproto.StackModeAbove, // Keep on top
		},
	)

	// Set background color
	xproto.ChangeWindowAttributes(
		conn,
		wid,
		xproto.CwBackPixel,
		[]uint32{color},
	)

	// Clear window to show new color
	xproto.ClearArea(conn, false, wid, 0, 0, 0, 0)
}
