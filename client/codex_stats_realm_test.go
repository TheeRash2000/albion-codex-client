package client

import (
	"testing"
	"time"
)

// Most wanted is per realm. The client starts before it has seen a game packet,
// so its first read of the account stats names no realm and comes back with no
// list. That answer must not be kept for the full hour once the realm is known,
// or the card stays missing until then (seen 2026-11-11).
func TestStatsFreshFor(t *testing.T) {
	cases := []struct {
		name          string
		cached        bool
		age           time.Duration
		fetchedServer string
		currentServer string
		want          bool
	}{
		{"nothing cached is never fresh", false, 0, "", "", false},
		{"older than the limit is stale", true, statsMaxAge, "asia", "asia", false},
		{"recent and for this realm is fresh", true, time.Minute, "asia", "asia", true},
		{"realm case does not matter", true, time.Minute, "Asia", "asia", true},
		{"fetched before the realm was known, realm now known: stale", true, time.Minute, "", "asia", false},
		{"the realm changed: stale", true, time.Minute, "europe", "asia", false},
		{"realm still unknown: keep what we have", true, time.Minute, "", "", true},
		{"realm lost again: keep what we have", true, time.Minute, "asia", "", true},
	}
	for _, c := range cases {
		if got := statsFreshFor(c.cached, c.age, c.fetchedServer, c.currentServer); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}
