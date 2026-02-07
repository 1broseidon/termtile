package movemode

import (
	"github.com/1broseidon/termtile/internal/platform"
	"github.com/1broseidon/termtile/internal/terminals"
	"github.com/1broseidon/termtile/internal/tiling"
)

// Phase represents the current phase of move mode
type Phase int

const (
	// PhaseInactive means move mode is not active
	PhaseInactive Phase = iota
	// PhaseSelecting means user is choosing which window to move
	PhaseSelecting
	// PhaseGrabbed means a window is grabbed and user is choosing target slot
	PhaseGrabbed
)

// String returns the string representation of the phase
func (p Phase) String() string {
	switch p {
	case PhaseInactive:
		return "inactive"
	case PhaseSelecting:
		return "selecting"
	case PhaseGrabbed:
		return "grabbed"
	default:
		return "unknown"
	}
}

// Direction represents an arrow key direction
type Direction int

const (
	DirUp Direction = iota
	DirDown
	DirLeft
	DirRight
)

// TerminalSlot represents a terminal window with its assigned slot
type TerminalSlot struct {
	Window   terminals.TerminalWindow
	SlotIdx  int // Index in the grid (0-based)
	SlotRect tiling.Rect
}

// State holds the current move mode state
type State struct {
	Phase           Phase
	SelectedIndex   int                // Index of highlighted terminal in Terminals slice
	GrabbedWindow   platform.WindowID  // Window ID of grabbed window (0 if none)
	TargetSlotIndex int                // Target slot index for grabbed window
	Terminals       []TerminalSlot     // Windows with slot assignments
	SlotPositions   []tiling.Rect      // Grid slot geometries
	GridRows        int                // Number of rows in the grid
	GridCols        int                // Number of columns in the grid
}

// NewState creates a new inactive state
func NewState() *State {
	return &State{
		Phase:           PhaseInactive,
		SelectedIndex:   0,
		GrabbedWindow:   0,
		TargetSlotIndex: 0,
		Terminals:       nil,
		SlotPositions:   nil,
	}
}

// Reset resets the state to inactive
func (s *State) Reset() {
	s.Phase = PhaseInactive
	s.SelectedIndex = 0
	s.GrabbedWindow = 0
	s.TargetSlotIndex = 0
	s.Terminals = nil
	s.SlotPositions = nil
	s.GridRows = 0
	s.GridCols = 0
}

// SelectedTerminal returns the currently selected terminal, or nil if none
func (s *State) SelectedTerminal() *TerminalSlot {
	if s.SelectedIndex < 0 || s.SelectedIndex >= len(s.Terminals) {
		return nil
	}
	return &s.Terminals[s.SelectedIndex]
}

// TargetSlotRect returns the rect for the current target slot, or nil if invalid
func (s *State) TargetSlotRect() *tiling.Rect {
	if s.TargetSlotIndex < 0 || s.TargetSlotIndex >= len(s.SlotPositions) {
		return nil
	}
	return &s.SlotPositions[s.TargetSlotIndex]
}
