//go:build windows

package platform

import (
	"errors"
	"strings"

	"keepawake-jiggler/internal/core"
)

// isFocused reports whether the target window is the foreground window.
func isFocused(target core.Target) (bool, error) {
	if target.Handle == 0 {
		return false, errors.New("target has no window handle")
	}
	fg, _, _ := procGetForegroundWindow.Call()
	return fg == target.Handle, nil
}

// matchesApp reports whether a target's application name or window title
// matches a query, ignoring case.
func matchesApp(t core.Target, query string) bool {
	q := strings.ToLower(query)
	return strings.Contains(strings.ToLower(t.App), q) ||
		strings.Contains(strings.ToLower(t.Title), q)
}
