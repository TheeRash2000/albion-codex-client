//go:build windows

package client

import (
	"testing"

	"github.com/gonutz/w32"
)

// The panel window's taskbar icon is built from the embedded .ico at runtime.
// If that ever returns 0, Windows silently falls back to the generic icon, so
// assert a real HICON comes back at both system icon sizes.
func TestIconFromICOBuildsAnIcon(t *testing.T) {
	for _, size := range []int{16, 32, 48, 0} {
		h := iconFromICO(size)
		if h == 0 {
			t.Fatalf("size %d: CreateIconFromResourceEx returned no icon", size)
		}
		w32.DestroyIcon(h)
	}
}
