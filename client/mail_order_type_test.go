package client

import "testing"

// mailListResponse is a GetMailInfos response in the layout the game sent on
// 2026-11-10 (captured from a live session, trimmed to three mails): city at 7,
// mail type at 11, received time at 12. Positions 6 and 10 hold flags, which is
// what the client used to read the city and the mail type from.
func mailListResponse() map[uint8]interface{} {
	return map[uint8]interface{}{
		0:   []byte{0x89, 0xcf, 0xc7, 0xa6},
		2:   int8(0),
		3:   []int32{1637845419, 1637842331, 1219140354},
		6:   []bool{true, true, true},
		7:   []string{"4002", "4002", "Albion Online"},
		10:  []bool{false, true, true},
		11:  []string{"MARKETPLACE_SELLORDER_FINISHED_SUMMARY", "MARKETPLACE_BUYORDER_FINISHED_SUMMARY", "MAIL_RECOVERVAULT_HIDEOUT_PLAYER"},
		12:  []int64{639298734818391450, 639298729443057710, 638885952002300290},
		13:  []bool{false, false, false},
		253: int16(opGetMailInfos),
	}
}

// The mail list must decode, and give each mail its type and city. When it did
// not, no mail had a known type and every finished order was filed as a sale.
func TestMailListGivesEachMailItsTypeAndCity(t *testing.T) {
	MailInfos = nil
	defer func() { MailInfos = nil }()

	op, err := decodeResponse(mailListResponse())
	if err != nil {
		t.Fatalf("the mail list did not decode: %v", err)
	}
	if op == nil {
		t.Fatal("the mail list was not recognised")
	}
	op.Process(&albionState{})

	sell := MailInfos.getMailInfo(1637845419)
	if sell == nil || sell.OrderType != "MARKETPLACE_SELLORDER_FINISHED_SUMMARY" || sell.LocationID != "4002" {
		t.Errorf("sell mail: got %+v", sell)
	}
	buy := MailInfos.getMailInfo(1637842331)
	if buy == nil || buy.OrderType != "MARKETPLACE_BUYORDER_FINISHED_SUMMARY" || buy.LocationID != "4002" {
		t.Errorf("buy mail: got %+v", buy)
	}
}

// Reopening the mailbox re-sends the list; a mail must not be cached twice.
func TestMailListReopenedDoesNotDuplicate(t *testing.T) {
	MailInfos = nil
	defer func() { MailInfos = nil }()

	for i := 0; i < 3; i++ {
		op, err := decodeResponse(mailListResponse())
		if err != nil {
			t.Fatal(err)
		}
		op.Process(&albionState{})
	}
	if len(MailInfos) != 3 {
		t.Errorf("cached %d mail infos after three opens, want 3", len(MailInfos))
	}
}

// A list whose columns are not all the same length must not crash the client.
func TestMailListWithShortColumnsDoesNotPanic(t *testing.T) {
	MailInfos = nil
	defer func() { MailInfos = nil }()

	operationGetMailInfosResponse{
		MailIDs:    []int{1, 2, 3},
		Locations:  []string{"4002"},
		OrderTypes: []string{"MARKETPLACE_SELLORDER_FINISHED_SUMMARY", "MARKETPLACE_BUYORDER_FINISHED_SUMMARY"},
	}.Process(&albionState{})

	if got := MailInfos.getMailInfo(2); got == nil || got.OrderType != "MARKETPLACE_BUYORDER_FINISHED_SUMMARY" || got.LocationID != "" {
		t.Errorf("mail 2: got %+v", got)
	}
	if got := MailInfos.getMailInfo(3); got == nil || got.OrderType != "" {
		t.Errorf("mail 3: got %+v", got)
	}
}

// A finished buy order and a finished sell order have the same mail body, so the
// body can never decide which one it is. Only the mail type can. With no type,
// the mail is "unknown" and must not be recorded as either.
func TestMailKindComesFromTheMailTypeNeverTheBody(t *testing.T) {
	// The two mails read in the captured session: 5 bought (order price 4,800,
	// paid 23,869) and 2 sold at 500.
	bought := []string{"5", "T4_OFF_ORB_MORGANA", "238690000", "48000000", "0"}
	sold := []string{"2", "T4_OFF_ORB_MORGANA", "10800000", "5000000", "0"}

	cases := []struct {
		name      string
		orderType string
		body      []string
		want      mailKind
	}{
		{"sell mail", "MARKETPLACE_SELLORDER_FINISHED_SUMMARY", sold, mailSale},
		{"buy mail", "MARKETPLACE_BUYORDER_FINISHED_SUMMARY", bought, mailPurchase},
		{"expired sell mail", "MARKETPLACE_SELLORDER_EXPIRED_SUMMARY", []string{"1", "3", "5000000"}, mailSellExpired},
		{"buy mail, type not known", "", bought, mailUnknownOrder},
		{"sell mail, type not known", "", sold, mailUnknownOrder},
		{"player mail, type not known", "", []string{"hello there"}, mailOther},
		{"system mail", "MAIL_RECOVERVAULT_HIDEOUT_PLAYER", []string{"x"}, mailOther},
	}
	for _, c := range cases {
		if got := classifyMail(c.orderType, c.body); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

// End to end on the captured purchase: with the list loaded, the "Items Bought"
// mail is a purchase at the price actually paid (23,869 / 5), in Fort Sterling.
func TestCapturedBuyMailIsAPurchaseAtThePaidPrice(t *testing.T) {
	MailInfos = nil
	defer func() { MailInfos = nil }()

	op, err := decodeResponse(mailListResponse())
	if err != nil {
		t.Fatal(err)
	}
	op.Process(&albionState{})

	const mailID = 1637842331
	body := []string{"5", "T4_OFF_ORB_MORGANA", "238690000", "48000000", "0"}
	info := MailInfos.getMailInfo(mailID)
	if info == nil {
		t.Fatal("mail not in the list")
	}
	if kind := classifyMail(info.OrderType, body); kind != mailPurchase {
		t.Fatalf("kind = %v, want a purchase", kind)
	}
	p, ok := decodeBuyPurchase(mailID, body, info)
	if !ok {
		t.Fatal("purchase did not decode")
	}
	if p.Amount != 5 || p.ItemID != "T4_OFF_ORB_MORGANA" || p.UnitPrice != 4773 || p.City != "4002" {
		t.Errorf("purchase = %+v, want 5x T4_OFF_ORB_MORGANA at 4773 in 4002", p)
	}
}
