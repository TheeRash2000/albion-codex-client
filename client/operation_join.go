package client

import (
	"github.com/ao-data/albiondata-client/lib"
	"github.com/ao-data/albiondata-client/log"
)

type operationJoinResponse struct {
	CharacterID   lib.CharacterID `mapstructure:"1"`
	CharacterName string          `mapstructure:"2"`
	Location      string          `mapstructure:"8"`
	GuildID       lib.CharacterID `mapstructure:"56"`
	GuildName     string          `mapstructure:"58"`
}

//CharacterPartsJSON string          `mapstructure:"6"`
//Edition            string          `mapstructure:"38"`

func (op operationJoinResponse) Process(state *albionState) {
	log.Debugf("Got JoinResponse operation...")

	// Reset the AODataServerID here. This leads to a fresh execution
	// of SetServerID() incase the player switched servers
	state.AODataServerID = 0

	location := normalizeLocationID(op.Location)
	if location != "" {
		log.Infof("Updating player location to %v.", location)
		state.LocationId = location
	} else {
		log.Debugf("Ignoring implausible join location value: %q", op.Location)
	}

	// A different CharacterId means the player switched account/character (JoinResponse
	// also fires on every zone change with the SAME id, so gate on the id changing).
	switched := state.CharacterName != "" && state.CharacterId != op.CharacterID
	if state.CharacterId != op.CharacterID {
		log.Infof("Updating player ID to %v.", op.CharacterID)
	}
	state.CharacterId = op.CharacterID

	if state.CharacterName != op.CharacterName {
		log.Infof("Updating player to %v.", op.CharacterName)
	}
	state.CharacterName = op.CharacterName

	if switched {
		// Reflect the new character now and drop the old one's board status. Crucially,
		// DISCARD any pending board: it was captured under (or before) the PREVIOUS
		// character and must never be flushed under this new identity. The game caches
		// the board client-side, so right after a switch a stale board can still be in
		// flight; attaching it to the new character is the corruption we are preventing.
		// The new character's board uploads when its own event 151 arrives.
		Status.switchCharacter(op.CharacterName)
		dropPendingBoard()
		return
	}

	// Same character (a zone-change join fires with the same id) or first login: if a
	// board was captured before we knew the identity (event 151 raced ahead of this
	// join), upload it now that CharacterId/Name are set. No-op otherwise.
	flushPendingBoard(state)

	// Likewise, flush any market browse that arrived before the location was set
	// (a browse right after login/relog). No-op when nothing is buffered.
	flushPendingMarket(state)
}
