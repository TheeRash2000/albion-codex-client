package client

import (
	"sync"

	"github.com/ao-data/albiondata-client/lib"
	"github.com/ao-data/albiondata-client/log"
)

// pendingBoard holds a destiny board captured (event 151) before we knew the
// player's identity. Event 151 and the join response (operation_join.go, which
// sets CharacterId -> albion_id) both fire around login/zone-change, but the
// order varies. If event 151 wins the race, uploading immediately would send the
// board with NO albion_id and the server could only match it by name. So we stash
// it here and flush it once the join response provides the id.
var (
	pendingBoardMu sync.Mutex
	pendingBoard   *capturedBoard
)

type capturedBoard struct {
	server string
	skills []*lib.Skill
}

// stashBoard remembers a board to upload once the character id is known.
func stashBoard(server string, skills []*lib.Skill) {
	pendingBoardMu.Lock()
	pendingBoard = &capturedBoard{server: server, skills: skills}
	pendingBoardMu.Unlock()
}

// flushPendingBoard uploads a stashed board once the player's identity is known.
// Called from the join handler after CharacterId/Name are set. No-op if nothing
// is pending or the identity is still unknown. Idempotent (re-upload is harmless;
// the server upserts), and it fires on every join including zone changes - after
// the first flush there is simply nothing pending.
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
	log.Debug("Uploading the destiny board that was captured before the character id was known.")
	Status.setCharacter(state.CharacterName)
	uploadBoardToCodex(state.CharacterName, albionID, pb.server, pb.skills)
}
