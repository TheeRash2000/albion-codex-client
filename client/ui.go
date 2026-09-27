package client

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"time"

	"github.com/ao-data/albiondata-client/log"
)

// The action surface the tray UI drives.
//
// This replaces the local HTTP control panel (panel.go / panel.html), which
// served its own JSON API to a browser window. Everything that used to be an
// HTTP handler is a plain exported function here, so the UI is an in-process
// call rather than a localhost round trip - no listener, no port, no browser.
//
// OpenBrowser survives and is still used, but only to open PAGES ON THE CODEX
// WEBSITE (the pairing verification URL, the account page) and the Npcap
// download. Those are genuinely external destinations; the client's own UI is
// no longer one of them.

// UIStatus returns the current state for rendering. Thin alias so the tray
// package never touches the Status global directly.
func UIStatus() StatusSnapshot { return Status.Snapshot() }

// StartLinking begins device pairing and hands back the code to show.
//
// It also opens the verification page and copies the code to the clipboard,
// because the tray cannot render a selectable code the way the old panel's
// copy button could - and a code you can see but not copy is worse than no
// code at all. copied reports whether the clipboard write actually landed, so
// the caller can tell the player to type it instead of silently assuming.
func StartLinking() (userCode, verifyURL string, copied bool, err error) {
	userCode, verifyURL, err = StartUIPairing()
	if err != nil {
		return "", "", false, err
	}
	copied = copyToClipboard(userCode)
	OpenBrowser(verifyURL)
	return userCode, verifyURL, copied, nil
}

// OpenCodexAccount opens the player's Codex account page (device management).
func OpenCodexAccount() { OpenBrowser(CodexAccountURL()) }

// OpenNpcapDownload opens the Npcap download page. Capture cannot start without
// the driver, so this is the one action that matters when it is missing.
func OpenNpcapDownload() { OpenBrowser("https://npcap.com/#download") }

// MemoryLine is a one-line memory + goroutine reading for the tray.
//
// Deliberately always visible rather than hidden behind a debug flag: the
// client runs unattended for days, and "is this thing growing?" should be
// answerable by looking at it, not by asking a tester to install pprof tooling.
func MemoryLine() string {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return fmt.Sprintf("Memory: %d MB (%d goroutines)", m.Sys/1024/1024, runtime.NumGoroutine())
}

// diagnosticsDir is where heap profiles are written. Sits next to the token and
// settings so everything the client owns lives in one place a tester can find.
func diagnosticsDir() string {
	base := ""
	if pd := os.Getenv("ProgramData"); pd != "" {
		base = filepath.Join(pd, "CodexClient")
	} else if dir, err := os.UserConfigDir(); err == nil && dir != "" {
		base = filepath.Join(dir, "CodexClient")
	} else {
		base = "."
	}
	d := filepath.Join(base, "diagnostics")
	_ = os.MkdirAll(d, 0o755)
	return d
}

// WriteHeapProfile dumps a pprof heap profile and returns the file path.
//
// runtime.GC() runs first on purpose: the heap profile records what is still
// reachable AFTER a collection, so without it a dump is dominated by garbage
// that was about to be freed and a healthy client looks like a leaking one.
// That is the whole point of the dump - it has to be able to exonerate.
//
// Read it with: go tool pprof -top <file>
func WriteHeapProfile() (string, error) {
	runtime.GC()

	path := filepath.Join(diagnosticsDir(), fmt.Sprintf("heap-%s.pprof", time.Now().Format("20060102-150405")))
	f, err := os.Create(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	if err := pprof.WriteHeapProfile(f); err != nil {
		return "", err
	}
	log.Infof("Wrote heap profile to %s", path)
	return path, nil
}

// CopyPairingCode re-copies an in-flight pairing code to the clipboard.
//
// The tray shows the code on the Link item while a handshake is running, so a
// second click should hand the player the code again rather than abandoning
// that handshake and starting a new one.
func CopyPairingCode(code string) bool { return copyToClipboard(code) }
