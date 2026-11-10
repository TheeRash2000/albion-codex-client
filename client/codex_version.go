package client

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/ao-data/albiondata-client/log"
)

// versionReportInterval re-reports the running build and the market heartbeat.
// Hourly (was daily) so the admin roster shows a market capture that stopped or
// is failing within the hour, not the next day: one small request per hour.
const versionReportInterval = time.Hour

// startVersionReporter reports this client's build version and market-capture
// health to Codex on startup and hourly thereafter, so the admin device roster
// shows which build each device runs and whether its market capture works.
// Fire-and-forget and paired-only: an unpaired client skips it, and any failure
// is logged and swallowed so it never blocks startup or capture.
func startVersionReporter(version string) {
	go func() {
		reportVersionToCodex(version)
		t := time.NewTicker(versionReportInterval)
		defer t.Stop()
		for range t.C {
			reportVersionToCodex(version)
		}
	}()
}

// marketHeartbeat is the market-capture health sent with each check-in. Without
// it the server cannot tell "market upload switched off" or "every upload is
// failing" from a player who is not browsing a market (2026-10-04).
type marketHeartbeat struct {
	On         bool   `json:"on"`
	LastSentAt string `json:"last_sent_at,omitempty"` // last accepted upload this run
	Failed     int    `json:"failed"`                 // failed uploads this run
}

type versionReport struct {
	Version string          `json:"version"`
	Market  marketHeartbeat `json:"market"`
}

// versionReportBody encodes the check-in. Servers before migration 072 read only
// "version" and ignore the rest, so this is safe to ship in either order.
func versionReportBody(version string) []byte {
	lastSent, failed := Status.streamStats(marketStream)
	hb := marketHeartbeat{On: marketUploadEnabled(), Failed: failed}
	if !lastSent.IsZero() {
		hb.LastSentAt = lastSent.UTC().Format(time.RFC3339)
	}
	b, err := json.Marshal(versionReport{Version: version, Market: hb})
	if err != nil {
		log.Errorf("Codex: could not encode version report: %v", err)
		return nil
	}
	return b
}

// reportVersionToCodex POSTs {"version": <build>, "market": {...}} to /api/client/version with the
// device bearer token. No-op (single debug line) when not paired. The version
// string matches the one stamped into the build / shown on the /download page
// (e.g. "alpha-0.1.10"). The server truncates to 40 chars.
func reportVersionToCodex(version string) {
	token := strings.TrimSpace(ConfigGlobal.CodexToken)
	if token == "" {
		log.Debug("Codex: not paired (no device token); version report skipped.")
		return
	}
	version = strings.TrimSpace(version)
	if version == "" {
		return
	}

	body := versionReportBody(version)
	if body == nil {
		return
	}

	base := strings.TrimRight(strings.TrimSpace(ConfigGlobal.CodexBaseURL), "/")
	if base == "" {
		base = "https://albioncodex.com"
	}
	req, err := http.NewRequest(http.MethodPost, base+"/api/client/version", bytes.NewReader(body))
	if err != nil {
		log.Errorf("Codex: could not build version request: %v", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("User-Agent", codexUserAgent())

	resp, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
	if err != nil {
		// Best-effort telemetry; never disrupt the client.
		log.Debugf("Codex: version report failed: %v", err)
		return
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		log.Debugf("Codex: reported client version %s.", version)
		Status.setTokenRejected(false)
	case resp.StatusCode == http.StatusUnauthorized:
		// The version report fires at startup, so this is usually the FIRST place a
		// revoked device finds out its token is dead - flip the panel to the
		// re-link state right away instead of waiting for a capture upload to fail.
		log.Error("Codex: device token rejected (401) on version report. Re-pair the client in Codex settings.")
		Status.setTokenRejected(true)
	default:
		log.Debugf("Codex: version report rejected with status %d.", resp.StatusCode)
	}
}
