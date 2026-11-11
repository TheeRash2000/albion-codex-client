package client

import "testing"

// The activity feed should call an item what the game and the market search
// call it, the same name Most wanted shows for it.
func TestItemLabel(t *testing.T) {
	cases := []struct{ id, want string }{
		{"T5_2H_ARCANESTAFF_CRYSTAL", "Expert's Astral Staff"},
		{"T5_2H_ARCANESTAFF_CRYSTAL@2", "Expert's Astral Staff (5.2)"},
		{"T5_2H_ARCANESTAFF_CRYSTAL@0", "Expert's Astral Staff"},
		{"T4_BAG", "Adept's Bag"},
		{"T4_MAIN_SWORD@4", "Adept's Broadsword (4.4)"},
		{"T5_MOUNT_SPIDER_HELL", "Hellspinner"},
		{"T4_PLANKS", "Pine Planks"},
		// A resource's name already carries its enchant level.
		{"T7_PLANKS_LEVEL2@2", "Rare Ashenbark Planks"},
		{"T7_PLANKS_LEVEL2", "Rare Ashenbark Planks"},
		{"T1_HIDE", "Scraps of Hide"},
		// Not in the table: the id-based label, never an empty line.
		{"T9_SOMETHING_NEW", humanizeItemID("T9_SOMETHING_NEW")},
		{"T9_SOMETHING_NEW@1", humanizeItemID("T9_SOMETHING_NEW@1")},
		{"", humanizeItemID("")},
	}
	for _, c := range cases {
		if got := itemLabel(c.id); got != c.want {
			t.Errorf("itemLabel(%q) = %q, want %q", c.id, got, c.want)
		}
	}
}

func TestItemNamesTableLoads(t *testing.T) {
	if _, ok := itemName("T4_BAG"); !ok {
		t.Fatal("the embedded name table did not load")
	}
	if len(itemNames) < 4000 {
		t.Fatalf("name table has %d entries, expected several thousand", len(itemNames))
	}
}

func TestQualityName(t *testing.T) {
	want := map[int]string{0: "", 1: "", 2: "Good", 3: "Outstanding", 4: "Excellent", 5: "Masterpiece", 6: ""}
	for q, w := range want {
		if got := qualityName(q); got != w {
			t.Errorf("qualityName(%d) = %q, want %q", q, got, w)
		}
	}
}
