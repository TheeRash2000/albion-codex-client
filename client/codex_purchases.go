package client

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/ao-data/albiondata-client/log"
)

// codexPurchase is one realized marketplace PURCHASE (from a finished buy-order
// mail) - the cost-basis side of the P&L ledger: what the player ACTUALLY paid for
// materials. UnitPrice is the true average paid per unit (total / amount), which for
// a buy order is <= the order price because of the "better price" deposit refund -
// so it is computed from the mail total, NOT the order unit price in the body.
type codexPurchase struct {
	MailID    int    `json:"mail_id"` // dedupe key (a re-read mail is the same purchase)
	ItemID    string `json:"item_id"` // already a unique NAME (e.g. T8_ARMOR_LEATHER_SET3)
	City      string `json:"city"`
	Amount    int    `json:"amount"`
	UnitPrice int    `json:"unit_price"` // silver per unit actually paid (total/amount, /10000)
}

type codexPurchasesPayload struct {
	Server    string          `json:"server,omitempty"`
	Purchases []codexPurchase `json:"purchases"`
}

// uploadPurchaseToCodex contributes a realized purchase to Codex (the ledger's
// cost basis). Best-effort and opt-in: no-op unless market upload is enabled
// (--codex-market) and the client is paired. Routed through the delivery queue, so
// a transient failure is retried rather than dropping the buy.
func uploadPurchaseToCodex(p codexPurchase, server string) {
	if !ConfigGlobal.CodexMarketUpload {
		return
	}
	token := strings.TrimSpace(ConfigGlobal.CodexToken)
	if token == "" || p.ItemID == "" || p.Amount <= 0 || p.UnitPrice <= 0 {
		return
	}

	body, err := json.Marshal(codexPurchasesPayload{Server: server, Purchases: []codexPurchase{p}})
	if err != nil {
		log.Errorf("Codex purchases: could not encode: %v", err)
		return
	}

	// A realized purchase is unique and only appears once (when the mail is read),
	// so it is high-value: never shed from a full queue.
	enqueueCodexUpload("Purchases", "/api/client/purchases", body, 1, false, func() {
		log.Infof("Codex: recorded a purchase (%dx %s @ %d).", p.Amount, p.ItemID, p.UnitPrice)
		Status.addActivity("Recorded your purchase: " + strconv.Itoa(p.Amount) + "x " + humanizeItemID(p.ItemID) + " at " + formatSilver(p.UnitPrice) + " each")
		refreshClientStatsAfterTrade()
	})
}

// decodeBuyPurchase reads a finished BUY-order mail body ("amount|item|total|
// unitPrice|0") into a codexPurchase. The paid unit price is total/amount (the
// actual average paid after the buy-order refund), NOT body[3] (the order price).
// Returns ok=false on a malformed or empty body so a bad mail is skipped, not sent.
func decodeBuyPurchase(mailID int, body []string, mailInfo *MailInfo) (codexPurchase, bool) {
	if len(body) < 4 {
		return codexPurchase{}, false
	}
	amount, err := strconv.Atoi(strings.TrimSpace(body[0]))
	if err != nil || amount <= 0 {
		return codexPurchase{}, false
	}
	item := strings.TrimSpace(body[1])
	if item == "" {
		return codexPurchase{}, false
	}
	// body[2] is the total actually paid, in silver x10000 (the wire scale AODP
	// divides out). Average paid per unit = total/amount, then /10000 to silver.
	total, err := strconv.Atoi(strings.TrimSpace(body[2]))
	if err != nil || total <= 0 {
		return codexPurchase{}, false
	}
	unit := (total / amount) / 10000
	if unit <= 0 {
		return codexPurchase{}, false
	}

	city := ""
	if mailInfo != nil {
		city = mailInfo.LocationID
	}
	return codexPurchase{MailID: mailID, ItemID: item, City: city, Amount: amount, UnitPrice: unit}, true
}
