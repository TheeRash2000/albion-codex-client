package client

import (
	"strings"
	"testing"
)

// The client must not decide for itself what a numeric item id means: those
// numbers shift on every game patch, and a bundled list named the wrong item
// (a planks chart shown as a steel bar, 2026-11-11). The site answers; with no
// answer the feed names no item.
func TestHistoryReplyItem(t *testing.T) {
	cases := map[string]string{
		`{"ok":true,"item_id":"T8_PLANKS_LEVEL3@3","inserted":41}`: "T8_PLANKS_LEVEL3@3",
		`{"ok":true,"item_id":"  T4_BAG "}`:                        "T4_BAG",
		`{"ok":true}`:                                              "", // an older site
		`{"item_id":7}`:                                            "",
		`not json`:                                                 "",
		``:                                                         "",
	}
	for reply, want := range cases {
		if got := historyReplyItem([]byte(reply)); got != want {
			t.Errorf("reply %q: got %q, want %q", reply, got, want)
		}
	}
}

func TestHistoryLineNamesNoItemWithoutAnAnswer(t *testing.T) {
	pts := []codexHistoryPoint{{ItemCount: 2, SilverTotal: 2 * 753106 * 10000}}
	got := historyLine(historyReplyItem([]byte(`{"ok":true}`)), 1, 2, pts)
	if got != "Sent price history (1 points) to Codex" {
		t.Errorf("got %q", got)
	}
	got = historyLine(historyReplyItem([]byte(`{"ok":true,"item_id":"T8_PLANKS_LEVEL3@3"}`)), 1, 2, pts)
	if got != "Exceptional Whitewood Planks - 4 week average 753,106" {
		t.Errorf("got %q", got)
	}
}

func TestEMVActivityUsesTheSiteNames(t *testing.T) {
	saved := Status
	Status = &ClientStatus{}
	defer func() { Status = saved }()

	rows := []codexEMVItem{{AlbionID: 1101, Value: 7531060000}, {AlbionID: 42, Value: 9800000}}
	recordEMVActivity(rows, emvReplyItems([]byte(`{"ok":true,"items":{"1101":"T8_PLANKS_LEVEL3@3"}}`)))

	var lines []string
	for _, a := range Status.activity {
		lines = append(lines, a.msg)
	}
	joined := strings.Join(lines, " | ")
	if !strings.Contains(joined, "Exceptional Whitewood Planks - est. value 753,106") {
		t.Errorf("named item missing: %s", joined)
	}
	// Not named by the site: the number, never a guess.
	if !strings.Contains(joined, "Item #42 - est. value 980") {
		t.Errorf("unnamed item should show its number: %s", joined)
	}
}

func TestEMVReplyItemsFromAnOlderSite(t *testing.T) {
	if got := emvReplyItems([]byte(`{"ok":true}`)); len(got) != 0 {
		t.Errorf("got %v, want none", got)
	}
	if got := emvReplyItems([]byte(`garbage`)); got != nil {
		t.Errorf("got %v, want nil", got)
	}
}
