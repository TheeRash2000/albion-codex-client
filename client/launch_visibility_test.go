package client

import (
	"reflect"
	"sync"
	"testing"
	"time"
)

func TestShouldOpenPanelOnStart(t *testing.T) {
	cases := []struct {
		name                      string
		openPanel, background, ok bool
		want                      bool
	}{
		{"player double-clicks the exe", false, false, true, true},
		{"Windows login (autostart)", false, true, true, false},
		{"restart after a self-update", false, true, true, false},
		{"-panel wins over -background", true, true, true, true},
		{"never linked: always open, linking lives in the window", false, true, false, true},
	}
	// quietStart folds in the first start after an upgrade from a build whose
	// autostart entry had no -background: quiet, like a background launch.
	if quietStart(false, true) != true || quietStart(true, false) != true || quietStart(false, false) != false {
		t.Error("quietStart must be background OR upgraded-from-legacy")
	}
	for _, c := range cases {
		if got := shouldOpenPanelOnStart(c.openPanel, c.background, c.ok); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

func TestWithBackgroundFlag(t *testing.T) {
	cases := []struct{ in, want []string }{
		{nil, []string{"-background"}},
		{[]string{"-debug"}, []string{"-background", "-debug"}},
		{[]string{"-background"}, []string{"-background"}},
		{[]string{"--background"}, []string{"--background"}},
		{[]string{"-background=false", "-debug"}, []string{"-background=false", "-debug"}}, // an explicit choice is kept
	}
	for _, c := range cases {
		if got := withBackgroundFlag(c.in); !reflect.DeepEqual(got, c.want) {
			t.Errorf("withBackgroundFlag(%q) = %q, want %q", c.in, got, c.want)
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
