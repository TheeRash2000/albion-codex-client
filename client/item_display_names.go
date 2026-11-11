package client

import (
	"bufio"
	"bytes"
	"compress/gzip"
	_ "embed"
	"strings"
	"sync"
)

// In-game item names for the panel's activity lines.
//
// The feed used to build a label from the item id ("T5 2h Arcanestaff Crystal"),
// which is not what the game or the market search calls anything ("Expert's
// Astral Staff"). itemnames.tsv.gz is "id<TAB>name" per line, English, from the
// game's own localization; ids carry no "@n" enchant suffix because enchanted
// gear shares its plain name.
//
// The table follows game patches only when regenerated. An id it does not know
// (a new item) falls back to the id-based label, so a stale table costs a nicer
// name, never a missing line.

//go:embed itemnames.tsv.gz
var itemNamesGz []byte

var (
	itemNamesOnce sync.Once
	itemNames     map[string]string
)

func loadItemNames() {
	itemNames = map[string]string{}
	zr, err := gzip.NewReader(bytes.NewReader(itemNamesGz))
	if err != nil {
		return
	}
	defer zr.Close()
	sc := bufio.NewScanner(zr)
	for sc.Scan() {
		id, name, ok := strings.Cut(sc.Text(), "\t")
		if ok && id != "" && name != "" {
			itemNames[id] = name
		}
	}
}

// itemName returns the in-game name for an item id, ignoring any "@n" enchant
// suffix. ok is false when the table does not know the item.
func itemName(id string) (name string, ok bool) {
	itemNamesOnce.Do(loadItemNames)
	if i := strings.IndexByte(id, '@'); i >= 0 {
		id = id[:i]
	}
	name, ok = itemNames[id]
	return name, ok
}

// itemLabel is what the activity feed calls an item: its in-game name, with the
// tier and enchant level in the game's own "5.2" form when the item is
// enchanted gear. Resources need no suffix, their name already says the level
// ("Rare Ashenbark Planks"). Unknown ids fall back to humanizeItemID.
func itemLabel(id string) string {
	name, ok := itemName(id)
	if !ok {
		return humanizeItemID(id)
	}
	if strings.Contains(id, "_LEVEL") {
		return name
	}
	i := strings.IndexByte(id, '@')
	if i < 0 || i+1 >= len(id) || id[i+1:] == "0" {
		return name
	}
	tier := ""
	if len(id) >= 3 && id[0] == 'T' && id[1] >= '0' && id[1] <= '9' && id[2] == '_' {
		tier = id[1:2]
	}
	return name + " (" + tier + "." + id[i+1:] + ")"
}

// qualityName is the game's word for a quality level; "" for Normal or unknown,
// which the feed leaves unsaid.
func qualityName(q int) string {
	switch q {
	case 2:
		return "Good"
	case 3:
		return "Outstanding"
	case 4:
		return "Excellent"
	case 5:
		return "Masterpiece"
	}
	return ""
}
