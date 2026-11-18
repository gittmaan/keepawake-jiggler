package platform

import (
	"fmt"
	"os"
	"time"

	"keepawake-jiggler/internal/core"
)

// focusNudger is implemented by backends that can deliver real, HID-level
// input to a target by briefly bringing it to the front (macOS only).
type focusNudger interface {
	FocusNudge(t core.Target, key string) error
	CheckKey(key string) error
}

// focusBackend routes the runner's targeted nudge through FocusNudge, so
// the core run loop needs no changes. Errors are logged because the runner
// deliberately ignores nudge failures.
type focusBackend struct {
	core.Backend
	fn  focusNudger
	key string
}

// JiggleTarget ignores the computed point: FocusNudge aims at the centre of
// the target's current largest window instead.
func (f focusBackend) JiggleTarget(t core.Target, _ core.Point) error {
	err := f.fn.FocusNudge(t, f.key)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s focus nudge failed: %v\n", time.Now().Format("15:04:05"), err)
	}
	return err
}

// WithFocusNudge wraps b so that targeted nudges use focus-based HID input.
// It must be applied after the target has been resolved, because the wrapper
// hides the per-platform target lookup.
func WithFocusNudge(b core.Backend, key string) (core.Backend, error) {
	fn, ok := b.(focusNudger)
	if !ok {
		return nil, fmt.Errorf("-hid-focus is only supported on macOS")
	}
	if err := fn.CheckKey(key); err != nil {
		return nil, err
	}
	return focusBackend{Backend: b, fn: fn, key: key}, nil
}
