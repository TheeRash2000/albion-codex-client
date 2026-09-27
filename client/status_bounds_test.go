package client

import (
	"strconv"
	"testing"

	"github.com/ao-data/albiondata-client/lib"
)

// The client runs unattended for days, so every in-memory feed it appends to has
// to be a ring rather than a growing slice. All three below were already capped;
// nothing here is a fix. They are pinned because the caps are the load-bearing
// part of "the UI cannot be the thing that leaks", and a cap that no test asserts
// is one refactor away from being dropped silently.
//
// A fresh ClientStatus is used rather than the package-level Status so these do
// not pollute the feeds the other tests in this package read.

func TestActivityFeedStaysBounded(t *testing.T) {
	s := &ClientStatus{}

	const writes = maxActivity * 3
	for i := 0; i < writes; i++ {
		s.addActivity("event " + strconv.Itoa(i))
	}

	s.mu.RLock()
	got := len(s.activity)
	oldest := s.activity[0].msg
	newest := s.activity[len(s.activity)-1].msg
	s.mu.RUnlock()

	if got != maxActivity {
		t.Fatalf("activity feed held %d entries after %d writes, want the cap of %d", got, writes, maxActivity)
	}
	// The ring must drop the OLDEST, not the newest: a feed that discarded new
	// entries would go quiet exactly when something started happening.
	if want := "event " + strconv.Itoa(writes-1); newest != want {
		t.Fatalf("newest entry is %q, want %q - the ring is dropping the wrong end", newest, want)
	}
	if want := "event " + strconv.Itoa(writes-maxActivity); oldest != want {
		t.Fatalf("oldest entry is %q, want %q", oldest, want)
	}
}

func TestIssuesFeedStaysBounded(t *testing.T) {
	s := &ClientStatus{}

	const writes = maxIssues * 3
	for i := 0; i < writes; i++ {
		s.flagIssue("upload", "failure "+strconv.Itoa(i))
	}

	s.mu.RLock()
	got := len(s.issues)
	newest := s.issues[len(s.issues)-1].msg
	s.mu.RUnlock()

	if got != maxIssues {
		t.Fatalf("issues feed held %d entries after %d writes, want the cap of %d", got, writes, maxIssues)
	}
	if want := "failure " + strconv.Itoa(writes-1); newest != want {
		t.Fatalf("newest issue is %q, want %q", newest, want)
	}
}

// recordFailed is the path that actually runs in production (every failed upload
// flags an issue), so the cap has to hold through it, not just through flagIssue.
func TestRecordFailedKeepsIssuesBounded(t *testing.T) {
	s := &ClientStatus{}

	for i := 0; i < maxIssues*2; i++ {
		s.recordFailed("market", "boom "+strconv.Itoa(i))
	}

	s.mu.RLock()
	issues := len(s.issues)
	failed := s.streams["market"].failed
	s.mu.RUnlock()

	if issues != maxIssues {
		t.Fatalf("issues feed held %d entries, want the cap of %d", issues, maxIssues)
	}
	// The COUNTER is not a ring and must keep counting past the feed's cap,
	// otherwise the health numbers would silently stop at 30 failures.
	if failed != maxIssues*2 {
		t.Fatalf("failure counter is %d, want %d - counters must not be capped by the feed", failed, maxIssues*2)
	}
}

// The pre-location market buffer is the one feed fed directly by the capture hot
// path, so an unresolved location must not be able to grow it without bound.
func TestPendingMarketOrdersStayBounded(t *testing.T) {
	pendingMarketMu.Lock()
	prev, prevServer := pendingOrders, pendingServer
	pendingOrders, pendingServer = nil, ""
	pendingMarketMu.Unlock()
	t.Cleanup(func() {
		pendingMarketMu.Lock()
		pendingOrders, pendingServer = prev, prevServer
		pendingMarketMu.Unlock()
	})

	batch := make([]*lib.MarketOrder, 1000)
	for i := range batch {
		batch[i] = &lib.MarketOrder{ID: i}
	}
	for i := 0; i < 10; i++ {
		stashMarketOrders(batch, "europe")
	}

	pendingMarketMu.Lock()
	got := len(pendingOrders)
	newestID := pendingOrders[len(pendingOrders)-1].ID
	pendingMarketMu.Unlock()

	if got != maxPendingOrders {
		t.Fatalf("pending buffer held %d orders after 10,000 stashed, want the cap of %d", got, maxPendingOrders)
	}
	if newestID != 999 {
		t.Fatalf("newest pending order is %d, want 999 - the buffer is dropping the wrong end", newestID)
	}
}
