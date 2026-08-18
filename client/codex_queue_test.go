package client

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// stubServer stands up an ingest endpoint that returns a fixed status (+ optional
// Retry-After) and points the Codex base URL at it for the duration of the test.
func stubServer(t *testing.T, status int, retryAfter string) func() {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if retryAfter != "" {
			w.Header().Set("Retry-After", retryAfter)
		}
		w.WriteHeader(status)
	}))
	prev := ConfigGlobal.CodexBaseURL
	ConfigGlobal.CodexBaseURL = srv.URL
	return func() { ConfigGlobal.CodexBaseURL = prev; srv.Close() }
}

func healthFor(stream string) streamHealthItem {
	for _, h := range Status.snapshot().Health {
		if h.Name == stream {
			return h
		}
	}
	return streamHealthItem{}
}

func TestQueueSuccessRunsOnOKAndCountsSent(t *testing.T) {
	Status = &ClientStatus{}
	defer func() { Status = &ClientStatus{} }()
	defer stubServer(t, 200, "")()

	q := &codexUploadQueue{}
	ran := false
	j := &codexUploadJob{stream: "Sales", path: "/api/client/sales", body: []byte("{}"), rows: 1, onOK: func() { ran = true }}
	q.send(&http.Client{Timeout: 5 * time.Second}, "tok", j)

	if !ran {
		t.Fatal("onOK must run on a 2xx")
	}
	if len(q.jobs) != 0 {
		t.Fatalf("a successful job must not requeue, got %d", len(q.jobs))
	}
	if h := healthFor("Sales"); h.Sent != 1 {
		t.Fatalf("expected Sales sent=1, got %d", h.Sent)
	}
}

func TestQueueRetriesOn5xx(t *testing.T) {
	Status = &ClientStatus{}
	defer func() { Status = &ClientStatus{} }()
	defer stubServer(t, 500, "")()

	q := &codexUploadQueue{}
	j := &codexUploadJob{stream: "Market prices", path: "/api/client/market", body: []byte("{}"), rows: 3, lowValue: true}
	q.send(&http.Client{Timeout: 5 * time.Second}, "tok", j)

	if len(q.jobs) != 1 {
		t.Fatalf("a 5xx must requeue for retry, got %d", len(q.jobs))
	}
	if q.jobs[0].attempts != 1 {
		t.Fatalf("attempts should be 1, got %d", q.jobs[0].attempts)
	}
	if !q.jobs[0].nextAt.After(time.Now()) {
		t.Fatal("nextAt should be in the future (backoff)")
	}
	// A retry is NOT a terminal failure - the health panel must not count it yet.
	if h := healthFor("Market prices"); h.Failed != 0 {
		t.Fatalf("a pending retry must not be counted failed, got %d", h.Failed)
	}
}

func TestQueue429HonorsRetryAfter(t *testing.T) {
	Status = &ClientStatus{}
	defer func() { Status = &ClientStatus{} }()
	defer stubServer(t, http.StatusTooManyRequests, "2")()

	q := &codexUploadQueue{}
	before := time.Now()
	j := &codexUploadJob{stream: "Market prices", path: "/api/client/market", body: []byte("{}"), rows: 1, lowValue: true}
	q.send(&http.Client{Timeout: 5 * time.Second}, "tok", j)

	if len(q.jobs) != 1 {
		t.Fatalf("a 429 must requeue, got %d", len(q.jobs))
	}
	if q.jobs[0].nextAt.Before(before.Add(1500 * time.Millisecond)) {
		t.Fatal("a 429 must honor the ~2s Retry-After")
	}
}

func TestQueuePausesOn401KeepsData(t *testing.T) {
	Status = &ClientStatus{}
	defer func() { Status = &ClientStatus{} }()
	defer stubServer(t, http.StatusUnauthorized, "")()

	q := &codexUploadQueue{}
	j := &codexUploadJob{stream: "Destiny board", path: "/api/client/ingest", body: []byte("{}"), rows: 5}
	q.send(&http.Client{Timeout: 5 * time.Second}, "tok", j)

	if !Status.isTokenRejected() {
		t.Fatal("a 401 must set tokenRejected (re-link state)")
	}
	if len(q.jobs) != 1 {
		t.Fatal("a 401 must KEEP the job to retry after a re-link")
	}
	if q.jobs[0].attempts != 0 {
		t.Fatalf("a 401 must not burn a retry, attempts=%d", q.jobs[0].attempts)
	}
}

func TestQueueDropsOnOther4xx(t *testing.T) {
	Status = &ClientStatus{}
	defer func() { Status = &ClientStatus{} }()
	defer stubServer(t, http.StatusBadRequest, "")()

	q := &codexUploadQueue{}
	j := &codexUploadJob{stream: "EMV", path: "/api/client/emv", body: []byte("{}"), rows: 2, lowValue: true}
	q.send(&http.Client{Timeout: 5 * time.Second}, "tok", j)

	if len(q.jobs) != 0 {
		t.Fatal("a non-429 4xx is permanent and must drop, not requeue")
	}
	if Status.isTokenRejected() {
		t.Fatal("a 400 must not trip the token-rejected state")
	}
	if h := healthFor("EMV"); h.Failed != 1 {
		t.Fatalf("expected EMV failed=1, got %d", h.Failed)
	}
}

func TestQueueGivesUpAfterMaxTries(t *testing.T) {
	Status = &ClientStatus{}
	defer func() { Status = &ClientStatus{} }()

	q := &codexUploadQueue{}
	j := &codexUploadJob{stream: "Gold", rows: 1, lowValue: true, attempts: codexUploadMaxTry - 1}
	q.retryOrGiveUp(j, 500, nil, 0)

	if len(q.jobs) != 0 {
		t.Fatal("must drop after reaching the retry ceiling")
	}
	if h := healthFor("Gold"); h.Failed != 1 {
		t.Fatalf("a terminal give-up must count failed once, got %d", h.Failed)
	}
}

func TestShedPrefersOldestLowValue(t *testing.T) {
	q := &codexUploadQueue{jobs: []*codexUploadJob{
		{stream: "Destiny board"},               // high-value
		{stream: "Market prices", lowValue: true}, // low-value, oldest low -> shed this
		{stream: "Sales"},                        // high-value
		{stream: "Gold", lowValue: true},         // low-value, newer
	}}
	shed := q.shedOldestLowValueLocked()
	if shed == nil || shed.stream != "Market prices" {
		t.Fatalf("must shed the oldest low-value job, got %+v", shed)
	}
	if len(q.jobs) != 3 {
		t.Fatalf("shed must remove exactly one, got %d", len(q.jobs))
	}

	// A queue with only high-value jobs sheds nothing.
	q.jobs = []*codexUploadJob{{stream: "Sales"}, {stream: "Destiny board"}}
	if q.shedOldestLowValueLocked() != nil {
		t.Fatal("no low-value job present -> must return nil (nothing sheddable)")
	}
}

func TestParseRetryAfter(t *testing.T) {
	cases := []struct {
		in   string
		want time.Duration
	}{
		{"", 0},
		{"5", 5 * time.Second},
		{"abc", 0},
		{"0", 0},
		{"99999", 120 * time.Second}, // capped
	}
	for _, c := range cases {
		if got := parseRetryAfter(c.in); got != c.want {
			t.Errorf("parseRetryAfter(%q)=%v want %v", c.in, got, c.want)
		}
	}
}

func TestBackoffIsBounded(t *testing.T) {
	for a := 1; a <= codexUploadMaxTry; a++ {
		d := codexBackoff(a)
		if d <= 0 {
			t.Fatalf("backoff(%d)=%v must be positive", a, d)
		}
		if d > codexRetryCap+time.Second {
			t.Fatalf("backoff(%d)=%v must stay within the cap", a, d)
		}
	}
}
