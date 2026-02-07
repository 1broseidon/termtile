package tiling

import (
	"testing"

	"github.com/1broseidon/termtile/internal/config"
)

func TestCalculatePositionsWithLayout_MaxTerminalWidthDoesNotCompressGrid(t *testing.T) {
	layout := &config.Layout{
		Mode: config.LayoutModeFixed,
		FixedGrid: config.FixedGrid{
			Rows: 1,
			Cols: 2,
		},
		TileRegion: config.TileRegion{Type: config.RegionFull},
		// Smaller than available slot width.
		MaxTerminalWidth: 50,
	}
	monitor := Rect{X: 0, Y: 0, Width: 210, Height: 100}

	positions, err := CalculatePositionsWithLayout(2, monitor, layout, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(positions) != 2 {
		t.Fatalf("expected 2 positions, got %d", len(positions))
	}

	// With width=210, gap=10, cols=2:
	// total gaps = 30, slotWidth=(210-30)/2=90, windowWidth=50, center offset=(90-50)/2=20
	// x0 = 10 + 0*(90+10) + 20 = 30
	// x1 = 10 + 1*(90+10) + 20 = 130
	if positions[0].X != 30 {
		t.Fatalf("expected pos0.X=30, got %d", positions[0].X)
	}
	if positions[1].X != 130 {
		t.Fatalf("expected pos1.X=130, got %d", positions[1].X)
	}
	if positions[0].Width != 50 || positions[1].Width != 50 {
		t.Fatalf("expected both widths to be 50, got %d and %d", positions[0].Width, positions[1].Width)
	}
}

func TestCalculatePositionsWithLayout_ErrorsWhenInsufficientSpace(t *testing.T) {
	layout := &config.Layout{
		Mode: config.LayoutModeFixed,
		FixedGrid: config.FixedGrid{
			Rows: 1,
			Cols: 2,
		},
		TileRegion: config.TileRegion{Type: config.RegionFull},
	}
	monitor := Rect{X: 0, Y: 0, Width: 20, Height: 10}

	_, err := CalculatePositionsWithLayout(2, monitor, layout, 20)
	if err == nil {
		t.Fatalf("expected error for insufficient space")
	}
}

func TestApplyRegion_CustomClampsToMinimumSize(t *testing.T) {
	monitor := Rect{X: 0, Y: 0, Width: 10, Height: 10}
	region := config.TileRegion{
		Type:          config.RegionCustom,
		XPercent:      0,
		YPercent:      0,
		WidthPercent:  1,
		HeightPercent: 1,
	}

	adjusted := ApplyRegion(monitor, region)
	if adjusted.Width != 1 || adjusted.Height != 1 {
		t.Fatalf("expected 1x1, got %dx%d", adjusted.Width, adjusted.Height)
	}
}
