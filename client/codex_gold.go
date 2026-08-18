package client

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/ao-data/albiondata-client/lib"
	"github.com/ao-data/albiondata-client/log"
)

type codexGoldPayload struct {
	Server     string  `json:"server,omitempty"`
	Prices     []int   `json:"prices"`
	Timestamps []int64 `json:"timestamps"`
}

// uploadGoldToCodex contributes the gold/silver exchange price history (seen when
// the player opens the Gold Market) to Codex. Best-effort and opt-in: no-op unless
// market upload is enabled (--codex-market) and we're paired. Gold price is global
// per server (not city-specific).
func uploadGoldToCodex(up lib.GoldPricesUpload, server string) {
	if !ConfigGlobal.CodexMarketUpload {
		return
	}
	token := strings.TrimSpace(ConfigGlobal.CodexToken)
	if token == "" || len(up.Prices) == 0 {
		return
	}

	body, err := json.Marshal(codexGoldPayload{
		Server:     server,
		Prices:     up.Prices,
		Timestamps: up.TimeStamps,
	})
	if err != nil {
		log.Errorf("Codex gold: could not encode: %v", err)
		return
	}

	// Gold history is re-observable (the game re-fetches it): low-value (shed first).
	n := len(up.Prices)
	enqueueCodexUpload("Gold", "/api/client/gold", body, n, true, func() {
		log.Infof("Codex: uploaded %d gold prices.", n)
		Status.addActivity("Sent " + strconv.Itoa(n) + " gold prices to Codex")
	})
}
