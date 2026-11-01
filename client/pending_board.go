package client

import (
	"sync"

	"github.com/ao-data/albiondata-client/lib"
	"github.com/ao-data/albiondata-client/log"
)

// pendingBoard holds a destiny board captured (evFullAchievementInfo) before we knew the
// player's identity. The board event and the join response (operation_join.go, which
// sets CharacterId -> albion_id) both fire around login/zone-change, but the
// order varies. If the board event wins the race, uploading immediately would send the
// board with NO albion_id and the server could only match it by name. So we stash
// it here and flush it once the join response provides the id.
//
// The stash is BOUND to the character identity that was active when the board was
// captured (charID/charName, either of which may be empty if the board event fully won
// the race). The flush refuses to upload a board under a DIFFERENT identity than it
// was captured for, and a character switch drops the stash outright. Without this,
// a board captured while on character A could be flushed under character B's
// freshly-set identity after a switch - attaching A's board to B (and,
// symmetrically, overwriting A with B's board on the other ordering). That
// cross-character corruption is exactly what this guards against.
var (
	pendingBoardMu sync.Mutex
	pendingBoard   *capturedBoard
)

type capturedBoard struct {
	charID   string // CharacterId active at capture ("" if not yet known)
	charName string // CharacterName active at capture ("" if not yet known)
	server   string
	skills   []*lib.Skill
}

// stashBoard remembers a board to upload once the character id is known, bound to
// whatever identity was active at capture (possibly empty on the first-login race).
func stashBoard(charID, charName, server string, skills []*lib.Skill) {
	pendingBoardMu.Lock()
	pendingBoard = &capturedBoard{charID: charID, charName: charName, server: server, skills: skills}
	pendingBoardMu.Unlock()
}

// dropPendingBoard discards any stashed board without uploading it. Called on a
// character switch: the stashed board belonged to the previous character and must
// never be attached to the new one. The new character's board uploads when its own
// board event arrives.
func dropPendingBoard() {
	pendingBoardMu.Lock()
	dropped := pendingBoard
	pendingBoard = nil
	pendingBoardMu.Unlock()
	if dropped != nil {
		log.Debug("Dropping a destiny board captured before a character switch; it belongs to the previous character.")
	}
}

// pendingBoardIsForCurrent reports whether a stashed board may be uploaded under
// the current character id. A board captured before any id was known
// (stashedCharID == "", the first-login race) is acceptable for the current login;
// a board bound to a different, non-empty id belongs to another character and is
// stale - it must not be uploaded under the current identity.
func pendingBoardIsForCurrent(stashedCharID, currentCharID string) bool {
	return stashedCharID == "" || stashedCharID == currentCharID
}

// flushPendingBoard uploads a stashed board once the player's identity is known,
// but only if the board belongs to the CURRENT character: either it was captured
// before any id was known (charID == "", the first-login race) or its bound id
// matches the current one. A mismatch means the stash is stale (a board from a
// different character) and is dropped instead of uploaded. No-op if nothing is
// pending or the identity is still unknown.
func flushPendingBoard(state *albionState) {
	albionID := albionIDFromUUID(string(state.CharacterId))
	if albionID == "" || state.CharacterName == "" {
		return
	}
	pendingBoardMu.Lock()
	pb := pendingBoard
	pendingBoard = nil
	pendingBoardMu.Unlock()
	if pb == nil {
		return
	}
	// Only flush a board captured for THIS character (or before any id was known).
	// Anything else is a board that belongs to a different character.
	if !pendingBoardIsForCurrent(pb.charID, string(state.CharacterId)) {
		log.Debugf("Discarding a pending destiny board captured for a different character (%q) than the current one (%q).", pb.charName, state.CharacterName)
		return
	}
	log.Debug("Uploading the destiny board that was captured before the character id was known.")
	Status.setCharacter(state.CharacterName)
	uploadBoardToCodex(state.CharacterName, albionID, pb.server, pb.skills)
}
