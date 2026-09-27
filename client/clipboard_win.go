//go:build windows

package client

import (
	"syscall"
	"unsafe"

	"github.com/gonutz/w32"
)

const (
	cfUnicodeText = 13
	gmemMoveable  = 0x0002
)

// copyToClipboard puts s on the Windows clipboard as unicode text, reporting
// whether it actually landed.
//
// Uses w32, which the tray already depends on, rather than adding a clipboard
// library for one call. Every step is checked because a clipboard write can
// legitimately fail (another process holds it open), and the caller shows
// different copy depending on the outcome - telling a player their pairing code
// is on the clipboard when it is not would send them to paste nothing.
func copyToClipboard(s string) (ok bool) {
	if s == "" {
		return false
	}

	// UTF16FromString rejects an embedded NUL rather than silently truncating.
	chars, err := syscall.UTF16FromString(s)
	if err != nil {
		return false
	}

	if !w32.OpenClipboard(0) {
		return false
	}
	defer w32.CloseClipboard()

	if !w32.EmptyClipboard() {
		return false
	}

	h := w32.GlobalAlloc(gmemMoveable, uint32(len(chars)*2))
	if h == 0 {
		return false
	}
	// On success the clipboard takes OWNERSHIP of this allocation, so it must not
	// be freed. On any failure after this point it is still ours and would leak -
	// small, but this process runs for days, so free it explicitly.
	defer func() {
		if !ok {
			w32.GlobalFree(h)
		}
	}()

	p := w32.GlobalLock(h)
	if p == nil {
		return false
	}
	dst := unsafe.Slice((*uint16)(p), len(chars))
	copy(dst, chars)
	w32.GlobalUnlock(h)

	if w32.SetClipboardData(cfUnicodeText, w32.HANDLE(h)) == 0 {
		return false
	}
	ok = true
	return ok
}
