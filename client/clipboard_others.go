//go:build !windows

package client

// copyToClipboard is a no-op off Windows: the Linux build is the headless CLI
// (systray.Run is already a stub there), so there is no UI showing a pairing
// code that a clipboard copy would serve. Reporting false is the honest answer -
// it makes the caller tell the player to type the code.
func copyToClipboard(string) bool { return false }
