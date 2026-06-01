//go:build !windows

package client

// npcapInstalled is Windows-specific; on other platforms libpcap is a normal
// dependency, so assume capture is available.
func npcapInstalled() bool { return true }
