package client

import (
	"encoding/json"
	"strings"
	"testing"
)

// After a self-update the next start shows the release note once. The note is
// saved by the updater before it restarts and consumed on the next start.
func TestUpdateNoteShownOnceForTheVersionItWasSavedFor(t *testing.T) {
	s := withPendingUpdateNote(codexSettings{}, "alpha-0.1.20", "Your Codex+ progress now shows in this window")

	line, cleared, changed := takeUpdateNote(s, "alpha-0.1.20")
	if line != "Updated to alpha-0.1.20 - Your Codex+ progress now shows in this window" {
		t.Errorf("line = %q", line)
	}
	if !changed || cleared.PendingUpdateNote != nil {
		t.Errorf("note not cleared after showing: changed=%v note=%+v", changed, cleared.PendingUpdateNote)
	}

	// The start after that has nothing to show.
	if line, _, changed := takeUpdateNote(cleared, "alpha-0.1.20"); line != "" || changed {
		t.Errorf("shown again: %q (changed=%v)", line, changed)
	}
}

func TestUpdateNoteNotShownWhenItDoesNotApply(t *testing.T) {
	cases := []struct {
		name    string
		s       codexSettings
		running string
	}{
		{"fresh install: nothing saved", codexSettings{}, "alpha-0.1.20"},
		{"saved for a different build than the one running", withPendingUpdateNote(codexSettings{}, "alpha-0.1.20", "note"), "alpha-0.1.21"},
		{"release had no note", withPendingUpdateNote(codexSettings{}, "alpha-0.1.20", ""), "alpha-0.1.20"},
		{"note is only whitespace", withPendingUpdateNote(codexSettings{}, "alpha-0.1.20", "  \n "), "alpha-0.1.20"},
	}
	for _, c := range cases {
		line, cleared, _ := takeUpdateNote(c.s, c.running)
		if line != "" {
			t.Errorf("%s: showed %q", c.name, line)
		}
		// Whatever was pending is dropped either way, so a stale note can never
		// surface on some later start.
		if cleared.PendingUpdateNote != nil {
			t.Errorf("%s: pending note left behind: %+v", c.name, cleared.PendingUpdateNote)
		}
	}
}

// The note comes from a file on the internet. It is shown as one short line of
// plain text whatever it contains.
func TestUpdateNoteIsOneShortLine(t *testing.T) {
	long := strings.Repeat("word ", 60)
	line, _, _ := takeUpdateNote(withPendingUpdateNote(codexSettings{}, "alpha-0.1.20", long), "alpha-0.1.20")
	note := strings.TrimPrefix(line, "Updated to alpha-0.1.20 - ")
	if n := len([]rune(note)); n > updateNoteMaxRunes {
		t.Errorf("note is %d characters, limit %d", n, updateNoteMaxRunes)
	}

	line, _, _ = takeUpdateNote(withPendingUpdateNote(codexSettings{}, "alpha-0.1.20", "first line\r\nsecond\tline"), "alpha-0.1.20")
	if strings.ContainsAny(line, "\r\n\t") {
		t.Errorf("control characters survived: %q", line)
	}
	if line != "Updated to alpha-0.1.20 - first line second line" {
		t.Errorf("line = %q", line)
	}
}

// Saving a note must not disturb the player's other settings, and a settings
// file written by an older build (no note field) must still load.
func TestUpdateNoteLivesBesideOtherSettings(t *testing.T) {
	off := false
	s := withPendingUpdateNote(codexSettings{AODPUpload: &off}, "alpha-0.1.20", "note")
	b, err := json.Marshal(s)
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
	if back.PendingUpdateNote == nil || back.PendingUpdateNote.Version != "alpha-0.1.20" || back.PendingUpdateNote.Note != "note" {
		t.Errorf("note did not round-trip: %+v", back.PendingUpdateNote)
	}

	var old codexSettings
	if err := json.Unmarshal([]byte(`{"aodp_upload":false}`), &old); err != nil || old.PendingUpdateNote != nil {
		t.Errorf("older settings file: err=%v note=%+v", err, old.PendingUpdateNote)
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
