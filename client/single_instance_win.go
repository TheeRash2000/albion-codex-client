//go:build windows

package client

import (
	"syscall"
	"unsafe"
)

const errAlreadyExists = 183 // ERROR_ALREADY_EXISTS

// singleInstanceHandle holds the named-mutex handle for the process lifetime so it
// can be released explicitly before a self-update restart (otherwise the new
// process would see this one still holding the mutex and bail as a duplicate).
var singleInstanceHandle uintptr

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
	// The handle is normally left open for the process lifetime; the OS releases
	// the mutex when this process exits. We keep it so a restart can release early.
	singleInstanceHandle = ret
	if errno, ok := callErr.(syscall.Errno); ok && errno == errAlreadyExists {
		return false
	}
	return true
}

// releaseSingleInstance relinquishes the named mutex so a freshly self-updated
// process can acquire it without racing this (exiting) one. No-op if never acquired.
func releaseSingleInstance() {
	if singleInstanceHandle == 0 {
		return
	}
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	_, _, _ = kernel32.NewProc("ReleaseMutex").Call(singleInstanceHandle)
	_, _, _ = kernel32.NewProc("CloseHandle").Call(singleInstanceHandle)
	singleInstanceHandle = 0
}
