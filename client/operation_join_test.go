package client

import "testing"

// The tray must show the character as soon as the join response names it, not only
// after a destiny board is captured. When the board event code moved in the
// 2026-10-12 patch, 0.1.17 never captured a board, so the tray showed no character
// and players reported the client as "not tracking my character".
func TestJoinShowsCharacterWithoutBoard(t *testing.T) {
	Status = &ClientStatus{}
	defer func() { Status = &ClientStatus{} }()

	state := &albionState{}
	operationJoinResponse{CharacterID: "char-a", CharacterName: "MainChar"}.Process(state)

	snap := Status.Snapshot()
	if snap.Character != "MainChar" {
		t.Fatalf("Character = %q after join, want %q", snap.Character, "MainChar")
	}
	if snap.HasUploaded {
		t.Fatal("a join alone must not report a board upload")
	}

	// A zone change re-sends the same join; the name stays.
	operationJoinResponse{CharacterID: "char-a", CharacterName: "MainChar"}.Process(state)
	if got := Status.Snapshot().Character; got != "MainChar" {
		t.Fatalf("Character = %q after zone change, want %q", got, "MainChar")
	}

	// A character switch shows the new name.
	operationJoinResponse{CharacterID: "char-b", CharacterName: "Alt"}.Process(state)
	if got := Status.Snapshot().Character; got != "Alt" {
		t.Fatalf("Character = %q after switch, want %q", got, "Alt")
	}
}
