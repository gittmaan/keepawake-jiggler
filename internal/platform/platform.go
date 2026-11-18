// Package platform holds the operating-system-specific half of the
// keep-awake process: the power assertions, the pointer nudge, and the list
// of windows a nudge can be aimed at.
//
// Nothing in this package needs administrator or root rights.
package platform

import (
	"fmt"
	"runtime"

	"keepawake-jiggler/internal/core"
)

// TargetByPID finds the window owned by a process.
func TargetByPID(backend core.Backend, pid int32) (core.Target, bool) {
	if b, ok := backend.(targetFinder); ok {
		return b.targetByPID(pid)
	}
	return core.Target{}, false
}

// targetFinder is implemented by the per-platform backends.
type targetFinder interface {
	targetByPID(pid int32) (core.Target, bool)
}

// unsupported is the backend for platforms this tool does not cover.
type unsupported struct{}

func (unsupported) Acquire(bool) (func(), error) {
	return nil, fmt.Errorf("keep-awake is not supported on %s", runtime.GOOS)
}
func (unsupported) PointerLocation() (core.Point, error) {
	return core.Point{}, fmt.Errorf("pointer control is not supported on %s", runtime.GOOS)
}
func (unsupported) DisplayBounds() ([]core.Rect, error) {
	return nil, fmt.Errorf("pointer control is not supported on %s", runtime.GOOS)
}
func (unsupported) JiggleGlobal(core.Point) error {
	return fmt.Errorf("pointer control is not supported on %s", runtime.GOOS)
}
func (unsupported) JiggleTarget(core.Target, core.Point) error {
	return fmt.Errorf("pointer control is not supported on %s", runtime.GOOS)
}
func (unsupported) ListTargets() ([]core.Target, error) {
	return nil, fmt.Errorf("pointer control is not supported on %s", runtime.GOOS)
}

// MatchesApp reports whether a target's application name or window title
// matches a query, ignoring case.
func MatchesApp(t core.Target, query string) bool {
	return matchesApp(t, query)
}
