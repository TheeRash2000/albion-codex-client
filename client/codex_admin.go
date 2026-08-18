package client

import (
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/ao-data/albiondata-client/log"
)

// Admin / diagnostic mode (-admin or CODEX_ADMIN). The client behaves normally
// (still captures + uploads) but ALSO always records the raw session to a rotating
// .gob, so any capture problem can be replayed offline and diagnosed later without
// having to reproduce it live. The diagnostic packet dump (codex_diag.go) stays
// available via CODEX_DIAG and is what gets run against these recordings. Inert
// when admin mode is off.

const adminKeepRecordings = 10

// adminMode reports whether admin/diagnostic mode is on.
func adminMode() bool {
	return ConfigGlobal.AdminMode
}

// adminRecordingsDir is where always-on recordings are written: under ProgramData
// (outside OneDrive, machine-wide), matching where the device token lives.
func adminRecordingsDir() string {
	base := os.Getenv("ProgramData")
	if base == "" {
		base = os.TempDir()
	}
	return filepath.Join(base, "CodexClient", "recordings")
}

// setupAdminRecording, when admin mode is on and no explicit -record was passed,
// points RecordPath at a fresh timestamped file and prunes old ones. Must run
// before the router starts (the router opens RecordPath when it begins).
func setupAdminRecording() {
	if !adminMode() || ConfigGlobal.RecordPath != "" {
		return
	}
	dir := adminRecordingsDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		log.Errorf("Admin: could not create recordings dir %s: %v", dir, err)
		return
	}
	pruneOldRecordings(dir, adminKeepRecordings)
	name := "codex-" + time.Now().Format("20060102-150405") + ".gob"
	ConfigGlobal.RecordPath = filepath.Join(dir, name)
	log.Infof("ADMIN MODE: recording this session to %s", ConfigGlobal.RecordPath)
}

// pruneOldRecordings keeps the newest `keep` .gob files in dir and deletes the
// rest, so always-on recording never fills the disk.
func pruneOldRecordings(dir string, keep int) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	type rec struct {
		path string
		mod  time.Time
	}
	var recs []rec
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".gob" {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		recs = append(recs, rec{filepath.Join(dir, e.Name()), info.ModTime()})
	}
	if len(recs) <= keep {
		return
	}
	sort.Slice(recs, func(i, j int) bool { return recs[i].mod.After(recs[j].mod) })
	for _, r := range recs[keep:] {
		if err := os.Remove(r.path); err == nil {
			log.Debugf("Admin: pruned old recording %s", r.path)
		}
	}
}
