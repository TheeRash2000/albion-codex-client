package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/ao-data/albiondata-client/log"
)

// legacyTokenFile is the old cwd-relative token location used by early builds.
// It is read as a fallback and migrated forward to the stable path, so a device
// that paired once never has to pair again even if the exe moves.
const legacyTokenFile = "codex-token"

// machineTokenPath returns the machine-wide token location
// (%PROGRAMDATA%\CodexClient\codex-token on Windows), or "" if there is no such
// dir. This is the most robust home for the token: it is outside OneDrive (so it
// never dehydrates), independent of which folder the exe runs from, and shared
// across the same machine's user/elevation contexts (ProgramData is machine-wide,
// not per-user) - so "Run as administrator" and a normal launch read the same
// token. It is the root-cause fix for the "still Not linked -> re-pair -> new
// device every time" loop.
func machineTokenPath() string {
	// ProgramData is Windows-only; empty elsewhere, where this is simply skipped.
	if pd := strings.TrimSpace(os.Getenv("ProgramData")); pd != "" {
		return filepath.Join(pd, "CodexClient", "codex-token")
	}
	return ""
}

// tokenLocations lists every place we read/write the device token, in priority
// order. The machine-wide ProgramData path comes first so the token survives a
// different folder, a OneDrive-synced exe dir, and "Run as administrator". The
// exe-dir and per-user config dir follow as compatible second homes (and keep
// older paired installs working); the bare cwd file is legacy read support.
func tokenLocations() []string {
	locs := []string{}
	if m := machineTokenPath(); m != "" {
		locs = append(locs, m)
	}
	if exe, err := os.Executable(); err == nil {
		locs = append(locs, filepath.Join(filepath.Dir(exe), "codex-token"))
	}
	if dir, err := os.UserConfigDir(); err == nil && dir != "" {
		locs = append(locs, filepath.Join(dir, "CodexClient", "codex-token"))
	}
	locs = append(locs, legacyTokenFile) // cwd, legacy read-only fallback
	return locs
}

// primaryTokenPath is the most robust, user-facing save location: the machine-wide
// ProgramData path when available, otherwise next to the exe. Used only for the
// "token saved to ..." message.
func primaryTokenPath() string {
	if m := machineTokenPath(); m != "" {
		return m
	}
	if exe, err := os.Executable(); err == nil {
		return filepath.Join(filepath.Dir(exe), "codex-token")
	}
	return legacyTokenFile
}

func readTokenFile(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// loadCodexToken returns the saved device token from the first location that has
// one. If it was found anywhere but the primary (exe-dir) path, it is migrated
// forward so the next launch finds it immediately.
func loadCodexToken() string {
	locs := tokenLocations()
	for i, p := range locs {
		if t := readTokenFile(p); t != "" {
			if i > 0 {
				_ = saveCodexToken(t)
			}
			return t
		}
	}
	return ""
}

type pairStartResp struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
}

type pairPollResp struct {
	Status string `json:"status"`
	Token  string `json:"token"`
}

func codexBase() string {
	base := strings.TrimRight(strings.TrimSpace(ConfigGlobal.CodexBaseURL), "/")
	if base == "" {
		base = "https://albioncodex.com"
	}
	return base
}

// RunCodexPairing executes the GitHub-style device pairing flow: request a code,
// show the user where to enter it, poll until they confirm, and persist the issued
// token. Returns true when a token was obtained and saved.
func RunCodexPairing() bool {
	base := codexBase()
	label, _ := os.Hostname()

	start, err := codexPairStart(base, label)
	if err != nil {
		log.Errorf("Codex pairing: could not start (%v). Check your connection and that %s is reachable.", err, base)
		return false
	}

	fmt.Println()
	fmt.Println("  Link this PC to your Codex account:")
	fmt.Printf("    1. Open  %s\n", start.VerificationURI)
	fmt.Printf("    2. Enter this code:  %s\n", start.UserCode)
	fmt.Println()
	fmt.Println("  Waiting for confirmation...")

	// Convenience: pop the link page open. Harmless if it fails (the URL is printed).
	OpenBrowser(start.VerificationURI)

	interval := start.Interval
	if interval < 2 {
		interval = 5
	}
	expires := start.ExpiresIn
	if expires < 60 {
		expires = 600
	}
	deadline := time.Now().Add(time.Duration(expires) * time.Second)

	for time.Now().Before(deadline) {
		time.Sleep(time.Duration(interval) * time.Second)

		poll, err := codexPairPoll(base, start.DeviceCode)
		if err != nil {
			log.Debugf("Codex pairing: poll error, will retry: %v", err)
			continue
		}
		switch poll.Status {
		case "paired":
			if err := saveCodexToken(poll.Token); err != nil {
				log.Errorf("Codex pairing: linked, but could not save the token: %v", err)
				fmt.Printf("  Linked. Set this as CODEX_TOKEN to use it: %s\n", poll.Token)
				return true
			}
			ConfigGlobal.CodexToken = poll.Token
			fmt.Printf("  Linked. Token saved next to the client (%s) - this PC stays linked; just keep this client running.\n", primaryTokenPath())
			return true
		case "expired", "not_found":
			log.Error("Codex pairing: the code expired before it was confirmed. Run pairing again.")
			return false
		default:
			// "pending" - keep polling
		}
	}

	log.Error("Codex pairing: timed out waiting for confirmation. Run pairing again.")
	return false
}

// StartPanelPairing drives the device flow from the control panel: it requests a
// code, starts polling in the background, and returns the code + link URL for the
// panel to display. On success it saves the token and flips Status to linked.
func StartPanelPairing() (userCode, verifyURL string, err error) {
	base := codexBase()
	label, _ := os.Hostname()

	start, err := codexPairStart(base, label)
	if err != nil {
		return "", "", err
	}
	Status.setPairing(true, start.UserCode, start.VerificationURI)

	interval := start.Interval
	if interval < 2 {
		interval = 5
	}
	expires := start.ExpiresIn
	if expires < 60 {
		expires = 600
	}

	go func() {
		deadline := time.Now().Add(time.Duration(expires) * time.Second)
		for time.Now().Before(deadline) {
			time.Sleep(time.Duration(interval) * time.Second)
			poll, perr := codexPairPoll(base, start.DeviceCode)
			if perr != nil {
				continue
			}
			switch poll.Status {
			case "paired":
				if err := saveCodexToken(poll.Token); err != nil {
					log.Errorf("Codex pairing: linked, but could not save token: %v", err)
				}
				ConfigGlobal.CodexToken = poll.Token
				Status.setPairing(false, "", "")
				log.Info("Codex: this PC is now linked.")
				return
			case "expired", "not_found":
				Status.setPairing(false, "", "")
				return
			}
		}
		Status.setPairing(false, "", "")
	}()

	return start.UserCode, start.VerificationURI, nil
}

func codexPairStart(base, label string) (*pairStartResp, error) {
	body, _ := json.Marshal(map[string]string{"label": label})
	resp, err := http.Post(base+"/api/client/pair/start", "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}
	var out pairStartResp
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	if out.DeviceCode == "" || out.UserCode == "" {
		return nil, fmt.Errorf("incomplete pairing response")
	}
	return &out, nil
}

func codexPairPoll(base, deviceCode string) (*pairPollResp, error) {
	body, _ := json.Marshal(map[string]string{"device_code": deviceCode})
	resp, err := http.Post(base+"/api/client/pair/poll", "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var out pairPollResp
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// saveCodexToken writes the token to every stable location (next to the exe AND
// the per-user config dir) so the next run finds it however it is launched.
// Succeeds if at least one write lands.
func saveCodexToken(token string) error {
	var lastErr error
	wrote := false
	for _, p := range tokenLocations() {
		if p == legacyTokenFile {
			continue // never write the bare cwd file
		}
		if dir := filepath.Dir(p); dir != "" {
			_ = os.MkdirAll(dir, 0o700)
		}
		if err := os.WriteFile(p, []byte(token+"\n"), 0o600); err != nil {
			lastErr = err
			continue
		}
		wrote = true
	}
	if wrote {
		return nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no writable token location")
	}
	return lastErr
}

// OpenBrowser opens a URL in the user's default browser. Best-effort.
func OpenBrowser(url string) {
	var err error
	switch runtime.GOOS {
	case "windows":
		err = exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	case "darwin":
		err = exec.Command("open", url).Start()
	default:
		err = exec.Command("xdg-open", url).Start()
	}
	if err != nil {
		log.Debugf("Codex: could not open browser (%v); open it manually: %s", err, url)
	}
}

// CodexLinked reports whether this client has a device token.
func CodexLinked() bool {
	return strings.TrimSpace(ConfigGlobal.CodexToken) != ""
}

// CodexAccountURL is the Codex page where the user links/manages devices.
func CodexAccountURL() string {
	return codexBase() + "/account/link"
}
