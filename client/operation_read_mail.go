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

	// MailInfos (order type + city) comes from a SEPARATE GetMailInfos response that
	// is frequently NOT loaded when ReadMail fires - a cross-packet dependency that
	// silently dropped every realized sale. The ReadMail body is self-contained
	// ("amount|itemId|total|unitPrice|0" for a finished sell order), so we read the
	// sale from the body and only USE MailInfos for the order type/city when present.
	mailInfo := MailInfos.getMailInfo(op.ID)
	orderType := ""
	if mailInfo != nil {
		orderType = mailInfo.OrderType
	}

	sellFinished := orderType == "MARKETPLACE_SELLORDER_FINISHED_SUMMARY" ||
		(orderType == "" && looksLikeSellFinished(body))

	if sellFinished {
		log.Debug("Read finished sell order.")
		notification = decodeSellNotification(op, body, state, mailInfo)

		// Codex: contribute the realized sale (what actually sold + at what price).
		if sell, ok := notification.(*lib.MarketSellNotification); ok && sell != nil {
			srv, _ := state.GetServer()
			uploadSaleToCodex(codexSale{
				MailID:    sell.MailID,
				ItemID:    sell.ItemID,
				City:      sell.LocationID,
				Amount:    sell.Amount,
				UnitPrice: sell.Price,
			}, codexServerName(srv))
		}
	} else if orderType == "MARKETPLACE_BUYORDER_FINISHED_SUMMARY" {
		// A finished BUY order = a realized purchase (the ledger's cost basis). A
		// finished buy and finished sell mail have the SAME body shape, so this is
		// gated on the confirmed order type (from GetMailInfos), never the body alone
		// - otherwise a buy could be misread as a sale. Codex-only: no legacy
		// market-notification for purchases, so return after uploading.
		log.Debug("Read finished buy order (purchase).")
		if p, ok := decodeBuyPurchase(op.ID, body, mailInfo); ok {
			srv, _ := state.GetServer()
			uploadPurchaseToCodex(p, codexServerName(srv))
		}
		return
	} else if orderType == "MARKETPLACE_SELLORDER_EXPIRED_SUMMARY" {
		log.Debug("Read expired sell order.")
		notification = decodeExpiryNotification(op, body)
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

// looksLikeSellFinished infers a finished-sell-order mail from the body alone
// ("amount|itemId|total|unitPrice|0"): a numeric amount and unit price with a
// non-numeric item id in slot 1. Used when the MailInfos cache (order type) is not
// loaded yet, so a real sale is never dropped.
func looksLikeSellFinished(body []string) bool {
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
