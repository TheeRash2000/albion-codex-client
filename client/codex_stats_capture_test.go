package client

import (
	"testing"
	"time"
)

// A new city must show in the coverage list soon after it is scanned, without
// one read of the site per uploaded batch.
func TestCaptureRefreshPlan(t *testing.T) {
	now := time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name      string
		last      time.Time
		pending   bool
		wantNow   bool
		wantDelay time.Duration
	}{
		{"first upload of the session refetches at once", time.Time{}, false, true, 0},
		{"an upload long after the last refetch refetches at once", now.Add(-10 * time.Minute), false, true, 0},
		{"an upload exactly one window later refetches at once", now.Add(-captureRefreshEvery), false, true, 0},
		{"an upload inside the window arms one refetch for the end of it", now.Add(-10 * time.Second), false, false, captureRefreshEvery - 10*time.Second},
		{"more uploads while one is armed do nothing", now.Add(-30 * time.Second), true, false, 0},
		{"armed wins even when the window has passed", now.Add(-10 * time.Minute), true, false, 0},
	}
	for _, c := range cases {
		gotNow, gotDelay := captureRefreshPlan(c.last, now, c.pending)
		if gotNow != c.wantNow || gotDelay != c.wantDelay {
			t.Errorf("%s: got (%v, %v), want (%v, %v)", c.name, gotNow, gotDelay, c.wantNow, c.wantDelay)
		}
	}
}
