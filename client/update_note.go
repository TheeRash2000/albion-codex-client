package client

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ao-data/albiondata-client/log"
)

// A self-update is silent: the client replaces itself and restarts. So that
// players learn what changed, the first start of a new build shows the release's
// note (the published manifest's "notes") once in the activity feed.
//
// The NEW build works out for itself that it has just been updated. It cannot
// rely on the old build to leave a message: builds up to 0.1.19 have no such
// code, so a note saved by the outgoing version would never reach anyone on
// their first update to a build that has this feature.
//
// How it knows: the settings file records the version that last ran. A different
// version on this start means an update. A build that never recorded one (0.1.19
// and earlier) is told apart from a fresh install by the file the updater leaves
// behind: the old exe, renamed and hidden beside the new one.

// updateNoteMaxRunes bounds the note. It comes from a file on the internet and
// is shown as one feed line.
const updateNoteMaxRunes = 140

// updateNoteDelay lets the panel and capture settle before a network read that
// only decides whether to show one line of text.
const updateNoteDelay = 5 * time.Second

// justUpdated reports whether this start follows an update of this install.
func justUpdated(lastRun, running string, oldBinary bool) bool {
	if parseVersionNums(running) == nil {
		return false // a local or unversioned build: nothing to announce
	}
	if lastRun == running {
		return false
	}
	if lastRun != "" {
		return true
	}
	return oldBinary
}

// updateNoteLine is the feed line for a manifest, or "" for none. The note is
// only shown when the published manifest describes the build that is running:
// if a newer release is already out, its note is not this build's.
func updateNoteLine(running, manifestVersion, notes string) string {
	note := cleanUpdateNote(notes)
	if note == "" || manifestVersion != running {
		return ""
	}
	return "Updated to " + running + " - " + note
}

// cleanUpdateNote makes the note one short line of plain text.
func cleanUpdateNote(note string) string {
	note = strings.Join(strings.Fields(note), " ") // collapses newlines, tabs, runs of spaces
	if r := []rune(note); len(r) > updateNoteMaxRunes {
		note = strings.TrimSpace(string(r[:updateNoteMaxRunes]))
	}
	return note
}

// oldBinaryPath is where the updater leaves the previous exe (see
// minio/selfupdate: "/path/to/.target.old", hidden on Windows).
func oldBinaryPath(exe string) string {
	return filepath.Join(filepath.Dir(exe), "."+filepath.Base(exe)+".old")
}

func oldBinaryPresent() bool {
	exe, err := os.Executable()
	if err != nil {
		return false
	}
	_, err = os.Stat(oldBinaryPath(exe))
	return err == nil
}

// announceUpdate runs once at start, in the background. It records the running
// version and, when this start follows an update, shows the release note.
// Best-effort throughout: a failure costs the player one line of text.
func announceUpdate(running string) {
	if parseVersionNums(running) == nil {
		return
	}
	s := LoadCodexSettings()
	lastRun := s.LastRunVersion
	if lastRun == running {
		return
	}
	updated := justUpdated(lastRun, running, oldBinaryPresent())

	// Record the version first, so the note can never show on a second start
	// even if the read below fails or the client is closed during it.
	s.LastRunVersion = running
	if err := SaveCodexSettings(s); err != nil {
		log.Debugf("Could not record the running version: %v", err)
		return
	}
	if !updated {
		return
	}

	time.Sleep(updateNoteDelay)
	m, err := fetchManifest()
	if err != nil {
		log.Debugf("No release note: %v", err)
		return
	}
	if line := updateNoteLine(running, m.Version, m.Notes); line != "" {
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
