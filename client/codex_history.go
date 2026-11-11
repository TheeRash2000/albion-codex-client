package client

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/ao-data/albiondata-client/lib"
	"github.com/ao-data/albiondata-client/log"
)

// codexHistoryPoint is one time-bucket of the game's own market history: the
// quantity sold and the total silver in that bucket (avg price = silver/count).
type codexHistoryPoint struct {
	ItemCount   int64  `json:"item_count"`
	SilverTotal uint64 `json:"silver_total"`
	Timestamp   uint64 `json:"timestamp"`
}

type codexHistoryPayload struct {
	Server string `json:"server,omitempty"`
	// AlbionID is the NUMERIC game item id (as the price-history op carries it);
	// Codex resolves it to an item name server-side (the client has no name map).
	AlbionID  int32               `json:"albion_id"`
	City      string              `json:"city"`
	Quality   int                 `json:"quality"`
	Timescale int                 `json:"timescale"`
	Points    []codexHistoryPoint `json:"points"`
}

// uploadHistoryToCodex contributes the game's own market price history (the chart
// data behind an item's price graph) to Codex - the deep, instant history that our
// estimates engine wants, vs. slowly accumulating daily candles. Best-effort and
// opt-in: no-op unless market upload is enabled (--codex-market) and we're paired.
func uploadHistoryToCodex(up lib.MarketHistoriesUpload, server string) {
	if !ConfigGlobal.CodexMarketUpload {
		return
	}
	token := strings.TrimSpace(ConfigGlobal.CodexToken)
	if token == "" || len(up.Histories) == 0 || up.LocationId == "" {
		return
	}

	points := make([]codexHistoryPoint, 0, len(up.Histories))
	for _, h := range up.Histories {
		if h == nil {
			continue
		}
		points = append(points, codexHistoryPoint{
			ItemCount:   h.ItemAmount,
			SilverTotal: h.SilverAmount,
			Timestamp:   h.Timestamp,
		})
	}
	if len(points) == 0 {
		return
	}

	body, err := json.Marshal(codexHistoryPayload{
		Server:    server,
		AlbionID:  up.AlbionId,
		City:      up.LocationId,
		Quality:   int(up.QualityLevel),
		Timescale: int(up.Timescale),
		Points:    points,
	})
	if err != nil {
		log.Errorf("Codex history: could not encode: %v", err)
		return
	}

	// History is re-fetched by the game and thus re-observable: low-value (shed first).
	n := len(points)
	albionID := up.AlbionId
	quality, timescale := int(up.QualityLevel), int(up.Timescale)
	enqueueCodexUploadWithReply("Market history", "/api/client/market-history", body, n, true, func(reply []byte) {
		log.Infof("Codex: uploaded %d market-history points for item %d.", n, albionID)
		Status.addActivity(historyLine(historyReplyItem(reply), quality, timescale, points))
	})
}

// historyReplyItem reads which item the site says the chart was for.
//
// The chart request carries only the game's NUMERIC item id, a position in the
// game's item list that shifts on every patch. The client used to resolve it
// with a bundled copy of that list; the copy went stale and the feed named the
// wrong item (a planks chart shown as a steel bar, 2026-11-11). The site keeps
// the current list, so it answers, and without an answer the line names no item
// at all rather than guess.
func historyReplyItem(reply []byte) string {
	var r struct {
		ItemID string `json:"item_id"`
	}
	if json.Unmarshal(reply, &r) != nil {
		return ""
	}
	return strings.TrimSpace(r.ItemID)
}

// historyAverage is the game's "Average Price" for a price chart: all the silver
// traded in the window divided by all the items traded, NOT the mean of the
// per-bucket averages (a quiet day would count as much as a busy one). Checked
// against the game 2026-11-11: 41 buckets, 1,188 items, shown as "753 k", this
// gives 753,106. Silver on the wire is x10000. ok is false when nothing sold.
func historyAverage(points []codexHistoryPoint) (avg int, ok bool) {
	var items, silver uint64
	for _, p := range points {
		if p.ItemCount <= 0 {
			continue
		}
		items += uint64(p.ItemCount)
		silver += p.SilverTotal
	}
	if items == 0 {
		return 0, false
	}
	return int((silver + items*5000) / (items * 10000)), true
}

// historyWindow names the chart window the game sent, the way its own buttons do.
func historyWindow(timescale int) string {
	switch timescale {
	case 0:
		return "24 hour"
	case 1:
		return "7 day"
	case 2:
		return "4 week"
	}
	return ""
}

// historyLine is the activity line for an uploaded price chart: which item, and
// the average price the game itself shows for that window, so the player can
// check it against their screen. Falls back to the plain count when the item or
// the window is not known, or nothing sold in it.
func historyLine(itemID string, quality, timescale int, points []codexHistoryPoint) string {
	plain := "Sent price history (" + strconv.Itoa(len(points)) + " points) to Codex"
	window := historyWindow(timescale)
	if itemID == "" || window == "" {
		return plain
	}
	label := itemLabel(itemID)
	if q := qualityName(quality); q != "" {
		label += ", " + q
	}
	avg, ok := historyAverage(points)
	if !ok {
		return label + " - no sales in the " + window + " chart"
	}
	return label + " - " + window + " average " + formatSilver(avg)
}
