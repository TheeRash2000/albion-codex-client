//go:build windows || darwin

package systray

import (
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/ao-data/albiondata-client/client"
	"github.com/ao-data/albiondata-client/icon"
	"github.com/ao-data/albiondata-client/log"
	"github.com/ao-data/albiondata-client/notification"
	"github.com/getlantern/systray"
)

// The Codex Client's UI.
//
// This replaces the local web control panel, which served an HTML page to a
// chromeless Edge/Chrome window. That design cost a browser process per open,
// and because "Open Control Panel" and every duplicate launch both shelled out
// to a browser with no process tracking, those accumulated across a session that
// is meant to run for days.
//
// The tray carries the same information the panel did. It is not a compromise
// for the sake of dropping the browser: every element of that panel was a short
// status line, a counter, a toggle or a button, and a menu renders all four
// natively at no standing cost.
//
// Not built on Linux: the tray there needs GTK/AppIndicator development headers
// the documented bare-machine build does not have, and Run() has always been a
// no-op stub on that platform (the Linux client is the headless CLI).

// How many rows each detail submenu holds.
//
// Fixed slots, not dynamic rebuilds: getlantern/systray has no API to REMOVE a
// menu item, so a feed rendered by appending would grow the menu for as long as
// the client runs - the same unbounded-append shape that makes a long-lived UI
// leak in the first place. Slots are allocated once and reused, with the surplus
// hidden. The caps sit at or below the source feeds' own caps in
// client/status.go (40 activity, 30 issues), so the UI can never be the thing
// that grows.
const (
	healthSlots   = 8
	activitySlots = 10
	issueSlots    = 8
)

// refreshInterval is how often the menu re-reads the client's status. The old
// panel polled its HTTP API every 2s from JavaScript; this is the same cadence,
// but it is a struct copy and a handful of changed-only title writes, with no
// listener, no socket and no renderer.
const refreshInterval = 2 * time.Second

// maxRowText stops a long activity or error line stretching the menu across the
// screen. The full text is still in the log.
const maxRowText = 68

// label is a menu item plus the last text written to it.
//
// Every SetTitle is a menu-update syscall and the refresh loop runs for the life
// of the process, so writes happen only when the text actually changed. An idle
// client therefore does no menu work at all between ticks.
type label struct {
	item    *systray.MenuItem
	last    string
	visible bool
}

func wrap(item *systray.MenuItem) *label {
	return &label{item: item, visible: true}
}

// info creates a non-clickable status row.
func info(title, tooltip string) *label {
	l := wrap(systray.AddMenuItem(title, tooltip))
	l.item.Disable()
	return l
}

// subInfo creates a non-clickable row inside a submenu.
func subInfo(parent *systray.MenuItem, title string) *label {
	l := wrap(parent.AddSubMenuItem(title, ""))
	l.item.Disable()
	return l
}

func (l *label) set(text string) {
	if text == l.last {
		return
	}
	l.item.SetTitle(text)
	l.last = text
}

func (l *label) show(v bool) {
	if v == l.visible {
		return
	}
	if v {
		l.item.Show()
	} else {
		l.item.Hide()
	}
	l.visible = v
}

// setChecked mirrors a toggle's real state.
func (l *label) setChecked(on bool) {
	if on == l.item.Checked() {
		return
	}
	if on {
		l.item.Check()
	} else {
		l.item.Uncheck()
	}
}

type trayUI struct {
	status    *label
	character *label
	game      *label
	board     *label
	queue     *label

	healthParent   *systray.MenuItem
	health         []*label
	activityParent *systray.MenuItem
	activity       []*label
	issuesParent   *systray.MenuItem
	issues         []*label

	market *label
	aodp   *label

	panel   *systray.MenuItem
	link    *label
	account *systray.MenuItem
	npcap   *label

	memory  *label
	heap    *systray.MenuItem
	console *label
	quit    *systray.MenuItem

	// pairCode is the code from the running handshake, so clicking the item again
	// re-copies it rather than abandoning that handshake and starting another.
	pairCode string
}

func buildMenu(version string) *trayUI {
	u := &trayUI{}

	u.status = info("Status: starting...", "Whether this PC is linked to your Codex account")
	u.character = info("", "The character currently being captured")
	systray.AddSeparator()

	u.game = info("Seeing your game: checking...", "Whether Albion packets from your session are being seen")
	u.board = info("Destiny Board: not captured yet", "Your last Destiny Board upload")
	u.queue = info("", "Uploads waiting to be retried")
	systray.AddSeparator()

	u.healthParent = systray.AddMenuItem("Capture health", "Per-stream upload counts")
	for i := 0; i < healthSlots; i++ {
		u.health = append(u.health, subInfo(u.healthParent, ""))
	}
	u.activityParent = systray.AddMenuItem("Recent activity", "What has been captured and sent")
	for i := 0; i < activitySlots; i++ {
		u.activity = append(u.activity, subInfo(u.activityParent, ""))
	}
	u.issuesParent = systray.AddMenuItem("Issues", "Problems with data that did not reach Codex")
	for i := 0; i < issueSlots; i++ {
		u.issues = append(u.issues, subInfo(u.issuesParent, ""))
	}
	systray.AddSeparator()

	u.market = wrap(systray.AddMenuItemCheckbox(
		"Upload market data",
		"Contribute prices you scan in-game to your Codex network",
		client.MarketUploadEnabled(),
	))
	u.aodp = wrap(systray.AddMenuItemCheckbox(
		"Contribute to AODP",
		"Also send captured market data to the Albion Online Data Project",
		client.AODPUploadEnabled(),
	))
	systray.AddSeparator()

	u.panel = systray.AddMenuItem("Open Codex Client", "Open the panel window")
	u.link = wrap(systray.AddMenuItem("Link this PC", "Link this PC to your Codex account"))
	u.account = systray.AddMenuItem("Manage devices on Codex", "Open your Codex account page in the browser")
	u.npcap = wrap(systray.AddMenuItem("Install Npcap (required)", "Capture cannot run without the Npcap driver"))
	u.npcap.show(false)
	systray.AddSeparator()

	u.memory = info("Memory: measuring...", "Memory in use by the client")
	u.heap = systray.AddMenuItem("Save memory report", "Write a heap profile to disk for diagnosing memory growth")
	u.console = wrap(systray.AddMenuItem(consoleActionTitle(), "Show or hide the console window"))
	u.console.show(canHideConsole)
	info("Version "+version, "The build this client is running")
	u.quit = systray.AddMenuItem("Quit", "Close the Codex Client")

	return u
}

// refresh re-renders every live row from one status snapshot.
func (u *trayUI) refresh() {
	s := client.UIStatus()

	u.status.set(statusLine(s))
	u.character.set("Character: " + s.Character)
	u.character.show(s.Character != "")

	if s.Tracking {
		u.game.set("Seeing your game: yes")
	} else {
		u.game.set("Seeing your game: no - is Albion running?")
	}

	u.board.set(boardLine(s))

	u.queue.set("Pending delivery: " + commas(s.QueuedUploads) + " queued (retrying)")
	u.queue.show(s.QueuedUploads > 0)

	u.fillHealth(s)
	u.fillActivity(s)
	u.fillIssues(s)

	u.market.setChecked(s.MarketOn)
	u.aodp.setChecked(s.AodpUpload)

	u.pairCode = s.PairCode
	u.link.set(linkLine(s))
	u.npcap.show(s.NpcapMissing)

	u.memory.set(client.MemoryLine())
	u.console.set(consoleActionTitle())
}

func (u *trayUI) fillHealth(s client.StatusSnapshot) {
	u.healthParent.SetTitle(countedTitle("Capture health", len(s.Health)))
	if len(s.Health) == 0 {
		u.health[0].set("Nothing captured yet")
		u.health[0].show(true)
		for _, slot := range u.health[1:] {
			slot.show(false)
		}
		return
	}
	for i, slot := range u.health {
		if i >= len(s.Health) {
			slot.show(false)
			continue
		}
		h := s.Health[i]
		row := h.Name + ": " + commas(h.Sent) + " sent"
		if h.Failed > 0 {
			row += ", " + commas(h.Failed) + " failed"
		}
		if h.LastSent != "" {
			row += " (" + h.LastSent + ")"
		}
		slot.set(truncate(row))
		slot.show(true)
	}
}

func (u *trayUI) fillActivity(s client.StatusSnapshot) {
	if len(s.Activity) == 0 {
		u.activity[0].set("No activity yet - log in and change zones")
		u.activity[0].show(true)
		for _, slot := range u.activity[1:] {
			slot.show(false)
		}
		return
	}
	for i, slot := range u.activity {
		if i >= len(s.Activity) {
			slot.show(false)
			continue
		}
		a := s.Activity[i]
		slot.set(truncate(a.Msg + " - " + a.Ago))
		slot.show(true)
	}
}

func (u *trayUI) fillIssues(s client.StatusSnapshot) {
	u.issuesParent.SetTitle(countedTitle("Issues", len(s.Issues)))
	if len(s.Issues) == 0 {
		u.issues[0].set("No issues")
		u.issues[0].show(true)
		for _, slot := range u.issues[1:] {
			slot.show(false)
		}
		return
	}
	for i, slot := range u.issues {
		if i >= len(s.Issues) {
			slot.show(false)
			continue
		}
		is := s.Issues[i]
		slot.set(truncate(is.Msg + " - " + is.Ago))
		slot.show(true)
	}
}

// loop handles clicks and drives the refresh tick. Runs for the life of the app.
func (u *trayUI) loop() {
	u.refresh()
	tick := time.NewTicker(refreshInterval)
	defer tick.Stop()

	for {
		select {
		case <-tick.C:
			u.refresh()

		case <-u.market.item.ClickedCh:
			u.toggleMarket()

		case <-u.aodp.item.ClickedCh:
			u.toggleAODP()

		case <-u.panel.ClickedCh:
			client.OpenPanel()

		case <-u.link.item.ClickedCh:
			u.startLinking()

		case <-u.account.ClickedCh:
			client.OpenCodexAccount()

		case <-u.npcap.item.ClickedCh:
			client.OpenNpcapDownload()

		case <-u.heap.ClickedCh:
			u.saveHeapProfile()

		case <-u.console.item.ClickedCh:
			toggleConsole()
			u.console.set(consoleActionTitle())

		case <-u.quit.ClickedCh:
			log.Info("Quit requested from the tray.")
			systray.Quit()
			os.Exit(0)
		}
	}
}

// toggleMarket flips Codex market capture. The checkbox is set from the RESULT,
// never optimistically: if persistence failed the setting is live but will not
// survive a restart, and the player is told rather than left with a tick that
// silently lies after the next launch.
func (u *trayUI) toggleMarket() {
	on := !client.MarketUploadEnabled()
	if err := client.SetMarketUpload(on); err != nil {
		log.Errorf("Could not save the market-upload setting: %v", err)
		notification.Push("Market upload changed for this session, but the choice could not be saved.")
	}
	u.market.setChecked(client.MarketUploadEnabled())
}

func (u *trayUI) toggleAODP() {
	on := !client.AODPUploadEnabled()
	if err := client.SetAODPUpload(on); err != nil {
		log.Errorf("Could not save the AODP setting: %v", err)
		notification.Push("AODP contribution changed for this session, but the choice could not be saved.")
	}
	u.aodp.setChecked(client.AODPUploadEnabled())
}

// startLinking begins pairing, or re-copies the code when a handshake is already
// running - clicking a menu item that is showing a code should hand you the code,
// not throw away the handshake and start another.
func (u *trayUI) startLinking() {
	if code := u.pairCode; code != "" && client.UIStatus().Pairing {
		client.CopyPairingCode(code)
		return
	}

	code, _, copied, err := client.StartLinking()
	if err != nil {
		log.Errorf("Could not start linking: %v", err)
		notification.Push("Could not reach Codex to link this PC. Check your connection and try again.")
		return
	}
	if copied {
		notification.Push("Pairing code " + code + " copied. Paste it into the browser window that just opened.")
	} else {
		notification.Push("Pairing code " + code + " - type it into the browser window that just opened.")
	}
	u.refresh()
}

func (u *trayUI) saveHeapProfile() {
	path, err := client.WriteHeapProfile()
	if err != nil {
		log.Errorf("Could not write the heap profile: %v", err)
		notification.Push("Could not write the memory report.")
		return
	}
	notification.Push("Memory report saved to " + path)
}

// ---------------------------------------------------------------------------
// Rendering helpers
// ---------------------------------------------------------------------------

func statusLine(s client.StatusSnapshot) string {
	switch {
	case s.Pairing && s.PairCode != "":
		return "Status: linking this PC - code " + s.PairCode
	case s.NpcapMissing:
		return "Status: Npcap missing - capture is off"
	case s.Linked && s.TokenRejected:
		return "Status: link broken - re-link this PC"
	case s.Linked:
		return "Status: linked to Codex"
	default:
		return "Status: not linked"
	}
}

func boardLine(s client.StatusSnapshot) string {
	switch {
	case s.HasUploaded:
		return "Destiny Board: " + commas(s.LastUploadNodes) + " nodes, " + s.LastUploadAgo
	case !s.Linked:
		return "Destiny Board: link this PC first"
	default:
		return "Destiny Board: change zones once to send it"
	}
}

// linkLine offers a re-link even when a token already exists. The panel only
// showed a Link button when there was NO token, which is exactly why a revoked
// device could not recover from the UI: the client still had a token, so the
// only affordance for fixing it was hidden.
func linkLine(s client.StatusSnapshot) string {
	switch {
	case s.Pairing && s.PairCode != "":
		return "Pairing code " + s.PairCode + " - click to copy"
	case s.TokenRejected:
		return "Re-link this PC"
	case s.Linked:
		return "Re-link this PC"
	default:
		return "Link this PC"
	}
}

// countedTitle appends a count so a collapsed submenu still says how much is
// inside it. The panel showed these as always-open lists; a submenu that hides
// its own contents would otherwise be easy to ignore.
func countedTitle(base string, n int) string {
	if n == 0 {
		return base
	}
	return base + " (" + commas(n) + ")"
}

func truncate(s string) string {
	if len(s) <= maxRowText {
		return s
	}
	return strings.TrimSpace(s[:maxRowText-3]) + "..."
}

// commas formats an int with thousands separators. Upload counts reach six
// figures in a long session and are hard to read without them.
func commas(n int) string {
	s := strconv.Itoa(n)
	neg := strings.HasPrefix(s, "-")
	if neg {
		s = s[1:]
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteString(",")
		}
		b.WriteByte(s[i])
	}
	if neg {
		return "-" + b.String()
	}
	return b.String()
}

// onReady is the shared systray entry point for the platforms that have a tray.
func onReady(version string) {
	if client.ConfigGlobal.Minimize && canHideConsole {
		hideConsole()
	}
	systray.SetIcon(icon.Data)
	systray.SetTitle(trayTitle)
	systray.SetTooltip("Codex Client - capture for albioncodex.com")

	ui := buildMenu(version)
	go ui.loop()
}

func onExit() {}
