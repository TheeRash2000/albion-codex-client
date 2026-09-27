//go:build darwin

package systray

import (
	"github.com/getlantern/systray"
)

// trayTitle is empty on macOS: the icon sits in the menu bar and a text title
// beside it just eats horizontal space.
const trayTitle = ""

// canHideConsole reports whether this platform can show/hide the console window.
// macOS has no equivalent, so the tray hides that menu entry entirely.
const canHideConsole = false

func hideConsole() {}

func showConsole() {}

func toggleConsole() {}

func consoleActionTitle() string { return "" }

// Run starts the tray and blocks. On macOS this must be called on the main
// thread (see main()), which is why the client runs in the goroutine there and
// the tray does not.
func Run(version string) {
	systray.Run(func() { onReady(version) }, onExit)
}
