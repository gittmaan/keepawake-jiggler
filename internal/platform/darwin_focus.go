//go:build darwin

package platform

import (
	"errors"
	"strings"

	"keepawake-jiggler/internal/core"
)

// targetByPID finds an on-screen window owned by the given process.
func (b *Backend) targetByPID(pid int32) (core.Target, bool) {
	targets, err := b.ListTargets()
	if err != nil {
		return core.Target{}, false
	}
	for _, t := range targets {
		if t.PID == pid {
			return t, true
		}
	}
	return core.Target{}, false
}

// isFocused reports whether the target application is the frontmost one.
//
// CGWindowListCopyWindowInfo returns windows front to back, so the first
// on-screen window on layer 0 belongs to the frontmost application. This
// agrees with NSWorkspace.frontmostApplication and needs no Objective-C.
func isFocused(target core.Target) (bool, error) {
	if target.PID == 0 {
		return false, errors.New("target has no process id")
	}
	front, err := frontmostAppPID()
	if err != nil {
		return false, err
	}
	return front == target.PID, nil
}

// matchesApp reports whether a target's application name or window title
// matches a query, ignoring case.
func matchesApp(t core.Target, query string) bool {
	q := strings.ToLower(query)
	return strings.Contains(strings.ToLower(t.App), q) ||
		strings.Contains(strings.ToLower(t.Title), q)
}
