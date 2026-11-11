package client

import (
	"bytes"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// codex_queue.go: a single delivery queue with retry in front of every Codex
// upload. Before this, each capture path POSTed once, best-effort, and dropped the
// data on any network blip / 429 / 5xx (the capture-health panel surfaced the loss
// but nothing recovered it). Now captures enqueue and a worker drains with backoff,
// so a transient failure is retried instead of lost.
//
// Design notes:
//   - The body is encoded at enqueue time; the device token + base URL are read
//     fresh at SEND time, so a re-pair (new token) is picked up for jobs already in
//     the queue.
//   - A 401 does not burn a retry: it flags the token rejected (re-link state) and
//     the worker pauses until a re-link, then the held job drains.
//   - Non-2xx that are not 429/5xx (i.e. other 4xx) are permanent - dropped, not
//     spun on.
//   - The queue is bounded. On overflow it sheds the oldest LOW-value job (market
//     spot / history / gold / EMV - all re-observable) first, keeping high-value
//     ones (board, realized sales, and future craft/buy cost basis). A drop is
//     always reported to the Issues feed - never silent.
type codexUploadJob struct {
	stream   string // capture-health label (e.g. "Market prices")
	path     string // ingest path, e.g. "/api/client/market"
	body     []byte
	rows     int    // row count for recordUploadOutcome
	lowValue bool   // sheddable first on overflow
	onOK     func() // caller-specific success side effects (activity / status)
	// onReply is onOK for a caller that needs what the site answered (the item a
	// numeric id resolved to). Given the reply body, capped at maxReplyBytes.
	onReply func(reply []byte)

	attempts int
	nextAt   time.Time
}

const (
	codexQueueMax     = 500                    // hard cap; overflow sheds low-value oldest first
	codexUploadMaxTry = 6                      // give up after this many failed attempts (~1m of backoff)
	codexQueuePoll    = 750 * time.Millisecond // worker tick
	codexRetryCap     = 60 * time.Second       // max single backoff
)

type codexUploadQueue struct {
	mu   sync.Mutex
	jobs []*codexUploadJob
}

var (
	codexQ      = &codexUploadQueue{}
	codexQStart sync.Once
	// codexQDepth mirrors len(codexQ.jobs) as an atomic so the status snapshot can
	// read the queue depth without taking codexQ.mu (avoids a lock-order inversion
	// with the status mutex). Updated by every mutator while holding codexQ.mu.
	codexQDepth atomic.Int64
)

// startCodexUploadQueue launches the single delivery worker (idempotent).
func startCodexUploadQueue() {
	codexQStart.Do(func() { go codexQ.run() })
}

// enqueueCodexUpload queues a Codex upload for delivery with retry. Safe from any
// capture goroutine. On a full queue it sheds a lower-value job to make room; if
// nothing is sheddable it drops this job and flags an Issue - a loss is never
// silent.
func enqueueCodexUpload(stream, path string, body []byte, rows int, lowValue bool, onOK func()) {
	enqueueCodexJob(&codexUploadJob{stream: stream, path: path, body: body, rows: rows, lowValue: lowValue, onOK: onOK, nextAt: time.Now()})
}

// enqueueCodexUploadWithReply is enqueueCodexUpload for a caller that reads the
// site's answer on success.
func enqueueCodexUploadWithReply(stream, path string, body []byte, rows int, lowValue bool, onReply func(reply []byte)) {
	enqueueCodexJob(&codexUploadJob{stream: stream, path: path, body: body, rows: rows, lowValue: lowValue, onReply: onReply, nextAt: time.Now()})
}

// maxReplyBytes bounds how much of an upload reply is kept for onReply.
const maxReplyBytes = 64 << 10

func enqueueCodexJob(j *codexUploadJob) {
	stream := j.stream

	// Mutate the queue under the lock; run Status reporting (which takes the status
	// lock) only AFTER releasing codexQ.mu, to keep a single lock order.
	codexQ.mu.Lock()
	var shed *codexUploadJob
	dropped := false
	if len(codexQ.jobs) >= codexQueueMax {
		if shed = codexQ.shedOldestLowValueLocked(); shed == nil {
			dropped = true
		}
	}
	if !dropped {
		codexQ.jobs = append(codexQ.jobs, j)
	}
	codexQDepth.Store(int64(len(codexQ.jobs)))
	codexQ.mu.Unlock()

	if shed != nil {
		Status.recordFailed(shed.stream, "shed from a full upload queue (re-observable data, client offline too long)")
	}
	if dropped {
		Status.recordFailed(stream, "upload queue full ("+strconv.Itoa(codexQueueMax)+") - dropped; client offline too long")
		return
	}
	startCodexUploadQueue()
}

// shedOldestLowValueLocked removes and returns the oldest LOW-value job, or nil when
// the queue is all high-value (board/sales/craft/buy). Caller holds the lock.
func (q *codexUploadQueue) shedOldestLowValueLocked() *codexUploadJob {
	for i, j := range q.jobs {
		if j.lowValue {
			q.jobs = append(q.jobs[:i], q.jobs[i+1:]...)
			return j
		}
	}
	return nil
}

// codexQueueDepth is the number of pending uploads, for the capture-health panel.
// Reads an atomic mirror so it never contends with (or inverts against) the queue
// or status locks.
func codexQueueDepth() int {
	return int(codexQDepth.Load())
}

func (q *codexUploadQueue) run() {
	client := &http.Client{Timeout: 20 * time.Second}
	for {
		time.Sleep(codexQueuePoll)

		// Paused while the token is missing or rejected: don't hammer a dead token;
		// held jobs wait for a re-link, then drain.
		if Status.isTokenRejected() {
			continue
		}
		token := strings.TrimSpace(ConfigGlobal.CodexToken)
		if token == "" {
			continue
		}

		j := q.next()
		if j == nil {
			continue
		}
		q.send(client, token, j)
	}
}

// next removes and returns the ready job with the earliest nextAt, or nil if none
// are due yet.
func (q *codexUploadQueue) next() *codexUploadJob {
	q.mu.Lock()
	defer q.mu.Unlock()
	now := time.Now()
	best := -1
	for i, j := range q.jobs {
		if j.nextAt.After(now) {
			continue
		}
		if best == -1 || j.nextAt.Before(q.jobs[best].nextAt) {
			best = i
		}
	}
	if best == -1 {
		return nil
	}
	j := q.jobs[best]
	q.jobs = append(q.jobs[:best], q.jobs[best+1:]...)
	codexQDepth.Store(int64(len(q.jobs)))
	return j
}

func (q *codexUploadQueue) requeue(j *codexUploadJob) {
	q.mu.Lock()
	q.jobs = append(q.jobs, j)
	codexQDepth.Store(int64(len(q.jobs)))
	q.mu.Unlock()
}

func (q *codexUploadQueue) send(client *http.Client, token string, j *codexUploadJob) {
	req, err := http.NewRequest(http.MethodPost, codexBase()+j.path, bytes.NewReader(j.body))
	if err != nil {
		recordUploadOutcome(j.stream, j.rows, 0, err) // unbuildable request = permanent
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("User-Agent", codexUserAgent())

	resp, err := client.Do(req)
	if err != nil {
		q.retryOrGiveUp(j, 0, err, 0)
		return
	}
	sc := resp.StatusCode
	retryAfter := parseRetryAfter(resp.Header.Get("Retry-After"))
	var reply []byte
	if j.onReply != nil && sc >= 200 && sc < 300 {
		// Best-effort: a reply that cannot be read is an empty one, and the caller
		// falls back to a line without the detail. The upload itself succeeded.
		reply, _ = io.ReadAll(io.LimitReader(resp.Body, maxReplyBytes))
	}
	resp.Body.Close()

	switch {
	case sc >= 200 && sc < 300:
		recordUploadOutcome(j.stream, j.rows, sc, nil) // counts sent + clears tokenRejected
		if j.onOK != nil {
			j.onOK()
		}
		if j.onReply != nil {
			j.onReply(reply)
		}
	case sc == http.StatusUnauthorized:
		// Token revoked: flag the re-link state (recordUploadOutcome sets it) and
		// KEEP the data. The worker pauses on tokenRejected until a re-link, then
		// this job drains. Not counted against the retry budget.
		recordUploadOutcome(j.stream, j.rows, sc, nil)
		j.nextAt = time.Now().Add(2 * time.Second)
		q.requeue(j)
	case sc == http.StatusTooManyRequests || sc >= 500:
		q.retryOrGiveUp(j, sc, nil, retryAfter)
	default:
		recordUploadOutcome(j.stream, j.rows, sc, nil) // other 4xx = permanent, don't spin
	}
}

func (q *codexUploadQueue) retryOrGiveUp(j *codexUploadJob, sc int, err error, retryAfter time.Duration) {
	j.attempts++
	if j.attempts >= codexUploadMaxTry {
		recordUploadOutcome(j.stream, j.rows, sc, err) // terminal failure -> Issues feed
		return
	}
	backoff := retryAfter
	if backoff <= 0 {
		backoff = codexBackoff(j.attempts)
	}
	j.nextAt = time.Now().Add(backoff)
	q.requeue(j)
}

// codexBackoff is exponential (1s, 2s, 4s, ...) capped at codexRetryCap, with +/-10%
// jitter so retries across streams don't align into bursts.
func codexBackoff(attempt int) time.Duration {
	d := time.Second << (attempt - 1)
	if d > codexRetryCap || d <= 0 {
		d = codexRetryCap
	}
	span := int64(d / 5) // 20% window
	jitter := time.Duration(time.Now().UnixNano()%span) - time.Duration(span/2)
	return d + jitter
}

// parseRetryAfter reads the delta-seconds form of a Retry-After header (the form
// Vercel/our API emits). HTTP-date form is ignored (falls back to backoff). Capped
// so a hostile header can't stall the queue.
func parseRetryAfter(v string) time.Duration {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0
	}
	secs, err := strconv.Atoi(v)
	if err != nil || secs <= 0 {
		return 0
	}
	d := time.Duration(secs) * time.Second
	if d > 120*time.Second {
		d = 120 * time.Second
	}
	return d
}
