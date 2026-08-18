package client

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/ao-data/albiondata-client/log"
)

// versionReportInterval re-reports the running build once a day so the admin
// device roster stays current for long-lived (autostart) clients.
const versionReportInterval = 24 * time.Hour

// startVersionReporter reports this client's build version to Codex on startup and
// once per day thereafter, so the admin device roster shows which build each device
// runs. Fire-and-forget and paired-only: an unpaired client skips it, and any
// failure is logged and swallowed so it never blocks startup or capture.
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

// reportVersionToCodex POSTs {"version": <build>} to /api/client/version with the
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

	body, err := json.Marshal(map[string]string{"version": version})
	if err != nil {
		log.Errorf("Codex: could not encode version report: %v", err)
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
	req.Header.Set("User-Agent", codexClientVersion)

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
