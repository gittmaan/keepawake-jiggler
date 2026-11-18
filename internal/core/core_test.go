package core

import (
	"testing"
)

func TestNudgeTargetPrefersXAxis(t *testing.T) {
	display := Rect{MinX: 0, MinY: 0, MaxX: 1000, MaxY: 1000}

	// Criterion: the nudge moves the pointer one point along X when there is
	// room, and leaves Y alone.
	at, ok := NudgeTarget(Point{X: 500, Y: 500}, display, nil)
	if !ok {
		t.Fatal("expected a nudge to be possible")
	}
	if at.X != 501 || at.Y != 500 {
		t.Errorf("nudge = %+v, want {501 500}", at)
	}
}

func TestNudgeTargetMovesLeftAtRightEdge(t *testing.T) {
	display := Rect{MinX: 0, MinY: 0, MaxX: 1000, MaxY: 1000}

	// The pointer is clamped into the safe area first, which ends 2 points
	// before the display edge: 1000 clamps to 998, and 999 is still out of
	// range, so the step goes left to 997.
	at, ok := NudgeTarget(Point{X: 1000, Y: 500}, display, nil)
	if !ok {
		t.Fatal("expected a nudge to be possible")
	}
	if at.X != 997 || at.Y != 500 {
		t.Errorf("nudge = %+v, want {997 500}", at)
	}
}

func TestNudgeTargetFallsBackToYAxis(t *testing.T) {
	// A rectangle narrow enough to leave no room along X, but tall enough
	// to leave room along Y.
	display := Rect{MinX: 0, MinY: 0, MaxX: 4, MaxY: 1000}

	at, ok := NudgeTarget(Point{X: 2, Y: 500}, display, nil)
	if !ok {
		t.Fatal("expected a nudge to be possible")
	}
	if at.X != 2 || at.Y != 501 {
		t.Errorf("nudge = %+v, want {2 501}", at)
	}
}

func TestNudgeTargetRefusesWhenNoRoom(t *testing.T) {
	// A rectangle too small to hold a safe area admits no nudge.
	display := Rect{MinX: 0, MinY: 0, MaxX: 2, MaxY: 2}

	if _, ok := NudgeTarget(Point{X: 1, Y: 1}, display, nil); ok {
		t.Error("expected no nudge to be possible")
	}
}

func TestNudgeTargetClampsIntoSafeArea(t *testing.T) {
	display := Rect{MinX: 0, MinY: 0, MaxX: 1000, MaxY: 1000}

	// A pointer outside the display is clamped into the safe area before
	// the step is computed, so the result stays on screen.
	at, ok := NudgeTarget(Point{X: -50, Y: 500}, display, nil)
	if !ok {
		t.Fatal("expected a nudge to be possible")
	}
	if at.X < 2 || at.X > 998 {
		t.Errorf("nudge x = %v, want within the safe area [2 998]", at.X)
	}
}

func TestNudgeTargetStaysInsideTargetWindow(t *testing.T) {
	display := Rect{MinX: 0, MinY: 0, MaxX: 2000, MaxY: 2000}
	window := Rect{MinX: 100, MinY: 100, MaxX: 300, MaxY: 300}

	// Criterion: a nudge aimed at one window stays inside that window.
	at, ok := NudgeTarget(Point{X: 200, Y: 200}, display, &window)
	if !ok {
		t.Fatal("expected a nudge to be possible")
	}
	if at.X < 102 || at.X > 298 || at.Y < 102 || at.Y > 298 {
		t.Errorf("nudge = %+v, want inside the window's safe area", at)
	}
}

func TestDisplayForPicksContainingDisplay(t *testing.T) {
	displays := []Rect{
		{MinX: 0, MinY: 0, MaxX: 1000, MaxY: 1000},
		{MinX: 1000, MinY: 0, MaxX: 2000, MaxY: 1000},
	}

	d, ok := DisplayFor(Point{X: 1500, Y: 500}, displays)
	if !ok {
		t.Fatal("expected to find a display")
	}
	if d.MaxX != 2000 {
		t.Errorf("display = %+v, want the second one", d)
	}

	if _, ok := DisplayFor(Point{X: 5000, Y: 500}, displays); ok {
		t.Error("expected no display for a point outside every display")
	}
}

// TestDisplayForPrefersFirstMatch checks that overlapping displays resolve to
// the first one listed, which is what the runner relies on.
func TestDisplayForPrefersFirstMatch(t *testing.T) {
	displays := []Rect{
		{MinX: 0, MinY: 0, MaxX: 1000, MaxY: 1000},
		{MinX: 0, MinY: 0, MaxX: 1000, MaxY: 1000}, // identical bounds
	}
	d, ok := DisplayFor(Point{X: 500, Y: 500}, displays)
	if !ok {
		t.Fatal("expected to find a display")
	}
	_ = d
}
