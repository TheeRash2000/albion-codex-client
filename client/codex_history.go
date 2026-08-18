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
	enqueueCodexUpload("Market history", "/api/client/market-history", body, n, true, func() {
		log.Infof("Codex: uploaded %d market-history points for item %d.", n, albionID)
		Status.addActivity("Sent price history (" + strconv.Itoa(n) + " points) to Codex")
	})
}
