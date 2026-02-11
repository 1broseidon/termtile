package movemode

import (
	"fmt"

	"github.com/1broseidon/termtile/internal/tiling"
	"github.com/BurntSushi/xgb/xproto"
	"github.com/BurntSushi/xgbutil"
)

// Border colors
const (
	ColorSelection = 0x3498db // Blue - window selection
	ColorGrabbed   = 0x27ae60 // Green - grabbed window
	ColorTarget    = 0x7f8c8d // Gray - target slot / empty slot preview
	ColorInactive  = 0x95a5a6 // Light gray - non-selected terminals
	ColorHintText  = 0xf5f7fa // Light text for hint overlay
	ColorHintBg    = 0x1f2933 // Dark hint background
)

// Border thickness in pixels
const BorderThickness = 4

const (
	hintMargin     = 12
	hintPaddingX   = 10
	hintPaddingY   = 8
	hintLineHeight = 16
	hintCharWidth  = 7
	hintMinWidth   = 220
)

// HintPhase controls which key legend is shown in the on-screen hint overlay.
type HintPhase int

const (
	HintPhaseNone HintPhase = iota
	HintPhaseSelecting
	HintPhaseMove
	HintPhaseConfirmDelete
)

// hintOverlay is a compact single-window text panel for move-mode key hints.
type hintOverlay struct {
	Window   xproto.Window
	GC       xproto.Gcontext
	Font     xproto.Font
	created  bool
	mapped   bool
	disabled bool
}

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
	xu   *xgbutil.XUtil
	root xproto.Window

	terminalBorders []*BorderOverlay // Borders around terminal windows (decorated rects)
	slotBorders     []*BorderOverlay // Borders around every grid slot (preview)
	hint            *hintOverlay     // Text legend for move-mode shortcuts
}

// NewOverlayManager creates a new overlay manager
func NewOverlayManager(xu *xgbutil.XUtil, root xproto.Window) *OverlayManager {
	return &OverlayManager{
		xu:              xu,
		root:            root,
		terminalBorders: nil,
		slotBorders:     nil,
		hint:            &hintOverlay{},
	}
}

// Render draws borders for all terminals and all grid slots.
//
// Slots are rendered first and terminals after, so terminal borders appear on top.
func (m *OverlayManager) Render(terminalRects []tiling.Rect, terminalColors []uint32, slotRects []tiling.Rect, slotColors []uint32, allSlotRects []tiling.Rect, hintPhase HintPhase) error {
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

	m.renderHint(hintPhase, allSlotRects, terminalRects)
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
	m.hideHint()
}

// Cleanup destroys all overlay windows
func (m *OverlayManager) Cleanup() {
	for _, border := range m.terminalBorders {
		m.destroyBorder(border)
	}
	for _, border := range m.slotBorders {
		m.destroyBorder(border)
	}
	m.destroyHint()

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
	xproto.MapWindow(m.xu.Conn(), border.Top)
	xproto.MapWindow(m.xu.Conn(), border.Bottom)
	xproto.MapWindow(m.xu.Conn(), border.Left)
	xproto.MapWindow(m.xu.Conn(), border.Right)

	border.mapped = true
	return nil
}

// hideBorder unmaps the border windows (but doesn't destroy them)
func (m *OverlayManager) hideBorder(border *BorderOverlay) {
	if !border.mapped {
		return
	}

	xproto.UnmapWindow(m.xu.Conn(), border.Top)
	xproto.UnmapWindow(m.xu.Conn(), border.Bottom)
	xproto.UnmapWindow(m.xu.Conn(), border.Left)
	xproto.UnmapWindow(m.xu.Conn(), border.Right)

	border.mapped = false
}

// destroyBorder destroys the border windows
func (m *OverlayManager) destroyBorder(border *BorderOverlay) {
	if border.Top != 0 {
		xproto.DestroyWindow(m.xu.Conn(), border.Top)
	}
	if border.Bottom != 0 {
		xproto.DestroyWindow(m.xu.Conn(), border.Bottom)
	}
	if border.Left != 0 {
		xproto.DestroyWindow(m.xu.Conn(), border.Left)
	}
	if border.Right != 0 {
		xproto.DestroyWindow(m.xu.Conn(), border.Right)
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
	conn := m.xu.Conn()
	screen := m.xu.Screen()

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
		m.root,
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
	conn := m.xu.Conn()

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

func (m *OverlayManager) renderHint(phase HintPhase, allSlotRects []tiling.Rect, avoidRects []tiling.Rect) {
	lines := hintLinesForPhase(phase)
	if len(lines) == 0 {
		m.hideHint()
		return
	}

	if !m.ensureHintResources() {
		m.hideHint()
		return
	}

	hint := m.hint
	conn := m.xu.Conn()

	width, height := hintDimensions(lines)
	bounds := m.resolveHintBounds(allSlotRects)
	if bounds.Width <= 0 || bounds.Height <= 0 {
		m.hideHint()
		return
	}

	maxWidth := bounds.Width - 2*hintMargin
	if maxWidth < 1 {
		maxWidth = bounds.Width
	}
	if maxWidth < 1 {
		m.hideHint()
		return
	}
	if width > maxWidth {
		width = maxWidth
	}
	if width < 1 {
		width = 1
	}

	maxHeight := bounds.Height - 2*hintMargin
	if maxHeight < 1 {
		maxHeight = bounds.Height
	}
	if maxHeight < 1 {
		m.hideHint()
		return
	}
	if height > maxHeight {
		height = maxHeight
	}
	if height < 1 {
		height = 1
	}

	x, y := chooseHintPosition(bounds, avoidRects, width, height)

	xproto.ConfigureWindow(
		conn,
		hint.Window,
		xproto.ConfigWindowX|xproto.ConfigWindowY|xproto.ConfigWindowWidth|xproto.ConfigWindowHeight|xproto.ConfigWindowStackMode,
		[]uint32{
			uint32(x),
			uint32(y),
			uint32(width),
			uint32(height),
			xproto.StackModeAbove,
		},
	)
	xproto.ChangeWindowAttributes(conn, hint.Window, xproto.CwBackPixel, []uint32{ColorHintBg})
	xproto.ChangeGC(
		conn,
		hint.GC,
		xproto.GcForeground|xproto.GcBackground,
		[]uint32{ColorHintText, ColorHintBg},
	)
	xproto.ClearArea(conn, false, hint.Window, 0, 0, 0, 0)

	baseline := hintPaddingY + hintLineHeight - 4
	for i, line := range lines {
		if line == "" {
			continue
		}
		if len(line) > 255 {
			line = line[:255]
		}
		lineY := baseline + i*hintLineHeight
		xproto.ImageText8(
			conn,
			byte(len(line)),
			xproto.Drawable(hint.Window),
			hint.GC,
			int16(hintPaddingX),
			int16(lineY),
			line,
		)
	}

	xproto.MapWindow(conn, hint.Window)
	hint.mapped = true
}

func (m *OverlayManager) ensureHintResources() bool {
	if m.hint == nil {
		m.hint = &hintOverlay{}
	}

	if m.hint.disabled {
		return false
	}
	if m.hint.created {
		return true
	}
	if m.xu == nil {
		m.disableHint()
		return false
	}

	conn := m.xu.Conn()

	hintWindow, err := m.createOverrideRedirectWindow()
	if err != nil {
		m.disableHint()
		return false
	}

	font, err := xproto.NewFontId(conn)
	if err != nil {
		xproto.DestroyWindow(conn, hintWindow)
		m.disableHint()
		return false
	}

	fontNames := []string{"fixed", "9x15", "8x13", "6x13"}
	opened := false
	for _, fontName := range fontNames {
		err = xproto.OpenFontChecked(conn, font, uint16(len(fontName)), fontName).Check()
		if err == nil {
			opened = true
			break
		}
	}
	if !opened {
		xproto.DestroyWindow(conn, hintWindow)
		m.disableHint()
		return false
	}

	gc, err := xproto.NewGcontextId(conn)
	if err != nil {
		xproto.CloseFont(conn, font)
		xproto.DestroyWindow(conn, hintWindow)
		m.disableHint()
		return false
	}

	err = xproto.CreateGCChecked(
		conn,
		gc,
		xproto.Drawable(hintWindow),
		xproto.GcForeground|xproto.GcBackground|xproto.GcFont|xproto.GcGraphicsExposures,
		[]uint32{
			ColorHintText, // foreground
			ColorHintBg,   // background
			uint32(font),  // font
			0,             // graphics_exposures=false
		},
	).Check()
	if err != nil {
		xproto.FreeGC(conn, gc)
		xproto.CloseFont(conn, font)
		xproto.DestroyWindow(conn, hintWindow)
		m.disableHint()
		return false
	}

	m.hint.Window = hintWindow
	m.hint.GC = gc
	m.hint.Font = font
	m.hint.created = true
	return true
}

func (m *OverlayManager) disableHint() {
	m.destroyHint()
	if m.hint == nil {
		m.hint = &hintOverlay{}
	}
	m.hint.disabled = true
}

func (m *OverlayManager) hideHint() {
	if m.hint == nil || !m.hint.mapped || m.xu == nil {
		return
	}
	xproto.UnmapWindow(m.xu.Conn(), m.hint.Window)
	m.hint.mapped = false
}

func (m *OverlayManager) destroyHint() {
	if m.hint == nil || m.xu == nil {
		return
	}

	if m.hint.GC != 0 {
		xproto.FreeGC(m.xu.Conn(), m.hint.GC)
	}
	if m.hint.Font != 0 {
		xproto.CloseFont(m.xu.Conn(), m.hint.Font)
	}
	if m.hint.Window != 0 {
		xproto.DestroyWindow(m.xu.Conn(), m.hint.Window)
	}

	m.hint.Window = 0
	m.hint.GC = 0
	m.hint.Font = 0
	m.hint.created = false
	m.hint.mapped = false
}

func hintLinesForPhase(phase HintPhase) []string {
	switch phase {
	case HintPhaseSelecting:
		return []string{
			"Move Mode: select terminal",
			"Arrows  cycle terminals",
			"Enter   grab selected",
			"d       delete selected",
			"n       add after selected",
			"a       append terminal",
			"Esc     cancel",
		}
	case HintPhaseMove:
		return []string{
			"Move Mode: choose target slot",
			"Arrows  select target slot",
			"Enter   move or swap",
			"Esc     cancel",
		}
	case HintPhaseConfirmDelete:
		return []string{
			"Move Mode: confirm delete",
			"Enter   delete terminal",
			"Esc     keep terminal",
		}
	default:
		return nil
	}
}

func hintDimensions(lines []string) (width, height int) {
	maxChars := 0
	for _, line := range lines {
		if len(line) > maxChars {
			maxChars = len(line)
		}
	}
	width = maxChars*hintCharWidth + 2*hintPaddingX
	if width < hintMinWidth {
		width = hintMinWidth
	}
	height = len(lines)*hintLineHeight + 2*hintPaddingY
	return width, height
}

func (m *OverlayManager) resolveHintBounds(allSlotRects []tiling.Rect) tiling.Rect {
	if bounds, ok := unionRect(allSlotRects); ok {
		return bounds
	}
	if m.xu == nil || m.xu.Screen() == nil {
		return tiling.Rect{X: 0, Y: 0, Width: 800, Height: 600}
	}
	screen := m.xu.Screen()
	return tiling.Rect{
		X:      0,
		Y:      0,
		Width:  int(screen.WidthInPixels),
		Height: int(screen.HeightInPixels),
	}
}

func chooseHintPosition(bounds tiling.Rect, avoidRects []tiling.Rect, width, height int) (int, int) {
	if width < 1 {
		width = 1
	}
	if height < 1 {
		height = 1
	}

	left := bounds.X + hintMargin
	right := bounds.X + bounds.Width - hintMargin - width
	top := bounds.Y + hintMargin
	bottom := bounds.Y + bounds.Height - hintMargin - height

	if right < left {
		right = left
	}
	if bottom < top {
		bottom = top
	}

	candidates := []tiling.Rect{
		{X: right, Y: top, Width: width, Height: height},
		{X: left, Y: top, Width: width, Height: height},
		{X: right, Y: bottom, Width: width, Height: height},
		{X: left, Y: bottom, Width: width, Height: height},
	}

	for _, candidate := range candidates {
		obscuresSelection := false
		for _, avoid := range avoidRects {
			if rectsIntersect(candidate, avoid) {
				obscuresSelection = true
				break
			}
		}
		if !obscuresSelection {
			return clampHintOrigin(candidate.X, candidate.Y, bounds, width, height)
		}
	}

	// Fallback if all corners overlap the selected/grabbed terminal.
	return clampHintOrigin(candidates[0].X, candidates[0].Y, bounds, width, height)
}

func clampHintOrigin(x, y int, bounds tiling.Rect, width, height int) (int, int) {
	left := bounds.X + hintMargin
	right := bounds.X + bounds.Width - hintMargin - width
	if right < left {
		left = bounds.X
		right = bounds.X + bounds.Width - width
	}
	if right < left {
		right = left
	}

	top := bounds.Y + hintMargin
	bottom := bounds.Y + bounds.Height - hintMargin - height
	if bottom < top {
		top = bounds.Y
		bottom = bounds.Y + bounds.Height - height
	}
	if bottom < top {
		bottom = top
	}

	if x < left {
		x = left
	}
	if x > right {
		x = right
	}
	if y < top {
		y = top
	}
	if y > bottom {
		y = bottom
	}

	return x, y
}

func unionRect(rects []tiling.Rect) (tiling.Rect, bool) {
	if len(rects) == 0 {
		return tiling.Rect{}, false
	}

	minX := rects[0].X
	minY := rects[0].Y
	maxX := rects[0].X + rects[0].Width
	maxY := rects[0].Y + rects[0].Height

	for _, rect := range rects[1:] {
		if rect.X < minX {
			minX = rect.X
		}
		if rect.Y < minY {
			minY = rect.Y
		}
		if rect.X+rect.Width > maxX {
			maxX = rect.X + rect.Width
		}
		if rect.Y+rect.Height > maxY {
			maxY = rect.Y + rect.Height
		}
	}

	return tiling.Rect{
		X:      minX,
		Y:      minY,
		Width:  maxX - minX,
		Height: maxY - minY,
	}, true
}

func rectsIntersect(a, b tiling.Rect) bool {
	return a.X < b.X+b.Width &&
		a.X+a.Width > b.X &&
		a.Y < b.Y+b.Height &&
		a.Y+a.Height > b.Y
}
