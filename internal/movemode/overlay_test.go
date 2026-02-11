package movemode

import (
	"strings"
	"testing"

	"github.com/1broseidon/termtile/internal/tiling"
)

func TestHintLinesForPhaseSelectingIncludesActionKeys(t *testing.T) {
	lines := hintLinesForPhase(HintPhaseSelecting)
	text := strings.Join(lines, "\n")

	expected := []string{
		"d       delete selected",
		"n       add after selected",
		"a       append terminal",
	}
	for _, want := range expected {
		if !strings.Contains(text, want) {
			t.Fatalf("selecting hint missing %q; got:\n%s", want, text)
		}
	}
}

func TestChooseHintPositionAvoidsOverlapWhenPossible(t *testing.T) {
	bounds := tiling.Rect{X: 0, Y: 0, Width: 800, Height: 600}
	width, height := 220, 80

	// Occupy top-right so placement should choose another corner.
	avoid := []tiling.Rect{{X: 568, Y: 12, Width: 220, Height: 80}}
	x, y := chooseHintPosition(bounds, avoid, width, height)
	got := tiling.Rect{X: x, Y: y, Width: width, Height: height}

	if rectsIntersect(got, avoid[0]) {
		t.Fatalf("hint overlaps avoid rect: got=%+v avoid=%+v", got, avoid[0])
	}
	if got.X < bounds.X || got.Y < bounds.Y {
		t.Fatalf("hint escaped upper bounds: got=%+v bounds=%+v", got, bounds)
	}
	if got.X+got.Width > bounds.X+bounds.Width || got.Y+got.Height > bounds.Y+bounds.Height {
		t.Fatalf("hint escaped lower bounds: got=%+v bounds=%+v", got, bounds)
	}
}

func TestChooseHintPositionClampsOversizedHintToBoundsOrigin(t *testing.T) {
	bounds := tiling.Rect{X: 100, Y: 200, Width: 140, Height: 90}
	x, y := chooseHintPosition(bounds, nil, 260, 160)

	if x != bounds.X || y != bounds.Y {
		t.Fatalf("expected oversized hint to clamp to bounds origin (%d,%d), got (%d,%d)", bounds.X, bounds.Y, x, y)
	}
}
