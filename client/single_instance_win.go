//go:build windows

package client

import (
	"syscall"
	"unsafe"
)

const errAlreadyExists = 183 // ERROR_ALREADY_EXISTS

// acquireSingleInstance creates a process-lifetime named mutex. It returns false
// if another Codex Client already holds it, so the caller can reopen that
// instance's window and exit instead of spawning a duplicate (which is what
// produced multiple panel windows). Fails open (returns true) on any error so a
// mutex problem never blocks startup.
func acquireSingleInstance() bool {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	createMutex := kernel32.NewProc("CreateMutexW")
	name, err := syscall.UTF16PtrFromString("CodexClientSingleInstance")
	if err != nil {
		return true
	}
	ret, _, callErr := createMutex.Call(0, 0, uintptr(unsafe.Pointer(name)))
	if ret == 0 {
		return true
	}
	// The handle is intentionally left open for the process lifetime; the OS
	// releases the mutex when this process exits.
	if errno, ok := callErr.(syscall.Errno); ok && errno == errAlreadyExists {
		return false
	}
	return true
}
