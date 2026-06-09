package client

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/ao-data/albiondata-client/log"
)

// codexSale is one realized marketplace sale (from a finished sell-order mail) -
// the data nobody else has: what ACTUALLY sold, not just what's listed.
type codexSale struct {
	MailID    int    `json:"mail_id"` // dedupe key (a re-read mail is the same sale)
	ItemID    string `json:"item_id"` // already a unique NAME (e.g. T4_BAG)
	City      string `json:"city"`
	Amount    int    `json:"amount"`
	UnitPrice int    `json:"unit_price"` // silver per unit (already /10000)
}

type codexSalesPayload struct {
	Server string      `json:"server,omitempty"`
	Sales  []codexSale `json:"sales"`
}

// uploadSaleToCodex contributes a realized sale to Codex (powers sell-through /
// real-demand intel). Best-effort and opt-in: no-op unless market upload is enabled
// (--codex-market) and the client is paired.
func uploadSaleToCodex(s codexSale, server string) {
	if !ConfigGlobal.CodexMarketUpload {
		return
	}
	token := strings.TrimSpace(ConfigGlobal.CodexToken)
	if token == "" || s.ItemID == "" || s.Amount <= 0 || s.UnitPrice <= 0 {
		return
	}

	body, err := json.Marshal(codexSalesPayload{Server: server, Sales: []codexSale{s}})
	if err != nil {
		log.Errorf("Codex sales: could not encode: %v", err)
		return
	}

	req, err := http.NewRequest(http.MethodPost, codexBase()+"/api/client/sales", bytes.NewReader(body))
	if err != nil {
		log.Errorf("Codex sales: could not build request: %v", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("User-Agent", codexClientVersion)

	resp, err := (&http.Client{Timeout: 20 * time.Second}).Do(req)
	if err != nil {
		log.Errorf("Codex sales: upload failed: %v", err)
		return
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		log.Infof("Codex: recorded a sale (%dx %s @ %d).", s.Amount, s.ItemID, s.UnitPrice)
		Status.addActivity("Recorded your sale: " + strconv.Itoa(s.Amount) + "x " + humanizeItemID(s.ItemID) + " at " + formatSilver(s.UnitPrice) + " each")
	case resp.StatusCode == http.StatusUnauthorized:
		log.Error("Codex sales: device token rejected (401).")
	default:
		log.Errorf("Codex sales: rejected with status %d.", resp.StatusCode)
	}
}
