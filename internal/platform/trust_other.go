//go:build !darwin

package platform

// WarnIfNotTrusted is a no-op on platforms without that requirement.
func WarnIfNotTrusted() {}
