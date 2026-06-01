//go:build windows

package systray

import (
	"fmt"
	"os"

	"github.com/ao-data/albiondata-client/client"

	"github.com/ao-data/albiondata-client/icon"
	"github.com/getlantern/systray"
	"github.com/gonutz/w32"
)

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

func GetActionTitle() string {
	if consoleHidden {
		return "Show Console"
	} else {
		return "Hide Console"
	}
}

func Run() {
	systray.Run(onReady, onExit)
}

func onExit() {

}

func onReady() {
	// Don't hide the console automatically
	// Unless started from the scheduled task or with the parameter
	// People think it is crashing
	if client.ConfigGlobal.Minimize {
		hideConsole()
	}
	systray.SetIcon(icon.Data)
	systray.SetTitle("Codex Client")
	systray.SetTooltip("Codex Client - destiny board capture for albioncodex.com")

	mStatus := systray.AddMenuItem(codexStatusTitle(), "Whether this PC is linked to your Codex account")
	mStatus.Disable()
	systray.AddSeparator()

	mPanel := systray.AddMenuItem("Open Control Panel", "Open the Codex Client window")
	mAccount := systray.AddMenuItem("Manage devices on Codex", "Open your Codex account page in the browser")
	systray.AddSeparator()

	mConHideShow := systray.AddMenuItem(GetActionTitle(), "Show/Hide Console")
	mQuit := systray.AddMenuItem("Quit", "Close the Codex Client")

	for {
		select {
		case <-mQuit.ClickedCh:
			fmt.Println("Requesting quit")
			systray.Quit()
			os.Exit(0)

		case <-mPanel.ClickedCh:
			client.OpenPanel()
			mStatus.SetTitle(codexStatusTitle())

		case <-mAccount.ClickedCh:
			client.OpenBrowser(client.CodexAccountURL())

		case <-mConHideShow.ClickedCh:
			if consoleHidden {
				showConsole()
				mConHideShow.SetTitle(GetActionTitle())
			} else {
				hideConsole()
				mConHideShow.SetTitle(GetActionTitle())
			}
		}
	}
}
