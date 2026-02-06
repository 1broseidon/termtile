package movemode

import "github.com/1broseidon/termtile/internal/tiling"

// NavigateSlot calculates the new slot index after moving in a direction.
// Uses spatial navigation: arrow keys move in the grid based on direction.
// Wraps around at edges.
// NOTE: This assumes a uniform grid. For layouts with variable columns per row
// (like flexible_last_row), use NavigateSlotSpatial instead.
func NavigateSlot(currentIdx int, dir Direction, rows, cols int) int {
	if rows <= 0 || cols <= 0 {
		return currentIdx
	}

	row := currentIdx / cols
	col := currentIdx % cols

	switch dir {
	case DirUp:
		row = (row - 1 + rows) % rows
	case DirDown:
		row = (row + 1) % rows
	case DirLeft:
		col = (col - 1 + cols) % cols
	case DirRight:
		col = (col + 1) % cols
	}

	return row*cols + col
}

// NavigateSlotSpatial navigates slots using actual slot positions.
// This handles layouts with variable columns per row (like flexible_last_row).
// Falls back to grid math if positions is empty.
func NavigateSlotSpatial(currentIdx int, dir Direction, positions []tiling.Rect, rows, cols int) int {
	// Fall back to grid math if no positions available
	if len(positions) == 0 {
		return NavigateSlot(currentIdx, dir, rows, cols)
	}

	if currentIdx < 0 || currentIdx >= len(positions) {
		return 0
	}

	// Get current slot center
	current := positions[currentIdx]
	cx := current.X + current.Width/2
	cy := current.Y + current.Height/2

	// Find the best candidate in the direction of movement
	bestIdx := -1
	bestDist := -1

	for i, slot := range positions {
		if i == currentIdx {
			continue
		}

		slotCx := slot.X + slot.Width/2
		slotCy := slot.Y + slot.Height/2

		// Check if this slot is in the direction of movement
		inDirection := false
		switch dir {
		case DirUp:
			inDirection = slotCy < cy
		case DirDown:
			inDirection = slotCy > cy
		case DirLeft:
			inDirection = slotCx < cx
		case DirRight:
			inDirection = slotCx > cx
		}

		if !inDirection {
			continue
		}

		// Use Manhattan distance to find closest slot in direction
		dist := abs(slotCx-cx) + abs(slotCy-cy)
		if bestIdx == -1 || dist < bestDist {
			bestDist = dist
			bestIdx = i
		}
	}

	if bestIdx >= 0 {
		return bestIdx
	}

	// No slot found in direction - wrap around to opposite edge
	// Find the slot furthest in the opposite direction, preferring same row/col
	bestIdx = -1
	bestScore := 0

	for i, slot := range positions {
		if i == currentIdx {
			continue
		}

		slotCx := slot.X + slot.Width/2
		slotCy := slot.Y + slot.Height/2

		// Calculate score: prioritize same row/col, then distance to edge
		var score int
		crossDist := 0

		switch dir {
		case DirUp:
			// Wrapping up -> go to bottom. Want max Y, min X difference
			crossDist = abs(slotCx - cx)
			score = slotCy*10000 - crossDist
		case DirDown:
			// Wrapping down -> go to top. Want min Y, min X difference
			crossDist = abs(slotCx - cx)
			score = -slotCy*10000 - crossDist
		case DirLeft:
			// Wrapping left -> go to right. Want max X, min Y difference
			crossDist = abs(slotCy - cy)
			score = slotCx*10000 - crossDist
		case DirRight:
			// Wrapping right -> go to left. Want min X, min Y difference
			crossDist = abs(slotCy - cy)
			score = -slotCx*10000 - crossDist
		}

		if bestIdx == -1 || score > bestScore {
			bestScore = score
			bestIdx = i
		}
	}

	if bestIdx >= 0 {
		return bestIdx
	}

	return currentIdx
}

// NavigateTerminal cycles through terminals in a list.
// For PhaseSelecting: cycles through available terminals.
// Wraps around at edges.
func NavigateTerminal(currentIdx int, dir Direction, count int) int {
	if count <= 0 {
		return 0
	}
	if count == 1 {
		return 0
	}

	// For terminal selection, Up/Left = previous, Down/Right = next
	switch dir {
	case DirUp, DirLeft:
		return (currentIdx - 1 + count) % count
	case DirDown, DirRight:
		return (currentIdx + 1) % count
	}

	return currentIdx
}

// FindClosestSlot finds the slot index whose position is closest to the given point.
// Used when entering grabbed mode to start at the current window's slot.
func FindClosestSlot(x, y int, state *State) int {
	if len(state.SlotPositions) == 0 {
		return 0
	}

	bestIdx := 0
	bestDist := -1

	for i, slot := range state.SlotPositions {
		// Calculate center of slot
		cx := slot.X + slot.Width/2
		cy := slot.Y + slot.Height/2

		// Manhattan distance (simpler and sufficient for this use case)
		dist := abs(x-cx) + abs(y-cy)

		if bestDist < 0 || dist < bestDist {
			bestDist = dist
			bestIdx = i
		}
	}

	return bestIdx
}

// FindTerminalAtSlot returns the index of the terminal at the given slot, or -1 if empty.
func FindTerminalAtSlot(slotIdx int, state *State) int {
	for i, term := range state.Terminals {
		if term.SlotIdx == slotIdx {
			return i
		}
	}
	return -1
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
