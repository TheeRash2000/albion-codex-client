//go:build windows

package client

import (
	"os"
	"os/exec"
	"syscall"

	"github.com/ao-data/albiondata-client/log"
)

const autostartRunKey = `HKCU\Software\Microsoft\Windows\CurrentVersion\Run`
const autostartName = "CodexClient"

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
	cmd := exec.Command("reg", "add", autostartRunKey,
		"/v", autostartName, "/t", "REG_SZ", "/d", exe, "/f")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if err := cmd.Run(); err != nil {
		log.Debugf("Autostart: could not register: %v", err)
		return
	}
	log.Info("This client will now start automatically when you log in to Windows.")
}
