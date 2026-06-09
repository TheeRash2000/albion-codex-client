package client

import (
	_ "embed"
	"encoding/json"
	"strconv"

	"github.com/ao-data/albiondata-client/log"
)

// game_item_ids.json maps the game's NUMERIC item id -> unique name (e.g.
// "900" -> "T4_METALBAR_LEVEL1@1"). Generated from the AODP item index list.
// Embedded so we can show readable item names for packets that carry only the
// numeric id (the Estimated Market Value event), matching the market lines.
//
//go:embed game_item_ids.json
var gameItemIDsJSON []byte

var numToItemName map[string]string

func init() {
	if err := json.Unmarshal(gameItemIDsJSON, &numToItemName); err != nil {
		log.Debugf("item names: could not load id map: %v", err)
		numToItemName = map[string]string{}
	}
}

// itemNameFromID returns the unique item name for a numeric game item id, or ""
// if unknown.
func itemNameFromID(id int) string {
	return numToItemName[strconv.Itoa(id)]
}
