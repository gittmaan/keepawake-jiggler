package core

import (
	"context"
	"errors"
	"testing"
	"time"
)

// fakeBackend records what the runner did to it.
type fakeBackend struct {
	acquired     int
	released     int
	acquireErr   error
	displaySleep bool
	jiggles      int
	jiggleErr    error
}

func (f *fakeBackend) Acquire(preventDisplaySleep bool) (func(), error) {
	if f.acquireErr != nil {
		return nil, f.acquireErr
	}
	f.acquired++
	f.displaySleep = preventDisplaySleep
	return func() { f.released++ }, nil
}
func (f *fakeBackend) PointerLocation() (Point, error) { return Point{X: 500, Y: 500}, nil }
func (f *fakeBackend) DisplayBounds() ([]Rect, error) {
	return []Rect{{MinX: 0, MinY: 0, MaxX: 1000, MaxY: 1000}}, nil
}
func (f *fakeBackend) JiggleGlobal(Point) error {
	f.jiggles++
	return f.jiggleErr
}
func (f *fakeBackend) JiggleTarget(Target, Point) error { return nil }
func (f *fakeBackend) ListTargets() ([]Target, error)   { return nil, nil }

// TestRunReleasesOnCancel checks that the assertion is released when the
// context is cancelled, including when the run never nudges.
func TestRunReleasesOnCancel(t *testing.T) {
	b := &fakeBackend{}
	r := &Runner{Backend: b}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := r.Run(ctx, Options{}, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("Run = %v, want context.Canceled", err)
	}
	if b.acquired != 1 || b.released != 1 {
		t.Errorf("acquired=%d released=%d, want 1 and 1", b.acquired, b.released)
	}
}

// TestRunWhileReleasesOnGateError checks that the assertion is released even
// when the gate fails — the path with no other test coverage.
func TestRunWhileReleasesOnGateError(t *testing.T) {
	b := &fakeBackend{}
	r := &Runner{Backend: b}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ticks := make(chan time.Time, 2)
	ticks <- time.Now() // gate holds → assertion taken
	ticks <- time.Now() // gate errors → session ends, assertion must be released
	close(ticks)

	calls := 0
	gateErr := errors.New("gate failed")
	err := r.RunWhile(ctx, Options{Jiggle: true, Interval: time.Second}, ticks,
		func() (bool, error) {
			calls++
			if calls == 1 {
				return true, nil
			}
			return false, gateErr
		})
	if !errors.Is(err, gateErr) {
		t.Fatalf("RunWhile = %v, want the gate error", err)
	}
	if b.acquired != 1 {
		t.Errorf("acquired=%d, want one acquire before the gate error", b.acquired)
	}
	if b.released != 1 {
		t.Errorf("released=%d, want the assertion released when the gate errors", b.released)
	}
}

// TestRunWhileTogglesAssertionWithGate checks that the assertion is taken
// while the gate holds and dropped when it does not.
func TestRunWhileTogglesAssertionWithGate(t *testing.T) {
	b := &fakeBackend{}
	r := &Runner{Backend: b}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ticks := make(chan time.Time, 3)
	for range 3 {
		ticks <- time.Now()
	}
	close(ticks)
	idx := 0
	err := r.RunWhile(ctx, Options{Jiggle: true, Interval: time.Second, PreventDisplaySleep: true}, ticks,
		func() (bool, error) {
			// Hold on the first tick, drop on the second, hold on the third.
			want := []bool{true, false, true}
			w := want[idx]
			idx++
			return w, nil
		})
	if err != nil {
		t.Fatalf("RunWhile: %v", err)
	}
	// Held after tick 1, dropped after tick 2, taken again after tick 3.
	if b.acquired != 2 || b.released != 2 {
		t.Errorf("acquired=%d released=%d, want 2 and 2", b.acquired, b.released)
	}
	if !b.displaySleep {
		t.Error("expected the display-sleep flag to be forwarded to Acquire")
	}
}

// TestNudgeFailureDoesNotEndSession checks that a failed nudge is swallowed.
func TestNudgeFailureDoesNotEndSession(t *testing.T) {
	b := &fakeBackend{jiggleErr: errors.New("post failed")}
	r := &Runner{Backend: b}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ticks := make(chan time.Time, 2)
	ticks <- time.Now()
	ticks <- time.Now()
	close(ticks)

	if err := r.Run(ctx, Options{Jiggle: true, Interval: time.Second}, ticks); err != nil {
		t.Fatalf("Run = %v, want no error: nudges must not end a session", err)
	}
	if b.jiggles != 2 {
		t.Errorf("jiggles=%d, want both ticks attempted", b.jiggles)
	}
}
