//go:build windows

package client

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/ao-data/albiondata-client/log"
)

const autostartRunKey = `HKCU\Software\Microsoft\Windows\CurrentVersion\Run`
const autostartName = "CodexClient"

// isVolatileExePath reports whether exe lives in a folder Windows can wipe between
// sessions (a temp dir). Registering autostart to such a path is how the entry
// ended up pointing at C:\Windows\Temp\codexNNNN\codex-client.exe - which Storage
// Sense / Disk Cleanup can delete, leaving autostart launching nothing.
func isVolatileExePath(exe string) bool {
	dir := strings.ToLower(filepath.Dir(exe))
	roots := []string{
		strings.ToLower(os.TempDir()),      // per-process TEMP (incl. C:\Windows\Temp when elevated)
		`c:\windows\temp`,                  // system temp, regardless of this process's TEMP
		strings.ToLower(os.Getenv("TEMP")), // user TEMP
		strings.ToLower(os.Getenv("TMP")),  // user TMP
	}
	for _, r := range roots {
		if r != "" && strings.HasPrefix(dir, r) {
			return true
		}
	}
	return false
}

// ensureAutostart registers the client to launch at Windows login (HKCU Run key),
// so it is already capturing before the player logs into Albion - the game only
// sends the Destiny Board (and market data) while the client is running. It points
// the entry at the CURRENT exe path and overwrites any prior value, so moving the
// exe just updates it on the next run. Best-effort; failures are non-fatal.
func ensureAutostart() {
	exe, err := os.Executable()
	if err != nil {
		log.Debugf("Autostart: could not resolve exe path: %v", err)
		return
	}
	// Never point autostart at a volatile temp copy: Windows can purge it, after
	// which login launches nothing. Leave any existing (stable) entry untouched.
	if isVolatileExePath(exe) {
		log.Debugf("Autostart: running from a temp folder (%s); leaving the existing autostart entry untouched.", exe)
		return
	}
	cmd := exec.Command("reg", "add", autostartRunKey,
		"/v", autostartName, "/t", "REG_SZ", "/d", exe, "/f")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if err := cmd.Run(); err != nil {
		log.Debugf("Autostart: could not register: %v", err)
		return
	}
	log.Info("This client will now start automatically when you log in to Windows.")
}
