// Codex Client icon as a Windows .ico, on every platform.
//
// Data (iconwin.go / icondarwin.go) is the tray icon and is platform-specific.
// ICO is the same mark in .ico form, available everywhere, so the panel's local
// HTTP server can serve /favicon.ico on any OS and the Windows panel window can
// build its title-bar and taskbar icon from it.
package icon

import (
	_ "embed"
	"encoding/binary"
	"errors"
)

//go:embed codex-client.ico
var ICO []byte

// ICOImage returns the raw image bytes (PNG or DIB) of the entry in an .ico
// file whose width is closest to size, preferring the smallest entry that is
// at least size so Windows scales down rather than up.
//
// The bytes are what CreateIconFromResourceEx expects. Note this parses the ICO
// FILE format (ICONDIR with byte offsets), which LookupIconIdFromDirectoryEx
// does not accept: that API wants the RT_GROUP_ICON resource layout instead.
func ICOImage(data []byte, size int) ([]byte, int, error) {
	if len(data) < 6 || binary.LittleEndian.Uint16(data[0:]) != 0 || binary.LittleEndian.Uint16(data[2:]) != 1 {
		return nil, 0, errors.New("icon: not an .ico file")
	}
	count := int(binary.LittleEndian.Uint16(data[4:]))
	if count == 0 || len(data) < 6+16*count {
		return nil, 0, errors.New("icon: truncated .ico directory")
	}

	best, bestW := -1, 0
	for i := 0; i < count; i++ {
		e := data[6+16*i:]
		w := int(e[0])
		if w == 0 {
			w = 256 // a 0 byte means 256 in the ICO format
		}
		off := int(binary.LittleEndian.Uint32(e[12:]))
		n := int(binary.LittleEndian.Uint32(e[8:]))
		if off < 0 || n <= 0 || off+n > len(data) {
			continue
		}
		if best < 0 || better(w, bestW, size) {
			best, bestW = i, w
		}
	}
	if best < 0 {
		return nil, 0, errors.New("icon: no usable .ico entry")
	}
	e := data[6+16*best:]
	off := int(binary.LittleEndian.Uint32(e[12:]))
	n := int(binary.LittleEndian.Uint32(e[8:]))
	return data[off : off+n], bestW, nil
}

// better reports whether an entry of width w beats the current pick cur for a
// requested size: any entry >= size beats one below it; among entries >= size
// the smaller wins; among entries below size the larger wins.
func better(w, cur, size int) bool {
	wBig, curBig := w >= size, cur >= size
	switch {
	case wBig && !curBig:
		return true
	case !wBig && curBig:
		return false
	case wBig:
		return w < cur
	default:
		return w > cur
	}
}
