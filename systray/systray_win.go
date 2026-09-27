//go:build windows

package systray

import (
	"github.com/getlantern/systray"
	"github.com/gonutz/w32"
)

// trayTitle is shown next to the icon where the platform supports it. Windows
// tray icons show only the tooltip, so this is effectively cosmetic here.
const trayTitle = "Codex Client"

// canHideConsole reports whether this platform can show/hide the console window.
const canHideConsole = true

var consoleHidden bool

func hideConsole() {
	console := w32.GetConsoleWindow()
	if console != 0 {
		_, consoleProcID := w32.GetWindowThreadProcessId(console)
		if w32.GetCurrentProcessId() == consoleProcID {
			w32.ShowWindowAsync(console, w32.SW_HIDE)
		}
	}
	consoleHidden = true
}

func showConsole() {
	console := w32.GetConsoleWindow()
	if console != 0 {
		_, consoleProcID := w32.GetWindowThreadProcessId(console)
		if w32.GetCurrentProcessId() == consoleProcID {
			w32.ShowWindowAsync(console, w32.SW_SHOW)
		}
	}
	consoleHidden = false
}

func toggleConsole() {
	if consoleHidden {
		showConsole()
		return
	}
	hideConsole()
}

func consoleActionTitle() string {
	if consoleHidden {
		return "Show console"
	}
	return "Hide console"
}

// Run starts the tray and blocks. version is passed in rather than read from the
// client package because on every platform except macOS the tray starts BEFORE
// the client does, so a package-level version variable would still be empty.
func Run(version string) {
	systray.Run(func() { onReady(version) }, onExit)
}
