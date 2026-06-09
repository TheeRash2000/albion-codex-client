//go:build !windows

package client

// acquireSingleInstance is a no-op on non-Windows platforms (always allows start).
func acquireSingleInstance() bool { return true }
