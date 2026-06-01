//go:build windows

package client

import (
	"os"
	"path/filepath"
)

// npcapInstalled reports whether the Npcap capture driver is present. Npcap ships
// wpcap.dll either under System32\Npcap (default) or directly in System32 (WinPcap
// API-compatible mode); we check both (and the WoW64 paths) rather than trying to
// load it, so this is safe to call before any pcap function.
func npcapInstalled() bool {
	root := os.Getenv("SystemRoot")
	if root == "" {
		root = `C:\Windows`
	}
	for _, p := range []string{
		filepath.Join(root, "System32", "Npcap", "wpcap.dll"),
		filepath.Join(root, "System32", "wpcap.dll"),
		filepath.Join(root, "SysWOW64", "Npcap", "wpcap.dll"),
		filepath.Join(root, "SysWOW64", "wpcap.dll"),
	} {
		if _, err := os.Stat(p); err == nil {
			return true
		}
	}
	return false
}
