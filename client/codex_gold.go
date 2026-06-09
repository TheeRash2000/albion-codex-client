package client

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

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

	req, err := http.NewRequest(http.MethodPost, codexBase()+"/api/client/gold", bytes.NewReader(body))
	if err != nil {
		log.Errorf("Codex gold: could not build request: %v", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("User-Agent", codexClientVersion)

	resp, err := (&http.Client{Timeout: 20 * time.Second}).Do(req)
	if err != nil {
		log.Errorf("Codex gold: upload failed: %v", err)
		return
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		log.Infof("Codex: uploaded %d gold prices.", len(up.Prices))
		Status.addActivity("Sent " + strconv.Itoa(len(up.Prices)) + " gold prices to Codex")
	case resp.StatusCode == http.StatusUnauthorized:
		log.Error("Codex gold: device token rejected (401).")
	default:
		log.Errorf("Codex gold: rejected with status %d.", resp.StatusCode)
	}
}
