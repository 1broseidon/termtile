package movemode

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/1broseidon/termtile/internal/config"
	"github.com/1broseidon/termtile/internal/platform"
	"github.com/1broseidon/termtile/internal/terminals"
	"github.com/1broseidon/termtile/internal/tiling"
	"github.com/BurntSushi/xgb/xproto"
	"github.com/BurntSushi/xgbutil"
	"github.com/BurntSushi/xgbutil/keybind"
	"github.com/BurntSushi/xgbutil/xevent"
)

// Default timeout for move mode (in seconds)
const DefaultTimeout = 10

const (
	keysymUp      = 0xff52
	keysymDown    = 0xff54
	keysymLeft    = 0xff51
	keysymRight   = 0xff53
	keysymReturn  = 0xff0d
	keysymEscape  = 0xff1b
	keysymKPEnter = 0xff8d
	keysymA       = 0x0041
	keysyma       = 0x0061
	keysymD       = 0x0044
	keysymd       = 0x0064
	keysymN       = 0x004e
	keysymn       = 0x006e
)

// LayoutProvider supplies the currently active layout name.
type LayoutProvider interface {
	GetActiveLayoutName() string
}

// x11Accessor is an optional interface for backends that expose X11 internals.
type x11Accessor interface {
	XUtil() *xgbutil.XUtil
	RootWindow() xproto.Window
}

// MoveResult contains information about a completed move operation.
type MoveResult struct {
	// SourceSlot is the original slot index of the moved window.
	SourceSlot int
	// TargetSlot is the new slot index of the moved window.
	TargetSlot int
	// IsSwap indicates whether two windows were swapped.
	IsSwap bool
}

type overlayHighlight struct {
	Rect  tiling.Rect
	Color uint32
}

type overlayRenderModel struct {
	TerminalHighlights []overlayHighlight
	SlotHighlights     []overlayHighlight
	AllSlotRects       []tiling.Rect
	HintPhase          HintPhase
}

// OnMoveCompleteFunc is called after a move operation completes.
// It receives the move result for post-processing (e.g., renaming tmux sessions).
type OnMoveCompleteFunc func(result MoveResult)

// TerminalActionRunner executes existing terminal CLI subcommands.
type TerminalActionRunner func(args []string) error

// Mode is the main move mode controller
type Mode struct {
	mu              sync.Mutex
	backend         platform.Backend
	xu              *xgbutil.XUtil
	root            xproto.Window
	detector        *terminals.Detector
	config          *config.Config
	layoutProvider  LayoutProvider
	state           *State
	overlay         *OverlayManager
	timeout         *time.Timer
	timeoutDuration time.Duration

	grabWindow         xproto.Window
	keyHandlerAttached bool

	// OnMoveComplete is called after a successful move/swap operation.
	OnMoveComplete OnMoveCompleteFunc
	actionRunner   TerminalActionRunner
}

// NewMode creates a new move mode controller
func NewMode(backend platform.Backend, detector *terminals.Detector, cfg *config.Config, layoutProvider LayoutProvider) *Mode {
	timeout := DefaultTimeout
	if cfg.MoveModeTimeout > 0 {
		timeout = cfg.MoveModeTimeout
	}

	// Extract X11 internals via type assertion
	var xu *xgbutil.XUtil
	var root xproto.Window
	if accessor, ok := backend.(x11Accessor); ok {
		xu = accessor.XUtil()
		root = accessor.RootWindow()
	}

	return &Mode{
		backend:         backend,
		xu:              xu,
		root:            root,
		detector:        detector,
		config:          cfg,
		layoutProvider:  layoutProvider,
		state:           NewState(),
		overlay:         NewOverlayManager(xu, root),
		timeoutDuration: time.Duration(timeout) * time.Second,
		actionRunner:    runTerminalActionViaCLI,
	}
}

// IsActive returns true if move mode is currently active
func (m *Mode) IsActive() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.state.Phase != PhaseInactive
}

// Enter activates move mode, starting in the selecting phase
func (m *Mode) Enter() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.state.Phase != PhaseInactive {
		// Already active, do nothing
		return nil
	}

	log.Println("Entering move mode")

	// Get current monitor
	display, err := m.backend.ActiveDisplay()
	if err != nil {
		log.Printf("Move mode: failed to get active monitor: %v", err)
		return err
	}

	// Get the active layout (defaults to config default; can be overridden by provider).
	layoutName := m.config.DefaultLayout
	if m.layoutProvider != nil {
		if active := m.layoutProvider.GetActiveLayoutName(); active != "" {
			layoutName = active
		}
	}
	layout, err := m.config.GetLayout(layoutName)
	if err != nil {
		log.Printf("Move mode: failed to get layout: %v", err)
		return err
	}

	// Apply screen padding (match tiler behavior).
	padding := m.config.ScreenPadding
	bounds := display.Bounds
	bounds.X += padding.Left
	bounds.Y += padding.Top
	bounds.Width -= (padding.Left + padding.Right)
	bounds.Height -= (padding.Top + padding.Bottom)
	if bounds.Width < 1 || bounds.Height < 1 {
		return fmt.Errorf(
			"screen_padding leaves no usable space: %dx%d at %d,%d",
			bounds.Width, bounds.Height, bounds.X, bounds.Y,
		)
	}

	// Find terminals on the current monitor (after padding).
	terminalWindows, err := m.detector.FindTerminals(m.backend, display.ID, bounds)
	if err != nil {
		log.Printf("Move mode: failed to find terminals: %v", err)
		return err
	}

	if len(terminalWindows) == 0 {
		log.Println("Move mode: no terminals found")
		return nil
	}

	sortTerminals(m.backend, terminalWindows, m.config.TerminalSort)

	// Apply tile region
	monitorRect := tiling.Rect{X: bounds.X, Y: bounds.Y, Width: bounds.Width, Height: bounds.Height}
	adjMonitor := tiling.ApplyRegion(monitorRect, layout.TileRegion)

	// Calculate grid dimensions
	rows, cols := m.calculateGridDimensions(len(terminalWindows), layout)
	capacity := rows * cols
	if capacity < 1 {
		log.Printf("Move mode: invalid grid size rows=%d cols=%d", rows, cols)
		return fmt.Errorf("invalid grid size rows=%d cols=%d", rows, cols)
	}

	// Respect fixed layouts that have less capacity than the number of terminals.
	if len(terminalWindows) > capacity {
		log.Printf("Move mode: %d terminals exceeds layout capacity %d; only first %d will be movable", len(terminalWindows), capacity, capacity)
		terminalWindows = terminalWindows[:capacity]
	}

	// Calculate slot positions using actual terminal count.
	positions, err := tiling.CalculatePositionsWithLayout(
		len(terminalWindows),
		adjMonitor,
		layout,
		m.config.GapSize,
	)
	if err != nil {
		log.Printf("Move mode: failed to calculate positions: %v", err)
		return err
	}

	assignedSlots := assignTerminalsToSlots(terminalWindows, positions)

	// Build terminal slots (slotIdx maps each terminal to a distinct grid slot).
	termSlots := make([]TerminalSlot, 0, len(terminalWindows))
	for i, tw := range terminalWindows {
		slotIdx := assignedSlots[i]
		if slotIdx < 0 || slotIdx >= len(positions) {
			continue
		}
		termSlots = append(termSlots, TerminalSlot{
			Window:   tw,
			SlotIdx:  slotIdx,
			SlotRect: positions[slotIdx],
		})
	}
	if len(termSlots) == 0 {
		log.Println("Move mode: no terminals could be mapped to layout slots")
		return nil
	}

	// Initialize state
	m.state.Phase = PhaseSelecting
	m.state.Terminals = termSlots
	m.state.SlotPositions = positions
	m.state.GridRows = rows
	m.state.GridCols = cols
	m.state.SelectedIndex = 0
	m.state.GrabbedWindow = 0
	m.state.TargetSlotIndex = 0
	m.state.ClearPendingAction()

	// Find the active window and select it if it's a terminal
	activeWin, _ := m.backend.ActiveWindow()
	for i, ts := range termSlots {
		if ts.Window.WindowID == activeWin {
			m.state.SelectedIndex = i
			break
		}
	}

	// Grab keyboard for navigation
	if err := m.grabKeyboard(); err != nil {
		log.Printf("Move mode: failed to grab keyboard: %v", err)
		m.state.Reset()
		return err
	}

	// Show selection border
	m.updateOverlays()

	// Start timeout
	m.startTimeout()

	log.Printf("Move mode: entered selecting phase with %d terminals", len(termSlots))
	return nil
}

// Exit deactivates move mode
func (m *Mode) Exit() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.exitLocked()
}

func (m *Mode) exitLocked() {
	if m.state.Phase == PhaseInactive {
		return
	}

	log.Println("Exiting move mode")

	// Stop timeout
	if m.timeout != nil {
		m.timeout.Stop()
		m.timeout = nil
	}

	// Release keyboard grab
	m.ungrabKeyboard()

	// Hide all overlays
	m.overlay.HideAll()

	// Reset state
	m.state.Reset()
}

// HandleArrowKey processes an arrow key press
func (m *Mode) HandleArrowKey(dir Direction) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.state.Phase == PhaseInactive {
		return
	}

	m.handleArrowKeyLocked(dir)
}

// HandleConfirm processes the Enter key press
func (m *Mode) HandleConfirm() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.state.Phase == PhaseInactive {
		return
	}

	m.handleConfirmLocked()
}

// HandleCancel processes the Escape key press
func (m *Mode) HandleCancel() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.state.Phase == PhaseInactive {
		return
	}

	m.handleCancelLocked()
}

// executeMove moves the grabbed window to the target slot
func (m *Mode) executeMove() {
	if m.state.GrabbedWindow == 0 {
		return
	}

	targetRect := m.state.TargetSlotRect()
	if targetRect == nil {
		return
	}

	// Find if there's a window currently at the target slot
	targetTermIdx := FindTerminalAtSlot(m.state.TargetSlotIndex, m.state)
	grabbedTermIdx := -1
	for i, t := range m.state.Terminals {
		if t.Window.WindowID == m.state.GrabbedWindow {
			grabbedTermIdx = i
			break
		}
	}

	if grabbedTermIdx < 0 {
		return
	}

	grabbedTerm := &m.state.Terminals[grabbedTermIdx]
	sourceRect := &grabbedTerm.SlotRect

	// Apply terminal margins
	margins := m.config.GetMargins(grabbedTerm.Window.Class)
	adjustedTarget := tiling.Rect{
		X:      targetRect.X + margins.Left,
		Y:      targetRect.Y + margins.Top,
		Width:  targetRect.Width - margins.Left - margins.Right,
		Height: targetRect.Height - margins.Top - margins.Bottom,
	}

	// Move grabbed window to target slot
	log.Printf("Move mode: moving window %d to slot %d (%d,%d %dx%d)",
		m.state.GrabbedWindow, m.state.TargetSlotIndex,
		adjustedTarget.X, adjustedTarget.Y, adjustedTarget.Width, adjustedTarget.Height)

	err := m.backend.MoveResize(
		m.state.GrabbedWindow,
		platform.Rect{X: adjustedTarget.X, Y: adjustedTarget.Y, Width: adjustedTarget.Width, Height: adjustedTarget.Height},
	)
	if err != nil {
		log.Printf("Move mode: failed to move window: %v", err)
	}

	// If there was a window at the target, swap it to the source position
	if targetTermIdx >= 0 && targetTermIdx != grabbedTermIdx {
		otherTerm := &m.state.Terminals[targetTermIdx]
		otherMargins := m.config.GetMargins(otherTerm.Window.Class)
		adjustedSource := tiling.Rect{
			X:      sourceRect.X + otherMargins.Left,
			Y:      sourceRect.Y + otherMargins.Top,
			Width:  sourceRect.Width - otherMargins.Left - otherMargins.Right,
			Height: sourceRect.Height - otherMargins.Top - otherMargins.Bottom,
		}

		log.Printf("Move mode: swapping window %d to slot %d (%d,%d %dx%d)",
			otherTerm.Window.WindowID, grabbedTerm.SlotIdx,
			adjustedSource.X, adjustedSource.Y, adjustedSource.Width, adjustedSource.Height)

		err := m.backend.MoveResize(
			otherTerm.Window.WindowID,
			platform.Rect{X: adjustedSource.X, Y: adjustedSource.Y, Width: adjustedSource.Width, Height: adjustedSource.Height},
		)
		if err != nil {
			log.Printf("Move mode: failed to swap window: %v", err)
		}
	}

	// Notify callback about the completed move
	if m.OnMoveComplete != nil {
		sourceSlot := grabbedTerm.SlotIdx
		targetSlot := m.state.TargetSlotIndex
		isSwap := targetTermIdx >= 0 && targetTermIdx != grabbedTermIdx

		// Call callback without holding the lock (callback may need to do I/O)
		result := MoveResult{
			SourceSlot: sourceSlot,
			TargetSlot: targetSlot,
			IsSwap:     isSwap,
		}
		go m.OnMoveComplete(result)
	}
}

// updateOverlays updates the visual overlays based on current state
func (m *Mode) updateOverlays() {
	if m.state.Phase == PhaseInactive {
		m.overlay.HideAll()
		return
	}

	model, ok := m.buildRenderModel()
	if !ok {
		m.overlay.HideAll()
		return
	}

	terminalRects, terminalColors := splitOverlayHighlights(model.TerminalHighlights)
	slotRects, slotColors := splitOverlayHighlights(model.SlotHighlights)

	if err := m.overlay.Render(terminalRects, terminalColors, slotRects, slotColors, model.AllSlotRects, model.HintPhase); err != nil {
		log.Printf("Move mode: overlay render failed: %v", err)
	}
}

func splitOverlayHighlights(highlights []overlayHighlight) ([]tiling.Rect, []uint32) {
	rects := make([]tiling.Rect, 0, len(highlights))
	colors := make([]uint32, 0, len(highlights))
	for _, h := range highlights {
		rects = append(rects, h.Rect)
		colors = append(colors, h.Color)
	}
	return rects, colors
}

func (m *Mode) buildRenderModel() (overlayRenderModel, bool) {
	model := overlayRenderModel{
		AllSlotRects: append([]tiling.Rect(nil), m.state.SlotPositions...),
		HintPhase:    HintPhaseNone,
	}

	switch m.state.Phase {
	case PhaseSelecting:
		model.HintPhase = HintPhaseSelecting
		term := m.state.SelectedTerminal()
		if term == nil {
			return overlayRenderModel{}, false
		}
		model.TerminalHighlights = append(model.TerminalHighlights, overlayHighlight{
			Rect:  m.resolveTerminalRect(*term),
			Color: uint32(ColorSelection),
		})

	case PhaseConfirmDelete:
		model.HintPhase = HintPhaseConfirmDelete
		term := m.state.SelectedTerminal()
		if term == nil {
			return overlayRenderModel{}, false
		}
		model.TerminalHighlights = append(model.TerminalHighlights, overlayHighlight{
			Rect:  m.resolveTerminalRect(*term),
			Color: uint32(ColorTarget),
		})

	case PhaseGrabbed:
		model.HintPhase = HintPhaseMove
		grabbedTerm, foundGrabbed := m.findGrabbedTerminal()
		if foundGrabbed {
			model.TerminalHighlights = append(model.TerminalHighlights, overlayHighlight{
				Rect:  m.resolveTerminalRect(grabbedTerm),
				Color: uint32(ColorGrabbed),
			})
		}

		targetSlot := m.state.TargetSlotRect()
		if targetSlot != nil {
			class := ""
			if foundGrabbed {
				class = grabbedTerm.Window.Class
			}
			model.SlotHighlights = append(model.SlotHighlights, overlayHighlight{
				Rect:  m.normalizeSlotPreviewRect(*targetSlot, class),
				Color: uint32(ColorSelection),
			})
		}
	}

	return model, true
}

func (m *Mode) findGrabbedTerminal() (TerminalSlot, bool) {
	for _, term := range m.state.Terminals {
		if term.Window.WindowID == m.state.GrabbedWindow {
			return term, true
		}
	}
	return TerminalSlot{}, false
}

func (m *Mode) resolveTerminalRect(term TerminalSlot) tiling.Rect {
	if liveRect, ok := m.getClientWindowRect(term.Window.WindowID); ok {
		return liveRect
	}
	return tiling.Rect{
		X:      term.Window.X,
		Y:      term.Window.Y,
		Width:  term.Window.Width,
		Height: term.Window.Height,
	}
}

func (m *Mode) normalizeSlotPreviewRect(slot tiling.Rect, terminalClass string) tiling.Rect {
	preview := slot

	if terminalClass != "" {
		margins := m.config.GetMargins(terminalClass)
		preview.X += margins.Left
		preview.Y += margins.Top
		preview.Width -= margins.Left + margins.Right
		preview.Height -= margins.Top + margins.Bottom
	}

	if preview.Width < 1 {
		preview.Width = 1
	}
	if preview.Height < 1 {
		preview.Height = 1
	}

	return preview
}

func (m *Mode) getClientWindowRect(windowID platform.WindowID) (tiling.Rect, bool) {
	if m.xu == nil {
		return tiling.Rect{}, false
	}

	conn := m.xu.Conn()
	xpWin := xproto.Window(windowID)

	geom, err := xproto.GetGeometry(conn, xproto.Drawable(xpWin)).Reply()
	if err != nil {
		return tiling.Rect{}, false
	}

	translate, err := xproto.TranslateCoordinates(
		conn,
		xpWin,
		m.root,
		0, 0,
	).Reply()
	if err != nil {
		return tiling.Rect{}, false
	}

	return tiling.Rect{
		X:      int(translate.DstX),
		Y:      int(translate.DstY),
		Width:  int(geom.Width),
		Height: int(geom.Height),
	}, true
}

// startTimeout starts or resets the auto-exit timeout
func (m *Mode) startTimeout() {
	if m.timeout != nil {
		m.timeout.Stop()
	}

	m.timeout = time.AfterFunc(m.timeoutDuration, func() {
		m.mu.Lock()
		defer m.mu.Unlock()

		if m.state.Phase != PhaseInactive {
			log.Println("Move mode: timeout - auto-exiting")
			m.exitLocked()
		}
	})
}

// calculateGridDimensions determines rows and cols based on layout and terminal count
func (m *Mode) calculateGridDimensions(termCount int, layout *config.Layout) (rows, cols int) {
	switch layout.Mode {
	case config.LayoutModeAuto:
		return tiling.CalculateGrid(termCount)
	case config.LayoutModeFixed:
		return layout.FixedGrid.Rows, layout.FixedGrid.Cols
	case config.LayoutModeVertical:
		return termCount, 1
	case config.LayoutModeHorizontal:
		return 1, termCount
	default:
		return tiling.CalculateGrid(termCount)
	}
}

func sortTerminals(backend platform.Backend, windows []terminals.TerminalWindow, mode string) {
	switch mode {
	case "client_list":
		return
	case "window_id":
		sort.Slice(windows, func(i, j int) bool {
			return windows[i].WindowID < windows[j].WindowID
		})
	case "active_first":
		activeWin, _ := backend.ActiveWindow()
		sort.SliceStable(windows, func(i, j int) bool {
			wi, wj := windows[i], windows[j]
			if activeWin != 0 {
				if wi.WindowID == activeWin && wj.WindowID != activeWin {
					return true
				}
				if wj.WindowID == activeWin && wi.WindowID != activeWin {
					return false
				}
			}

			if wi.Y != wj.Y {
				return wi.Y < wj.Y
			}
			if wi.X != wj.X {
				return wi.X < wj.X
			}
			return wi.WindowID < wj.WindowID
		})
	default:
		sort.Slice(windows, func(i, j int) bool {
			wi, wj := windows[i], windows[j]
			if wi.Y != wj.Y {
				return wi.Y < wj.Y
			}
			if wi.X != wj.X {
				return wi.X < wj.X
			}
			return wi.WindowID < wj.WindowID
		})
	}
}

func assignTerminalsToSlots(windows []terminals.TerminalWindow, slots []tiling.Rect) []int {
	assignments := make([]int, len(windows))
	for i := range assignments {
		assignments[i] = -1
	}
	if len(windows) == 0 || len(slots) == 0 {
		return assignments
	}

	type candidate struct {
		termIdx int
		slotIdx int
		dist    int
	}

	candidates := make([]candidate, 0, len(windows)*len(slots))
	for ti, w := range windows {
		wx := w.X + w.Width/2
		wy := w.Y + w.Height/2
		for si, s := range slots {
			sx := s.X + s.Width/2
			sy := s.Y + s.Height/2
			dist := abs(wx-sx) + abs(wy-sy)
			candidates = append(candidates, candidate{
				termIdx: ti,
				slotIdx: si,
				dist:    dist,
			})
		}
	}

	sort.Slice(candidates, func(i, j int) bool {
		ci, cj := candidates[i], candidates[j]
		if ci.dist != cj.dist {
			return ci.dist < cj.dist
		}
		if ci.termIdx != cj.termIdx {
			return ci.termIdx < cj.termIdx
		}
		return ci.slotIdx < cj.slotIdx
	})

	usedTerm := make([]bool, len(windows))
	usedSlot := make([]bool, len(slots))
	assigned := 0

	for _, c := range candidates {
		if usedTerm[c.termIdx] || usedSlot[c.slotIdx] {
			continue
		}
		usedTerm[c.termIdx] = true
		usedSlot[c.slotIdx] = true
		assignments[c.termIdx] = c.slotIdx
		assigned++
		if assigned == len(windows) {
			break
		}
	}

	// Fallback for any unassigned terminals.
	lastSlot := len(slots) - 1
	if lastSlot < 0 {
		lastSlot = 0
	}
	for i := range assignments {
		if assignments[i] < 0 {
			if i <= lastSlot {
				assignments[i] = i
			} else {
				assignments[i] = lastSlot
			}
		}
	}

	return assignments
}

// UpdateConfig updates the mode's configuration reference
func (m *Mode) UpdateConfig(cfg *config.Config) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.config = cfg

	timeout := DefaultTimeout
	if cfg.MoveModeTimeout > 0 {
		timeout = cfg.MoveModeTimeout
	}
	m.timeoutDuration = time.Duration(timeout) * time.Second
}

// grabKeyboard grabs the keyboard and sets up key event handling
func (m *Mode) grabKeyboard() error {
	xu := m.xu
	if err := m.ensureGrabWindow(); err != nil {
		return err
	}

	// Grab the keyboard
	grab := func() (*xproto.GrabKeyboardReply, error) {
		cookie := xproto.GrabKeyboard(
			xu.Conn(),
			false,                  // owner_events (report events to grab_window)
			m.root,                 // grab_window (must be viewable)
			xproto.TimeCurrentTime, // time
			xproto.GrabModeAsync,   // pointer_mode
			xproto.GrabModeAsync,   // keyboard_mode
		)
		return cookie.Reply()
	}

	reply, err := grab()
	if err != nil {
		return err
	}

	// When move mode is entered from a globally grabbed hotkey, the keyboard may
	// already be grabbed by this client. If so, ungrab and retry.
	if reply.Status == xproto.GrabStatusAlreadyGrabbed {
		xproto.UngrabKeyboard(xu.Conn(), xproto.TimeCurrentTime)
		reply, err = grab()
		if err != nil {
			return err
		}
	}

	if reply.Status != xproto.GrabStatusSuccess {
		return fmt.Errorf("keyboard grab failed with status %d", reply.Status)
	}

	// Redirect all key events to our grab window while move mode is active.
	xevent.RedirectKeyEvents(xu, m.grabWindow)

	// Connect key press handler on our dedicated window (safe to detach later).
	if !m.keyHandlerAttached {
		xevent.KeyPressFun(m.handleKeyPress).Connect(xu, m.grabWindow)
		m.keyHandlerAttached = true
	}

	log.Println("Move mode: keyboard grabbed")
	return nil
}

// ungrabKeyboard releases the keyboard grab
func (m *Mode) ungrabKeyboard() {
	xu := m.xu

	// Ungrab the keyboard
	xproto.UngrabKeyboard(xu.Conn(), xproto.TimeCurrentTime)

	// Stop redirecting key events.
	xevent.RedirectKeyEvents(xu, 0)

	// Detach key press handler from our dedicated grab window.
	if m.keyHandlerAttached && m.grabWindow != 0 {
		xevent.Detach(xu, m.grabWindow)
		m.keyHandlerAttached = false
	}

	log.Println("Move mode: keyboard released")
}

func (m *Mode) ensureGrabWindow() error {
	if m.grabWindow != 0 {
		return nil
	}

	conn := m.xu.Conn()

	wid, err := xproto.NewWindowId(conn)
	if err != nil {
		return err
	}

	// InputOnly window that never draws anything; used solely as a safe target
	// for key event callbacks while the keyboard is grabbed.
	err = xproto.CreateWindowChecked(
		conn,
		0, // depth (must be 0 for InputOnly)
		wid,
		m.root,
		0, 0, // x, y
		1, 1, // width, height
		0, // border_width
		xproto.WindowClassInputOnly,
		xproto.Visualid(0), // CopyFromParent
		xproto.CwEventMask,
		[]uint32{uint32(xproto.EventMaskKeyPress)},
	).Check()
	if err != nil {
		return err
	}

	xproto.MapWindow(conn, wid)

	m.grabWindow = wid
	return nil
}

// handleKeyPress processes key events while keyboard is grabbed
func (m *Mode) handleKeyPress(xu *xgbutil.XUtil, ev xevent.KeyPressEvent) {
	// Look up the keysym for this keycode
	keysym := keybind.KeysymGet(xu, ev.Detail, 0)

	m.mu.Lock()
	defer m.mu.Unlock()

	if m.state.Phase == PhaseInactive {
		return
	}

	switch keysym {
	case keysymUp:
		m.handleArrowKeyLocked(DirUp)
	case keysymDown:
		m.handleArrowKeyLocked(DirDown)
	case keysymLeft:
		m.handleArrowKeyLocked(DirLeft)
	case keysymRight:
		m.handleArrowKeyLocked(DirRight)
	case keysymReturn, keysymKPEnter:
		m.handleConfirmLocked()
	case keysymEscape:
		m.handleCancelLocked()
	default:
		if action, ok := actionFromKeysym(uint32(keysym)); ok {
			m.handleActionKeyLocked(action)
		}
	}
}

// handleArrowKeyLocked processes arrow key (must be called with lock held)
func (m *Mode) handleArrowKeyLocked(dir Direction) {
	// Reset timeout
	m.startTimeout()

	switch m.state.Phase {
	case PhaseSelecting:
		newIdx := NavigateTerminal(m.state.SelectedIndex, dir, len(m.state.Terminals))
		if newIdx != m.state.SelectedIndex {
			m.state.SelectedIndex = newIdx
			m.updateOverlays()
			log.Printf("Move mode: selected terminal %d", newIdx)
		}

	case PhaseConfirmDelete:
		// Keep confirmation target stable until Enter/Escape.

	case PhaseGrabbed:
		newIdx := NavigateSlotSpatial(m.state.TargetSlotIndex, dir, m.state.SlotPositions, m.state.GridRows, m.state.GridCols)
		if newIdx < len(m.state.SlotPositions) && newIdx != m.state.TargetSlotIndex {
			m.state.TargetSlotIndex = newIdx
			m.updateOverlays()
			log.Printf("Move mode: target slot %d", newIdx)
		}
	}
}

// handleConfirmLocked processes Enter key (must be called with lock held)
func (m *Mode) handleConfirmLocked() {
	switch m.state.Phase {
	case PhaseSelecting:
		term := m.state.SelectedTerminal()
		if term == nil {
			return
		}

		m.state.Phase = PhaseGrabbed
		m.state.GrabbedWindow = term.Window.WindowID
		m.state.TargetSlotIndex = term.SlotIdx

		m.updateOverlays()
		m.startTimeout()

		log.Printf("Move mode: grabbed window %d, starting at slot %d", term.Window.WindowID, term.SlotIdx)

	case PhaseGrabbed:
		m.executeMove()
		m.exitLocked()

	case PhaseConfirmDelete:
		if m.state.PendingAction != ActionDeleteSelected || m.state.PendingSlot < 0 {
			log.Println("Move mode: delete confirmation had no valid pending target; returning to selection")
			m.state.Phase = PhaseSelecting
			m.state.ClearPendingAction()
			m.updateOverlays()
			m.startTimeout()
			return
		}
		m.executeTerminalActionLocked(ActionDeleteSelected, m.state.PendingSlot)
	}
}

func (m *Mode) handleCancelLocked() {
	switch m.state.Phase {
	case PhaseConfirmDelete:
		m.state.Phase = PhaseSelecting
		m.state.ClearPendingAction()
		m.updateOverlays()
		m.startTimeout()
		log.Println("Move mode: delete confirmation cancelled")
	default:
		log.Println("Move mode: cancelled")
		m.exitLocked()
	}
}

func (m *Mode) handleActionKeyLocked(action Action) {
	// Keep mode alive while user works through action keys.
	m.startTimeout()

	if m.state.Phase == PhaseInactive || m.state.Phase == PhaseGrabbed {
		return
	}
	if m.state.Phase == PhaseConfirmDelete && action != ActionDeleteSelected {
		return
	}

	switch action {
	case ActionDeleteSelected:
		if m.state.Phase == PhaseConfirmDelete {
			return
		}
		term := m.state.SelectedTerminal()
		if term == nil {
			return
		}
		m.state.BeginDeleteConfirmation(term.SlotIdx)
		m.updateOverlays()
		log.Printf("Move mode: delete requested for slot %d (press Enter to confirm, Escape to cancel)", term.SlotIdx)
	case ActionInsertAfterSelected:
		term := m.state.SelectedTerminal()
		if term == nil {
			return
		}
		m.executeTerminalActionLocked(ActionInsertAfterSelected, term.SlotIdx)
	case ActionAppend:
		m.executeTerminalActionLocked(ActionAppend, -1)
	}
}

func (m *Mode) executeTerminalActionLocked(action Action, selectedSlot int) {
	args, err := terminalActionArgs(action, selectedSlot)
	if err != nil {
		log.Printf("Move mode: cannot run %s action: %v", action, err)
		m.state.Phase = PhaseSelecting
		m.state.ClearPendingAction()
		m.updateOverlays()
		return
	}

	runner := m.actionRunner
	if runner == nil {
		log.Printf("Move mode: no action runner configured for %s", action)
		m.state.Phase = PhaseSelecting
		m.state.ClearPendingAction()
		m.updateOverlays()
		return
	}

	log.Printf("Move mode: executing action %s (%s)", action, strings.Join(args, " "))
	m.exitLocked()

	go func(a Action, argv []string) {
		if err := runner(argv); err != nil {
			log.Printf("Move mode: action %s failed: %v", a, err)
			return
		}
		log.Printf("Move mode: action %s completed", a)
	}(action, append([]string(nil), args...))
}

func actionFromKeysym(keysym uint32) (Action, bool) {
	switch keysym {
	case keysymD, keysymd:
		return ActionDeleteSelected, true
	case keysymN, keysymn:
		return ActionInsertAfterSelected, true
	case keysymA, keysyma:
		return ActionAppend, true
	default:
		return ActionNone, false
	}
}

func terminalActionArgs(action Action, selectedSlot int) ([]string, error) {
	switch action {
	case ActionDeleteSelected:
		if selectedSlot < 0 {
			return nil, fmt.Errorf("invalid slot for delete action: %d", selectedSlot)
		}
		return []string{"remove", "--slot", strconv.Itoa(selectedSlot)}, nil
	case ActionInsertAfterSelected:
		if selectedSlot < 0 {
			return nil, fmt.Errorf("invalid slot for insert action: %d", selectedSlot)
		}
		return []string{"add", "--slot", strconv.Itoa(selectedSlot + 1)}, nil
	case ActionAppend:
		return []string{"add"}, nil
	default:
		return nil, fmt.Errorf("unsupported action %q", action.String())
	}
}

func runTerminalActionViaCLI(args []string) error {
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve executable: %w", err)
	}
	cmdArgs := make([]string, 0, len(args)+1)
	cmdArgs = append(cmdArgs, "terminal")
	cmdArgs = append(cmdArgs, args...)

	cmd := exec.Command(exePath, cmdArgs...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg != "" {
			return fmt.Errorf("terminal command failed: %w: %s", err, msg)
		}
		return fmt.Errorf("terminal command failed: %w", err)
	}
	return nil
}
