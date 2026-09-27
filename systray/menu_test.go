//go:build windows || darwin

package systray

import (
	"strings"
	"testing"

	"github.com/ao-data/albiondata-client/client"
)

// The tray now carries every status line the web panel used to render, so these
// pin the states that are easy to get subtly wrong - particularly the revoked
// device, which is the case the old panel could not express at all.

func TestStatusLineCoversEveryState(t *testing.T) {
	cases := []struct {
		name string
		in   client.StatusSnapshot
		want string
	}{
		{"unlinked", client.StatusSnapshot{}, "Status: not linked"},
		{"linked", client.StatusSnapshot{Linked: true}, "Status: linked to Codex"},
		{
			"revoked device",
			client.StatusSnapshot{Linked: true, TokenRejected: true},
			"Status: link broken - re-link this PC",
		},
		{
			"npcap missing outranks linked",
			client.StatusSnapshot{Linked: true, NpcapMissing: true},
			"Status: Npcap missing - capture is off",
		},
		{
			"pairing outranks everything",
			client.StatusSnapshot{Pairing: true, PairCode: "AB12-CD34", NpcapMissing: true},
			"Status: linking this PC - code AB12-CD34",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := statusLine(c.in); got != c.want {
				t.Fatalf("got %q want %q", got, c.want)
			}
		})
	}
}

// A revoked device still HAS a token, so CodexLinked stays true. The panel only
// rendered its Link button when no token existed, which is precisely why a
// revoked PC could not be recovered from the UI. The tray must always offer it.
func TestLinkLineAlwaysOffersRelink(t *testing.T) {
	if got := linkLine(client.StatusSnapshot{}); got != "Link this PC" {
		t.Fatalf("unlinked: got %q", got)
	}
	if got := linkLine(client.StatusSnapshot{Linked: true}); got != "Re-link this PC" {
		t.Fatalf("a linked client must still offer a re-link, got %q", got)
	}
	if got := linkLine(client.StatusSnapshot{Linked: true, TokenRejected: true}); got != "Re-link this PC" {
		t.Fatalf("revoked: got %q", got)
	}
	got := linkLine(client.StatusSnapshot{Pairing: true, PairCode: "AB12-CD34"})
	if !strings.Contains(got, "AB12-CD34") {
		t.Fatalf("a running handshake must show the code, got %q", got)
	}
}

func TestBoardLine(t *testing.T) {
	if got := boardLine(client.StatusSnapshot{}); got != "Destiny Board: link this PC first" {
		t.Fatalf("unlinked: got %q", got)
	}
	if got := boardLine(client.StatusSnapshot{Linked: true}); got != "Destiny Board: change zones once to send it" {
		t.Fatalf("linked, nothing captured: got %q", got)
	}
	got := boardLine(client.StatusSnapshot{
		Linked: true, HasUploaded: true, LastUploadNodes: 1308, LastUploadAgo: "2 mins ago",
	})
	if got != "Destiny Board: 1,308 nodes, 2 mins ago" {
		t.Fatalf("captured: got %q", got)
	}
}

func TestCommas(t *testing.T) {
	cases := map[int]string{0: "0", 7: "7", 999: "999", 1000: "1,000", 68296: "68,296", 2389897: "2,389,897", -1234: "-1,234"}
	for in, want := range cases {
		if got := commas(in); got != want {
			t.Fatalf("commas(%d) = %q want %q", in, got, want)
		}
	}
}

// A long error string must not stretch the menu across the screen.
func TestTruncateBoundsRowWidth(t *testing.T) {
	long := strings.Repeat("x", maxRowText*2)
	got := truncate(long)
	if len(got) > maxRowText {
		t.Fatalf("truncate returned %d chars, want at most %d", len(got), maxRowText)
	}
	if !strings.HasSuffix(got, "...") {
		t.Fatalf("a truncated row should show an ellipsis, got %q", got)
	}
	if short := truncate("fine"); short != "fine" {
		t.Fatalf("a short row must pass through unchanged, got %q", short)
	}
}

func TestCountedTitle(t *testing.T) {
	if got := countedTitle("Issues", 0); got != "Issues" {
		t.Fatalf("zero should not show a count, got %q", got)
	}
	if got := countedTitle("Issues", 3); got != "Issues (3)" {
		t.Fatalf("got %q", got)
	}
}

// The submenu slot counts must stay at or below the feeds that fill them, or the
// tray would allocate rows that can never be filled and, worse, imply the client
// keeps more history than it does.
func TestSlotCountsDoNotExceedTheSourceFeeds(t *testing.T) {
	if activitySlots > 40 {
		t.Fatalf("activitySlots (%d) exceeds the activity feed cap of 40 in client/status.go", activitySlots)
	}
	if issueSlots > 30 {
		t.Fatalf("issueSlots (%d) exceeds the issues feed cap of 30 in client/status.go", issueSlots)
	}
}
