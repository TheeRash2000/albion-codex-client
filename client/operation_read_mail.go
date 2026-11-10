package client

import (
	"strconv"
	"strings"

	"github.com/ao-data/albiondata-client/lib"
	"github.com/ao-data/albiondata-client/log"
	uuid "github.com/nu7hatch/gouuid"
)

type operationReadMail struct {
	ID   int    `mapstructure:"0"`
	Body string `mapstructure:"1"`
}

func (op operationReadMail) Process(state *albionState) {
	log.Debug("Got ReadMail operation...")
	var notification lib.MarketNotification

	// split the mail body
	body := strings.Split(op.Body, "|")

	// The mail's type and city come from the mailbox list (GetMailInfos), which the
	// game sends when the mailbox opens - before any mail can be read.
	mailInfo := MailInfos.getMailInfo(op.ID)
	orderType := ""
	if mailInfo != nil {
		orderType = mailInfo.OrderType
	}

	switch classifyMail(orderType, body) {
	case mailSale:
		log.Debug("Read finished sell order.")
		notification = decodeSellNotification(op, body, state, mailInfo)

		// Codex: contribute the realized sale (what actually sold + at what price).
		// Decoded on its own: the legacy notification above carries the LISTED
		// price, which is not what the player received when the order filled
		// above it.
		if sale, ok := decodeSale(op.ID, body, mailInfo, state.LocationId); ok {
			srv, _ := state.GetServer()
			uploadSaleToCodex(sale, codexServerName(srv))
		}
	case mailPurchase:
		// A finished BUY order = a realized purchase (the ledger's cost basis).
		// Codex-only: no legacy market-notification for purchases, so return after
		// uploading.
		log.Debug("Read finished buy order (purchase).")
		if p, ok := decodeBuyPurchase(op.ID, body, mailInfo); ok {
			srv, _ := state.GetServer()
			uploadPurchaseToCodex(p, codexServerName(srv))
		}
		return
	case mailSellExpired:
		log.Debug("Read expired sell order.")
		notification = decodeExpiryNotification(op, body)
	case mailUnknownOrder:
		// Recording a guess would put a purchase in the sales data (or the reverse)
		// at the wrong price, so record nothing and say so.
		log.Infof("Mail %d is a finished market order, but the mailbox list that says whether it was a sale or a purchase was not seen; not recorded. Close and reopen the mailbox, then read it again.", op.ID)
		Status.addActivity("Could not tell if a market mail was a sale or a purchase - reopen your mailbox and read it again")
		return
	}

	if notification == nil {
		return
	}

	upload := lib.MarketNotificationUpload{
		Type:         notification.Type(),
		Notification: notification,
	}

	identifier, _ := uuid.NewV4()
	sendMsgToPrivateUploaders(&upload, lib.NatsMarketNotifications, state, identifier.String())
}

// decodeSale reads a finished SELL-order mail body ("amount|item|total|listedPrice|0")
// into a codexSale. The unit price is what was realized, total/amount, NOT body[3]
// (the price the order was listed at): a sell order that matches a higher buy
// order fills above its listing. Seen live 2026-11-10: 2 listed at 500, filled by
// a 540 buy order, total 1,080. The total is before market tax, as the ledger
// expects. Without a usable total it falls back to the listed price rather than
// drop the sale. City comes from the mailbox list, else the player's location.
// Mirrors decodeBuyPurchase.
func decodeSale(mailID int, body []string, mailInfo *MailInfo, playerLocation string) (codexSale, bool) {
	if len(body) < 4 {
		return codexSale{}, false
	}
	amount, err := strconv.Atoi(strings.TrimSpace(body[0]))
	if err != nil || amount <= 0 {
		return codexSale{}, false
	}
	item := strings.TrimSpace(body[1])
	if item == "" {
		return codexSale{}, false
	}
	// Silver on the wire is x10000.
	unit := 0
	if total, err := strconv.Atoi(strings.TrimSpace(body[2])); err == nil && total > 0 {
		unit = (total / amount) / 10000
	}
	if unit <= 0 {
		if listed, err := strconv.Atoi(strings.TrimSpace(body[3])); err == nil {
			unit = listed / 10000
		}
	}
	if unit <= 0 {
		return codexSale{}, false
	}
	city := playerLocation
	if mailInfo != nil && mailInfo.LocationID != "" {
		city = mailInfo.LocationID
	}
	return codexSale{MailID: mailID, ItemID: item, City: city, Amount: amount, UnitPrice: unit}, true
}

// mailKind is what a read mail turned out to be.
type mailKind int

const (
	mailOther        mailKind = iota // not a market mail we record
	mailSale                         // finished sell order
	mailPurchase                     // finished buy order
	mailSellExpired                  // expired sell order
	mailUnknownOrder                 // a finished order, but sale or purchase is not known
)

// classifyMail decides what a mail is from its type (from the mailbox list). A
// finished buy order and a finished sell order have the SAME body
// ("amount|itemId|total|unitPrice|0"), so the body can never decide between
// them: with no type, a finished-order body is mailUnknownOrder, never a sale.
func classifyMail(orderType string, body []string) mailKind {
	switch orderType {
	case "MARKETPLACE_SELLORDER_FINISHED_SUMMARY":
		return mailSale
	case "MARKETPLACE_BUYORDER_FINISHED_SUMMARY":
		return mailPurchase
	case "MARKETPLACE_SELLORDER_EXPIRED_SUMMARY":
		return mailSellExpired
	case "":
		if looksLikeFinishedOrder(body) {
			return mailUnknownOrder
		}
	}
	return mailOther
}

// looksLikeFinishedOrder reports whether a mail body has the finished-order shape
// ("amount|itemId|total|unitPrice|0"): a numeric amount and unit price with a
// non-numeric item id in slot 1. Both finished sell and finished buy orders match.
func looksLikeFinishedOrder(body []string) bool {
	if len(body) < 4 {
		return false
	}
	if _, err := strconv.Atoi(strings.TrimSpace(body[0])); err != nil {
		return false
	}
	if _, err := strconv.Atoi(strings.TrimSpace(body[3])); err != nil {
		return false
	}
	// item id is non-numeric (letters / underscore), e.g. T8_ARMOR_LEATHER_SET3
	return strings.ContainsAny(body[1], "ABCDEFGHIJKLMNOPQRSTUVWXYZ_")
}

func decodeSellNotification(op operationReadMail, body []string, state *albionState, mailInfo *MailInfo) lib.MarketNotification {
	if len(body) < 4 {
		return nil
	}
	notification := &lib.MarketSellNotification{}
	notification.MailID = op.ID

	amount, err := strconv.Atoi(body[0])
	if err != nil {
		log.Error("Could not parse amount in market sell notification ", err)
		return nil
	}

	price, err := strconv.Atoi(body[3])
	if err != nil {
		log.Error("Could not parse price in market sell notification ", err)
		return nil
	}

	notification.Amount = amount
	notification.ItemID = body[1]
	notification.Price = price / 10000
	notification.TotalAfterTaxes = float32(float32(notification.Price) * float32(notification.Amount) * (1.0 - lib.SalesTax))

	// Location/expiry come from the mail list when it is loaded; otherwise fall back
	// to the current city so the sale still carries a location.
	if mailInfo != nil {
		notification.LocationID = mailInfo.LocationID
		notification.Expires = mailInfo.StringExpires()
	} else {
		notification.LocationID = state.LocationId
	}

	return notification
}

func decodeExpiryNotification(op operationReadMail, body []string) lib.MarketNotification {
	notification := &lib.MarketExpiryNotification{}
	notification.MailID = op.ID

	sold, err := strconv.Atoi(body[0])
	if err != nil {
		log.Error("Could not parse amount in market sell notification ", err)
		return nil
	}

	amount, err := strconv.Atoi(body[1])
	if err != nil {
		log.Error("Could not parse amount in market sell notification ", err)
		return nil
	}

	price, err := strconv.Atoi(body[2])
	if err != nil {
		log.Error("Could not parse price in market sell notification ", err)
		return nil
	}

	notification.Amount = amount
	notification.ItemID = body[1]
	notification.Price = price / 10000
	notification.Sold = sold

	mailInfo := MailInfos.getMailInfo(op.ID)
	notification.LocationID = mailInfo.LocationID
	notification.Expires = mailInfo.StringExpires()

	return notification
}
