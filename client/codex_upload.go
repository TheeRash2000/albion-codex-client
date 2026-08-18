package client

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"strconv"
	"strings"

	"github.com/ao-data/albiondata-client/lib"
	"github.com/ao-data/albiondata-client/log"
)

// codexClientVersion identifies this client to the Codex ingest API.
const codexClientVersion = "codex-client/alpha-0.1.15"

// codexCharacter mirrors the POST body the Codex ingest route expects:
//
//	{ "character": { "name": string, "skills": { "<nodeId>": level } } }
//
// nodeId is the destiny-board node ID (the in-game SkillId); the web side maps
// it to a Codex spec_key via the generated node-spec-map.
type codexCharacter struct {
	Name   string         `json:"name"`
	ID     string         `json:"id,omitempty"`     // Albion player id (gameinfo base64url form)
	Server string         `json:"server,omitempty"` // detected region (americas/europe/asia)
	Skills map[string]int `json:"skills"`
}

// albionIDFromUUID converts the client's hyphenated character UUID into the Albion
// player id string the gameinfo API uses (URL-safe base64 of the same 16 bytes, no
// padding). This lets the server reconcile a captured character with a website-
// imported one by identity instead of name. Returns "" if the UUID is malformed.
func albionIDFromUUID(uuid string) string {
	h := strings.ReplaceAll(uuid, "-", "")
	b, err := hex.DecodeString(h)
	if err != nil || len(b) != 16 {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

type codexBoardPayload struct {
	Character codexCharacter `json:"character"`
}

// uploadBoardToCodex sends the player's destiny board to their Codex account,
// authenticated with the device's pairing token. It is best-effort: any failure
// is logged and swallowed so a Codex outage never disrupts capture.
//
// No-op (with a single debug line) when the client is not paired, i.e. no token
// is configured. The Codex base URL defaults to https://albioncodex.com.
func uploadBoardToCodex(characterName, albionID, server string, skills []*lib.Skill) {
	token := strings.TrimSpace(ConfigGlobal.CodexToken)
	if token == "" {
		log.Debug("Codex: not paired (no device token); board upload skipped.")
		return
	}
	if strings.TrimSpace(characterName) == "" {
		log.Debug("Codex: character name not yet known; board upload skipped.")
		return
	}

	board := map[string]int{}
	for _, s := range skills {
		if s.Level > 0 { // only trained nodes; the web mapper ignores level<=0 anyway
			board[strconv.Itoa(s.ID)] = s.Level
		}
	}
	if len(board) == 0 {
		return
	}

	body, err := json.Marshal(codexBoardPayload{
		Character: codexCharacter{Name: characterName, ID: albionID, Server: server, Skills: board},
	})
	if err != nil {
		log.Errorf("Codex: could not encode board: %v", err)
		return
	}

	// The Destiny board is high-value (a full capture only re-arrives on the next
	// login / zone change), so it is never shed from a full queue.
	nodes := len(board)
	enqueueCodexUpload("Destiny board", "/api/client/ingest", body, nodes, false, func() {
		log.Infof("Codex: uploaded %d destiny-board nodes for %v.", nodes, characterName)
		Status.recordUpload(characterName, nodes, true)
		Status.addActivity("Destiny Board sent to Codex (" + characterName + ", " + strconv.Itoa(nodes) + " nodes)")
	})
}
