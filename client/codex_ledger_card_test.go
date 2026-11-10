package client

import (
	"encoding/json"
	"testing"
)

func TestShortSilver(t *testing.T) {
	for in, want := range map[int64]string{
		0: "0", 950: "950", 999: "999", 1000: "1k", 4800: "4.8k", 9950: "10k", 23869: "24k",
		840000: "840k", 999499: "999k", 999500: "1m", 1200000: "1.2m", 4100000: "4.1m",
		99940000: "99.9m", 150000000: "150m", 999500000: "1b", 1260000000: "1.3b", -5: "0",
	} {
		if got := shortSilver(in); got != want {
			t.Errorf("shortSilver(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestClientStatsLedgerDecodes(t *testing.T) {
	raw := `{"captures":{"pricesTotal":10},"ledger":{"since":"2026-11-10T02:21:07.615919+00:00",
		"d7":{"sold":1200000,"sales":14,"bought":840000,"purchases":9},
		"d30":{"sold":4100000,"sales":51,"bought":2600000,"purchases":30}}}`
	var s ClientStats
	if err := json.Unmarshal([]byte(raw), &s); err != nil {
		t.Fatal(err)
	}
	l := s.Ledger
	if l == nil || l.Since == nil || l.D7.Sold != 1200000 || l.D7.Purchases != 9 || l.D30.Bought != 2600000 || l.D30.Sales != 51 {
		t.Fatalf("ledger decoded wrong: %+v", l)
	}
	for _, older := range []string{`{"captures":{}}`, `{"captures":{},"ledger":null}`} {
		var o ClientStats
		if err := json.Unmarshal([]byte(older), &o); err != nil || o.Ledger != nil {
			t.Errorf("%s: err=%v ledger=%+v, want nil", older, err, o.Ledger)
		}
	}
}

func TestLedgerCard(t *testing.T) {
	full := &ClientLedger{
		Since: str("2026-11-10T02:21:07.615919+00:00"),
		D7:    LedgerWindow{Sold: 1200000, Sales: 14, Bought: 840000, Purchases: 9},
		D30:   LedgerWindow{Sold: 4100000, Sales: 51, Bought: 2600000, Purchases: 30},
	}

	// The site sent no ledger (older deploy, or its read failed): no card at all,
	// never a card of zeros that would read as "you sold nothing".
	if got := ledgerCard(nil, true); got != nil {
		t.Errorf("no ledger: got %+v, want no card", got)
	}

	got := ledgerCard(full, true)
	want := LedgerCard{
		Week:   "This week: sold 1.2m, bought 840k",
		Month:  "Last 30 days: sold 4.1m, bought 2.6m",
		Footer: "Before market tax. Counted since 10 Nov.",
		Link:   "Open full ledger",
		Member: true,
	}
	if got == nil || *got != want {
		t.Errorf("member:\n got  %+v\n want %+v", got, want)
	}

	// Without Codex+ the totals are the same; only the link differs.
	got = ledgerCard(full, false)
	want.Link, want.Member = "Full ledger is part of Codex+", false
	if got == nil || *got != want {
		t.Errorf("non-member:\n got  %+v\n want %+v", got, want)
	}

	// Nothing counted yet: say how to get started, show no zero totals.
	got = ledgerCard(&ClientLedger{}, false)
	if got == nil || got.Week != "Open a sold or bought mail in game and it shows here" || got.Month != "" || got.Footer != "" {
		t.Errorf("nothing counted: got %+v", got)
	}

	// Quiet week, activity earlier in the month: the week line says so plainly.
	got = ledgerCard(&ClientLedger{D30: LedgerWindow{Sold: 500000, Sales: 3}}, false)
	if got == nil || got.Week != "This week: nothing sold or bought" || got.Month != "Last 30 days: sold 500k, bought 0" {
		t.Errorf("quiet week: got %+v", got)
	}
	// No usable start date: the footer still states the tax basis.
	if got.Footer != "Before market tax." {
		t.Errorf("footer without a date = %q", got.Footer)
	}
}

// The card reaches the panel through the stats response, and Codex+ membership
// comes from the contribution the site sends.
func TestPanelStatsCarriesLedgerCard(t *testing.T) {
	s := &ClientStats{
		Contribution: &ClientContribution{Threshold: 100000, CodexPlus: true},
		Ledger:       &ClientLedger{D7: LedgerWindow{Sold: 1000, Sales: 1}, D30: LedgerWindow{Sold: 1000, Sales: 1}},
	}
	body := panelStats(s, plusNow)
	if body.LedgerCard == nil || !body.LedgerCard.Member || body.LedgerCard.Link != "Open full ledger" {
		t.Errorf("ledger card = %+v", body.LedgerCard)
	}
	if ledgerURL(s) != CodexLedgerURL() {
		t.Errorf("member link = %q", ledgerURL(s))
	}

	s.Contribution.CodexPlus = false
	if ledgerURL(s) != CodexPlusURL() {
		t.Errorf("non-member link = %q", ledgerURL(s))
	}
	// No stats yet: still somewhere sensible to go.
	if ledgerURL(nil) != CodexPlusURL() {
		t.Errorf("no stats link = %q", ledgerURL(nil))
	}

	s.Ledger = nil
	if body := panelStats(s, plusNow); body.LedgerCard != nil {
		t.Errorf("no ledger from the site, but a card was built: %+v", body.LedgerCard)
	}
}
