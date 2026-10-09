package client

import "testing"

const goodOffer = `{"Id":1,"ItemTypeId":"T4_BAG","LocationId":null,"QualityLevel":2,"EnchantmentLevel":0,"UnitPriceSilver":50000,"Amount":3,"AuctionType":"offer","Expires":"2026-10-30T00:00:00"}`
const denOffer = `{"Id":2,"ItemTypeId":"T4_BAG","LocationId":"0fce2dc7-c22a-4379-addb-d2def3590565@BLACKBANK-3345","QualityLevel":1,"UnitPriceSilver":60000,"Amount":1,"AuctionType":"offer"}`
const goodRequest = `{"Id":3,"ItemTypeId":"T5_PLANKS","QualityLevel":1,"UnitPriceSilver":40000,"Amount":10,"AuctionType":"request"}`

// One malformed order used to hit log.Fatal, which exits the whole client: every
// capture (board, market, sales) stopped until the next launch. It must be skipped
// and the rest of the browse kept.
func TestDecodeOfferOrdersSkipsMalformedOrder(t *testing.T) {
	orders := decodeOfferOrders([]string{goodOffer, `{"Id":`, denOffer}, "3008")
	if len(orders) != 2 {
		t.Fatalf("got %d orders, want 2 (the malformed one skipped)", len(orders))
	}
	if orders[0].ItemID != "T4_BAG" || orders[0].LocationID != "3008" || orders[0].Price != 50000 {
		t.Errorf("city order decoded wrong: %+v", orders[0])
	}
	// A Smuggler's Den order keeps its own location, not the player's.
	if orders[1].LocationID != "0fce2dc7-c22a-4379-addb-d2def3590565@BLACKBANK-3345" {
		t.Errorf("den order lost its location: %q", orders[1].LocationID)
	}
}

func TestDecodeRequestOrdersSkipsMalformedOrder(t *testing.T) {
	orders := decodeRequestOrders([]string{"not json", goodRequest}, "3008")
	if len(orders) != 1 {
		t.Fatalf("got %d orders, want 1 (no empty order for the malformed one)", len(orders))
	}
	if orders[0].ItemID != "T5_PLANKS" || orders[0].LocationID != "3008" {
		t.Errorf("request decoded wrong: %+v", orders[0])
	}
}

// A field whose type no longer matches (e.g. after a game patch) is NOT skipped:
// that keeps today's behaviour (log and keep the fields that did decode), so this
// change cannot drop data that uploads now.
func TestDecodeOfferOrdersKeepsOrderWithTypeMismatch(t *testing.T) {
	mismatch := `{"Id":4,"ItemTypeId":"T4_BAG","QualityLevel":1,"UnitPriceSilver":50000,"Amount":"3","AuctionType":"offer"}`
	orders := decodeOfferOrders([]string{mismatch}, "3008")
	if len(orders) != 1 || orders[0].ItemID != "T4_BAG" || orders[0].Price != 50000 {
		t.Fatalf("type-mismatched order should be kept as before, got %+v", orders)
	}
}

// The response handlers must survive a batch that is entirely malformed. Before the
// fix this test process itself exited inside Process.
func TestAuctionResponsesSurviveMalformedOrders(t *testing.T) {
	state := &albionState{LocationId: "3008"}
	operationAuctionGetOffersResponse{MarketOrders: []string{`{"Id":`}}.Process(state)
	operationAuctionGetRequestsResponse{MarketOrders: []string{`{"Id":`}}.Process(state)
}
