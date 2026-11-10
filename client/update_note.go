package client

import (
	"strings"

	"github.com/ao-data/albiondata-client/log"
)

// A self-update is silent: the client replaces itself and restarts. So that
// players learn what changed, the updater saves the release's note (the manifest's
// "notes") before it restarts, and the next start shows it once in the activity
// feed. A fresh install or an installer upgrade has nothing saved and shows
// nothing.

// updateNoteMaxRunes bounds the note. It comes from a file on the internet and
// is shown as one feed line.
const updateNoteMaxRunes = 140

// pendingUpdateNote is the note waiting to be shown, and the build it is for.
type pendingUpdateNote struct {
	Version string `json:"version"`
	Note    string `json:"note"`
}

// withPendingUpdateNote returns the settings with a note to show on the next
// start of the given build.
func withPendingUpdateNote(s codexSettings, version, note string) codexSettings {
	s.PendingUpdateNote = &pendingUpdateNote{Version: version, Note: note}
	return s
}

// takeUpdateNote returns the feed line to show (empty for none) and the settings
// with the pending note removed. The note is shown only to the build it was
// saved for, and it is dropped either way, so a stale note can never surface on
// a later start. changed reports whether the settings need saving.
func takeUpdateNote(s codexSettings, running string) (line string, cleared codexSettings, changed bool) {
	p := s.PendingUpdateNote
	if p == nil {
		return "", s, false
	}
	s.PendingUpdateNote = nil
	note := cleanUpdateNote(p.Note)
	if note == "" || p.Version != running {
		return "", s, true
	}
	return "Updated to " + p.Version + " - " + note, s, true
}

// cleanUpdateNote makes the note one short line of plain text.
func cleanUpdateNote(note string) string {
	note = strings.Join(strings.Fields(note), " ") // collapses newlines, tabs, runs of spaces
	if r := []rune(note); len(r) > updateNoteMaxRunes {
		note = strings.TrimSpace(string(r[:updateNoteMaxRunes]))
	}
	return note
}

// savePendingUpdateNote is called by the updater just before it restarts.
// Best-effort: a failure costs the player one line of text, never the update.
func savePendingUpdateNote(version, note string) {
	if cleanUpdateNote(note) == "" {
		return
	}
	if err := SaveCodexSettings(withPendingUpdateNote(LoadCodexSettings(), version, note)); err != nil {
		log.Debugf("Codex updater: could not save the release note: %v", err)
	}
}

// showPendingUpdateNote is called once at start. It shows the note saved by the
// update that produced this build, if any, and clears it.
func showPendingUpdateNote(running string) {
	line, cleared, changed := takeUpdateNote(LoadCodexSettings(), running)
	if !changed {
		return
	}
	if err := SaveCodexSettings(cleared); err != nil {
		// Not cleared on disk: do not show it, or it would show on every start.
		log.Debugf("Could not clear the saved release note: %v", err)
		return
	}
	if line != "" {
		log.Info(line)
		Status.addActivity(line)
	}
}

// codexUserAgent identifies the build to the Codex API. It was a constant that
// stayed at 0.1.15 through four releases.
func codexUserAgent() string {
	v := strings.TrimSpace(version)
	if v == "" {
		v = "dev"
	}
	return "codex-client/" + v
}
