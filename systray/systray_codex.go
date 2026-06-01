package systray

import "github.com/ao-data/albiondata-client/client"

// codexStatusTitle is the tray status line shown across platforms.
func codexStatusTitle() string {
	if client.CodexLinked() {
		return "Status: linked to Codex"
	}
	return "Status: not linked"
}
