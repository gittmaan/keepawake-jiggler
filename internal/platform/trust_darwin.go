//go:build darwin

package platform

import (
	"fmt"
	"os"
)

// WarnIfNotTrusted prints a warning when the process cannot post synthetic
// pointer events, because macOS silently drops them.
func WarnIfNotTrusted() {
	if !trustedForAccessibility() {
		fmt.Fprintln(os.Stderr,
			"warning: this process is not trusted for Accessibility, so pointer nudges will be ignored")
		fmt.Fprintln(os.Stderr,
			"         grant it in System Settings > Privacy & Security > Accessibility")
	}
}
