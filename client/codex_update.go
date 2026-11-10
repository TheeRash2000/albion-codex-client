package client

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/ao-data/albiondata-client/log"
	"github.com/minio/selfupdate"
)

// updateManifestURLs are the places the latest build's version.json is published,
// tried in order; the first one that answers with a manifest wins.
//
//  1. The latest GitHub release of the public client repo, where releases are
//     built from the published source.
//  2. The Supabase Storage "client" bucket that builds up to alpha-0.1.18 were
//     served from (scripts/build-codex.ps1 emits its version.json). Kept so the
//     updater keeps working until the first GitHub release exists; remove it
//     once every supported build has moved across.
//
// Overridable via CODEX_UPDATE_MANIFEST_URL for testing against a staging file.
var updateManifestURLs = []string{
	"https://github.com/TheeRash2000/albion-codex-client/releases/latest/download/version.json",
	"https://blpjzxfbjwxrdfzeqxhl.supabase.co/storage/v1/object/public/client/version.json",
}

const (
	updateInitialDelay = 45 * time.Second // let capture settle before the first check
	updateCheckEvery   = 6 * time.Hour
	maxBinaryBytes     = 100 << 20 // bound memory on a bad URL; the exe is ~21 MB
)

// updateManifest is the version.json published alongside the exe.
type updateManifest struct {
	Version string `json:"version"`
	URL     string `json:"url"`
	SHA256  string `json:"sha256"`
	Notes   string `json:"notes"`
}

// lastUpdateAttempt is the version we last tried to apply this session, so a
// repeatedly-failing manifest doesn't re-download every check.
var lastUpdateAttempt string

// startCodexUpdater periodically checks the published manifest and self-updates the
// running binary when a newer build is available, verifying its SHA-256 before
// applying and then restarting. Windows-only (the only published binary) and
// best-effort: any failure is logged and the current build keeps running. Disabled
// for dev builds (a version with no numbers) so a local build never replaces itself.
func startCodexUpdater(currentVersion string) {
	if runtime.GOOS != "windows" {
		return
	}
	if parseVersionNums(currentVersion) == nil {
		log.Debugf("Codex updater: build %q has no comparable version; auto-update disabled.", currentVersion)
		return
	}
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Errorf("Codex updater: recovered from panic: %v", r)
			}
		}()
		time.Sleep(updateInitialDelay)
		checkForUpdate(currentVersion)
		t := time.NewTicker(updateCheckEvery)
		defer t.Stop()
		for range t.C {
			checkForUpdate(currentVersion)
		}
	}()
}

func manifestURLs() []string {
	if u := strings.TrimSpace(os.Getenv("CODEX_UPDATE_MANIFEST_URL")); u != "" {
		return []string{u}
	}
	return updateManifestURLs
}

func checkForUpdate(currentVersion string) {
	m, err := fetchManifest()
	if err != nil {
		log.Debugf("Codex updater: manifest check skipped: %v", err)
		return
	}
	if !versionIsNewer(m.Version, currentVersion) {
		log.Debugf("Codex updater: up to date (current %s, latest %s).", currentVersion, m.Version)
		return
	}
	if m.Version == lastUpdateAttempt {
		log.Debugf("Codex updater: already attempted %s this session; not retrying.", m.Version)
		return
	}
	lastUpdateAttempt = m.Version

	sum, err := manifestChecksum(m)
	if err != nil {
		log.Errorf("Codex updater: %v", err)
		return
	}

	log.Infof("Codex updater: new build %s available; downloading.", m.Version)
	Status.addActivity("Update available (" + m.Version + "); downloading")

	bin, err := downloadBinary(m.URL)
	if err != nil {
		log.Errorf("Codex updater: download failed: %v", err)
		return
	}
	// Defence in depth: verify before handing to Apply (which re-verifies with the
	// same checksum and rolls back on any failure).
	got := sha256.Sum256(bin)
	if !bytes.Equal(got[:], sum) {
		log.Errorf("Codex updater: downloaded %s has the wrong checksum; not applying.", m.Version)
		return
	}

	if err := selfupdate.Apply(bytes.NewReader(bin), selfupdate.Options{Checksum: sum}); err != nil {
		log.Errorf("Codex updater: apply failed: %v", err)
		if rerr := selfupdate.RollbackError(err); rerr != nil {
			log.Errorf("Codex updater: rollback also failed (binary may be in a bad state): %v", rerr)
		}
		return
	}

	log.Infof("Codex updater: updated to %s; restarting.", m.Version)
	Status.addActivity("Updated to " + m.Version + "; restarting")
	restartAfterUpdate()
}

// manifestChecksum validates the manifest's url+sha256 and returns the decoded sum.
func manifestChecksum(m *updateManifest) ([]byte, error) {
	if strings.TrimSpace(m.URL) == "" || strings.TrimSpace(m.SHA256) == "" {
		return nil, fmt.Errorf("manifest for %s is missing url or sha256", m.Version)
	}
	sum, err := hex.DecodeString(strings.TrimSpace(m.SHA256))
	if err != nil || len(sum) != sha256.Size {
		return nil, fmt.Errorf("manifest sha256 for %s is not valid hex", m.Version)
	}
	return sum, nil
}

// fetchManifest returns the manifest from the first source that serves one. A
// source that is unreachable, missing or unparseable is skipped; the error
// returned when all fail is the last one seen.
func fetchManifest() (*updateManifest, error) {
	var lastErr error
	for _, u := range manifestURLs() {
		m, err := fetchManifestFrom(u)
		if err == nil {
			return m, nil
		}
		log.Debugf("Codex updater: no manifest at %s: %v", u, err)
		lastErr = err
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no manifest source configured")
	}
	return nil, lastErr
}

func fetchManifestFrom(url string) (*updateManifest, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", codexClientVersion)
	req.Header.Set("Cache-Control", "no-cache")
	resp, err := (&http.Client{Timeout: 20 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("manifest status %d", resp.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	if err != nil {
		return nil, err
	}
	// Tolerate a UTF-8 BOM (some editors / PowerShell writers add one); Go's json
	// decoder does not skip it and would otherwise fail to parse.
	raw = bytes.TrimPrefix(raw, []byte{0xEF, 0xBB, 0xBF})
	var m updateManifest
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

func downloadBinary(url string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", codexClientVersion)
	resp, err := (&http.Client{Timeout: 5 * time.Minute}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download status %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, maxBinaryBytes))
}

// restartAfterUpdate relaunches the (now-replaced) executable and exits. The
// single-instance mutex is released first so the new process can acquire it instead
// of seeing this one still holding it and bailing as a duplicate.
func restartAfterUpdate() {
	exe, err := os.Executable()
	if err != nil {
		log.Errorf("Codex updater: cannot find executable to restart: %v", err)
		return
	}
	releaseSingleInstance()
	// The same flags as the launch being replaced: a login autostart restarts
	// quietly, a launch the player started reopens its window.
	cmd := exec.Command(exe, os.Args[1:]...)
	cmd.Env = os.Environ()
	if err := cmd.Start(); err != nil {
		log.Errorf("Codex updater: failed to start the updated process: %v", err)
		return
	}
	os.Exit(0)
}

var versionNumRe = regexp.MustCompile(`\d+`)

// parseVersionNums extracts the ordered integer groups from a version string
// ("alpha-0.1.10" -> [0 1 10]). Returns nil when there are none (e.g. "dev"),
// which callers treat as "no comparable version".
func parseVersionNums(v string) []int {
	groups := versionNumRe.FindAllString(v, -1)
	if len(groups) == 0 {
		return nil
	}
	nums := make([]int, 0, len(groups))
	for _, g := range groups {
		n, err := strconv.Atoi(g)
		if err != nil {
			return nil
		}
		nums = append(nums, n)
	}
	return nums
}

// versionIsNewer reports whether candidate is a strictly higher version than
// current, comparing integer groups left to right. Unparseable inputs are never
// "newer" (safer: no surprise update or downgrade).
func versionIsNewer(candidate, current string) bool {
	c := parseVersionNums(candidate)
	cur := parseVersionNums(current)
	if c == nil || cur == nil {
		return false
	}
	for i := 0; i < len(c) && i < len(cur); i++ {
		if c[i] != cur[i] {
			return c[i] > cur[i]
		}
	}
	return len(c) > len(cur)
}
