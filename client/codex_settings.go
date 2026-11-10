package client

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/ao-data/albiondata-client/log"
)

// User-toggleable settings that need to survive a restart.
//
// The CLI flags are the power-user surface; this file is what the panel's
// checkboxes write, so a choice made in the UI is still in effect tomorrow. It
// deliberately reuses the device token's location strategy (codex_pair.go):
// machine-wide %PROGRAMDATA%\CodexClient first, because it is outside OneDrive
// (which dehydrates files), independent of the folder the exe runs from, and
// shared across elevation contexts - so a setting chosen in a normal launch is
// still there after "Run as administrator".
//
// Kept separate from config.yaml on purpose: that file is hand-edited by users
// and read by viper, and rewriting it from the app would fight the person
// editing it and could drop their comments.

// AODPPlaceholderURL is the magic value dispatcher.go swaps, per realm, for the
// correct regional AODP ingest host (pow.west / pow.east / pow.europe). Storing
// the placeholder rather than a resolved URL is what makes the setting survive
// the player changing server.
const AODPPlaceholderURL = "https+pow://albion-online-data.com"

const settingsFileName = "codex-settings.json"

// defaultAODPUpload is what an UNCONFIGURED client does. On since 0.1.15: the
// Codex Client is a fork of the AODP client, we read AODP heavily, and a player
// running this instead of the upstream client should not silently stop
// contributing to the project the rest of the ecosystem depends on. Disclosed in
// the panel, the changelog and on /download, and switchable off in one click.
const defaultAODPUpload = true

type codexSettings struct {
	// Contribute captured market data to the Albion Online Data Project as well
	// as to Codex.
	//
	// A POINTER, deliberately. A plain bool cannot tell "never chose" from
	// "chose false", because both are the zero value - so flipping the default
	// to on would have silently re-enabled uploads for every player who had
	// already turned them OFF. nil means unconfigured (take the default); an
	// explicit false is honoured forever.
	AODPUpload *bool `json:"aodp_upload,omitempty"`

	// Contribute market prices scanned in-game to Codex. Same tri-state
	// reasoning as AODPUpload: nil means unconfigured (take the flag default),
	// an explicit false is honoured across restarts.
	MarketUpload *bool `json:"market_upload,omitempty"`

	// The release note saved by a self-update, to show once on the next start
	// (update_note.go). Absent except between an update and that start.
	PendingUpdateNote *pendingUpdateNote `json:"pending_update_note,omitempty"`
}

// aodpUploadOrDefault resolves the tri-state: an explicit choice wins, absence
// takes the shipped default.
func (s codexSettings) aodpUploadOrDefault() bool {
	if s.AODPUpload == nil {
		return defaultAODPUpload
	}
	return *s.AODPUpload
}

var (
	settingsMu     sync.RWMutex
	settingsCached *codexSettings
)

// settingsLocations lists read locations in priority order, mirroring
// tokenLocations() so both files live together and move together.
func settingsLocations() []string {
	locs := []string{}
	if pd := strings.TrimSpace(os.Getenv("ProgramData")); pd != "" {
		locs = append(locs, filepath.Join(pd, "CodexClient", settingsFileName))
	}
	if exe, err := os.Executable(); err == nil {
		locs = append(locs, filepath.Join(filepath.Dir(exe), settingsFileName))
	}
	if dir, err := os.UserConfigDir(); err == nil && dir != "" {
		locs = append(locs, filepath.Join(dir, "CodexClient", settingsFileName))
	}
	return locs
}

// primarySettingsPath is where we write: the machine-wide path when there is
// one, else next to the exe.
func primarySettingsPath() string {
	locs := settingsLocations()
	if len(locs) > 0 {
		return locs[0]
	}
	return settingsFileName
}

// LoadCodexSettings returns the persisted settings, reading from disk once and
// caching. A missing or corrupt file is not an error: it yields defaults, so a
// half-written file can never stop the client from starting.
func LoadCodexSettings() codexSettings {
	settingsMu.RLock()
	if settingsCached != nil {
		s := *settingsCached
		settingsMu.RUnlock()
		return s
	}
	settingsMu.RUnlock()

	settingsMu.Lock()
	defer settingsMu.Unlock()
	if settingsCached != nil {
		return *settingsCached
	}

	s := codexSettings{}
	for _, p := range settingsLocations() {
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		if err := json.Unmarshal(b, &s); err != nil {
			// Corrupt file: fall back to defaults rather than refusing to start.
			log.Infof("Could not parse %v (%v); using default settings.", p, err)
			s = codexSettings{}
		}
		break
	}
	settingsCached = &s
	return s
}

// SaveCodexSettings persists settings and updates the cache. Writes through a
// temp file + rename so a crash mid-write cannot leave a truncated file that
// the next launch would read as "unconfigured" - which now means the DEFAULT,
// so a player who deliberately opted out could find themselves contributing
// again. Losing an explicit choice is the failure that matters here.
func SaveCodexSettings(s codexSettings) error {
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}

	path := primarySettingsPath()
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}

	settingsMu.Lock()
	c := s
	settingsCached = &c
	settingsMu.Unlock()
	return nil
}

// SetAODPUpload turns third-party (AODP) contribution on or off, persists it,
// and applies it to the LIVE config.
//
// Applying it live is possible because sendMsgToPublicUploaders rebuilds its
// uploaders from ConfigGlobal on every send, so the next captured browse
// already respects the new value - no restart, which matters because the
// natural moment to toggle this is while looking at the panel mid-session.
func SetAODPUpload(on bool) error {
	s := LoadCodexSettings()
	// Writing the pointer records an EXPLICIT choice, so a future change to
	// defaultAODPUpload can never override what the player picked.
	s.AODPUpload = &on
	if err := SaveCodexSettings(s); err != nil {
		return err
	}
	applyAODPUpload(on)
	return nil
}

// applyAODPUpload reflects the setting into ConfigGlobal.
//
// Only ever adds or removes OUR placeholder. If someone passed their own -i
// target (a private aggregator, a test endpoint), toggling this must not
// discard it, so the placeholder is treated as one comma-separated entry among
// whatever else is configured rather than as the whole value.
func applyAODPUpload(on bool) {
	cur := ConfigGlobal.PublicIngestBaseUrls
	parts := []string{}
	for _, p := range strings.Split(cur, ",") {
		p = strings.TrimSpace(p)
		if p == "" || p == AODPPlaceholderURL {
			continue
		}
		parts = append(parts, p)
	}
	if on {
		parts = append(parts, AODPPlaceholderURL)
	}
	ConfigGlobal.PublicIngestBaseUrls = strings.Join(parts, ",")
}

// applyStoredAODPSetting is called once at startup, after flags and env are
// resolved. An explicit -i on the command line already put a target in the
// config; the stored setting only ever ADDS the placeholder, so the two compose
// instead of one silently winning.
func applyStoredAODPSetting() {
	if LoadCodexSettings().aodpUploadOrDefault() {
		applyAODPUpload(true)
	}
}

// AODPUploadEnabled reports whether the live config currently contributes to
// AODP. Read from ConfigGlobal rather than the file so it stays honest when
// someone passed -i on the command line without ever touching the panel.
func AODPUploadEnabled() bool {
	return strings.Contains(ConfigGlobal.PublicIngestBaseUrls, AODPPlaceholderURL)
}

// ---------------------------------------------------------------------------
// Market capture toggle
// ---------------------------------------------------------------------------
//
// The browser panel could only ever READ this (it showed "Market Data: On/Off"
// with no way to change it); `-codex-market` was the sole control surface, which
// meant turning capture off required editing a shortcut. The tray UI makes it a
// checkbox, so like the AODP toggle it now needs to survive a restart.

// SetMarketUpload turns Codex market capture on or off, persists the choice, and
// applies it to the live config.
//
// Applying it live is safe because every capture path reads
// ConfigGlobal.CodexMarketUpload at send time (via marketUploadEnabled), so the
// next browse already respects the new value - no restart, which matters because
// the natural moment to toggle this is mid-session.
func SetMarketUpload(on bool) error {
	s := LoadCodexSettings()
	// A pointer for the same reason AODPUpload is one: a plain bool cannot tell
	// "never chose" from "chose false", so an explicit opt-out must be recorded
	// as an explicit value or a future default change would silently undo it.
	s.MarketUpload = &on
	if err := SaveCodexSettings(s); err != nil {
		return err
	}
	ConfigGlobal.CodexMarketUpload = on
	return nil
}

// applyStoredMarketSetting is called once at startup, after flags and env.
//
// Unlike the AODP setting (a list, so stored and CLI values compose), this is a
// plain bool where the two can genuinely conflict. An explicitly PASSED
// `-codex-market` wins: someone who types a flag means it for that launch, and a
// stored value silently overriding it would make the flag look broken. Absent
// that flag, the stored choice wins over the built-in default.
func applyStoredMarketSetting() {
	if flagWasSet("codex-market") {
		return
	}
	if v := LoadCodexSettings().MarketUpload; v != nil {
		ConfigGlobal.CodexMarketUpload = *v
	}
}

// flagWasSet reports whether the named flag was actually present on the command
// line, as opposed to sitting at its default.
func flagWasSet(name string) bool {
	found := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == name {
			found = true
		}
	})
	return found
}

// MarketUploadEnabled reports whether Codex market capture is currently on.
// Reads the live config, so it stays honest when the flag was passed.
func MarketUploadEnabled() bool { return marketUploadEnabled() }
