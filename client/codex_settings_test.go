package client

import (
	"os"
	"path/filepath"
	"testing"
)

// The behaviour worth pinning is applyAODPUpload's composition rule: the toggle
// must never discard a target the user configured themselves with -i, and must
// never add the placeholder twice.

func resetAODPConfig(t *testing.T, initial string) {
	t.Helper()
	prev := ConfigGlobal.PublicIngestBaseUrls
	ConfigGlobal.PublicIngestBaseUrls = initial
	t.Cleanup(func() { ConfigGlobal.PublicIngestBaseUrls = prev })
}

func TestApplyAODPUploadAddsAndRemovesPlaceholder(t *testing.T) {
	resetAODPConfig(t, "")

	applyAODPUpload(true)
	if ConfigGlobal.PublicIngestBaseUrls != AODPPlaceholderURL {
		t.Fatalf("on: got %q want %q", ConfigGlobal.PublicIngestBaseUrls, AODPPlaceholderURL)
	}
	if !AODPUploadEnabled() {
		t.Fatal("AODPUploadEnabled should be true once the placeholder is set")
	}

	applyAODPUpload(false)
	if ConfigGlobal.PublicIngestBaseUrls != "" {
		t.Fatalf("off: got %q want empty", ConfigGlobal.PublicIngestBaseUrls)
	}
	if AODPUploadEnabled() {
		t.Fatal("AODPUploadEnabled should be false once the placeholder is removed")
	}
}

func TestApplyAODPUploadKeepsUserConfiguredTargets(t *testing.T) {
	// Someone passed their own -i target. Toggling AODP off must not take it
	// away with it.
	const mine = "https://ingest.example.test"
	resetAODPConfig(t, mine)

	applyAODPUpload(true)
	got := ConfigGlobal.PublicIngestBaseUrls
	if got != mine+","+AODPPlaceholderURL {
		t.Fatalf("on: got %q", got)
	}

	applyAODPUpload(false)
	if ConfigGlobal.PublicIngestBaseUrls != mine {
		t.Fatalf("off: got %q want %q", ConfigGlobal.PublicIngestBaseUrls, mine)
	}
}

func TestApplyAODPUploadIsIdempotent(t *testing.T) {
	// Two "on" clicks must not produce a doubled target, which would upload
	// every capture to AODP twice.
	resetAODPConfig(t, "")
	applyAODPUpload(true)
	applyAODPUpload(true)
	if ConfigGlobal.PublicIngestBaseUrls != AODPPlaceholderURL {
		t.Fatalf("got %q, want a single placeholder", ConfigGlobal.PublicIngestBaseUrls)
	}
}

func TestApplyAODPUploadDropsEmptySegments(t *testing.T) {
	// A stray comma must not reach createUploaders as an empty target.
	resetAODPConfig(t, ",,")
	applyAODPUpload(true)
	if ConfigGlobal.PublicIngestBaseUrls != AODPPlaceholderURL {
		t.Fatalf("got %q", ConfigGlobal.PublicIngestBaseUrls)
	}
}

// The tri-state is the whole reason AODPUpload is a pointer: flipping the
// shipped default to ON must not reach a player who already chose OFF.
func TestAODPTriStateDefaultAndExplicitOptOut(t *testing.T) {
	if !(codexSettings{}).aodpUploadOrDefault() {
		t.Fatal("an unconfigured client should take the shipped default (on)")
	}
	no := false
	if (codexSettings{AODPUpload: &no}).aodpUploadOrDefault() {
		t.Fatal("an explicit opt-out must survive the default being on")
	}
	yes := true
	if !(codexSettings{AODPUpload: &yes}).aodpUploadOrDefault() {
		t.Fatal("an explicit opt-in should stay on")
	}
}

func TestSetAODPUploadRecordsAnExplicitChoice(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ProgramData", dir)
	resetAODPConfig(t, "")
	settingsMu.Lock()
	settingsCached = nil
	settingsMu.Unlock()
	t.Cleanup(func() {
		settingsMu.Lock()
		settingsCached = nil
		settingsMu.Unlock()
	})

	if err := SetAODPUpload(false); err != nil {
		t.Fatalf("save: %v", err)
	}
	settingsMu.Lock()
	settingsCached = nil
	settingsMu.Unlock()

	got := LoadCodexSettings()
	if got.AODPUpload == nil {
		t.Fatal("turning it off must persist an explicit false, not an absent value")
	}
	if got.aodpUploadOrDefault() {
		t.Fatal("a stored opt-out must resolve to off")
	}
}

func TestSettingsRoundTripAndCorruptFileFallsBackToDefaults(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ProgramData", dir)

	// Cache is process-wide; clear it so this test reads from our temp dir.
	settingsMu.Lock()
	settingsCached = nil
	settingsMu.Unlock()
	t.Cleanup(func() {
		settingsMu.Lock()
		settingsCached = nil
		settingsMu.Unlock()
	})

	on := true
	if err := SaveCodexSettings(codexSettings{AODPUpload: &on}); err != nil {
		t.Fatalf("save: %v", err)
	}
	path := filepath.Join(dir, "CodexClient", "codex-settings.json")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("settings file not written to %s: %v", path, err)
	}

	settingsMu.Lock()
	settingsCached = nil
	settingsMu.Unlock()
	if !LoadCodexSettings().aodpUploadOrDefault() {
		t.Fatal("round trip lost the setting")
	}

	// A truncated file must not stop the client starting.
	if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	settingsMu.Lock()
	settingsCached = nil
	settingsMu.Unlock()
	if LoadCodexSettings().AODPUpload != nil {
		t.Fatal("corrupt file should yield an UNCONFIGURED value, not a stale explicit choice")
	}
}
