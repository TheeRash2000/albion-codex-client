package client

import (
	"encoding/json"
	"reflect"
	"strconv"
	"strings"

	"github.com/ao-data/albiondata-client/log"
)

// eventEstimatedMarketValue is Photon event 465 (evEstimatedMarketValueUpdate; 464 before the 2026-10-12 patch):
// the server's OWN estimated market value for an item, pushed when the player
// views the item (the "Estimated Market Value" tooltip). This is a manipulation-
// resistant, server-authored price signal - a strong input for Codex pricing,
// independent of marketplace listings.
//
//	key0 = item type id(s)   - element type VARIES on the wire (uint8 or int16)
//	key1 = estimated value(s) - int32
//
// Decoded generically because of the varying key0 element type, then normalised.
type eventEstimatedMarketValue struct {
	ItemsRaw  interface{} `mapstructure:"0"`
	ValuesRaw interface{} `mapstructure:"1"`
}

func (event eventEstimatedMarketValue) Process(state *albionState) {
	items := numericSlice(event.ItemsRaw)
	values := numericSlice(event.ValuesRaw)
	if len(items) == 0 || len(values) == 0 {
		return
	}
	srv, _ := state.GetServer()
	uploadEMVToCodex(items, values, codexServerName(srv))
}

// numericSlice converts any slice of a numeric kind ([]uint8, []int16, []int32,
// []int, ...) to []int64. Returns nil for non-slice or non-numeric inputs.
func numericSlice(v interface{}) []int64 {
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Slice {
		return nil
	}
	out := make([]int64, 0, rv.Len())
	for i := 0; i < rv.Len(); i++ {
		el := rv.Index(i)
		switch el.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			out = append(out, el.Int())
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			out = append(out, int64(el.Uint()))
		default:
			// element is itself an interface{} wrapping a number
			if n, ok := asInt64(el.Interface()); ok {
				out = append(out, n)
			}
		}
	}
	return out
}

func asInt64(v interface{}) (int64, bool) {
	switch x := v.(type) {
	case int64:
		return x, true
	case int32:
		return int64(x), true
	case int16:
		return int64(x), true
	case int8:
		return int64(x), true
	case int:
		return int64(x), true
	case uint8:
		return int64(x), true
	case uint16:
		return int64(x), true
	case uint32:
		return int64(x), true
	case uint64:
		return int64(x), true
	}
	return 0, false
}

type codexEMVItem struct {
	AlbionID int64 `json:"albion_id"`
	Value    int64 `json:"value"`
}

type codexEMVPayload struct {
	Server string         `json:"server,omitempty"`
	Values []codexEMVItem `json:"values"`
}

// uploadEMVToCodex contributes the server's estimated market values to Codex.
// Item ids are NUMERIC (Codex resolves them to names server-side). Best-effort
// and opt-in: no-op unless market upload is enabled (--codex-market) and paired.
func uploadEMVToCodex(items, values []int64, server string) {
	if !ConfigGlobal.CodexMarketUpload {
		return
	}
	token := strings.TrimSpace(ConfigGlobal.CodexToken)
	if token == "" {
		return
	}

	n := len(items)
	if len(values) < n {
		n = len(values)
	}
	rows := make([]codexEMVItem, 0, n)
	for i := 0; i < n; i++ {
		if values[i] <= 0 {
			continue
		}
		rows = append(rows, codexEMVItem{AlbionID: items[i], Value: values[i]})
	}
	if len(rows) == 0 {
		return
	}

	body, err := json.Marshal(codexEMVPayload{Server: server, Values: rows})
	if err != nil {
		log.Errorf("Codex EMV: could not encode: %v", err)
		return
	}

	// EMV is game-cached and re-sends for uncached items: low-value (shed first).
	count := len(rows)
	enqueueCodexUpload("EMV", "/api/client/emv", body, count, true, func() {
		log.Infof("Codex: uploaded %d estimated market value(s).", count)
		recordEMVActivity(rows)
	})
}

// recordEMVActivity logs each captured estimated value to the panel feed as
// "<item> - est. value <silver>", resolving the numeric item id to a readable
// name using the authoritative ao-bin-dumps/formatted/items.txt mapping.
func recordEMVActivity(rows []codexEMVItem) {
	const maxLines = 15
	shown := len(rows)
	if shown > maxLines {
		shown = maxLines
	}
	if extra := len(rows) - shown; extra > 0 {
		Status.addActivity("...and " + strconv.Itoa(extra) + " more estimated values to Codex")
	}
	for i := 0; i < shown; i++ {
		label := humanizeItemID(itemNameFromID(int(rows[i].AlbionID)))
		if label == "" {
			label = "Item #" + strconv.FormatInt(rows[i].AlbionID, 10)
		}
		Status.addActivity(label + " - est. value " + formatSilver(int(rows[i].Value/10000)))
	}
}
