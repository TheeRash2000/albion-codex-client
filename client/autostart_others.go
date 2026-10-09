//go:build !windows

package client

// ensureAutostart is a no-op on non-Windows platforms.
func ensureAutostart() {}

// upgradedFromLegacyAutostart is always false where there is no autostart entry.
func upgradedFromLegacyAutostart() bool { return false }
