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
	// PhaseConfirmDelete means user must confirm terminal deletion
	PhaseConfirmDelete
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
	case PhaseConfirmDelete:
		return "confirm_delete"
	default:
		return "unknown"
	}
}

// Action represents a terminal action from move mode.
type Action int

const (
	// ActionNone means no action is pending.
	ActionNone Action = iota
	// ActionDeleteSelected removes the selected slot (requires confirmation).
	ActionDeleteSelected
	// ActionInsertAfterSelected inserts a terminal after the selected slot.
	ActionInsertAfterSelected
	// ActionAppend appends a terminal at the end.
	ActionAppend
)

// String returns a readable action name.
func (a Action) String() string {
	switch a {
	case ActionNone:
		return "none"
	case ActionDeleteSelected:
		return "delete"
	case ActionInsertAfterSelected:
		return "insert_after"
	case ActionAppend:
		return "append"
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
	SelectedIndex   int               // Index of highlighted terminal in Terminals slice
	GrabbedWindow   platform.WindowID // Window ID of grabbed window (0 if none)
	TargetSlotIndex int               // Target slot index for grabbed window
	PendingAction   Action            // Pending action awaiting confirmation
	PendingSlot     int               // Slot index targeted by the pending action (-1 if none)
	Terminals       []TerminalSlot    // Windows with slot assignments
	SlotPositions   []tiling.Rect     // Grid slot geometries
	GridRows        int               // Number of rows in the grid
	GridCols        int               // Number of columns in the grid
}

// NewState creates a new inactive state
func NewState() *State {
	return &State{
		Phase:           PhaseInactive,
		SelectedIndex:   0,
		GrabbedWindow:   0,
		TargetSlotIndex: 0,
		PendingAction:   ActionNone,
		PendingSlot:     -1,
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
	s.PendingAction = ActionNone
	s.PendingSlot = -1
	s.Terminals = nil
	s.SlotPositions = nil
	s.GridRows = 0
	s.GridCols = 0
}

// BeginDeleteConfirmation transitions state into delete-confirm mode.
func (s *State) BeginDeleteConfirmation(slot int) {
	s.Phase = PhaseConfirmDelete
	s.PendingAction = ActionDeleteSelected
	s.PendingSlot = slot
}

// ClearPendingAction clears any pending in-mode action.
func (s *State) ClearPendingAction() {
	s.PendingAction = ActionNone
	s.PendingSlot = -1
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
