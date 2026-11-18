//go:build windows

package platform

import "keepawake-jiggler/internal/core"

// TargetByPID finds a visible window owned by the given process.
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
