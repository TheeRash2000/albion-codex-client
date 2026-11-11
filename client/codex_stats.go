package client

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/ao-data/albiondata-client/log"
)

// Account-level capture stats, read back from Codex for the panel.
//
// Everything else the panel shows is process-local: counters that reset when the
// client restarts. This is the account view - what this player has contributed
// across every session and device, and which of their own cities have gone
// stale. It is the one thing the panel can show that no other surface can show
// as well, because the client is where a capturing player already is.
//
// Read through /api/client/stats with the device token (migration 046's
// client_stats RPC). Fetched on panel open, at most hourly, and after this client
// uploads a trade or market prices (paced, codex_stats_capture.go). NEVER on the
// panel's 2s status tick - the aggregate is cheap (78 ms measured against the
// heaviest contributor on prod) but it is a database round trip per call, and
// polling it every two seconds for the life of the process would be the same
// carelessness that this whole rewrite exists to undo.

// statsMaxAge is how stale a cached figure may get before a panel open refetches
// it. Capture totals move slowly; an hour is invisible to a human reading them.
const statsMaxAge = time.Hour

// statsTimeout bounds the request. The panel must render immediately from cache
// whatever the network is doing, so this only decides how long a background
// refresh hangs around before giving up.
const statsTimeout = 10 * time.Second

// ClientStats is the account view rendered by the panel. Field names match the
// RPC's JSON exactly so this is a straight decode.
type ClientStats struct {
	Captures struct {
		PricesTotal   int64 `json:"pricesTotal"`
		Prices7d      int64 `json:"prices7d"`
		HistorySeries int64 `json:"historySeries"`
		Characters    int64 `json:"characters"`
	} `json:"captures"`
	Bounties *struct {
		Server     string `json:"server"`
		ComputedAt string `json:"computedAt"`
		Gap        int64  `json:"gap"`
		Observed   int64  `json:"observed"`
		Total      int64  `json:"total"`
		Wanted     []struct {
			ID       string   `json:"id"`
			Kind     string   `json:"kind"`
			Blocks   int64    `json:"blocks"`
			Examples []string `json:"examples"`
			// Added by the site (acd1 #720). Older responses omit all three, and the
			// panel then falls back to the id-based label.
			Name     string `json:"name,omitempty"`
			Tier     int    `json:"tier,omitempty"`
			CopyText string `json:"copyText,omitempty"`
			// True when the item has an estimated price and no real one, rather
			// than no price at all. Older responses never set it.
			Estimated bool `json:"estimated,omitempty"`
		} `json:"wanted"`
	} `json:"bounties"`
	// The owner's Codex+ contribution. Nil when the site does not send it (an
	// older deploy, or its read failed): the panel then shows no numbers.
	Contribution *ClientContribution `json:"contribution"`
	// The owner's market sales and purchases totals. Nil when the site does not
	// send them: the panel then shows no ledger card.
	Ledger *ClientLedger `json:"ledger"`
	Cities []struct {
		City       string `json:"city"`
		Server     string `json:"server"`
		Rows       int64  `json:"rows"`
		Newest     string `json:"newest"`
		HoursStale int64  `json:"hours_stale"`
	} `json:"cities"`
}

var (
	statsMu        sync.RWMutex
	statsCache     *ClientStats
	statsFetchedAt time.Time
	statsInFlight  bool
	// statsStaleGen counts markClientStatsStale calls, so a fetch that was already
	// in flight when a trade was recorded does not stamp its older answer as fresh.
	statsStaleGen int
)

// CachedClientStats returns the last successfully fetched stats, or nil if none
// have landed yet. Never blocks on the network: the panel renders from whatever
// is cached and fills in when a refresh completes.
func CachedClientStats() *ClientStats {
	statsMu.RLock()
	defer statsMu.RUnlock()
	return statsCache
}

// clientStatsFresh reports whether the cached stats are recent enough to serve
// without a refetch.
func clientStatsFresh() bool {
	statsMu.RLock()
	defer statsMu.RUnlock()
	return statsCache != nil && time.Since(statsFetchedAt) < statsMaxAge
}

// markClientStatsStale makes the next read refetch. Called when this client has
// just changed what the site would report (a sale or purchase was recorded), so
// the ledger card does not sit on totals up to an hour old. The cached numbers
// stay on screen until the new ones arrive.
func markClientStatsStale() {
	statsMu.Lock()
	statsFetchedAt = time.Time{}
	statsStaleGen++
	statsMu.Unlock()
}

// refreshClientStatsAfterTrade marks the stats stale and starts the refetch now,
// so the panel's next poll already has the new totals.
func refreshClientStatsAfterTrade() {
	markClientStatsStale()
	RefreshClientStatsIfStale()
}

// RefreshClientStatsIfStale kicks off a background fetch when the cache is older
// than statsMaxAge (or empty). Returns immediately.
//
// Single-flight: a burst of panel opens must not fan out into a burst of
// requests. That is the mistake this codebase has made before, in the AODP
// estimate path, where one call became fifty.
func RefreshClientStatsIfStale() {
	statsMu.Lock()
	if statsInFlight {
		statsMu.Unlock()
		return
	}
	fresh := statsCache != nil && time.Since(statsFetchedAt) < statsMaxAge
	if fresh {
		statsMu.Unlock()
		return
	}
	statsInFlight = true
	gen := statsStaleGen
	statsMu.Unlock()

	go func() {
		s, err := fetchClientStats()

		statsMu.Lock()
		statsInFlight = false
		if err == nil && s != nil {
			statsCache = s
			// If a trade was recorded while this was in flight, the answer may
			// predate it: show it, but leave it stale so the next read refetches.
			if statsStaleGen == gen {
				statsFetchedAt = time.Now()
			}
		}
		statsMu.Unlock()

		if err != nil {
			// Debug, not Error: the panel degrades to session-only numbers and the
			// player loses nothing. A player who is offline, or simply not paired,
			// should not see this as a fault.
			log.Debugf("Codex: could not read account stats: %v", err)
		}
	}()
}

// fetchClientStats performs the authenticated read. Paired-only.
func fetchClientStats() (*ClientStats, error) {
	token := strings.TrimSpace(ConfigGlobal.CodexToken)
	if token == "" {
		return nil, errNotPaired
	}

	base := strings.TrimRight(strings.TrimSpace(ConfigGlobal.CodexBaseURL), "/")
	if base == "" {
		base = "https://albioncodex.com"
	}

	url := base + "/api/client/stats"
	// Send the realm we detected so the response can carry that region's bounties.
	// Omitted when unknown (no game packet seen yet), which the server answers with
	// null bounties rather than a wrong region's.
	if srv := strings.TrimSpace(Status.Snapshot().Server); srv != "" {
		url += "?server=" + srv
	}

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("User-Agent", codexUserAgent())

	client := &http.Client{Timeout: statsTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// A 401 here means the same thing it means on every upload: the device was
	// revoked on the website. Route it through the same state the upload path
	// uses so the panel offers a re-link rather than just going quiet.
	if resp.StatusCode == http.StatusUnauthorized {
		Status.setTokenRejected(true)
		return nil, errTokenRejected
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, errBadStatus
	}

	var s ClientStats
	if err := json.NewDecoder(resp.Body).Decode(&s); err != nil {
		return nil, err
	}
	return &s, nil
}

type statsError string

func (e statsError) Error() string { return string(e) }

const (
	errNotPaired     = statsError("not paired")
	errTokenRejected = statsError("device token rejected")
	errBadStatus     = statsError("unexpected status")
)
