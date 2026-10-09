//go:build windows

package client

import (
	"golang.org/x/sys/windows"

	"github.com/ao-data/albiondata-client/log"
)

// A second launch shows the running client's window.
//
// Only one client may run (single_instance_win.go), and it usually starts
// quietly at login. A player who then double-clicks the exe used to get a toast
// pointing at the tray, so it looked like nothing happened. Now the running
// client waits on a named event, and a second launch signals it to open (or
// raise) its window, then exits. Same session namespace as the instance mutex.
// A variable only so tests use their own name and never signal a real client
// running on the same machine.
var showWindowEventName = "CodexClientShowWindow"

// listenForShowWindow calls show each time another launch signals the event.
// Best-effort: if the event cannot be created, a second launch falls back to the
// tray toast.
func listenForShowWindow(show func()) {
	name, err := windows.UTF16PtrFromString(showWindowEventName)
	if err != nil {
		return
	}
	// Auto-reset, initially unset: one signal opens the window once.
	h, err := windows.CreateEvent(nil, 0, 0, name)
	if h == 0 {
		log.Debugf("Show-window event unavailable (%v); a second launch will show a toast instead.", err)
		return
	}
	go func() {
		for {
			ev, err := windows.WaitForSingleObject(h, windows.INFINITE)
			if err != nil || ev != windows.WAIT_OBJECT_0 {
				log.Debugf("Show-window event wait ended (%v).", err)
				return
			}
			show()
		}
	}()
}

// signalShowWindow asks the running client to show its window. Reports whether
// the signal was delivered (false: no running client is listening yet).
func signalShowWindow() bool {
	name, err := windows.UTF16PtrFromString(showWindowEventName)
	if err != nil {
		return false
	}
	h, err := windows.OpenEvent(windows.EVENT_MODIFY_STATE, false, name)
	if err != nil || h == 0 {
		return false
	}
	defer windows.CloseHandle(h)
	// This launch is the foreground app (the player just started it). Let the
	// running client take the foreground, or Windows only flashes its taskbar
	// button instead of raising the window.
	allowSetForegroundWindow()
	return windows.SetEvent(h) == nil
}

var procAllowSetForegroundWindow = windows.NewLazySystemDLL("user32.dll").NewProc("AllowSetForegroundWindow")

// asfwAny is ASFW_ANY: any process may set the foreground window.
const asfwAny = ^uint32(0)

func allowSetForegroundWindow() {
	_, _, _ = procAllowSetForegroundWindow.Call(uintptr(asfwAny))
}
