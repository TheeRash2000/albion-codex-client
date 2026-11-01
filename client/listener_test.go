package client

import "testing"

// A device that can't be opened for capture (e.g. a Wintun-based VPN
// adapter on Windows, or simply a nonexistent name) must not crash the
// whole process - startOnline runs on its own unrecovered goroutine (see
// albionProcessWatcher.createListeners), so a panic here previously took
// down capture on every other interface too.
func TestListener_StartOnline_UnopenableDeviceDoesNotPanic(t *testing.T) {
	l := newListener(newRouter())

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("startOnline panicked on an unopenable device: %v", r)
		}
	}()

	l.startOnline("this-device-does-not-exist", 5056)

	if l.handle != nil {
		t.Fatalf("expected handle to remain nil after a failed open, got %+v", l.handle)
	}
}

func TestListener_Stop_NilHandleDoesNotPanic(t *testing.T) {
	l := newListener(newRouter())

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("stop panicked on a listener whose handle was never opened: %v", r)
		}
	}()

	l.stop()
}

// Windows 11 URO coalesces market-order bursts into packets of up to ~14KB
// (upstream 1bf2769); this fork captured 2048 bytes and silently lost them.
func TestLiveCaptureSnapLen_FitsCoalescedMarketPackets(t *testing.T) {
	if liveCaptureSnapLen < 65535 {
		t.Fatalf("liveCaptureSnapLen = %d, must be at least 65535 so coalesced market responses are not truncated", liveCaptureSnapLen)
	}
}
