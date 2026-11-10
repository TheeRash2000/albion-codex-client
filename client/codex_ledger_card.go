package client

import (
	"strconv"
	"time"
)

// The panel's ledger card: what the owner sold and bought on the market, from
// the mails this client has read. Totals only, as the site reports them in the
// stats response. Sold is before market tax and counts only sales the site
// trusts (recorded by a build that reads the mail's type). There is no profit
// figure: sold minus bought is not profit, and the full ledger on the site owns
// that calculation.

// LedgerWindow is one period's totals, in silver.
type LedgerWindow struct {
	Sold      int64 `json:"sold"`
	Sales     int64 `json:"sales"`
	Bought    int64 `json:"bought"`
	Purchases int64 `json:"purchases"`
}

// ClientLedger is the site's summary for the device owner.
type ClientLedger struct {
	Since *string      `json:"since"` // earliest counted row
	D7    LedgerWindow `json:"d7"`
	D30   LedgerWindow `json:"d30"`
}

// LedgerCard is what the panel shows.
type LedgerCard struct {
	Week   string `json:"week"`
	Month  string `json:"month,omitempty"`
	Footer string `json:"footer,omitempty"`
	Link   string `json:"link"`
	Member bool   `json:"member"` // Codex+ is active: the link opens the full ledger
}

// ledgerCard builds the card. No ledger from the site (an older deploy, or its
// read failed) means no card, never a card of zeros.
func ledgerCard(l *ClientLedger, codexPlus bool) *LedgerCard {
	if l == nil {
		return nil
	}
	card := &LedgerCard{Member: codexPlus, Link: "Full ledger is part of Codex+"}
	if codexPlus {
		card.Link = "Open full ledger"
	}

	if l.D30.Sales == 0 && l.D30.Purchases == 0 {
		card.Week = "Open a sold or bought mail in game and it shows here"
		return card
	}

	if l.D7.Sales == 0 && l.D7.Purchases == 0 {
		card.Week = "This week: nothing sold or bought"
	} else {
		card.Week = "This week: sold " + shortSilver(l.D7.Sold) + ", bought " + shortSilver(l.D7.Bought)
	}
	card.Month = "Last 30 days: sold " + shortSilver(l.D30.Sold) + ", bought " + shortSilver(l.D30.Bought)

	card.Footer = "Before market tax."
	if l.Since != nil {
		if t, err := time.Parse(time.RFC3339, *l.Since); err == nil {
			card.Footer += " Counted since " + t.Format("2 Jan") + "."
		}
	}
	return card
}

// shortSilver writes an amount the way players say it: 950, 4.8k, 840k, 1.2m,
// 150m, 1.3b. One decimal below ten of a unit's hundreds, none above.
func shortSilver(n int64) string {
	if n <= 0 {
		return "0"
	}
	v := float64(n)
	if v < 999.5 {
		return strconv.FormatInt(n, 10)
	}
	// decimalBelow: show one decimal while the figure is under this many units.
	// Thousands get one only below 10 ("4.8k", then "24k"); millions and billions
	// keep it up to 100 ("99.9m", then "150m").
	units := []struct {
		size         float64
		suffix       string
		decimalBelow float64
	}{{1e3, "k", 10}, {1e6, "m", 100}, {1e9, "b", 100}}

	// Step up a unit when rounding would reach 1000 of the current one, so
	// 999,500 reads as 1m and not as 1000k.
	i := 0
	for i < len(units)-1 && v/units[i].size >= 999.5 {
		i++
	}
	u := units[i]
	x := v / u.size
	if x < u.decimalBelow-0.05 {
		s := strconv.FormatFloat(x, 'f', 1, 64)
		if len(s) > 2 && s[len(s)-2:] == ".0" {
			s = s[:len(s)-2]
		}
		return s + u.suffix
	}
	return strconv.FormatFloat(x, 'f', 0, 64) + u.suffix
}

// CodexLedgerURL is the full ledger on the site.
func CodexLedgerURL() string {
	return codexBase() + "/dashboard/ledger"
}

// ledgerURL is where the card's link goes: the full ledger for a Codex+ member,
// the Codex+ page for everyone else.
func ledgerURL(s *ClientStats) string {
	if s != nil && s.Contribution != nil && s.Contribution.CodexPlus {
		return CodexLedgerURL()
	}
	return CodexPlusURL()
}
