// Package core holds the platform-independent logic of the keep-awake
// process: the pointer-nudge geometry, the target model, and the run loop.
//
// The nudge geometry is a faithful reimplementation of the "Move pointer
// slightly" feature of Vorssaint 3.4.4 (KeepAwakeManager.jiggleMousePointer).
// See docs/vorssaint-reverse-engineering.md for the evidence behind it.
package core

import (
	"context"
	"time"
)

// Point is a screen-space coordinate in points (not pixels).
type Point struct {
	X, Y float64
}

// Rect is an axis-aligned screen-space rectangle.
type Rect struct {
	MinX, MinY, MaxX, MaxY float64
}

// Inset returns r shrunk by d points on every side.
func (r Rect) Inset(d float64) Rect {
	return Rect{
		MinX: r.MinX + d,
		MinY: r.MinY + d,
		MaxX: r.MaxX - d,
		MaxY: r.MaxY - d,
	}
}

// Contains reports whether p lies inside r.
func (r Rect) Contains(p Point) bool {
	return p.X >= r.MinX && p.X <= r.MaxX && p.Y >= r.MinY && p.Y <= r.MaxY
}

// IsEmpty reports whether the rectangle has no area.
func (r Rect) IsEmpty() bool {
	return r.MinX > r.MaxX || r.MinY > r.MaxY
}

// SafeInset is the margin, in points, that Vorssaint keeps between the
// pointer and the edge of the display it is on.
const SafeInset = 2.0

// NudgeStep is the distance, in points, that the pointer is moved.
const NudgeStep = 1.0

// NudgeTarget computes the pointer position for one nudge.
//
// The pointer is first clamped into the display's safe area (the display
// bounds inset by SafeInset on every side), then stepped one point along X
// if that stays inside the safe area, otherwise one point along Y. When
// neither axis has room, no nudge is possible and ok is false.
//
// This mirrors the branch structure recovered from the disassembly of
// KeepAwakeManager.jiggleMousePointer: the X axis is preferred, and each
// candidate is tested against the inset bounds before being accepted.
//
// When limit is non-nil the pointer is confined to that rectangle instead of
// the display, so a nudge aimed at one window stays inside that window.
func NudgeTarget(cur Point, display Rect, limit *Rect) (at Point, ok bool) {
	safe := display
	if limit != nil && !limit.IsEmpty() {
		safe = *limit
	}
	safe = safe.Inset(SafeInset)
	if safe.IsEmpty() {
		return cur, false
	}

	x := clamp(cur.X, safe.MinX, safe.MaxX)
	y := clamp(cur.Y, safe.MinY, safe.MaxY)

	switch {
	case x+NudgeStep <= safe.MaxX:
		return Point{X: x + NudgeStep, Y: y}, true
	case x-NudgeStep >= safe.MinX:
		return Point{X: x - NudgeStep, Y: y}, true
	case y+NudgeStep <= safe.MaxY:
		return Point{X: x, Y: y + NudgeStep}, true
	case y-NudgeStep >= safe.MinY:
		return Point{X: x, Y: y - NudgeStep}, true
	}
	return cur, false
}

// DisplayFor returns the display containing p, preferring the first match.
func DisplayFor(p Point, displays []Rect) (Rect, bool) {
	for _, d := range displays {
		if d.Contains(p) {
			return d, true
		}
	}
	return Rect{}, false
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// Target identifies one application window that a nudge can be aimed at.
type Target struct {
	// PID is the owning process ID.
	PID int32
	// Handle is the platform window handle (an HWND on Windows). Zero when
	// the platform identifies windows by process only.
	Handle uintptr
	// App is the owning application name.
	App string
	// Title is the window title, when the platform exposes it.
	Title string
	// Bounds is the window rectangle in screen space, when known.
	Bounds Rect
	// Known reports whether this target identifies a concrete window.
	Known bool
}

// Backend is the set of platform operations the run loop needs.
type Backend interface {
	// Acquire takes the power assertions that keep the machine awake and
	// returns a function that releases them.
	Acquire(preventDisplaySleep bool) (release func(), err error)
	// PointerLocation reports where the pointer currently is.
	PointerLocation() (Point, error)
	// DisplayBounds reports the bounds of every attached display.
	DisplayBounds() ([]Rect, error)
	// JiggleGlobal moves the pointer system-wide.
	JiggleGlobal(at Point) error
	// JiggleTarget delivers a pointer move to one application only.
	JiggleTarget(t Target, at Point) error
	// ListTargets enumerates the windows a nudge can be aimed at.
	ListTargets() ([]Target, error)
}

// Options configures a run.
type Options struct {
	// Jiggle enables the periodic pointer nudge.
	Jiggle bool
	// Interval is how often the pointer is nudged.
	Interval time.Duration
	// Target restricts the nudge to one application. Nil means system-wide.
	Target *Target
	// PreventDisplaySleep also keeps the display awake.
	PreventDisplaySleep bool
}

// GatePollInterval is how often RunWhile re-evaluates its gate.
const GatePollInterval = time.Second

// Runner drives a Backend.
type Runner struct {
	Backend Backend
}

// Run holds the power assertions and, when asked, nudges the pointer on
// every tick until ctx is cancelled. The release function is always called.
func (r *Runner) Run(ctx context.Context, opts Options, ticks <-chan time.Time) error {
	release, err := r.Backend.Acquire(opts.PreventDisplaySleep)
	if err != nil {
		return err
	}
	defer release()

	if !opts.Jiggle {
		<-ctx.Done()
		return ctx.Err()
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case _, ok := <-ticks:
			if !ok {
				return nil
			}
			r.nudge(opts)
		}
	}
}

// RunWhile holds the power assertions only while gate reports true. It is
// used to keep the machine awake only while a chosen application is in the
// foreground. The nudge still runs on every tick.
func (r *Runner) RunWhile(ctx context.Context, opts Options, ticks <-chan time.Time, gate func() (bool, error)) error {
	// The gate has to be polled on its own cadence, otherwise watch mode
	// without -jiggle would never re-evaluate focus.
	if ticks == nil {
		t := time.NewTicker(GatePollInterval)
		defer t.Stop()
		ticks = t.C
	}

	var release func()
	held := false

	// takeOrRelease applies the gate's answer to the assertion state.
	takeOrRelease := func(want bool) error {
		if want == held {
			return nil
		}
		if want {
			rel, err := r.Backend.Acquire(opts.PreventDisplaySleep)
			if err != nil {
				return err
			}
			release, held = rel, true
			return nil
		}
		if release != nil {
			release()
			release, held = nil, false
		}
		return nil
	}
	defer func() {
		if release != nil {
			release()
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case _, ok := <-ticks:
			if !ok {
				return nil
			}
			want, err := gate()
			if err != nil {
				return err
			}
			if err := takeOrRelease(want); err != nil {
				return err
			}
			if opts.Jiggle {
				r.nudge(opts)
			}
		}
	}
}

// nudge performs one pointer nudge. Failures are ignored on purpose: a
// missed nudge must not end a keep-awake session.
func (r *Runner) nudge(opts Options) {
	cur, err := r.Backend.PointerLocation()
	if err != nil {
		return
	}
	displays, err := r.Backend.DisplayBounds()
	if err != nil {
		return
	}
	display, ok := DisplayFor(cur, displays)
	if !ok {
		return
	}

	var limit *Rect
	if opts.Target != nil && !opts.Target.Bounds.IsEmpty() {
		limit = &opts.Target.Bounds
	}
	at, ok := NudgeTarget(cur, display, limit)
	if !ok {
		return
	}
	if opts.Target != nil {
		_ = r.Backend.JiggleTarget(*opts.Target, at)
		return
	}
	_ = r.Backend.JiggleGlobal(at)
}
