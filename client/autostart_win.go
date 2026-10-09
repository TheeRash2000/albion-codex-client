//go:build windows

package client

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"

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

// autostartValue is the Run-key command: the exe, quoted (paths have spaces),
// plus -background so a login launch stays in the tray instead of opening the
// window. Earlier builds stored the bare path; the next launch rewrites it.
func autostartValue(exe string) string {
	return `"` + exe + `" -` + backgroundFlag
}

// autostartRegCmdLine is the exact reg.exe command line that stores
// autostartValue. Written out rather than left to exec's argument escaping, so
// the value's embedded quotes are encoded one known way (\" inside a quoted
// argument, the rule reg.exe parses with). Tested by decoding it back.
func autostartRegCmdLine(exe string) string {
	quoted := strings.ReplaceAll(autostartValue(exe), `"`, `\"`)
	return `reg add "` + autostartRunKey + `" /v ` + autostartName + ` /t REG_SZ /d "` + quoted + `" /f`
}

// upgradedFromLegacyAutostart reports whether this machine's autostart entry was
// written by a build older than -background (0.1.16 and earlier). Read BEFORE
// ensureAutostart rewrites it.
//
// It identifies the first start after an auto-update from such a build: the OLD
// build performs the restart and passes no -background, so without this every
// upgraded client would open its window at whatever moment the update landed,
// mid-game included. Only that first start matches; it then rewrites the entry,
// and a fresh install has no entry at all, so a player's own launch still opens
// the window.
func upgradedFromLegacyAutostart() bool {
	v, ok := readRunValue(autostartName)
	return ok && isLegacyAutostartValue(v)
}

// isLegacyAutostartValue: an entry with no -background flag.
func isLegacyAutostartValue(v string) bool {
	return !strings.Contains(v, "-"+backgroundFlag)
}

// readRunValue reads one value from the HKCU Run key (read-only).
func readRunValue(name string) (string, bool) {
	keyPath, err := syscall.UTF16PtrFromString(`Software\Microsoft\Windows\CurrentVersion\Run`)
	if err != nil {
		return "", false
	}
	var k syscall.Handle
	if err := syscall.RegOpenKeyEx(syscall.HKEY_CURRENT_USER, keyPath, 0, syscall.KEY_READ, &k); err != nil {
		return "", false
	}
	defer syscall.RegCloseKey(k)

	valueName, err := syscall.UTF16PtrFromString(name)
	if err != nil {
		return "", false
	}
	var typ, size uint32
	if err := syscall.RegQueryValueEx(k, valueName, nil, &typ, nil, &size); err != nil || size == 0 {
		return "", false
	}
	if typ != syscall.REG_SZ && typ != syscall.REG_EXPAND_SZ {
		return "", false
	}
	buf := make([]uint16, size/2+1)
	if err := syscall.RegQueryValueEx(k, valueName, nil, &typ, (*byte)(unsafe.Pointer(&buf[0])), &size); err != nil {
		return "", false
	}
	return syscall.UTF16ToString(buf), true
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
	cmd := exec.Command("reg")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CmdLine: autostartRegCmdLine(exe)}
	if err := cmd.Run(); err != nil {
		log.Debugf("Autostart: could not register: %v", err)
		return
	}
	log.Info("This client will now start automatically when you log in to Windows.")
}
