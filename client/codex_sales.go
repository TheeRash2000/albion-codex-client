package client

import (
	"encoding/json"
	"strconv"
	"strings"

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

	// A realized sale is unique and unrecoverable if lost (it only appears once, when
	// the mail is read), so it is high-value: never shed from a full queue.
	enqueueCodexUpload("Sales", "/api/client/sales", body, 1, false, func() {
		log.Infof("Codex: recorded a sale (%dx %s @ %d).", s.Amount, s.ItemID, s.UnitPrice)
		Status.addActivity("Recorded your sale: " + strconv.Itoa(s.Amount) + "x " + humanizeItemID(s.ItemID) + " at " + formatSilver(s.UnitPrice) + " each")
		refreshClientStatsAfterTrade()
	})
}
