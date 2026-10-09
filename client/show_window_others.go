//go:build !windows

package client

// listenForShowWindow is a no-op on non-Windows platforms (no panel window).
func listenForShowWindow(show func()) {}

// signalShowWindow is a no-op on non-Windows platforms.
func signalShowWindow() bool { return false }
