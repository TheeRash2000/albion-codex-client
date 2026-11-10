//go:build windows

package client

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"

	"github.com/ao-data/albiondata-client/log"
	"golang.org/x/sys/windows"
)

// autostartRunSubkey is the per-user Run key, relative to HKEY_CURRENT_USER.
const autostartRunSubkey = `Software\Microsoft\Windows\CurrentVersion\Run`
const autostartName = "CodexClient"

var (
	modadvapi32         = windows.NewLazySystemDLL("advapi32.dll")
	procRegCreateKeyExW = modadvapi32.NewProc("RegCreateKeyExW")
	procRegSetValueExW  = modadvapi32.NewProc("RegSetValueExW")
)

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

// readUserRegString reads one string value under HKEY_CURRENT_USER (read-only).
func readUserRegString(subkey, name string) (string, bool) {
	keyPath, err := syscall.UTF16PtrFromString(subkey)
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

// writeUserRegString stores one REG_SZ value under HKEY_CURRENT_USER through the
// registry API. The value is passed as data, never through a command line, so
// its embedded quotes need no escaping. Earlier builds shelled out to a hidden
// reg.exe for this; an unsigned program doing that is what antivirus heuristics
// read as malware persistence, and it is not how installed apps register.
func writeUserRegString(subkey, name, value string) error {
	keyPath, err := syscall.UTF16PtrFromString(subkey)
	if err != nil {
		return err
	}
	valueName, err := syscall.UTF16PtrFromString(name)
	if err != nil {
		return err
	}
	data, err := syscall.UTF16FromString(value)
	if err != nil {
		return err
	}

	var k syscall.Handle
	// Opens the key, creating it only if it is missing (the Run key always exists).
	if rc, _, _ := procRegCreateKeyExW.Call(
		uintptr(syscall.HKEY_CURRENT_USER),
		uintptr(unsafe.Pointer(keyPath)),
		0, 0, 0,
		uintptr(syscall.KEY_SET_VALUE),
		0,
		uintptr(unsafe.Pointer(&k)),
		0,
	); rc != 0 {
		return fmt.Errorf("open HKCU\\%s: %w", subkey, syscall.Errno(rc))
	}
	defer syscall.RegCloseKey(k)

	// Size is in bytes and includes the terminating NUL.
	if rc, _, _ := procRegSetValueExW.Call(
		uintptr(k),
		uintptr(unsafe.Pointer(valueName)),
		0,
		uintptr(syscall.REG_SZ),
		uintptr(unsafe.Pointer(&data[0])),
		uintptr(len(data)*2),
	); rc != 0 {
		return fmt.Errorf("set %s: %w", name, syscall.Errno(rc))
	}
	return nil
}

// ensureAutostart registers the client to launch at Windows login (HKCU Run key),
// so it is already capturing before the player logs into Albion - the game only
// sends the Destiny Board (and market data) while the client is running. It points
// the entry at the CURRENT exe path, so moving the exe just updates it on the next
// run, and writes nothing when the entry is already correct. Best-effort; failures
// are non-fatal.
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
	want := autostartValue(exe)
	if have, ok := readUserRegString(autostartRunSubkey, autostartName); ok && have == want {
		return
	}
	if err := writeUserRegString(autostartRunSubkey, autostartName, want); err != nil {
		log.Debugf("Autostart: could not register: %v", err)
		return
	}
	log.Info("This client will now start automatically when you log in to Windows.")
}
