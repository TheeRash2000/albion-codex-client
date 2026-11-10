package client

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

// A self-update is silent, so the first start of a new build says once what
// changed. The NEW build works this out for itself: an older build cannot be
// relied on to leave anything behind (0.1.19 and earlier have no such code).

// justUpdated decides whether this start follows an update, from the version
// this install last ran and whether the updater's leftover file is present.
func TestJustUpdated(t *testing.T) {
	cases := []struct {
		name      string
		lastRun   string
		running   string
		oldBinary bool
		want      bool
	}{
		{"same build as last time", "alpha-0.1.20", "alpha-0.1.20", true, false},
		{"updated from a build that recorded its version", "alpha-0.1.20", "alpha-0.1.21", false, true},
		// 0.1.19 and earlier never recorded a version. The updater leaves the old
		// exe beside the new one, which a fresh install does not have.
		{"updated from a build that recorded nothing", "", "alpha-0.1.20", true, true},
		{"fresh install", "", "alpha-0.1.20", false, false},
		{"local build with no version", "alpha-0.1.20", "dev", true, false},
		{"unversioned build", "alpha-0.1.20", "", true, false},
	}
	for _, c := range cases {
		if got := justUpdated(c.lastRun, c.running, c.oldBinary); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

// The note is the published manifest's, and only when that manifest describes
// the build that is running.
func TestUpdateNoteLine(t *testing.T) {
	cases := []struct {
		name            string
		running         string
		manifestVersion string
		notes           string
		want            string
	}{
		{"the manifest describes this build", "alpha-0.1.20", "alpha-0.1.20", "Your Codex+ progress now shows in this window",
			"Updated to alpha-0.1.20 - Your Codex+ progress now shows in this window"},
		{"a newer release is already out: its note is not ours", "alpha-0.1.20", "alpha-0.1.21", "Something else", ""},
		{"running a build newer than the published one", "alpha-0.1.21", "alpha-0.1.20", "Old note", ""},
		{"the release had no note", "alpha-0.1.20", "alpha-0.1.20", "", ""},
		{"the note is only whitespace", "alpha-0.1.20", "alpha-0.1.20", "  \n ", ""},
	}
	for _, c := range cases {
		if got := updateNoteLine(c.running, c.manifestVersion, c.notes); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

// The note comes from a file on the internet. It is shown as one short line of
// plain text whatever it contains.
func TestUpdateNoteIsOneShortLine(t *testing.T) {
	long := strings.Repeat("word ", 60)
	line := updateNoteLine("alpha-0.1.20", "alpha-0.1.20", long)
	note := strings.TrimPrefix(line, "Updated to alpha-0.1.20 - ")
	if n := len([]rune(note)); n > updateNoteMaxRunes {
		t.Errorf("note is %d characters, limit %d", n, updateNoteMaxRunes)
	}

	line = updateNoteLine("alpha-0.1.20", "alpha-0.1.20", "first line\r\nsecond\tline")
	if line != "Updated to alpha-0.1.20 - first line second line" {
		t.Errorf("line = %q", line)
	}
}

// Recording the version must not disturb the player's other settings, and a
// settings file written by an older build must still load.
func TestLastRunVersionLivesBesideOtherSettings(t *testing.T) {
	off := false
	b, err := json.Marshal(codexSettings{AODPUpload: &off, LastRunVersion: "alpha-0.1.20"})
	if err != nil {
		t.Fatal(err)
	}
	var back codexSettings
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	if back.AODPUpload == nil || *back.AODPUpload != false {
		t.Error("the AODP opt-out was lost")
	}
	if back.LastRunVersion != "alpha-0.1.20" {
		t.Errorf("last run version = %q", back.LastRunVersion)
	}

	var old codexSettings
	if err := json.Unmarshal([]byte(`{"aodp_upload":false}`), &old); err != nil || old.LastRunVersion != "" {
		t.Errorf("older settings file: err=%v last=%q", err, old.LastRunVersion)
	}
}

// The updater's leftover is the old exe, renamed and hidden beside the new one.
func TestOldBinaryPath(t *testing.T) {
	got := oldBinaryPath(filepath.Join("CodexClient", "codex-client.exe"))
	if got != filepath.Join("CodexClient", ".codex-client.exe.old") {
		t.Errorf("old binary path = %q", got)
	}
}

// Uploads identify the build that sent them. This was hard-coded to 0.1.15.
func TestUserAgentCarriesTheRunningVersion(t *testing.T) {
	prev := version
	defer func() { version = prev }()

	version = "alpha-0.1.20"
	if got := codexUserAgent(); got != "codex-client/alpha-0.1.20" {
		t.Errorf("user agent = %q", got)
	}
	version = ""
	if got := codexUserAgent(); got != "codex-client/dev" {
		t.Errorf("unversioned build: user agent = %q", got)
	}
}
