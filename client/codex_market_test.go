package client

import "testing"

func TestHumanizeItemID(t *testing.T) {
	cases := map[string]string{
		"T7_PLANKS_LEVEL2@2": "T7.2 Planks",
		"T4_BAG":             "T4 Bag",
		"T8_2H_AXE@3":        "T8.3 2h Axe",
		"T5_PLANKS":          "T5 Planks",
		"T6_MEAL_OMELETTE":   "T6 Meal Omelette",
		"T8_2H_AXE_KEEPER":   "T8 2h Axe Keeper",
		"T4_METALBAR":        "T4 Metalbar",
	}
	for in, want := range cases {
		if got := humanizeItemID(in); got != want {
			t.Errorf("humanizeItemID(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFormatSilver(t *testing.T) {
	cases := map[int]string{
		0:       "0",
		999:     "999",
		1234:    "1,234",
		1234567: "1,234,567",
	}
	for in, want := range cases {
		if got := formatSilver(in); got != want {
			t.Errorf("formatSilver(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestFormatPriceLine(t *testing.T) {
	sell := 1234
	buy := 1180
	both := formatPriceLine(codexPrice{ItemID: "T7_PLANKS@2", Quality: 2, SellMin: &sell, BuyMax: &buy})
	if both != "T7.2 Planks Q2 - sell 1,234 / buy 1,180" {
		t.Errorf("both = %q", both)
	}
	sellOnly := formatPriceLine(codexPrice{ItemID: "T4_BAG", Quality: 1, SellMin: &sell})
	if sellOnly != "T4 Bag - sell 1,234" {
		t.Errorf("sellOnly = %q", sellOnly)
	}
}
