package client

import "testing"

// TestPendingBoardIsForCurrent locks the rule that prevents the cross-character
// destiny-board corruption: a board stashed under one character's identity must
// never be flushed under a different character's identity. A board captured before
// any id was known (empty stashed id, the first-login event-151-before-join race)
// is the only case allowed to flush under the now-known id.
func TestPendingBoardIsForCurrent(t *testing.T) {
	const clown = "psfPiWUhQ7uLE-q_nYxpig"
	const hoisen = "X2SfN_rkRWqq2_bxVoiaoA"

	cases := []struct {
		name     string
		stashed  string
		current  string
		expectOK bool
	}{
		{"unknown-at-capture flushes for first login", "", clown, true},
		{"same character (zone change) flushes", clown, clown, true},
		{"different character is rejected (the bug)", clown, hoisen, false},
		{"current unknown but stash bound is rejected", clown, "", false},
		{"both empty is allowed", "", "", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := pendingBoardIsForCurrent(c.stashed, c.current); got != c.expectOK {
				t.Fatalf("pendingBoardIsForCurrent(%q, %q) = %v, want %v", c.stashed, c.current, got, c.expectOK)
			}
		})
	}
}

// TestDropPendingBoardClears verifies a character switch discards a stashed board
// so it cannot later be flushed under the new identity.
func TestDropPendingBoardClears(t *testing.T) {
	stashBoard("someCharId", "SomeName", "asia", nil)
	if pendingBoard == nil {
		t.Fatal("expected a stashed board after stashBoard")
	}
	dropPendingBoard()
	if pendingBoard != nil {
		t.Fatal("expected pendingBoard to be nil after dropPendingBoard")
	}
}
