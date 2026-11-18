//go:build darwin || windows

package platform

import (
	"fmt"

	"keepawake-jiggler/internal/core"
)

// New returns the backend for the current operating system.
func New() core.Backend {
	return &Backend{}
}

// IsFocused reports whether the given target currently has focus.
func IsFocused(target core.Target) (bool, error) {
	if target.PID == 0 {
		return false, fmt.Errorf("target has no process id")
	}
	return isFocused(target)
}
