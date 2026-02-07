package tiling

import (
	"fmt"
	"log"
	"math"
	"regexp"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/1broseidon/termtile/internal/config"
	"github.com/1broseidon/termtile/internal/platform"
	"github.com/1broseidon/termtile/internal/terminals"
)

// sessionSlotRe extracts the trailing slot number from a termtile tmux session name
// e.g. "termtile-my-agents-0" → "0"
var sessionSlotRe = regexp.MustCompile(`^termtile-.*-(\d+)$`)

// Workspace tracks the tiling state for a monitor
type Workspace struct {
	MonitorID          int
	Terminals          []terminals.TerminalWindow
	LastTiledAt        time.Time
	PreviousGeometries map[platform.WindowID]Rect
}

// Tiler manages the tiling state across monitors
type Tiler struct {
	mu              sync.RWMutex
	backend         platform.Backend
	detector        *terminals.Detector
	config          *config.Config
	activeLayout    string
	workspaces      map[int]*Workspace
	previewID       int
	previewTimer    *time.Timer
	previewSnapshot map[platform.WindowID]Rect
}

// NewTiler creates a new tiler instance
func NewTiler(backend platform.Backend, detector *terminals.Detector, cfg *config.Config) *Tiler {
	return &Tiler{
		backend:      backend,
		detector:     detector,
		config:       cfg,
		activeLayout: cfg.DefaultLayout,
		workspaces:   make(map[int]*Workspace),
	}
}

// TileCurrentMonitor tiles all terminals on the currently active monitor
func (t *Tiler) TileCurrentMonitor() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.cancelPreviewLocked()

	log.Println("=== Starting tiling operation ===")

	// Step 1: Get the active layout
	layoutName := t.activeLayout
	if layoutName == "" {
		layoutName = t.config.DefaultLayout
	}

	layout, err := t.config.GetLayout(layoutName)
	if err != nil {
		log.Printf("Failed to get layout: %v", err)
		return err
	}
	log.Printf("Using layout: %s (mode: %s, region: %s)", layoutName, layout.Mode, layout.TileRegion.Type)

	// Step 2: Get the active monitor
	display, err := t.backend.ActiveDisplay()
	if err != nil {
		log.Printf("Failed to get active monitor: %v", err)
		return err
	}

	bounds := display.Bounds
	log.Printf("Active monitor: %s (%dx%d at %d,%d)",
		display.Name, bounds.Width, bounds.Height, bounds.X, bounds.Y)

	// Apply screen padding to create a safe area
	padding := t.config.ScreenPadding
	if padding.Top != 0 || padding.Bottom != 0 || padding.Left != 0 || padding.Right != 0 {
		log.Printf("Applying screen padding: top=%d, bottom=%d, left=%d, right=%d",
			padding.Top, padding.Bottom, padding.Left, padding.Right)

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

		log.Printf("Adjusted monitor area: %dx%d at %d,%d",
			bounds.Width, bounds.Height, bounds.X, bounds.Y)
	}

	// Step 3: Apply tile region
	monitorRect := rectFromPlatform(bounds)
	adjustedMonitor := ApplyRegion(monitorRect, layout.TileRegion)
	log.Printf("Tile region applied: %dx%d at %d,%d",
		adjustedMonitor.Width, adjustedMonitor.Height, adjustedMonitor.X, adjustedMonitor.Y)

	if adjustedMonitor.Width < 1 || adjustedMonitor.Height < 1 {
		return fmt.Errorf(
			"tile_region leaves no usable space: %dx%d at %d,%d",
			adjustedMonitor.Width, adjustedMonitor.Height, adjustedMonitor.X, adjustedMonitor.Y,
		)
	}

	// Step 4: Find all terminals on this monitor
	terminalWindows, err := t.detector.FindTerminals(t.backend, display.ID, bounds)
	if err != nil {
		log.Printf("Failed to find terminals: %v", err)
		return err
	}

	log.Printf("Found %d terminal(s) on monitor %s", len(terminalWindows), display.Name)

	if len(terminalWindows) == 0 {
		log.Println("No terminals to tile")
		return nil
	}

	// Master-stack sorts by session slot so agent-0 is always master.
	// The slot number is parsed from the tmux session name in the window title.
	sortMode := t.config.TerminalSort
	if layout.Mode == config.LayoutModeMasterStack {
		sortMode = "session_slot"
	}
	sortTerminals(t.backend, terminalWindows, sortMode)

	previous := make(map[platform.WindowID]Rect, len(terminalWindows))
	for _, term := range terminalWindows {
		previous[term.WindowID] = Rect{
			X:      term.X,
			Y:      term.Y,
			Width:  term.Width,
			Height: term.Height,
		}
	}

	// Log detected terminals
	for i, term := range terminalWindows {
		log.Printf("  Terminal %d: %s (ID: %d, title: %s)", i+1, term.Class, term.WindowID, term.Title)
	}

	// Step 5: Calculate positions using layout
	positions, err := CalculatePositionsWithLayout(
		len(terminalWindows),
		adjustedMonitor,
		layout,
		t.config.GapSize,
	)
	if err != nil {
		return err
	}

	// Log grid info
	var rows, cols int
	switch layout.Mode {
	case config.LayoutModeAuto:
		rows, cols = CalculateGrid(len(terminalWindows))
	case config.LayoutModeFixed:
		rows, cols = layout.FixedGrid.Rows, layout.FixedGrid.Cols
	case config.LayoutModeVertical:
		rows, cols = len(terminalWindows), 1
	case config.LayoutModeHorizontal:
		rows, cols = 1, len(terminalWindows)
	}
	log.Printf("Layout: %dx%d grid (%s mode) with %dpx gaps",
		rows, cols, layout.Mode, t.config.GapSize)

	// Step 6: Move and resize each terminal
	for i, term := range terminalWindows {
		if i >= len(positions) {
			log.Printf("Skipping terminal %d (exceeds layout capacity)", i+1)
			continue
		}

		pos := positions[i]

		// Apply per-terminal margin adjustments
		margins := t.config.GetMargins(term.Class)
		adjustedPos := Rect{
			X:      pos.X + margins.Left,
			Y:      pos.Y + margins.Top,
			Width:  pos.Width - margins.Left - margins.Right,
			Height: pos.Height - margins.Top - margins.Bottom,
		}

		if margins.Top != 0 || margins.Bottom != 0 || margins.Left != 0 || margins.Right != 0 {
			log.Printf("Applying margins for %s: top=%d, bottom=%d, left=%d, right=%d",
				term.Class, margins.Top, margins.Bottom, margins.Left, margins.Right)
		}

		log.Printf("Tiling terminal %d to position (%d,%d) size %dx%d",
			i+1, adjustedPos.X, adjustedPos.Y, adjustedPos.Width, adjustedPos.Height)

		if adjustedPos.Width < 1 || adjustedPos.Height < 1 {
			log.Printf(
				"Warning: Skipping terminal %d (invalid geometry after margins: %dx%d)",
				i+1, adjustedPos.Width, adjustedPos.Height,
			)
			continue
		}

		err := t.backend.MoveResize(
			term.WindowID,
			platform.Rect{X: adjustedPos.X, Y: adjustedPos.Y, Width: adjustedPos.Width, Height: adjustedPos.Height},
		)

		if err != nil {
			log.Printf("Warning: Failed to tile terminal %d: %v", i+1, err)
			// Continue with other windows even if one fails
		}
	}

	// Step 7: Update workspace state
	t.workspaces[display.ID] = &Workspace{
		MonitorID:          display.ID,
		Terminals:          terminalWindows,
		LastTiledAt:        time.Now(),
		PreviousGeometries: previous,
	}

	log.Printf("=== Tiling completed successfully ===")
	return nil
}

// TileWithOrder tiles terminals using a specific window order instead of sorting by position.
// This is used by workspace load to ensure windows end up in the correct slots.
func (t *Tiler) TileWithOrder(windowOrder []uint32) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.cancelPreviewLocked()

	log.Println("=== Starting ordered tiling operation ===")

	// Step 1: Get the active layout
	layoutName := t.activeLayout
	if layoutName == "" {
		layoutName = t.config.DefaultLayout
	}

	layout, err := t.config.GetLayout(layoutName)
	if err != nil {
		log.Printf("Failed to get layout: %v", err)
		return err
	}
	log.Printf("Using layout: %s (mode: %s, region: %s)", layoutName, layout.Mode, layout.TileRegion.Type)

	// Step 2: Get the active monitor
	display, err := t.backend.ActiveDisplay()
	if err != nil {
		log.Printf("Failed to get active monitor: %v", err)
		return err
	}

	bounds := display.Bounds
	log.Printf("Active monitor: %s (%dx%d at %d,%d)",
		display.Name, bounds.Width, bounds.Height, bounds.X, bounds.Y)

	// Apply screen padding to create a safe area
	padding := t.config.ScreenPadding
	if padding.Top != 0 || padding.Bottom != 0 || padding.Left != 0 || padding.Right != 0 {
		log.Printf("Applying screen padding: top=%d, bottom=%d, left=%d, right=%d",
			padding.Top, padding.Bottom, padding.Left, padding.Right)

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

		log.Printf("Adjusted monitor area: %dx%d at %d,%d",
			bounds.Width, bounds.Height, bounds.X, bounds.Y)
	}

	// Step 3: Apply tile region
	monitorRect := rectFromPlatform(bounds)
	adjustedMonitor := ApplyRegion(monitorRect, layout.TileRegion)
	log.Printf("Tile region applied: %dx%d at %d,%d",
		adjustedMonitor.Width, adjustedMonitor.Height, adjustedMonitor.X, adjustedMonitor.Y)

	if adjustedMonitor.Width < 1 || adjustedMonitor.Height < 1 {
		return fmt.Errorf(
			"tile_region leaves no usable space: %dx%d at %d,%d",
			adjustedMonitor.Width, adjustedMonitor.Height, adjustedMonitor.X, adjustedMonitor.Y,
		)
	}

	// Step 4: Find all terminals on this monitor
	terminalWindows, err := t.detector.FindTerminals(t.backend, display.ID, bounds)
	if err != nil {
		log.Printf("Failed to find terminals: %v", err)
		return err
	}

	log.Printf("Found %d terminal(s) on monitor %s, ordering by %d provided window IDs",
		len(terminalWindows), display.Name, len(windowOrder))

	if len(terminalWindows) == 0 {
		log.Println("No terminals to tile")
		return nil
	}

	// Build a map of window ID to terminal for quick lookup.
	termByID := make(map[uint32]terminals.TerminalWindow, len(terminalWindows))
	for _, term := range terminalWindows {
		termByID[uint32(term.WindowID)] = term
	}

	// Reorder terminals according to the explicit window order provided by workspace load.
	orderedTerminals := make([]terminals.TerminalWindow, 0, len(terminalWindows))
	matched := make(map[uint32]struct{}, len(windowOrder))
	for _, wid := range windowOrder {
		if _, already := matched[wid]; already {
			log.Printf("Warning: duplicate window ID %d in provided order", wid)
			continue
		}
		if term, ok := termByID[wid]; ok {
			orderedTerminals = append(orderedTerminals, term)
			matched[wid] = struct{}{}
		} else {
			log.Printf("Warning: window ID %d from order not found on monitor", wid)
		}
	}

	// Add any remaining terminals that weren't in the provided order.
	// Preserve detector enumeration order; do not re-sort by position.
	extra := 0
	for _, term := range terminalWindows {
		if _, ok := matched[uint32(term.WindowID)]; ok {
			continue
		}
		orderedTerminals = append(orderedTerminals, term)
		extra++
	}
	if extra > 0 {
		log.Printf("Added %d extra terminals not in provided order (preserving detector order)", extra)
	}

	previous := make(map[platform.WindowID]Rect, len(orderedTerminals))
	for _, term := range orderedTerminals {
		previous[term.WindowID] = Rect{
			X:      term.X,
			Y:      term.Y,
			Width:  term.Width,
			Height: term.Height,
		}
	}

	// Log ordered terminals
	for i, term := range orderedTerminals {
		log.Printf("  Terminal %d: %s (ID: %d)", i+1, term.Class, term.WindowID)
	}

	// Step 5: Calculate positions using layout
	positions, err := CalculatePositionsWithLayout(
		len(orderedTerminals),
		adjustedMonitor,
		layout,
		t.config.GapSize,
	)
	if err != nil {
		return err
	}

	// Step 6: Move and resize each terminal
	for i, term := range orderedTerminals {
		if i >= len(positions) {
			log.Printf("Skipping terminal %d (exceeds layout capacity)", i+1)
			continue
		}

		pos := positions[i]

		// Apply per-terminal margin adjustments
		margins := t.config.GetMargins(term.Class)
		adjustedPos := Rect{
			X:      pos.X + margins.Left,
			Y:      pos.Y + margins.Top,
			Width:  pos.Width - margins.Left - margins.Right,
			Height: pos.Height - margins.Top - margins.Bottom,
		}

		if margins.Top != 0 || margins.Bottom != 0 || margins.Left != 0 || margins.Right != 0 {
			log.Printf("Applying margins for %s: top=%d, bottom=%d, left=%d, right=%d",
				term.Class, margins.Top, margins.Bottom, margins.Left, margins.Right)
		}

		log.Printf("Tiling terminal %d to position (%d,%d) size %dx%d",
			i+1, adjustedPos.X, adjustedPos.Y, adjustedPos.Width, adjustedPos.Height)

		if adjustedPos.Width < 1 || adjustedPos.Height < 1 {
			log.Printf(
				"Warning: Skipping terminal %d (invalid geometry after margins: %dx%d)",
				i+1, adjustedPos.Width, adjustedPos.Height,
			)
			continue
		}

		err := t.backend.MoveResize(
			term.WindowID,
			platform.Rect{X: adjustedPos.X, Y: adjustedPos.Y, Width: adjustedPos.Width, Height: adjustedPos.Height},
		)

		if err != nil {
			log.Printf("Warning: Failed to tile terminal %d: %v", i+1, err)
		}
	}

	// Step 7: Update workspace state
	t.workspaces[display.ID] = &Workspace{
		MonitorID:          display.ID,
		Terminals:          orderedTerminals,
		LastTiledAt:        time.Now(),
		PreviousGeometries: previous,
	}

	log.Printf("=== Ordered tiling completed successfully ===")
	return nil
}

// UndoCurrentMonitor restores terminal windows to the geometry captured before the last tiling operation.
func (t *Tiler) UndoCurrentMonitor() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.cancelPreviewLocked()

	display, err := t.backend.ActiveDisplay()
	if err != nil {
		return err
	}

	ws := t.workspaces[display.ID]
	if ws == nil || len(ws.PreviousGeometries) == 0 {
		return nil
	}

	t.restoreWindowsLocked(ws.PreviousGeometries)
	ws.PreviousGeometries = nil
	return nil
}

// PreviewLayout temporarily applies a layout and restores previous geometry after a duration.
func (t *Tiler) PreviewLayout(layoutName string, duration time.Duration) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if duration <= 0 {
		duration = 3 * time.Second
	}

	// If a previous preview is active, restore it first.
	if t.previewTimer != nil {
		t.previewTimer.Stop()
		t.previewTimer = nil
		if t.previewSnapshot != nil {
			t.restoreWindowsLocked(t.previewSnapshot)
		}
		t.previewSnapshot = nil
	}

	layout, err := t.config.GetLayout(layoutName)
	if err != nil {
		return err
	}

	display, err := t.backend.ActiveDisplay()
	if err != nil {
		return err
	}

	bounds := display.Bounds

	// Apply screen padding to create a safe area
	padding := t.config.ScreenPadding
	if padding.Top != 0 || padding.Bottom != 0 || padding.Left != 0 || padding.Right != 0 {
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
	}

	monitorRect := rectFromPlatform(bounds)
	adjustedMonitor := ApplyRegion(monitorRect, layout.TileRegion)
	if adjustedMonitor.Width < 1 || adjustedMonitor.Height < 1 {
		return fmt.Errorf(
			"tile_region leaves no usable space: %dx%d at %d,%d",
			adjustedMonitor.Width, adjustedMonitor.Height, adjustedMonitor.X, adjustedMonitor.Y,
		)
	}

	terminalWindows, err := t.detector.FindTerminals(t.backend, display.ID, bounds)
	if err != nil {
		return err
	}
	if len(terminalWindows) == 0 {
		return nil
	}

	// Master-stack sorts by session slot so agent-0 is always master.
	sortMode := t.config.TerminalSort
	if layout.Mode == config.LayoutModeMasterStack {
		sortMode = "session_slot"
	}
	sortTerminals(t.backend, terminalWindows, sortMode)

	snapshot := make(map[platform.WindowID]Rect, len(terminalWindows))
	for _, term := range terminalWindows {
		snapshot[term.WindowID] = Rect{
			X:      term.X,
			Y:      term.Y,
			Width:  term.Width,
			Height: term.Height,
		}
	}

	positions, err := CalculatePositionsWithLayout(
		len(terminalWindows),
		adjustedMonitor,
		layout,
		t.config.GapSize,
	)
	if err != nil {
		return err
	}

	for i, term := range terminalWindows {
		if i >= len(positions) {
			continue
		}

		pos := positions[i]
		margins := t.config.GetMargins(term.Class)
		adjustedPos := Rect{
			X:      pos.X + margins.Left,
			Y:      pos.Y + margins.Top,
			Width:  pos.Width - margins.Left - margins.Right,
			Height: pos.Height - margins.Top - margins.Bottom,
		}
		if adjustedPos.Width < 1 || adjustedPos.Height < 1 {
			continue
		}

		_ = t.backend.MoveResize(
			term.WindowID,
			platform.Rect{X: adjustedPos.X, Y: adjustedPos.Y, Width: adjustedPos.Width, Height: adjustedPos.Height},
		)
	}

	t.previewID++
	previewID := t.previewID
	t.previewSnapshot = snapshot

	t.previewTimer = time.AfterFunc(duration, func() {
		t.mu.Lock()
		defer t.mu.Unlock()

		if t.previewTimer == nil || t.previewID != previewID {
			return
		}

		t.previewTimer = nil
		restore := t.previewSnapshot
		t.previewSnapshot = nil
		if restore != nil {
			t.restoreWindowsLocked(restore)
		}
	})

	return nil
}

func (t *Tiler) cancelPreviewLocked() {
	if t.previewTimer == nil {
		return
	}

	t.previewTimer.Stop()
	t.previewTimer = nil
	t.previewSnapshot = nil
}

func (t *Tiler) restoreWindowsLocked(snapshot map[platform.WindowID]Rect) {
	for windowID, rect := range snapshot {
		_ = t.backend.MoveResize(windowID, platform.Rect{X: rect.X, Y: rect.Y, Width: rect.Width, Height: rect.Height})
	}
}

// parseSessionSlot extracts the slot number from a termtile tmux session title.
// Returns math.MaxInt for non-matching titles so they sort after slotted windows.
func parseSessionSlot(title string) int {
	m := sessionSlotRe.FindStringSubmatch(title)
	if m == nil {
		return math.MaxInt
	}
	n, err := strconv.Atoi(m[1])
	if err != nil {
		return math.MaxInt
	}
	return n
}

func sortTerminals(backend platform.Backend, terminals []terminals.TerminalWindow, mode string) {
	switch mode {
	case "client_list":
		return
	case "session_slot":
		sort.SliceStable(terminals, func(i, j int) bool {
			si, sj := parseSessionSlot(terminals[i].Title), parseSessionSlot(terminals[j].Title)
			if si != sj {
				return si < sj
			}
			return terminals[i].WindowID < terminals[j].WindowID
		})
	case "window_id":
		sort.Slice(terminals, func(i, j int) bool {
			return terminals[i].WindowID < terminals[j].WindowID
		})
	case "active_first":
		activeWin, _ := backend.ActiveWindow()
		sort.SliceStable(terminals, func(i, j int) bool {
			ti, tj := terminals[i], terminals[j]
			if activeWin != 0 {
				if ti.WindowID == activeWin && tj.WindowID != activeWin {
					return true
				}
				if tj.WindowID == activeWin && ti.WindowID != activeWin {
					return false
				}
			}

			if ti.Y != tj.Y {
				return ti.Y < tj.Y
			}
			if ti.X != tj.X {
				return ti.X < tj.X
			}
			return ti.WindowID < tj.WindowID
		})
	default:
		sort.Slice(terminals, func(i, j int) bool {
			ti, tj := terminals[i], terminals[j]
			if ti.Y != tj.Y {
				return ti.Y < tj.Y
			}
			if ti.X != tj.X {
				return ti.X < tj.X
			}
			return ti.WindowID < tj.WindowID
		})
	}
}

// GetWorkspace returns a copy of the workspace for a given monitor ID.
func (t *Tiler) GetWorkspace(monitorID int) *Workspace {
	t.mu.RLock()
	defer t.mu.RUnlock()

	ws := t.workspaces[monitorID]
	if ws == nil {
		return nil
	}

	terminalsCopy := make([]terminals.TerminalWindow, len(ws.Terminals))
	copy(terminalsCopy, ws.Terminals)

	previousCopy := make(map[platform.WindowID]Rect, len(ws.PreviousGeometries))
	for windowID, rect := range ws.PreviousGeometries {
		previousCopy[windowID] = rect
	}

	wsCopy := *ws
	wsCopy.Terminals = terminalsCopy
	wsCopy.PreviousGeometries = previousCopy
	return &wsCopy
}

// GetTerminalCount returns the last known terminal count for a monitor.
func (t *Tiler) GetTerminalCount(monitorID int) int {
	t.mu.RLock()
	defer t.mu.RUnlock()

	ws := t.workspaces[monitorID]
	if ws == nil {
		return 0
	}
	return len(ws.Terminals)
}

// GetActiveLayoutName returns the current active layout name.
func (t *Tiler) GetActiveLayoutName() string {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if t.activeLayout != "" {
		return t.activeLayout
	}
	return t.config.DefaultLayout
}

// SetActiveLayout sets the current active layout (used by TileCurrentMonitor).
func (t *Tiler) SetActiveLayout(name string) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if _, err := t.config.GetLayout(name); err != nil {
		return err
	}
	t.activeLayout = name
	return nil
}

// CycleActiveLayout moves to the next/previous layout in sorted order.
func (t *Tiler) CycleActiveLayout(delta int) (string, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if len(t.config.Layouts) == 0 {
		return "", fmt.Errorf("no layouts configured")
	}

	names := make([]string, 0, len(t.config.Layouts))
	for name := range t.config.Layouts {
		names = append(names, name)
	}
	sort.Strings(names)

	current := t.activeLayout
	if current == "" {
		current = t.config.DefaultLayout
	}

	idx := 0
	for i, name := range names {
		if name == current {
			idx = i
			break
		}
	}

	n := len(names)
	next := (idx + delta) % n
	if next < 0 {
		next += n
	}

	t.activeLayout = names[next]
	return t.activeLayout, nil
}

// UpdateConfig updates the tiler's configuration
func (t *Tiler) UpdateConfig(cfg *config.Config) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.config = cfg
	if t.activeLayout == "" {
		t.activeLayout = cfg.DefaultLayout
		return
	}

	if _, err := cfg.GetLayout(t.activeLayout); err == nil {
		return
	}

	t.activeLayout = cfg.DefaultLayout
}

// rectFromPlatform converts a platform.Rect to a tiling Rect.
func rectFromPlatform(r platform.Rect) Rect {
	return Rect{X: r.X, Y: r.Y, Width: r.Width, Height: r.Height}
}
