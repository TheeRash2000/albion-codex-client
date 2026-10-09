//go:build windows

package client

import (
	"fmt"
	"testing"
	"time"
)

// The first start after an auto-update from a build without -background must stay
// quiet: the OLD build performs the restart and passes no flag, and the autostart
// entry it left behind has none either. That launch is recognised by the entry.
func TestLegacyAutostartValue(t *testing.T) {
	exe := `C:\Program Files\Codex Client\codex-client.exe`
	cases := []struct {
		value string
		want  bool
	}{
		{exe, true},                    // 0.1.16 and earlier stored the bare path
		{`"` + exe + `"`, true},        // quoted, still no flag
		{autostartValue(exe), false},   // 0.1.17+ format
		{`"` + exe + `" -debug`, true}, // flags, but not -background
		{`"` + exe + `" --background`, false},
	}
	for _, c := range cases {
		if got := isLegacyAutostartValue(c.value); got != c.want {
			t.Errorf("isLegacyAutostartValue(%q) = %v, want %v", c.value, got, c.want)
		}
	}
}

// A missing entry (fresh install) is not a legacy upgrade, so a first launch
// still opens the window. Read-only: a name that cannot exist.
func TestReadRunValueMissing(t *testing.T) {
	name := fmt.Sprintf("CodexClientTestMissing-%d", time.Now().UnixNano())
	if v, ok := readRunValue(name); ok {
		t.Fatalf("readRunValue(%q) = %q, true; want not found", name, v)
	}
}
