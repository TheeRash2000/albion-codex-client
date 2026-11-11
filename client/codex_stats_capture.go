package client

import (
	"sync"
	"time"
)

// Refreshing the account view after market prices upload.
//
// The panel's "Your coverage" list and capture totals come from the site and
// were only refetched hourly, so a player could scan a new city and not see it
// listed until much later. An accepted price upload now refetches them.
//
// Browsing a market uploads many small batches, so this is paced: the first
// upload refetches at once, and any more inside the window share one refetch at
// the end of it. That is at most one read per window however fast the batches
// arrive, and the last batch is never left out.

// captureRefreshEvery is the shortest gap between two refetches caused by
// price uploads. Thirty seconds: a scanned item leaves Most wanted on the next
// refetch, and at two minutes a player who scanned three in a row watched the
// list sit still for most of that. The read is a few small indexed queries, and
// it only repeats while prices are actually being uploaded.
const captureRefreshEvery = 30 * time.Second

var captureRefresh struct {
	mu      sync.Mutex
	last    time.Time
	pending bool
}

// captureRefreshPlan decides what one accepted upload should do: refetch now,
// arm a single later refetch after delay, or nothing because one is already
// armed.
func captureRefreshPlan(last, now time.Time, pending bool) (runNow bool, delay time.Duration) {
	if pending {
		return false, 0
	}
	wait := captureRefreshEvery - now.Sub(last)
	if last.IsZero() || wait <= 0 {
		return true, 0
	}
	return false, wait
}

// refreshClientStatsAfterCapture is called when the site accepts a batch of
// market prices.
func refreshClientStatsAfterCapture() {
	captureRefresh.mu.Lock()
	now := time.Now()
	runNow, delay := captureRefreshPlan(captureRefresh.last, now, captureRefresh.pending)
	switch {
	case runNow:
		captureRefresh.last = now
	case delay > 0:
		captureRefresh.pending = true
	}
	captureRefresh.mu.Unlock()

	if runNow {
		refreshClientStatsAfterTrade()
		return
	}
	if delay <= 0 {
		return
	}
	time.AfterFunc(delay, func() {
		captureRefresh.mu.Lock()
		captureRefresh.pending = false
		captureRefresh.last = time.Now()
		captureRefresh.mu.Unlock()
		refreshClientStatsAfterTrade()
	})
}
