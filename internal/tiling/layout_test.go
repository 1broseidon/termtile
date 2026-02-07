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

func masterStackLayout(maxRows, maxCols int) *config.Layout {
	return &config.Layout{
		Mode:       config.LayoutModeMasterStack,
		TileRegion: config.TileRegion{Type: config.RegionFull},
		MasterStack: config.MasterStack{
			MasterWidthPercent: 60,
			MaxStackRows:       maxRows,
			MaxStackCols:       maxCols,
		},
	}
}

func TestMasterStack_1Window(t *testing.T) {
	// 1 window: master pinned left at MasterWidthPercent (40%), no auto-expand
	monitor := Rect{X: 0, Y: 0, Width: 1000, Height: 600}
	layout := masterStackLayout(3, 2)

	positions, err := CalculatePositionsWithLayout(1, monitor, layout, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(positions) != 1 {
		t.Fatalf("expected 1 position, got %d", len(positions))
	}
	// masterWidth = (1000*60/100) - 10 = 590
	want := Rect{X: 10, Y: 10, Width: 590, Height: 580}
	if positions[0] != want {
		t.Fatalf("master position: got %+v, want %+v", positions[0], want)
	}
}

func TestMasterStack_2Windows(t *testing.T) {
	// 2 windows: master left, 1 stack right
	monitor := Rect{X: 0, Y: 0, Width: 1000, Height: 600}
	layout := masterStackLayout(3, 2)

	positions, err := CalculatePositionsWithLayout(2, monitor, layout, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(positions) != 2 {
		t.Fatalf("expected 2 positions, got %d", len(positions))
	}

	// masterWidth = 1000*60/100 - 10 = 590
	// Master: X=10, Y=10, W=590, H=580
	wantMaster := Rect{X: 10, Y: 10, Width: 590, Height: 580}
	if positions[0] != wantMaster {
		t.Fatalf("master: got %+v, want %+v", positions[0], wantMaster)
	}

	// rightStartX = 590 + 20 = 610
	// rightRegionWidth = 1000 - 590 - 30 = 380
	// 1 stack slot: cols=1, rows=1, cellWidth=380, cellHeight=580
	wantStack := Rect{X: 610, Y: 10, Width: 380, Height: 580}
	if positions[1] != wantStack {
		t.Fatalf("stack[0]: got %+v, want %+v", positions[1], wantStack)
	}
}

func TestMasterStack_4Windows(t *testing.T) {
	// 4 windows with MaxStackRows=2: 3 stack agents → ceil(3/2)=2 cols
	monitor := Rect{X: 0, Y: 0, Width: 1000, Height: 600}
	layout := masterStackLayout(2, 2) // MaxStackRows=2 triggers 2 cols for 3 agents

	positions, err := CalculatePositionsWithLayout(4, monitor, layout, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(positions) != 4 {
		t.Fatalf("expected 4 positions, got %d", len(positions))
	}

	// masterWidth = 590, rightStartX = 610, rightRegionWidth = 380
	// stackCount=3, stackCols=ceil(3/2)=2, stackRows=ceil(3/2)=2
	// cellWidth = (380 - 10) / 2 = 185
	// cellHeight = (580 - 10) / 2 = 285
	wantMaster := Rect{X: 10, Y: 10, Width: 590, Height: 580}
	if positions[0] != wantMaster {
		t.Fatalf("master: got %+v, want %+v", positions[0], wantMaster)
	}

	// Stack[0] (r=0,c=0): X=610, Y=10
	if positions[1] != (Rect{X: 610, Y: 10, Width: 185, Height: 285}) {
		t.Fatalf("stack[0]: got %+v", positions[1])
	}
	// Stack[1] (r=0,c=1): X=610+185+10=805, Y=10
	if positions[2] != (Rect{X: 805, Y: 10, Width: 185, Height: 285}) {
		t.Fatalf("stack[1]: got %+v", positions[2])
	}
	// Stack[2] (r=1,c=0): X=610, Y=10+285+10=305
	if positions[3] != (Rect{X: 610, Y: 305, Width: 185, Height: 285}) {
		t.Fatalf("stack[2]: got %+v", positions[3])
	}
}

func TestMasterStack_7Windows(t *testing.T) {
	// 7 windows: master + 6 stack in 3x2 grid (max capacity)
	monitor := Rect{X: 0, Y: 0, Width: 1000, Height: 600}
	layout := masterStackLayout(3, 2)

	positions, err := CalculatePositionsWithLayout(7, monitor, layout, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(positions) != 7 {
		t.Fatalf("expected 7 positions, got %d", len(positions))
	}

	// masterWidth = 590, rightStartX = 610, rightRegionWidth = 380
	// stackCount=6, stackCols=ceil(6/3)=2, stackRows=ceil(6/2)=3
	// cellWidth = (380 - 10) / 2 = 185
	// cellHeight = (580 - 20) / 3 = 186
	wantMaster := Rect{X: 10, Y: 10, Width: 590, Height: 580}
	if positions[0] != wantMaster {
		t.Fatalf("master: got %+v, want %+v", positions[0], wantMaster)
	}

	// Verify all 6 stack slots fill a 3-row x 2-col grid
	expectedStack := []Rect{
		{X: 610, Y: 10, Width: 185, Height: 186},   // r=0,c=0
		{X: 805, Y: 10, Width: 185, Height: 186},   // r=0,c=1
		{X: 610, Y: 206, Width: 185, Height: 186},  // r=1,c=0
		{X: 805, Y: 206, Width: 185, Height: 186},  // r=1,c=1
		{X: 610, Y: 402, Width: 185, Height: 186},  // r=2,c=0
		{X: 805, Y: 402, Width: 185, Height: 186},  // r=2,c=1
	}
	for i, want := range expectedStack {
		got := positions[i+1]
		if got != want {
			t.Fatalf("stack[%d]: got %+v, want %+v", i, got, want)
		}
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
