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

// An activity line names the item the way the game does, says the quality only
// when it is not Normal, and lists whichever sides were captured.
func TestFormatPriceLine(t *testing.T) {
	sell := 1234
	buy := 1180
	cases := []struct {
		name string
		p    codexPrice
		want string
	}{
		{"both sides, a quality above Normal",
			codexPrice{ItemID: "T4_BAG", Quality: 3, SellMin: &sell, BuyMax: &buy},
			"Adept's Bag, Outstanding - sell 1,234 / buy 1,180"},
		{"Normal quality is left unsaid",
			codexPrice{ItemID: "T4_BAG", Quality: 1, SellMin: &sell},
			"Adept's Bag - sell 1,234"},
		{"enchanted gear shows its level the way the game writes it",
			codexPrice{ItemID: "T5_2H_ARCANESTAFF_CRYSTAL@2", Quality: 1, BuyMax: &buy},
			"Expert's Astral Staff (5.2) - buy 1,180"},
		{"an enchanted resource is named by its level already",
			codexPrice{ItemID: "T7_PLANKS_LEVEL2@2", Quality: 1, SellMin: &sell},
			"Rare Ashenbark Planks - sell 1,234"},
		{"an item the name table does not know keeps the id-based label",
			codexPrice{ItemID: "T9_SOMETHING_NEW", Quality: 2, SellMin: &sell},
			"T9 Something New, Good - sell 1,234"},
		{"no price on either side",
			codexPrice{ItemID: "T4_BAG", Quality: 1},
			"Adept's Bag sent to Codex"},
	}
	for _, c := range cases {
		if got := formatPriceLine(c.p); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}
