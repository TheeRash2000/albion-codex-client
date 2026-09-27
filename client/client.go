package client

import (
	"os"
	"time"

	"github.com/ao-data/albiondata-client/log"
	"github.com/ao-data/albiondata-client/notification"
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

	if os.Getenv("CODEX_ADMIN") != "" {
		ConfigGlobal.AdminMode = true
	}
	if adminMode() {
		log.Info("ADMIN MODE ON: this session is always recorded (replayable for diagnosis); normal capture is unchanged.")
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
		// Single instance: only one client may run at a time.
		//
		// This used to reopen the running instance's browser window from a saved
		// URL, which meant every duplicate launch spawned ANOTHER Edge/Chrome app
		// process against the same client. With the tray there is nothing to
		// reopen - the icon is already there - so a duplicate launch tells the
		// player where to look and exits.
		if !acquireSingleInstance() {
			log.Info("Codex Client is already running; look for the Codex icon in the system tray.")
			notification.Push("Codex Client is already running - look for the icon in your system tray.")
			return nil
		}

		// Launch at Windows login so it is running before the player logs into
		// Albion (the game only sends the board/market data while capturing).
		ensureAutostart()

		// Start the Codex upload delivery queue (retry with backoff) before any
		// capture path enqueues to it. Idempotent.
		startCodexUploadQueue()

		// Tell the site which build this device runs (admin device roster). Paired-
		// only, fire-and-forget; placed after the single-instance guard so only the
		// live instance reports, not a duplicate launch.
		startVersionReporter(version)

		// Keep the client up to date: check the published manifest in the background
		// and self-update (checksum-verified) when a newer build ships, then restart.
		startCodexUpdater(version)

		// The local panel server. The window itself is opened from the tray, or
		// now if -panel was passed, or automatically when the client has never
		// been linked - a first-time user has no reason to know the tray icon is
		// where linking lives, and the old panel opened itself for that reason.
		StartPanel()
		if ConfigGlobal.CodexOpenPanel || !CodexLinked() {
			OpenPanel()
		}

		// Capture needs the Npcap driver. Without it pcap.OpenLive panics, so if
		// it is missing we surface that in the tray and hold here rather than start
		// capture. (The exe still runs - gopacket loads wpcap.dll at runtime.)
		if !npcapInstalled() {
			Status.setNpcapMissing(true)
			log.Error("Npcap is not installed. Install it from the Codex Client tray menu, then restart the app.")
			select {} // keep the tray alive; Quit from the tray exits
		}

		// Admin mode: point recording at a fresh rotating file before capture starts.
		setupAdminRecording()

		apw := newAlbionProcessWatcher()
		return apw.run()
	}
	return nil
}
