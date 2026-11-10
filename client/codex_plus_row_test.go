package client

import (
	"encoding/json"
	"testing"
	"time"
)

func f64(v float64) *float64 { return &v }
func str(v string) *string   { return &v }

var plusNow = time.Date(2026, 11, 10, 8, 0, 0, 0, time.UTC)

// The site's contribution object must survive the decode, and a response from a
// site that does not send it yet must decode to "no contribution", not zeros.
func TestClientStatsContributionDecodes(t *testing.T) {
	raw := `{"captures":{"pricesTotal":10},"contribution":{"score":62400,"threshold":100000,"keepAt":60000,
		"status":"below","pointsToEarn":37600,"earned":false,"codexPlus":false,"updatedAt":"2026-11-09T18:00:00Z"}}`
	var s ClientStats
	if err := json.Unmarshal([]byte(raw), &s); err != nil {
		t.Fatal(err)
	}
	c := s.Contribution
	if c == nil || c.Score == nil || *c.Score != 62400 || c.Threshold != 100000 || c.KeepAt != 60000 ||
		c.Status != "below" || c.PointsToEarn != 37600 || c.Earned || c.CodexPlus ||
		c.UpdatedAt == nil || *c.UpdatedAt != "2026-11-09T18:00:00Z" {
		t.Fatalf("contribution decoded wrong: %+v", c)
	}

	for _, older := range []string{`{"captures":{"pricesTotal":10}}`, `{"captures":{"pricesTotal":10},"contribution":null}`} {
		var o ClientStats
		if err := json.Unmarshal([]byte(older), &o); err != nil {
			t.Fatal(err)
		}
		if o.Contribution != nil {
			t.Errorf("%s: contribution = %+v, want nil", older, o.Contribution)
		}
	}
}

func TestCodexPlusRow(t *testing.T) {
	cases := []struct {
		name string
		c    *ClientContribution
		want PlusRow
	}{
		{
			name: "no data from the site",
			c:    nil,
			want: PlusRow{Title: "Codex+", Detail: "Earn it free by contributing with this client", Chip: "Learn more", Percent: -1},
		},
		{
			name: "working towards it",
			c: &ClientContribution{Score: f64(62400), Threshold: 100000, KeepAt: 60000, Status: "below",
				PointsToEarn: 37600, UpdatedAt: str("2026-11-09T18:00:00Z")},
			want: PlusRow{Title: "Codex+", Detail: "62,400 / 100,000 points - 37,600 to earn it free",
				Chip: "62%", Percent: 62, Updated: "Score updated 14 hours ago"},
		},
		{
			// A bar at 0% and "100,000 to go" tells a new player nothing useful.
			name: "just linked, nothing scored yet",
			c:    &ClientContribution{Score: nil, Threshold: 100000, KeepAt: 60000, Status: "below", PointsToEarn: 100000},
			want: PlusRow{Title: "Codex+", Detail: "Earn it free by contributing with this client", Chip: "Learn more", Percent: -1},
		},
		{
			// Far from the threshold the points are
			// shown, without a near-empty bar or a five-figure gap that reads as
			// unreachable.
			name: "contributing, far from the threshold",
			c: &ClientContribution{Score: f64(2106), Threshold: 100000, KeepAt: 60000, Status: "below",
				PointsToEarn: 97894, UpdatedAt: str("2026-11-09T18:00:00Z")},
			want: PlusRow{Title: "Codex+", Detail: "2,106 contribution points - top contributors earn Codex+ free",
				Chip: "Learn more", Percent: -1, Updated: "Score updated 14 hours ago"},
		},
		{
			name: "one point under where the bar starts",
			c:    &ClientContribution{Score: f64(9999), Threshold: 100000, KeepAt: 60000, Status: "below", PointsToEarn: 90001},
			want: PlusRow{Title: "Codex+", Detail: "9,999 contribution points - top contributors earn Codex+ free",
				Chip: "Learn more", Percent: -1},
		},
		{
			name: "where the bar starts",
			c:    &ClientContribution{Score: f64(10000), Threshold: 100000, KeepAt: 60000, Status: "below", PointsToEarn: 90000},
			want: PlusRow{Title: "Codex+", Detail: "10,000 / 100,000 points - 90,000 to earn it free", Chip: "10%", Percent: 10},
		},
		{
			name: "over the line, waiting for the daily update to grant it",
			c:    &ClientContribution{Score: f64(100500), Threshold: 100000, KeepAt: 60000, Status: "below", PointsToEarn: 0},
			want: PlusRow{Title: "Codex+", Detail: "100,500 / 100,000 points - granted at the next daily update",
				Chip: "100%", Percent: 100},
		},
		{
			name: "earned",
			c: &ClientContribution{Score: f64(120000), Threshold: 100000, KeepAt: 60000, Status: "earned",
				Earned: true, CodexPlus: true, UpdatedAt: str("2026-11-08T07:00:00Z")},
			want: PlusRow{Title: "Codex+ earned", Detail: "120,000 points - yours while you stay above 60,000",
				Chip: "Earned", Percent: 100, Held: true, Updated: "Score updated 2 days ago"},
		},
		{
			name: "earned but slipping",
			c: &ClientContribution{Score: f64(70000), Threshold: 100000, KeepAt: 60000, Status: "keeping",
				Earned: true, CodexPlus: true},
			want: PlusRow{Title: "Codex+ earned", Detail: "70,000 points - slipping: you keep it above 60,000",
				Chip: "Slipping", Percent: 70, Held: true},
		},
		{
			name: "paid member, not earned",
			c: &ClientContribution{Score: f64(5000), Threshold: 100000, KeepAt: 60000, Status: "below",
				PointsToEarn: 95000, CodexPlus: true},
			want: PlusRow{Title: "Codex+ active", Detail: "Contribution: 5,000 / 100,000 points", Chip: "Active",
				Percent: -1, Held: true},
		},
		{
			name: "paid member, well on the way to earning it",
			c: &ClientContribution{Score: f64(45000), Threshold: 100000, KeepAt: 60000, Status: "below",
				PointsToEarn: 55000, CodexPlus: true},
			want: PlusRow{Title: "Codex+ active", Detail: "Contribution: 45,000 / 100,000 points", Chip: "Active",
				Percent: 45, Held: true},
		},
		{
			name: "nonsense threshold is treated as no data",
			c:    &ClientContribution{Score: f64(10), Threshold: 0},
			want: PlusRow{Title: "Codex+", Detail: "Earn it free by contributing with this client", Chip: "Learn more", Percent: -1},
		},
	}
	for _, c := range cases {
		if got := codexPlusRow(c.c, plusNow); got != c.want {
			t.Errorf("%s:\n got  %+v\n want %+v", c.name, got, c.want)
		}
	}
}

// The panel's stats response carries the row, and stays null until stats exist
// (the page reads null as "session numbers only").
func TestPanelStatsCarriesPlusRow(t *testing.T) {
	if got := panelStats(nil, plusNow); got != nil {
		t.Fatalf("no stats yet: got %+v, want nil", got)
	}
	s := &ClientStats{Contribution: &ClientContribution{Score: f64(62400), Threshold: 100000, KeepAt: 60000,
		Status: "below", PointsToEarn: 37600}}
	s.Captures.PricesTotal = 10

	b, err := json.Marshal(panelStats(s, plusNow))
	if err != nil {
		t.Fatal(err)
	}
	var out struct {
		Captures struct {
			PricesTotal int64 `json:"pricesTotal"`
		} `json:"captures"`
		Plus PlusRow `json:"plus"`
	}
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	if out.Captures.PricesTotal != 10 {
		t.Errorf("existing stats lost: %s", b)
	}
	if out.Plus.Chip != "62%" || out.Plus.Percent != 62 {
		t.Errorf("plus row = %+v", out.Plus)
	}
}

func TestCommas(t *testing.T) {
	for in, want := range map[float64]string{0: "0", 999: "999", 1000: "1,000", 62400: "62,400", 1234567: "1,234,567", 99.6: "100", -5: "0"} {
		if got := commas(in); got != want {
			t.Errorf("commas(%v) = %q, want %q", in, got, want)
		}
	}
}
