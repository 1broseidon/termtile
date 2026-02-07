package tiling

import (
	"fmt"
	"math"

	"github.com/1broseidon/termtile/internal/config"
)

// Rect represents a window position and size
type Rect struct {
	X      int
	Y      int
	Width  int
	Height int
}

// CalculateGrid determines the optimal grid dimensions for the given number of windows
func CalculateGrid(numWindows int) (rows, cols int) {
	if numWindows == 0 {
		return 0, 0
	}

	// Calculate columns first (ceiling of square root)
	cols = int(math.Ceil(math.Sqrt(float64(numWindows))))

	// Calculate rows needed
	rows = int(math.Ceil(float64(numWindows) / float64(cols)))

	return rows, cols
}

// CalculatePositions computes window positions for a grid layout with gaps
func CalculatePositions(numWindows int, monitor Rect, gapSize int) []Rect {
	if numWindows == 0 {
		return nil
	}

	rows, cols := CalculateGrid(numWindows)

	// Calculate cell dimensions accounting for gaps
	// Total horizontal space: monitor.Width
	// Gaps: (cols + 1) * gapSize (one before each column, and one after)
	// Available space: monitor.Width - (cols + 1) * gapSize
	// Cell width: available space / cols

	totalHorizontalGaps := (cols + 1) * gapSize
	totalVerticalGaps := (rows + 1) * gapSize

	cellWidth := (monitor.Width - totalHorizontalGaps) / cols
	cellHeight := (monitor.Height - totalVerticalGaps) / rows

	positions := make([]Rect, numWindows)

	for i := 0; i < numWindows; i++ {
		row := i / cols
		col := i % cols

		positions[i] = Rect{
			X:      monitor.X + gapSize + col*(cellWidth+gapSize),
			Y:      monitor.Y + gapSize + row*(cellHeight+gapSize),
			Width:  cellWidth,
			Height: cellHeight,
		}
	}

	return positions
}

// CalculatePositionsWithLayout computes window positions using layout configuration
func CalculatePositionsWithLayout(
	numWindows int,
	monitor Rect,
	layout *config.Layout,
	gapSize int,
) ([]Rect, error) {
	if numWindows == 0 {
		return nil, nil
	}

	var rows, cols int
	flexibleLastRow := layout.FlexibleLastRow

	switch layout.Mode {
	case config.LayoutModeAuto:
		rows, cols = CalculateGrid(numWindows)

	case config.LayoutModeFixed:
		rows = layout.FixedGrid.Rows
		cols = layout.FixedGrid.Cols
		// Only tile up to rows*cols terminals
		if numWindows > rows*cols {
			numWindows = rows * cols
		}
		// Flexible last row doesn't apply to fixed grids
		flexibleLastRow = false

	case config.LayoutModeVertical:
		rows = numWindows
		cols = 1
		// Single column - flexible last row is meaningless
		flexibleLastRow = false

	case config.LayoutModeHorizontal:
		rows = 1
		cols = numWindows
		// Single row - flexible last row is meaningless
		flexibleLastRow = false

	default:
		return nil, fmt.Errorf("unsupported layout mode: %q", layout.Mode)
	}

	if rows <= 0 || cols <= 0 {
		return nil, fmt.Errorf("invalid grid dimensions: rows=%d cols=%d", rows, cols)
	}

	// Calculate cell dimensions with gaps
	totalHorizontalGaps := (cols + 1) * gapSize
	totalVerticalGaps := (rows + 1) * gapSize

	slotWidth := (monitor.Width - totalHorizontalGaps) / cols
	slotHeight := (monitor.Height - totalVerticalGaps) / rows

	if slotWidth <= 0 || slotHeight <= 0 {
		return nil, fmt.Errorf(
			"insufficient space for layout: monitor=%dx%d rows=%d cols=%d gap=%d (slot=%dx%d)",
			monitor.Width, monitor.Height, rows, cols, gapSize, slotWidth, slotHeight,
		)
	}

	windowWidth := slotWidth
	windowHeight := slotHeight

	// Apply max dimension constraints (within each slot)
	if layout.MaxTerminalWidth > 0 && windowWidth > layout.MaxTerminalWidth {
		windowWidth = layout.MaxTerminalWidth
	}
	if layout.MaxTerminalHeight > 0 && windowHeight > layout.MaxTerminalHeight {
		windowHeight = layout.MaxTerminalHeight
	}

	// Calculate last row info for flexible layout
	lastRowIndex := rows - 1
	windowsInLastRow := numWindows - (lastRowIndex * cols)
	if windowsInLastRow <= 0 {
		windowsInLastRow = cols // Full row
	}

	// Calculate last row dimensions if flexible
	var lastRowSlotWidth, lastRowWindowWidth int
	if flexibleLastRow && windowsInLastRow < cols && windowsInLastRow > 0 {
		// Last row has fewer windows - they expand to fill the width
		lastRowHorizontalGaps := (windowsInLastRow + 1) * gapSize
		lastRowSlotWidth = (monitor.Width - lastRowHorizontalGaps) / windowsInLastRow
		lastRowWindowWidth = lastRowSlotWidth
		if layout.MaxTerminalWidth > 0 && lastRowWindowWidth > layout.MaxTerminalWidth {
			lastRowWindowWidth = layout.MaxTerminalWidth
		}
	}

	positions := make([]Rect, numWindows)

	for i := 0; i < numWindows; i++ {
		row := i / cols
		col := i % cols

		// Check if this is on the last row and we need flexible sizing
		isLastRow := row == lastRowIndex
		useFlexible := flexibleLastRow && isLastRow && windowsInLastRow < cols

		var thisSlotWidth, thisWindowWidth int
		var x int

		if useFlexible {
			// Recalculate column index for the last row (0-based within last row)
			lastRowCol := i - (lastRowIndex * cols)
			thisSlotWidth = lastRowSlotWidth
			thisWindowWidth = lastRowWindowWidth
			x = monitor.X + gapSize + lastRowCol*(thisSlotWidth+gapSize)
		} else {
			thisSlotWidth = slotWidth
			thisWindowWidth = windowWidth
			x = monitor.X + gapSize + col*(slotWidth+gapSize)
		}

		y := monitor.Y + gapSize + row*(slotHeight+gapSize)

		// Center within the slot if terminal is smaller than available space
		if thisWindowWidth < thisSlotWidth {
			x += (thisSlotWidth - thisWindowWidth) / 2
		}
		if windowHeight < slotHeight {
			y += (slotHeight - windowHeight) / 2
		}

		positions[i] = Rect{
			X:      x,
			Y:      y,
			Width:  thisWindowWidth,
			Height: windowHeight,
		}
	}

	return positions, nil
}

// ApplyRegion applies the tile region to a monitor, returning adjusted bounds
func ApplyRegion(monitor Rect, region config.TileRegion) Rect {
	adjusted := monitor

	switch region.Type {
	case config.RegionFull:
		// No change

	case config.RegionLeftHalf:
		adjusted.Width = monitor.Width / 2

	case config.RegionRightHalf:
		adjusted.X = monitor.X + monitor.Width/2
		adjusted.Width = monitor.Width / 2

	case config.RegionTopHalf:
		adjusted.Height = monitor.Height / 2

	case config.RegionBottomHalf:
		adjusted.Y = monitor.Y + monitor.Height/2
		adjusted.Height = monitor.Height / 2

	case config.RegionCustom:
		adjusted.X = monitor.X + (monitor.Width * region.XPercent / 100)
		adjusted.Y = monitor.Y + (monitor.Height * region.YPercent / 100)
		adjusted.Width = monitor.Width * region.WidthPercent / 100
		adjusted.Height = monitor.Height * region.HeightPercent / 100
	}

	if adjusted.Width < 1 {
		adjusted.Width = 1
	}
	if adjusted.Height < 1 {
		adjusted.Height = 1
	}

	return adjusted
}
