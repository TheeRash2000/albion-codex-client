package client

import "testing"

// A finished buy-order mail body is "amount|item|total|unitPrice|0" where total is
// the ACTUAL silver paid (x10000) after the buy-order refund, so the paid unit is
// total/amount - which is <= body[3] (the order price). decodeBuyPurchase must use
// the total, not the order price.
func TestDecodeBuyPurchaseUsesPaidTotalNotOrderPrice(t *testing.T) {
	// 3x bought, actually paid 254,000 each (total 762,000 silver), order price was
	// 260,000. All silver values on the wire are x10000.
	body := []string{"3", "T8_ARMOR_LEATHER_SET3", "7620000000", "2600000000", "0"}
	mi := &MailInfo{ID: 42, LocationID: "Caerleon", OrderType: "MARKETPLACE_BUYORDER_FINISHED_SUMMARY"}

	p, ok := decodeBuyPurchase(42, body, mi)
	if !ok {
		t.Fatal("a well-formed buy-order body should decode")
	}
	if p.Amount != 3 {
		t.Fatalf("amount: got %d want 3", p.Amount)
	}
	if p.ItemID != "T8_ARMOR_LEATHER_SET3" {
		t.Fatalf("item: got %q", p.ItemID)
	}
	if p.UnitPrice != 254000 {
		t.Fatalf("unit price must be the PAID average (total/amount), got %d want 254000", p.UnitPrice)
	}
	if p.City != "Caerleon" {
		t.Fatalf("city: got %q want Caerleon", p.City)
	}
	if p.MailID != 42 {
		t.Fatalf("mail id: got %d want 42", p.MailID)
	}
}

func TestDecodeBuyPurchaseRejectsMalformed(t *testing.T) {
	cases := []struct {
		name string
		body []string
	}{
		{"short body", []string{"3", "T4_BAG"}},
		{"zero amount", []string{"0", "T4_BAG", "1000000", "1000000", "0"}},
		{"empty item", []string{"3", "", "1000000", "1000000", "0"}},
		{"non-numeric amount", []string{"x", "T4_BAG", "1000000", "1000000", "0"}},
		{"zero total", []string{"3", "T4_BAG", "0", "1000000", "0"}},
	}
	for _, c := range cases {
		if _, ok := decodeBuyPurchase(1, c.body, nil); ok {
			t.Errorf("%s: expected ok=false", c.name)
		}
	}
}

// City is optional: a buy decoded without loaded MailInfos still yields a purchase
// (the site stores city nullable), just with an empty city.
func TestDecodeBuyPurchaseNoMailInfo(t *testing.T) {
	body := []string{"1", "T4_PLANKS", "500000000", "500000000", "0"}
	p, ok := decodeBuyPurchase(7, body, nil)
	if !ok {
		t.Fatal("should decode without mail info")
	}
	if p.City != "" {
		t.Fatalf("city should be empty without mail info, got %q", p.City)
	}
	if p.UnitPrice != 50000 {
		t.Fatalf("unit price: got %d want 50000", p.UnitPrice)
	}
}
