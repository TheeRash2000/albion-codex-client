package client

import (
	"github.com/ao-data/albiondata-client/lib"
	"github.com/ao-data/albiondata-client/log"
)

// completedNodeLevel is a sentinel for nodes in event 151's "completed" list
// (key1). Those nodes carry no explicit level - the game flags them as MAXED.
// We can't know the cap here (mastery 100 vs combat spec 120) without the spec
// metadata, so we send this sentinel and the server assigns each node its real
// maxLevel. Any value above the real 0-120 range works as the marker.
const completedNodeLevel = 999

// eventFullAchievementInfo is Photon event 151 (evFullAchievementInfo): the
// player's COMPLETE destiny board, pushed once at login / first board load.
//
//	key1 = COMPLETED node IDs (no level array - they're maxed; this is where the
//	       player's finished masteries live)
//	key2 = in-progress node IDs (the achievement enumeration, ascending)
//	key3 = the matching levels for key2, one byte per node (0-120)
//	key4/key5 = unlock/visibility flags (unused here)
//
// We read BOTH key1 (completed -> level 100) and key2/key3 (in-progress). Reading
// only key2 was the bug that dropped every maxed mastery. Event 154
// (evFullAchievementProgressInfo) is only the fame-bearing subset.
type eventFullAchievementInfo struct {
	CompletedIDs []int  `mapstructure:"1"`
	NodeIDs      []int  `mapstructure:"2"`
	Levels       []byte `mapstructure:"3"`
}

func (event eventFullAchievementInfo) Process(state *albionState) {
	log.Debug("Got full achievement info (destiny board) event...")

	n := len(event.NodeIDs)
	if n == 0 || len(event.Levels) < n {
		log.Debugf("Destiny board event malformed: %d ids, %d levels", n, len(event.Levels))
		return
	}

	skills := make([]*lib.Skill, 0, n+len(event.CompletedIDs))
	for i := 0; i < n; i++ {
		skills = append(skills, &lib.Skill{ID: event.NodeIDs[i], Level: int(event.Levels[i])})
	}
	// key1: completed/maxed nodes the player has finished (their maxed masteries).
	for _, id := range event.CompletedIDs {
		skills = append(skills, &lib.Skill{ID: id, Level: completedNodeLevel})
	}

	albionID := albionIDFromUUID(string(state.CharacterId))
	srv, _ := state.GetServer()
	server := codexServerName(srv)
	log.Infof("Captured Destiny Board for %v (%v, %v): %d in-progress + %d completed nodes.",
		state.CharacterName, albionID, server, n, len(event.CompletedIDs))
	Status.addActivity("Captured your Destiny Board")

	// If the join response hasn't set the player's identity yet (event 151 can
	// arrive first), hold the board and let operation_join.go upload it once the
	// id is known - otherwise it would upload with no albion_id and match by name.
	if albionID == "" || state.CharacterName == "" {
		log.Debug("Character identity not known yet; holding the board until the join response sets it.")
		stashBoard(server, skills)
		return
	}

	Status.setCharacter(state.CharacterName)
	uploadBoardToCodex(state.CharacterName, albionID, server, skills)
}
