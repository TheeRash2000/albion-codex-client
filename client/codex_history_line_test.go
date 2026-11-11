package client

import "testing"

// The activity line for a price chart shows the average the GAME shows for that
// chart, so a player can check the client against their screen.
func TestHistoryAverageMatchesTheGame(t *testing.T) {
	// Seen 2026-11-11, Exceptional Whitewood Planks, 4 week chart: the game said
	// "Average Price: 753 k". Totals as uploaded: 1,188 items, silver x10000.
	pts := []codexHistoryPoint{
		{ItemCount: 1000, SilverTotal: 7531064285000},
		{ItemCount: 188, SilverTotal: 8946904370000 - 7531064285000},
	}
	avg, ok := historyAverage(pts)
	if !ok || avg != 753106 {
		t.Fatalf("got %d (ok=%v), want 753106", avg, ok)
	}
}

func TestHistoryAverageIsWeightedByItemsSold(t *testing.T) {
	// One busy bucket at 100 and one quiet bucket at 1,000. The mean of the two
	// bucket averages would be 550; the game divides all silver by all items.
	pts := []codexHistoryPoint{
		{ItemCount: 99, SilverTotal: 99 * 100 * 10000},
		{ItemCount: 1, SilverTotal: 1000 * 10000},
	}
	if avg, _ := historyAverage(pts); avg != 109 {
		t.Fatalf("got %d, want 109", avg)
	}
}

func TestHistoryAverageWithNoSales(t *testing.T) {
	if _, ok := historyAverage(nil); ok {
		t.Error("no points: want ok=false")
	}
	if _, ok := historyAverage([]codexHistoryPoint{{ItemCount: 0, SilverTotal: 5}}); ok {
		t.Error("no items sold: want ok=false")
	}
}

func TestHistoryLine(t *testing.T) {
	pts := []codexHistoryPoint{{ItemCount: 2, SilverTotal: 2 * 753106 * 10000}}
	cases := []struct {
		name               string
		id                 string
		quality, timescale int
		pts                []codexHistoryPoint
		want               string
	}{
		{"4 week chart", "T8_PLANKS_LEVEL3@3", 1, 2, pts, "Exceptional Whitewood Planks - 4 week average 753,106"},
		{"7 day chart", "T4_BAG", 1, 1, pts, "Adept's Bag - 7 day average 753,106"},
		{"24 hour chart, a quality above Normal", "T4_BAG", 3, 0, pts, "Adept's Bag, Outstanding - 24 hour average 753,106"},
		{"nothing sold in the window", "T4_BAG", 1, 2, []codexHistoryPoint{{}}, "Adept's Bag - no sales in the 4 week chart"},
		{"item id not known: the plain count", "", 1, 2, pts, "Sent price history (1 points) to Codex"},
		{"window not known: the plain count", "T4_BAG", 1, 9, pts, "Sent price history (1 points) to Codex"},
	}
	for _, c := range cases {
		if got := historyLine(c.id, c.quality, c.timescale, c.pts); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

// The game often answers twice in a row; the feed should not double the line.
func TestActivityCollapsesAnImmediateRepeat(t *testing.T) {
	s := &ClientStatus{}
	s.addActivity("Adept's Bag - sell 980")
	s.addActivity("Adept's Bag - sell 980")
	s.addActivity("Adept's Bag - buy 900")
	s.addActivity("Adept's Bag - sell 980") // not adjacent to its twin: a new event
	if len(s.activity) != 3 {
		t.Fatalf("got %d lines, want 3", len(s.activity))
	}
}
