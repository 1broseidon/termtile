package movemode

import (
	"reflect"
	"testing"

	"github.com/1broseidon/termtile/internal/tiling"
)

func TestNavigateSlot_UniformGrid(t *testing.T) {
	// Test 2x3 uniform grid
	// [0] [1] [2]
	// [3] [4] [5]
	rows, cols := 2, 3

	tests := []struct {
		name     string
		current  int
		dir      Direction
		expected int
	}{
		// Basic movement
		{"right from 0", 0, DirRight, 1},
		{"right from 1", 1, DirRight, 2},
		{"down from 0", 0, DirDown, 3},
		{"down from 1", 1, DirDown, 4},
		{"left from 1", 1, DirLeft, 0},
		{"up from 3", 3, DirUp, 0},

		// Wrapping
		{"right wrap from 2", 2, DirRight, 0},
		{"left wrap from 0", 0, DirLeft, 2},
		{"down wrap from 3", 3, DirDown, 0},
		{"up wrap from 0", 0, DirUp, 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NavigateSlot(tt.current, tt.dir, rows, cols)
			if got != tt.expected {
				t.Errorf("NavigateSlot(%d, %v, %d, %d) = %d, want %d",
					tt.current, tt.dir, rows, cols, got, tt.expected)
			}
		})
	}
}

func TestNavigateSlotSpatial_FallbackToGridMath(t *testing.T) {
	// When no positions provided, should fall back to NavigateSlot
	rows, cols := 2, 3

	tests := []struct {
		name     string
		current  int
		dir      Direction
		expected int
	}{
		{"right from 0", 0, DirRight, 1},
		{"down from 0", 0, DirDown, 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NavigateSlotSpatial(tt.current, tt.dir, nil, rows, cols)
			if got != tt.expected {
				t.Errorf("NavigateSlotSpatial with nil positions = %d, want %d", got, tt.expected)
			}
		})
	}
}

func TestNavigateSlotSpatial_FlexibleLastRow(t *testing.T) {
	// Test 2x3 grid with flexible last row (5 windows)
	// [0] [1] [2]    Row 0: Y=0, slots at X=0, 100, 200
	// [3] [4]        Row 1: Y=100, slots at X=50, 150 (centered)
	positions := []tiling.Rect{
		{X: 0, Y: 0, Width: 100, Height: 100},     // Slot 0
		{X: 100, Y: 0, Width: 100, Height: 100},   // Slot 1
		{X: 200, Y: 0, Width: 100, Height: 100},   // Slot 2
		{X: 50, Y: 100, Width: 100, Height: 100},  // Slot 3
		{X: 150, Y: 100, Width: 100, Height: 100}, // Slot 4
	}
	rows, cols := 2, 3

	tests := []struct {
		name     string
		current  int
		dir      Direction
		expected int
	}{
		// Basic movement within same row
		{"right from 0", 0, DirRight, 1},
		{"right from 1", 1, DirRight, 2},
		{"left from 2", 2, DirLeft, 1},
		{"right from 3", 3, DirRight, 4},
		{"left from 4", 4, DirLeft, 3},

		// Down from top row to bottom row
		{"down from 0 -> 3", 0, DirDown, 3}, // Closest by X
		{"down from 1 -> 3", 1, DirDown, 3}, // Slot 1 center=150, slot 3 center=100, slot 4 center=200
		{"down from 2 -> 4", 2, DirDown, 4}, // Slot 2 center=250, closest to slot 4 center=200

		// Up from bottom row to top row
		{"up from 3 -> 0", 3, DirUp, 0}, // Slot 3 center=100, closest to slot 0 center=50
		{"up from 4 -> 1", 4, DirUp, 1}, // Slot 4 center=200, equidistant to slot 1 (150) and 2 (250), picks first

		// Wrap from top row going up
		{"up wrap from 0 -> 3", 0, DirUp, 3}, // Wrap to bottom, closest X
		{"up wrap from 2 -> 4", 2, DirUp, 4}, // Wrap to bottom, closest X

		// Wrap from bottom row going down
		{"down wrap from 3 -> 0", 3, DirDown, 0}, // Wrap to top, closest X
		{"down wrap from 4 -> 1", 4, DirDown, 1}, // Wrap to top, equidistant to slot 1 and 2, picks first

		// Left/right wrap in top row
		{"right wrap from 2 -> 0", 2, DirRight, 0}, // Wrap to leftmost in same row
		{"left wrap from 0 -> 2", 0, DirLeft, 2},   // Wrap to rightmost in same row

		// Right from slot 4 goes to slot 2 (spatial - slot 2 is to the right of slot 4)
		{"right from 4 -> 2", 4, DirRight, 2}, // Slot 2 X=250 > Slot 4 X=200

		// Left from slot 3 goes to slot 0 (spatial - slot 0 is to the left of slot 3)
		{"left from 3 -> 0", 3, DirLeft, 0}, // Slot 0 X=50 < Slot 3 X=100
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NavigateSlotSpatial(tt.current, tt.dir, positions, rows, cols)
			if got != tt.expected {
				t.Errorf("NavigateSlotSpatial(%d, %v) = %d, want %d",
					tt.current, tt.dir, got, tt.expected)
			}
		})
	}
}

func TestNavigateSlotSpatial_SingleSlot(t *testing.T) {
	positions := []tiling.Rect{
		{X: 0, Y: 0, Width: 100, Height: 100},
	}

	// Navigation should stay at slot 0 when there's only one slot
	for _, dir := range []Direction{DirUp, DirDown, DirLeft, DirRight} {
		got := NavigateSlotSpatial(0, dir, positions, 1, 1)
		if got != 0 {
			t.Errorf("NavigateSlotSpatial with single slot, dir=%v = %d, want 0", dir, got)
		}
	}
}

func TestNavigateSlotSpatial_InvalidIndex(t *testing.T) {
	positions := []tiling.Rect{
		{X: 0, Y: 0, Width: 100, Height: 100},
		{X: 100, Y: 0, Width: 100, Height: 100},
	}

	// Invalid index should return 0
	got := NavigateSlotSpatial(-1, DirRight, positions, 1, 2)
	if got != 0 {
		t.Errorf("NavigateSlotSpatial with negative index = %d, want 0", got)
	}

	got = NavigateSlotSpatial(10, DirRight, positions, 1, 2)
	if got != 0 {
		t.Errorf("NavigateSlotSpatial with out-of-bounds index = %d, want 0", got)
	}
}

func TestNavigateTerminal(t *testing.T) {
	tests := []struct {
		name     string
		current  int
		dir      Direction
		count    int
		expected int
	}{
		{"next from 0", 0, DirDown, 3, 1},
		{"next from 2", 2, DirDown, 3, 0}, // wrap
		{"prev from 0", 0, DirUp, 3, 2},   // wrap
		{"prev from 1", 1, DirUp, 3, 0},
		{"right acts as next", 0, DirRight, 3, 1},
		{"left acts as prev", 0, DirLeft, 3, 2},
		{"single terminal", 0, DirDown, 1, 0},
		{"zero count", 0, DirDown, 0, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NavigateTerminal(tt.current, tt.dir, tt.count)
			if got != tt.expected {
				t.Errorf("NavigateTerminal(%d, %v, %d) = %d, want %d",
					tt.current, tt.dir, tt.count, got, tt.expected)
			}
		})
	}
}

func TestStateDeleteConfirmationLifecycle(t *testing.T) {
	state := NewState()
	state.Phase = PhaseSelecting

	state.BeginDeleteConfirmation(3)
	if state.Phase != PhaseConfirmDelete {
		t.Fatalf("phase = %v, want %v", state.Phase, PhaseConfirmDelete)
	}
	if state.PendingAction != ActionDeleteSelected {
		t.Fatalf("pending action = %v, want %v", state.PendingAction, ActionDeleteSelected)
	}
	if state.PendingSlot != 3 {
		t.Fatalf("pending slot = %d, want 3", state.PendingSlot)
	}

	state.ClearPendingAction()
	if state.PendingAction != ActionNone {
		t.Fatalf("pending action after clear = %v, want %v", state.PendingAction, ActionNone)
	}
	if state.PendingSlot != -1 {
		t.Fatalf("pending slot after clear = %d, want -1", state.PendingSlot)
	}

	state.Reset()
	if state.Phase != PhaseInactive {
		t.Fatalf("phase after reset = %v, want %v", state.Phase, PhaseInactive)
	}
	if state.PendingAction != ActionNone {
		t.Fatalf("pending action after reset = %v, want %v", state.PendingAction, ActionNone)
	}
}

func TestActionFromKeysym(t *testing.T) {
	tests := []struct {
		name   string
		keysym uint32
		want   Action
		ok     bool
	}{
		{name: "d", keysym: keysymd, want: ActionDeleteSelected, ok: true},
		{name: "D", keysym: keysymD, want: ActionDeleteSelected, ok: true},
		{name: "n", keysym: keysymn, want: ActionInsertAfterSelected, ok: true},
		{name: "N", keysym: keysymN, want: ActionInsertAfterSelected, ok: true},
		{name: "a", keysym: keysyma, want: ActionAppend, ok: true},
		{name: "A", keysym: keysymA, want: ActionAppend, ok: true},
		{name: "unsupported", keysym: keysymRight, want: ActionNone, ok: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := actionFromKeysym(tt.keysym)
			if ok != tt.ok {
				t.Fatalf("ok = %v, want %v", ok, tt.ok)
			}
			if got != tt.want {
				t.Fatalf("action = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTerminalActionArgs(t *testing.T) {
	tests := []struct {
		name        string
		action      Action
		slot        int
		wantArgs    []string
		expectError bool
	}{
		{
			name:     "delete selected slot",
			action:   ActionDeleteSelected,
			slot:     2,
			wantArgs: []string{"remove", "--slot", "2"},
		},
		{
			name:     "insert after selected slot",
			action:   ActionInsertAfterSelected,
			slot:     2,
			wantArgs: []string{"add", "--slot", "3"},
		},
		{
			name:     "append terminal",
			action:   ActionAppend,
			slot:     -1,
			wantArgs: []string{"add"},
		},
		{
			name:        "delete with invalid slot",
			action:      ActionDeleteSelected,
			slot:        -1,
			expectError: true,
		},
		{
			name:        "unsupported action",
			action:      ActionNone,
			slot:        0,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := terminalActionArgs(tt.action, tt.slot)
			if tt.expectError {
				if err == nil {
					t.Fatalf("expected error for action=%v slot=%d", tt.action, tt.slot)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, tt.wantArgs) {
				t.Fatalf("args = %v, want %v", got, tt.wantArgs)
			}
		})
	}
}
