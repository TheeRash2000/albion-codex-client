package client

import (
	"sync"
	"sync/atomic"
	"time"
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

	// lastSeenNano is the unix-nano time we last saw one of YOUR Albion packets
	// (capture is BPF-filtered to the game port). Atomic so the hot capture path
	// can stamp it without taking the mutex. Drives the "seeing your game" row.
	lastSeenNano atomic.Int64
}

// trackingWindow is how recently we must have seen a game packet to call the
// session "tracking". Albion sends keepalives, so a live session stamps this
// continuously; if it goes quiet, the game is closed or we are on the wrong NIC.
const trackingWindow = 30 * time.Second

// markSeen records that we just observed a packet from the player's game session.
func (s *ClientStatus) markSeen() {
	s.lastSeenNano.Store(time.Now().UnixNano())
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
// the new character's own event 151 arrives.
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

func (s *ClientStatus) setNpcapMissing(v bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.npcapMissing = v
}

// snapshot returns a copy of the current status for rendering.
type statusSnapshot struct {
	Linked          bool   `json:"linked"`
	Tracking        bool   `json:"tracking"`
	Character       string `json:"character"`
	LastUploadNodes int    `json:"lastUploadNodes"`
	LastUploadAgo   string `json:"lastUploadAgo"`
	LastUploadOK    bool   `json:"lastUploadOk"`
	HasUploaded     bool   `json:"hasUploaded"`
	MarketOn        bool   `json:"marketOn"`
	Pairing         bool   `json:"pairing"`
	PairCode        string `json:"pairCode"`
	PairURL         string `json:"pairUrl"`
	NpcapMissing    bool   `json:"npcapMissing"`
}

func (s *ClientStatus) snapshot() statusSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()

	tracking := false
	if ls := s.lastSeenNano.Load(); ls != 0 {
		tracking = time.Since(time.Unix(0, ls)) < trackingWindow
	}

	snap := statusSnapshot{
		Linked:          CodexLinked(),
		Tracking:        tracking,
		Character:       s.character,
		LastUploadNodes: s.lastUploadNodes,
		LastUploadOK:    s.lastUploadOK,
		HasUploaded:     !s.lastUploadAt.IsZero(),
		MarketOn:        marketUploadEnabled(),
		Pairing:         s.pairing,
		PairCode:        s.pairCode,
		PairURL:         s.pairURL,
		NpcapMissing:    s.npcapMissing,
	}
	if !s.lastUploadAt.IsZero() {
		snap.LastUploadAgo = humanizeSince(s.lastUploadAt)
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
