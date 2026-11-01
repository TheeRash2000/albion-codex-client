package client

import (
	"sync"
	"testing"
	"time"
)

// The window opens on every start, the Windows login autostart included
// (no launch may skip the window, because a player who never sees it never
// finds "Link this PC"). The -background flag that older autostart
// entries pass is accepted and ignored, and a self-update restart follows the
// same rule whatever flags it inherits.
func TestShouldOpenPanelOnStart(t *testing.T) {
	cases := []struct {
		name                      string
		openPanel, background, ok bool
		want                      bool
	}{
		{"player double-clicks the exe", false, false, true, true},
		{"Windows login (autostart, -background)", false, true, true, true},
		{"restart after a self-update of a login launch", false, true, true, true},
		{"restart after a self-update of a player's launch", false, false, true, true},
		{"-panel", true, true, true, true},
		{"never linked", false, true, false, true},
	}
	for _, c := range cases {
		if got := shouldOpenPanelOnStart(c.openPanel, c.background, c.ok); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

// The capturing toast fires once per run, from the first game packet, and says
// what to do when the PC is not linked.
func TestCapturingToastOncePerRun(t *testing.T) {
	prevStatus, prevPush := Status, pushNotification
	defer func() { Status, pushNotification = prevStatus, prevPush }()

	var mu sync.Mutex
	var got []string
	done := make(chan struct{}, 4)
	pushNotification = func(msg string) {
		mu.Lock()
		got = append(got, msg)
		mu.Unlock()
		done <- struct{}{}
	}

	Status = &ClientStatus{}
	Status.markSeen()
	Status.markSeen()
	Status.markSeen()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("no capturing toast was posted")
	}
	time.Sleep(50 * time.Millisecond) // a second (wrong) toast would land here
	mu.Lock()
	defer mu.Unlock()
	if len(got) != 1 {
		t.Fatalf("got %d toasts, want 1: %q", len(got), got)
	}
}

func TestCapturingMessage(t *testing.T) {
	if m := capturingMessage(true); m != "Codex Client is capturing. Click the tray icon to see what is being sent." {
		t.Errorf("linked message = %q", m)
	}
	if m := capturingMessage(false); m == capturingMessage(true) {
		t.Errorf("unlinked message should tell the player to link: %q", m)
	}
}
