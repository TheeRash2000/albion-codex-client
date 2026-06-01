package client

import (
	"os"
	"time"

	"github.com/ao-data/albiondata-client/log"
)

var version string

// Client struct base
type Client struct {
}

// NewClient return a new Client instance
func NewClient(_version string) *Client {
	version = _version
	return &Client{}
}

// Run starts client settings and run
func (client *Client) Run() error {
	log.Infof("Starting Codex Client, version: %s", version)
	log.Info("This is a third-party application and is in no way affiliated with Sandbox Interactive or Albion Online.")
	log.Info("Additional parameters can be listed by calling this file with the -h parameter.")

	if CodexLinked() {
		log.Info("This PC is linked to Codex. With this running, log in to Albion or travel to another zone and your Destiny Board uploads automatically (opening the board screen alone does not send it).")
	} else {
		log.Info("This PC is NOT linked to Codex yet. Click \"Link\" in the Codex Client window to link it once - after that it stays linked and your Destiny Board uploads on login.")
	}

	if ConfigGlobal.CodexMarketUpload {
		log.Info("Market collection: ON - prices you scan in-game will be contributed to Codex.")
	} else {
		log.Info("Market collection: off (pass --codex-market to contribute scanned prices).")
	}

	ConfigGlobal.setupDebugEvents()
	ConfigGlobal.setupDebugOperations()

	createDispatcher()

	if ConfigGlobal.Offline {
		processOffline(ConfigGlobal.OfflinePath)

		// Allow time for any async uploads/processing to complete, then exit.
		time.Sleep(10 * time.Second)
		os.Exit(0)

	} else {
		// Launch at Windows login so it is running before the player logs into
		// Albion (the game only sends the board/market data while capturing).
		ensureAutostart()

		// The control panel is the client's face: a small window with linked
		// status, what's tracked, last upload, and a Link button.
		StartPanel()

		// Capture needs the Npcap driver. Without it pcap.OpenLive panics, so if
		// it's missing we surface that in the panel and hold here rather than start
		// capture. (The exe still runs - gopacket loads wpcap.dll at runtime.)
		if !npcapInstalled() {
			Status.setNpcapMissing(true)
			log.Error("Npcap is not installed. Install it from the Codex Client window, then restart the app.")
			select {} // keep the panel + tray alive; Quit from the tray exits
		}

		apw := newAlbionProcessWatcher()
		return apw.run()
	}
	return nil
}
