//go:build !windows

package client

// ensureAutostart is a no-op on non-Windows platforms.
func ensureAutostart() {}
