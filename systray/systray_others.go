//go:build linux

package systray

// The Linux client is the headless CLI: there is no tray here, and there never
// has been - Run() has always been a no-op on this platform.
//
// Deliberately NOT wired to getlantern/systray. That package needs cgo plus
// GTK/AppIndicator development headers, which the documented bare-machine Linux
// build (scripts/build-codex-linux.sh, which needs only libpcap) does not have.
// Importing it here would trade a working CLI build for a tray nobody asked for.
//
// Status for Linux users comes from the log, the same as before.

// Run is a no-op. version is accepted so the call site in main() is identical
// across platforms.
func Run(version string) {}
