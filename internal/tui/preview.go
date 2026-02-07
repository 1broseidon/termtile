package tui

import (
	"fmt"
	"strings"

	"github.com/1broseidon/termtile/internal/config"
	"github.com/1broseidon/termtile/internal/tiling"
)

func summarizeLayout(layout *config.Layout, tileCount, gapSize int) string {
	if layout == nil {
		return ""
	}
	if tileCount < 1 {
		tileCount = 1
	}
	if gapSize < 0 {
		gapSize = 0
	}

	monitor := tiling.Rect{
		X:      0,
		Y:      0,
		Width:  1920,
		Height: 1080,
	}
	region := tiling.ApplyRegion(monitor, layout.TileRegion)

	rects, err := tiling.CalculatePositionsWithLayout(tileCount, region, layout, gapSize)
	if err != nil {
		rects = tiling.CalculatePositions(tileCount, region, gapSize)
	}
	if len(rects) == 0 {
		return "no tiles"
	}

	minW, minH := rects[0].Width, rects[0].Height
	maxW, maxH := rects[0].Width, rects[0].Height
	for _, r := range rects[1:] {
		if r.Width < minW {
			minW = r.Width
		}
		if r.Height < minH {
			minH = r.Height
		}
		if r.Width > maxW {
			maxW = r.Width
		}
		if r.Height > maxH {
			maxH = r.Height
		}
	}

	if minW == maxW && minH == maxH {
		return fmt.Sprintf("%d tiles • %d×%d px each", len(rects), minW, minH)
	}
	return fmt.Sprintf("%d tiles • min %d×%d • max %d×%d", len(rects), minW, minH, maxW, maxH)
}

// renderASCIIPreview generates an ASCII art representation of a layout.
func renderASCIIPreview(layout *config.Layout, tileCount, width, height int) []string {
	if layout == nil || width < 5 || height < 3 {
		return emptyCanvas(width, height)
	}

	// Create a character canvas
	canvas := make([][]rune, height)
	for i := range canvas {
		canvas[i] = make([]rune, width)
		for j := range canvas[i] {
			canvas[i][j] = ' '
		}
	}

	// Use the tiling math to calculate positions
	// We simulate a monitor that maps to our character canvas
	// Each character represents a "pixel" in the preview
	monitor := tiling.Rect{
		X:      0,
		Y:      0,
		Width:  width * 2,  // Scale up for better resolution
		Height: height * 2, // Scale up for better resolution
	}

	// Apply tile region
	adjustedMonitor := tiling.ApplyRegion(monitor, layout.TileRegion)

	// Calculate positions using layout math
	rects, err := tiling.CalculatePositionsWithLayout(
		tileCount,
		adjustedMonitor,
		layout,
		1, // minimal gap for preview
	)
	if err != nil {
		// Fallback to simple grid
		rects = tiling.CalculatePositions(tileCount, adjustedMonitor, 1)
	}

	// Draw each tile on the canvas
	for i, rect := range rects {
		drawTile(canvas, rect, i+1, monitor.Width, monitor.Height, width, height)
	}

	// Draw border around the entire preview area
	drawBorder(canvas, width, height)

	// Convert canvas to string lines
	lines := make([]string, height)
	for i, row := range canvas {
		lines[i] = string(row)
	}

	return lines
}

func drawTile(canvas [][]rune, rect tiling.Rect, num int, monW, monH, canvasW, canvasH int) {
	// Map rect coordinates to canvas coordinates
	x1 := rect.X * canvasW / monW
	y1 := rect.Y * canvasH / monH
	x2 := (rect.X + rect.Width) * canvasW / monW
	y2 := (rect.Y + rect.Height) * canvasH / monH

	// Clamp to canvas bounds
	if x1 < 1 {
		x1 = 1
	}
	if y1 < 1 {
		y1 = 1
	}
	if x2 >= canvasW-1 {
		x2 = canvasW - 2
	}
	if y2 >= canvasH-1 {
		y2 = canvasH - 2
	}

	// Need at least 2x2 for a tile
	if x2 <= x1 || y2 <= y1 {
		return
	}

	// Draw tile border
	for x := x1; x <= x2; x++ {
		if y1 >= 0 && y1 < canvasH {
			canvas[y1][x] = '─'
		}
		if y2 >= 0 && y2 < canvasH {
			canvas[y2][x] = '─'
		}
	}
	for y := y1; y <= y2; y++ {
		if x1 >= 0 && x1 < canvasW {
			canvas[y][x1] = '│'
		}
		if x2 >= 0 && x2 < canvasW {
			canvas[y][x2] = '│'
		}
	}

	// Draw corners
	if y1 >= 0 && y1 < canvasH && x1 >= 0 && x1 < canvasW {
		canvas[y1][x1] = '┌'
	}
	if y1 >= 0 && y1 < canvasH && x2 >= 0 && x2 < canvasW {
		canvas[y1][x2] = '┐'
	}
	if y2 >= 0 && y2 < canvasH && x1 >= 0 && x1 < canvasW {
		canvas[y2][x1] = '└'
	}
	if y2 >= 0 && y2 < canvasH && x2 >= 0 && x2 < canvasW {
		canvas[y2][x2] = '┘'
	}

	// Draw tile number in center
	centerY := (y1 + y2) / 2
	centerX := (x1 + x2) / 2
	if centerY > y1 && centerY < y2 && centerX > x1 && centerX < x2 {
		label := fmt.Sprintf("%d", num)
		startX := centerX - len(label)/2
		for i, r := range label {
			if startX+i > x1 && startX+i < x2 {
				canvas[centerY][startX+i] = r
			}
		}
	}
}

func drawBorder(canvas [][]rune, width, height int) {
	// Top and bottom borders
	for x := 0; x < width; x++ {
		canvas[0][x] = '═'
		canvas[height-1][x] = '═'
	}

	// Left and right borders
	for y := 0; y < height; y++ {
		canvas[y][0] = '║'
		canvas[y][width-1] = '║'
	}

	// Corners
	canvas[0][0] = '╔'
	canvas[0][width-1] = '╗'
	canvas[height-1][0] = '╚'
	canvas[height-1][width-1] = '╝'
}

func emptyCanvas(width, height int) []string {
	lines := make([]string, height)
	empty := strings.Repeat(" ", width)
	for i := range lines {
		lines[i] = empty
	}
	return lines
}
