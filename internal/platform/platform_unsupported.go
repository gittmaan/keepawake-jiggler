//go:build !darwin && !windows

package platform

import (
	"fmt"
	"runtime"

	"keepawake-jiggler/internal/core"
)

// New returns the backend for the current operating system.
func New() core.Backend { return unsupported{} }

// IsFocused reports whether the given target currently has focus.
func IsFocused(target core.Target) (bool, error) {
	return false, fmt.Errorf("focus is not supported on %s", runtime.GOOS)
}

// WarnIfNotTrusted is a no-op on platforms without that requirement.
func WarnIfNotTrusted() {}
