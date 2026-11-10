package client

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/ao-data/albiondata-client/icon"
	"github.com/ao-data/albiondata-client/log"
)

//go:embed panel.html
var panelHTML []byte

var panelURL string

// StartPanel serves the local control panel page and its JSON API.
//
// It does NOT open a window. The tray does that via OpenPanel, so the client
// runs with no UI process at all until the user asks for one. Best-effort:
// capture continues regardless of whether the server started.
func StartPanel() {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Errorf("Control panel: could not start (%v).", err)
		return
	}
	panelURL = fmt.Sprintf("http://127.0.0.1:%d", ln.Addr().(*net.TCPAddr).Port)

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(panelHTML)
	})
	// The Codex mark as the page icon. Without it the page falls back to the
	// generic globe in any browser or app window that shows the favicon.
	mux.HandleFunc("/favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/x-icon")
		w.Header().Set("Cache-Control", "public, max-age=86400")
		_, _ = w.Write(icon.ICO)
	})
	// Account stats, served from cache. The page polls this far less often than
	// /api/status, and a stale-cache refresh is single-flighted and backgrounded,
	// so a burst of requests here can never become a burst of round trips to
	// Codex. Answers 200 with a null body when nothing has been fetched yet,
	// which the page renders as session-only numbers rather than as an error.
	mux.HandleFunc("/api/stats", func(w http.ResponseWriter, r *http.Request) {
		RefreshClientStatsIfStale()
		writeJSON(w, panelStats(CachedClientStats(), time.Now()))
	})
	mux.HandleFunc("/api/status", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, Status.Snapshot())
	})
	mux.HandleFunc("/api/pair", func(w http.ResponseWriter, r *http.Request) {
		code, url, err := StartUIPairing()
		if err != nil {
			w.WriteHeader(http.StatusBadGateway)
			writeJSON(w, map[string]string{"error": "could not reach Codex"})
			return
		}
		OpenBrowser(url)
		writeJSON(w, map[string]string{"userCode": code, "verifyUrl": url})
	})
	// Toggle third-party (AODP) contribution. Persisted, and applied to the live
	// config so the next captured browse already respects it - no restart.
	mux.HandleFunc("/api/aodp", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			On bool `json:"on"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			writeJSON(w, map[string]string{"error": "bad request"})
			return
		}
		if err := SetAODPUpload(body.On); err != nil {
			// The setting is live either way; only persistence failed. Say so
			// rather than reporting success, or the player is told a choice was
			// remembered that will be gone next launch.
			w.WriteHeader(http.StatusInternalServerError)
			writeJSON(w, map[string]interface{}{"on": AODPUploadEnabled(), "error": "could not save the setting"})
			return
		}
		writeJSON(w, map[string]bool{"on": AODPUploadEnabled()})
	})
	// Toggle Codex market capture. Same shape as /api/aodp: the setting is
	// applied live and persisted, and a persistence failure is reported rather
	// than swallowed - a switch that silently forgets its position after a
	// restart is worse than one that says it could not save.
	mux.HandleFunc("/api/market-upload", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			On bool `json:"on"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			writeJSON(w, map[string]string{"error": "bad request"})
			return
		}
		if err := SetMarketUpload(body.On); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			writeJSON(w, map[string]interface{}{"on": MarketUploadEnabled(), "error": "could not save the setting"})
			return
		}
		writeJSON(w, map[string]bool{"on": MarketUploadEnabled()})
	})
	mux.HandleFunc("/api/open-codex", func(w http.ResponseWriter, r *http.Request) {
		OpenBrowser(CodexAccountURL())
		writeJSON(w, map[string]bool{"ok": true})
	})
	mux.HandleFunc("/api/open-plus", func(w http.ResponseWriter, r *http.Request) {
		OpenBrowser(CodexPlusURL())
		writeJSON(w, map[string]bool{"ok": true})
	})
	mux.HandleFunc("/api/open-npcap", func(w http.ResponseWriter, r *http.Request) {
		OpenBrowser("https://npcap.com/#download")
		writeJSON(w, map[string]bool{"ok": true})
	})

	go func() { _ = http.Serve(ln, mux) }()
	log.Infof("Control panel server ready at %s", panelURL)
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
