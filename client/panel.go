package client

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"

	"github.com/ao-data/albiondata-client/log"
)

//go:embed panel.html
var panelHTML []byte

var panelURL string

// StartPanel serves the local control panel and opens it as an app window. It is
// best-effort: capture continues regardless of whether the window opened.
func StartPanel() {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Errorf("Control panel: could not start (%v).", err)
		return
	}
	panelURL = fmt.Sprintf("http://127.0.0.1:%d", ln.Addr().(*net.TCPAddr).Port)
	writePanelURL(panelURL)

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(panelHTML)
	})
	mux.HandleFunc("/api/status", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, Status.snapshot())
	})
	mux.HandleFunc("/api/pair", func(w http.ResponseWriter, r *http.Request) {
		code, url, err := StartPanelPairing()
		if err != nil {
			w.WriteHeader(http.StatusBadGateway)
			writeJSON(w, map[string]string{"error": "could not reach Codex"})
			return
		}
		OpenBrowser(url)
		writeJSON(w, map[string]string{"userCode": code, "verifyUrl": url})
	})
	mux.HandleFunc("/api/open-codex", func(w http.ResponseWriter, r *http.Request) {
		OpenBrowser(CodexAccountURL())
		writeJSON(w, map[string]bool{"ok": true})
	})
	mux.HandleFunc("/api/open-npcap", func(w http.ResponseWriter, r *http.Request) {
		OpenBrowser("https://npcap.com/#download")
		writeJSON(w, map[string]bool{"ok": true})
	})

	go func() { _ = http.Serve(ln, mux) }()
	log.Infof("Control panel running at %s", panelURL)
	openPanelWindow(panelURL)
}

// PanelURL returns the running control-panel URL, or "" if it never started.
func PanelURL() string { return panelURL }

// OpenPanel re-opens the control-panel window (e.g. from the tray).
func OpenPanel() {
	if panelURL != "" {
		openPanelWindow(panelURL)
	}
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// openPanelWindow opens the panel as a chromeless app window via Edge or Chrome
// (--app mode), falling back to the default browser if neither is found.
func openPanelWindow(url string) {
	if runtime.GOOS == "windows" {
		for _, browser := range appModeBrowsers() {
			if fileExists(browser) {
				cmd := exec.Command(browser, "--app="+url, "--window-size=460,640")
				if err := cmd.Start(); err == nil {
					return
				}
			}
		}
	}
	OpenBrowser(url)
}

func appModeBrowsers() []string {
	return []string{
		`C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe`,
		`C:\Program Files\Microsoft\Edge\Application\msedge.exe`,
		`C:\Program Files\Google\Chrome\Application\chrome.exe`,
		`C:\Program Files (x86)\Google\Chrome\Application\chrome.exe`,
	}
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
