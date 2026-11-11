package client

import (
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ao-data/albiondata-client/notification"
)

// ClientStatus is the live state the control panel reads. A single shared
// instance is updated by the capture/upload path and the pairing flow.
type ClientStatus struct {
	mu sync.RWMutex

	character       string
	lastUploadNodes int
	lastUploadAt    time.Time
	lastUploadOK    bool

	// pairing handshake in progress (driven from the panel)
	pairing  bool
	pairCode string
	pairURL  string

	// npcapMissing is set at startup when the capture driver is not installed.
	npcapMissing bool

	// server is the realm detected from the player's own packets ("europe",
	// "americas", "asia"). Shown in the panel so a player can confirm the client
	// is watching the region they think they are on before wondering why their
	// prices look wrong.
	server string

	// tokenRejected is set when Codex answers 401 to the saved device token -
	// i.e. the device was revoked/removed on the website. The client still HAS a
	// token (so CodexLinked() is true), but it is dead: the panel must stop
	// claiming "Linked" and offer re-pairing. Cleared by any accepted upload and
	// by a successful pairing.
	tokenRejected bool

	// lastSeenNano is the unix-nano time we last saw one of YOUR Albion packets
	// (capture is BPF-filtered to the game port). Atomic so the hot capture path
	// can stamp it without taking the mutex. Drives the "seeing your game" row.
	lastSeenNano atomic.Int64

	// activity is a small ring of human-readable events (captures + uploads), shown
	// live in the panel so the user can watch data leave for Codex.
	activity []activityEntry

	// streams is per-stream upload health (sent/failed counts) and issues is a feed
	// of flagged data problems (a failed upload = data that did not reach Codex, plus
	// any anomalies). Both power the capture-health section of the panel.
	streams map[string]*streamHealth
	issues  []issueEntry
}

type activityEntry struct {
	at  time.Time
	msg string
}

const maxActivity = 40

// streamHealth tracks per-stream upload outcomes for the capture-health panel.
type streamHealth struct {
	sent      int
	failed    int
	lastSent  time.Time
	lastErr   string
	lastErrAt time.Time
}

// issueEntry is one flagged data problem (a failed upload, an anomaly, ...).
type issueEntry struct {
	at   time.Time
	kind string
	msg  string
}

const maxIssues = 30

// recordSent counts a successful upload of n rows on a stream.
func (s *ClientStatus) recordSent(stream string, n int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	h := s.streamLocked(stream)
	h.sent += n
	h.lastSent = time.Now()
}

// streamStats returns one stream's last accepted upload (zero if none this run)
// and its failed-upload count, for the market heartbeat on the check-in.
func (s *ClientStatus) streamStats(stream string) (lastSent time.Time, failed int) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if h := s.streams[stream]; h != nil {
		return h.lastSent, h.failed
	}
	return time.Time{}, 0
}

// recordFailed counts a failed upload AND flags it as an issue: a failed upload is
// data that did not reach Codex (uploads are not retried yet, so it is lost). This
// makes losses visible in the panel instead of silent.
func (s *ClientStatus) recordFailed(stream, reason string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	h := s.streamLocked(stream)
	h.failed++
	h.lastErr = reason
	h.lastErrAt = time.Now()
	s.addIssueLocked("upload", stream+" upload failed - "+reason)
}

// flagIssue records a non-fatal data-quality problem (anomaly, unresolved id, ...).
func (s *ClientStatus) flagIssue(kind, msg string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.addIssueLocked(kind, msg)
}

func (s *ClientStatus) streamLocked(name string) *streamHealth {
	if s.streams == nil {
		s.streams = map[string]*streamHealth{}
	}
	h := s.streams[name]
	if h == nil {
		h = &streamHealth{}
		s.streams[name] = h
	}
	return h
}

func (s *ClientStatus) addIssueLocked(kind, msg string) {
	s.issues = append(s.issues, issueEntry{at: time.Now(), kind: kind, msg: msg})
	if len(s.issues) > maxIssues {
		s.issues = s.issues[len(s.issues)-maxIssues:]
	}
}

// isTokenRejected reports whether the saved device token was rejected (revoked on
// the website). The upload queue reads it to pause draining until a re-link.
func (s *ClientStatus) isTokenRejected() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.tokenRejected
}

// setTokenRejected flips the dead-token state. On the first rejection it also
// flags an issue so the transition is visible in the feed.
func (s *ClientStatus) setTokenRejected(v bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if v && !s.tokenRejected {
		s.addIssueLocked("token", "This PC was unlinked on Codex - link it again from this window")
	}
	s.tokenRejected = v
}

// recordUploadOutcome feeds one upload's result into the capture-health panel:
// counts the rows on success, or flags a failure (lost data) on a network error,
// a 401 (token), or any non-2xx status. A 401 also marks the token as rejected
// (the device was revoked on the website) so the panel offers re-pairing; an
// accepted upload clears that state (the token evidently works).
func recordUploadOutcome(stream string, n int, statusCode int, err error) {
	switch {
	case err != nil:
		Status.recordFailed(stream, err.Error())
	case statusCode >= 200 && statusCode < 300:
		Status.recordSent(stream, n)
		Status.setTokenRejected(false)
		if stream == marketStream {
			// New prices change the coverage list and totals the panel shows.
			refreshClientStatsAfterCapture()
		}
	case statusCode == 401:
		Status.recordFailed(stream, "device token rejected (401) - re-pair the client")
		Status.setTokenRejected(true)
	default:
		Status.recordFailed(stream, "server returned status "+itoa(statusCode))
	}
}

// trackingWindow is how recently we must have seen a game packet to call the
// session "tracking". Albion sends keepalives, so a live session stamps this
// continuously; if it goes quiet, the game is closed or we are on the wrong NIC.
const trackingWindow = 30 * time.Second

// markSeen records that we just observed a packet from the player's game session.
// The first packet of the run also logs a one-time "seeing your game" line.
func (s *ClientStatus) markSeen() {
	if s.lastSeenNano.Swap(time.Now().UnixNano()) == 0 {
		s.addActivity("Seeing your game - capturing")
		// The client usually runs quietly in the tray, so say once per run that it
		// is working. Asynchronous: a toast takes a moment to post, and this is the
		// packet hot path.
		go pushNotification(capturingMessage(CodexLinked()))
	}
}

// pushNotification posts a desktop notification. A variable so tests can
// capture it instead of popping real toasts.
var pushNotification = notification.Push

// capturingMessage is the one-time toast when the client first sees the game.
// Unlinked, nothing reaches Codex yet, so it says what to do instead.
func capturingMessage(linked bool) string {
	if !linked {
		return "Codex Client sees your game, but this PC is not linked yet. Click the tray icon to link it."
	}
	return "Codex Client is capturing. Click the tray icon to see what is being sent."
}

// addActivity appends a human-readable event to the live activity feed.
func (s *ClientStatus) addActivity(msg string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.activity = append(s.activity, activityEntry{at: time.Now(), msg: msg})
	if len(s.activity) > maxActivity {
		s.activity = s.activity[len(s.activity)-maxActivity:]
	}
}

// Status is the process-wide control-panel state.
var Status = &ClientStatus{}

func (s *ClientStatus) recordUpload(character string, nodes int, ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if character != "" {
		s.character = character
	}
	s.lastUploadNodes = nodes
	s.lastUploadAt = time.Now()
	s.lastUploadOK = ok
}

func (s *ClientStatus) setCharacter(name string) {
	if name == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.character = name
}

// switchCharacter records that a DIFFERENT character is now active (the player
// switched account/character). It shows the new name immediately and clears the
// previous character's board-capture state, so the panel doesn't keep showing the
// old character's "N nodes" - the Destiny Board row resets to "not captured" until
// the new character's own board event arrives.
func (s *ClientStatus) switchCharacter(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.character = name
	s.lastUploadNodes = 0
	s.lastUploadAt = time.Time{}
	s.lastUploadOK = false
}

func (s *ClientStatus) setPairing(active bool, code, url string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pairing = active
	s.pairCode = code
	s.pairURL = url
}

// setServer records the detected realm. Called from codexServerName, so it runs
// on the capture path and must stay cheap; it no-ops when unchanged so a busy
// market browse is not taking the write lock on every order.
func (s *ClientStatus) setServer(name string) {
	s.mu.RLock()
	same := s.server == name
	s.mu.RUnlock()
	if same {
		return
	}
	s.mu.Lock()
	s.server = name
	s.mu.Unlock()
}

func (s *ClientStatus) setNpcapMissing(v bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.npcapMissing = v
}

// StatusSnapshot is a point-in-time copy of the client state, rendered by the
// tray UI. Exported because the systray package is the only consumer now that
// the browser control panel is gone; the JSON tags are kept so the same struct
// can still be marshalled for diagnostics.
type StatusSnapshot struct {
	Linked          bool               `json:"linked"`
	TokenRejected   bool               `json:"tokenRejected"`
	Tracking        bool               `json:"tracking"`
	Character       string             `json:"character"`
	LastUploadNodes int                `json:"lastUploadNodes"`
	LastUploadAgo   string             `json:"lastUploadAgo"`
	LastUploadOK    bool               `json:"lastUploadOk"`
	HasUploaded     bool               `json:"hasUploaded"`
	MarketOn        bool               `json:"marketOn"`
	AodpUpload      bool               `json:"aodpUpload"`
	Pairing         bool               `json:"pairing"`
	PairCode        string             `json:"pairCode"`
	PairURL         string             `json:"pairUrl"`
	NpcapMissing    bool               `json:"npcapMissing"`
	Server          string             `json:"server"`
	Version         string             `json:"version"`
	QueuedUploads   int                `json:"queuedUploads"`
	Activity        []ActivityItem     `json:"activity"`
	Health          []StreamHealthItem `json:"health"`
	Issues          []IssueItem        `json:"issues"`
}

type ActivityItem struct {
	Ago string `json:"ago"`
	Msg string `json:"msg"`
}

type StreamHealthItem struct {
	Name     string `json:"name"`
	Sent     int    `json:"sent"`
	Failed   int    `json:"failed"`
	LastSent string `json:"lastSent,omitempty"`
	LastErr  string `json:"lastErr,omitempty"`
}

type IssueItem struct {
	Ago  string `json:"ago"`
	Kind string `json:"kind"`
	Msg  string `json:"msg"`
}

// Snapshot returns a copy of the current status for rendering. Safe from any
// goroutine: it takes a read lock and copies everything it returns, so a caller
// never holds a reference into the live feeds.
func (s *ClientStatus) Snapshot() StatusSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()

	tracking := false
	if ls := s.lastSeenNano.Load(); ls != 0 {
		tracking = time.Since(time.Unix(0, ls)) < trackingWindow
	}

	snap := StatusSnapshot{
		Linked:          CodexLinked(),
		TokenRejected:   s.tokenRejected,
		Tracking:        tracking,
		Character:       s.character,
		LastUploadNodes: s.lastUploadNodes,
		LastUploadOK:    s.lastUploadOK,
		HasUploaded:     !s.lastUploadAt.IsZero(),
		MarketOn:        marketUploadEnabled(),
		AodpUpload:      AODPUploadEnabled(),
		Pairing:         s.pairing,
		PairCode:        s.pairCode,
		PairURL:         s.pairURL,
		NpcapMissing:    s.npcapMissing,
		Server:          s.server,
		Version:         version,
		QueuedUploads:   codexQueueDepth(),
	}
	if !s.lastUploadAt.IsZero() {
		snap.LastUploadAgo = humanizeSince(s.lastUploadAt)
	}
	// activity, most recent first
	for i := len(s.activity) - 1; i >= 0; i-- {
		snap.Activity = append(snap.Activity, ActivityItem{Ago: humanizeSince(s.activity[i].at), Msg: s.activity[i].msg})
	}
	// per-stream health, name-sorted for a stable panel order
	hnames := make([]string, 0, len(s.streams))
	for n := range s.streams {
		hnames = append(hnames, n)
	}
	sort.Strings(hnames)
	for _, n := range hnames {
		h := s.streams[n]
		item := StreamHealthItem{Name: n, Sent: h.sent, Failed: h.failed}
		if !h.lastSent.IsZero() {
			item.LastSent = humanizeSince(h.lastSent)
		}
		if h.failed > 0 {
			item.LastErr = h.lastErr
		}
		snap.Health = append(snap.Health, item)
	}
	// issues, most recent first
	for i := len(s.issues) - 1; i >= 0; i-- {
		snap.Issues = append(snap.Issues, IssueItem{Ago: humanizeSince(s.issues[i].at), Kind: s.issues[i].kind, Msg: s.issues[i].msg})
	}
	return snap
}

func humanizeSince(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return formatUnit(int(d.Minutes()), "min")
	case d < 24*time.Hour:
		return formatUnit(int(d.Hours()), "hour")
	default:
		return formatUnit(int(d.Hours()/24), "day")
	}
}

func formatUnit(n int, unit string) string {
	s := ""
	if n != 1 {
		s = "s"
	}
	return itoa(n) + " " + unit + s + " ago"
}

// itoa avoids pulling strconv into this file's surface for one tiny use.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

// marketUploadEnabled reports whether the client is contributing scanned market
// prices to Codex (--codex-market). Off by default.
func marketUploadEnabled() bool {
	return ConfigGlobal.CodexMarketUpload
}
