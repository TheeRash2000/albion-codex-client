//go:build windows

package client

import (
	"runtime"
	"sync"
	"unsafe"

	"github.com/gonutz/w32"
	"github.com/jchv/go-webview2"

	"github.com/ao-data/albiondata-client/icon"
	"github.com/ao-data/albiondata-client/log"
)

// The panel window.
//
// This is the piece that makes a web UI safe to ship. The old panel shelled out
// to `msedge.exe --app=<url>` with cmd.Start() and no Wait(), from two call
// sites, and nothing tracked the result - so every "Open Control Panel" click
// and every duplicate launch left another browser process running against the
// same client, each polling /api/status every 2s, on a process that autostarts
// at login and is meant to run for days.
//
// A WebView2 window is owned. One exists at a time, it is reused rather than
// respawned, and the renderer dies with it. Measured on a real machine: ~130 MB
// while open, nothing when closed, against ~130 MB per orphan forever before.
//
// go-webview2 is pure Go (COM through syscalls), so this adds no cgo on top of
// the cgo Npcap already requires, and vendors 677 KB rather than a GUI toolkit.

const (
	panelWidth  = 480
	panelHeight = 800

	// dwmUseImmersiveDarkMode makes Windows paint the window's own frame dark.
	// Only matters for the resize border here, since the caption is gone, but a
	// light hairline around a near-black page is very visible without it.
	dwmUseImmersiveDarkMode = 20
	// dwmWindowCornerPreference + round: Windows 11 gives rounded corners to a
	// normal window automatically but NOT to the popup style used below, so a
	// frameless window looks like a hard-edged rectangle unless asked.
	dwmWindowCornerPreference = 33
	dwmCornerRound            = 2
)

var (
	panelWinMu sync.Mutex
	panelWin   webview2.WebView // non-nil while a window is open
	panelHWND  w32.HWND
)

// OpenPanel opens the panel window, or focuses it if already open.
//
// Safe to call from the tray goroutine. The window runs on its OWN locked OS
// thread because WebView2 is COM apartment-threaded: its message loop must stay
// on the thread that created it, and Go will otherwise move a goroutine between
// threads mid-flight.
func OpenPanel() {
	if panelURL == "" {
		log.Error("Panel: cannot open, the local server did not start.")
		return
	}

	// Opening the panel is the moment the account figures are about to be looked
	// at, so this is where a stale-cache refresh belongs. Single-flighted and
	// backgrounded, so it never delays the window appearing.
	RefreshClientStatsIfStale()

	panelWinMu.Lock()
	if panelWin != nil {
		hwnd := panelHWND
		panelWinMu.Unlock()
		focusWindow(hwnd)
		return
	}
	panelWinMu.Unlock()

	go runPanelWindow(panelURL)
}

func runPanelWindow(url string) {
	// Pin this goroutine to one OS thread for the window's whole lifetime.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	w := webview2.NewWithOptions(webview2.WebViewOptions{
		Debug: false,
		WindowOptions: webview2.WindowOptions{
			Title:  "Codex Client",
			Width:  panelWidth,
			Height: panelHeight,
			Center: true,
		},
	})
	if w == nil {
		// Almost always a missing WebView2 runtime. It ships with Windows 11 and
		// with Edge on Windows 10, so this is rare - but capture must continue
		// regardless, which is why this logs instead of exiting.
		log.Error("Panel: could not create the window (is the WebView2 runtime installed?). Capture is unaffected; use the tray menu.")
		return
	}

	hwnd := w32.HWND(uintptr(unsafe.Pointer(w.Window())))

	panelWinMu.Lock()
	panelWin = w
	panelHWND = hwnd
	panelWinMu.Unlock()

	makeFrameless(hwnd)
	setWindowIcon(hwnd)
	bindWindowControls(w, hwnd)

	w.Navigate(url)
	w.Run() // blocks until the window is closed

	// Run() has returned, so the window is gone. Clear the handle BEFORE
	// destroying, so a click arriving in this gap opens a fresh window rather
	// than trying to focus a destroyed one.
	panelWinMu.Lock()
	panelWin = nil
	panelHWND = 0
	panelWinMu.Unlock()

	w.Destroy()
}

// makeFrameless strips the caption so the page draws its own title bar.
//
// WS_THICKFRAME is deliberately KEPT. It is what gives the window its resize
// borders; dropping it along with the caption produces a window the user cannot
// resize at all, which is a worse outcome than a visible title bar. With no
// caption the frame itself is invisible, so the borders are felt rather than
// seen.
func makeFrameless(hwnd w32.HWND) {
	if hwnd == 0 {
		return
	}

	style := uint32(w32.GetWindowLong(hwnd, w32.GWL_STYLE))
	style &^= w32.WS_CAPTION
	style |= w32.WS_THICKFRAME
	w32.SetWindowLong(hwnd, w32.GWL_STYLE, style)

	dark := int32(1)
	w32.DwmSetWindowAttribute(hwnd, dwmUseImmersiveDarkMode, w32.LPCVOID(unsafe.Pointer(&dark)), 4)
	corner := int32(dwmCornerRound)
	w32.DwmSetWindowAttribute(hwnd, dwmWindowCornerPreference, w32.LPCVOID(unsafe.Pointer(&corner)), 4)

	// SWP_FRAMECHANGED forces the non-client area to be recalculated. Without
	// it the old caption keeps being painted until something else resizes the
	// window, so the change appears not to have worked.
	w32.SetWindowPos(hwnd, 0, 0, 0, 0, 0,
		uint(w32.SWP_FRAMECHANGED|w32.SWP_NOMOVE|w32.SWP_NOSIZE|w32.SWP_NOZORDER|w32.SWP_NOACTIVATE))
}

var (
	panelIconOnce  sync.Once
	panelIconSmall w32.HICON
	panelIconBig   w32.HICON
)

// setWindowIcon gives the panel window the Codex mark in the taskbar and
// Alt+Tab.
//
// go-webview2 registers its window class with IDI_APPLICATION (the generic
// system icon) unless told a resource id, and the page favicon does not reach
// the host window at all. So the icon is set here with WM_SETICON, built from
// the .ico bytes embedded in the binary rather than from the exe's resource
// section, which keeps it correct even in a plain `go build` with no .syso.
//
// The HICONs are created once and kept for the life of the process: the window
// is reused, and a reopened one gets the same handles.
func setWindowIcon(hwnd w32.HWND) {
	if hwnd == 0 {
		return
	}
	panelIconOnce.Do(func() {
		panelIconSmall = iconFromICO(w32.GetSystemMetrics(w32.SM_CXSMICON))
		panelIconBig = iconFromICO(w32.GetSystemMetrics(w32.SM_CXICON))
	})
	if panelIconSmall != 0 {
		w32.SendMessage(hwnd, w32.WM_SETICON, w32.ICON_SMALL, uintptr(panelIconSmall))
	}
	if panelIconBig != 0 {
		w32.SendMessage(hwnd, w32.WM_SETICON, w32.ICON_BIG, uintptr(panelIconBig))
	}
	if panelIconSmall == 0 && panelIconBig == 0 {
		log.Debug("Panel: could not build the window icon from the embedded .ico.")
	}
}

// iconFromICO builds an HICON of the given pixel size from the embedded .ico.
func iconFromICO(size int) w32.HICON {
	if size <= 0 {
		size = 32
	}
	img, _, err := icon.ICOImage(icon.ICO, size)
	if err != nil {
		return 0
	}
	// 0x00030000 is the icon format version CreateIconFromResourceEx requires.
	return w32.CreateIconFromResourceEx(unsafe.Pointer(&img[0]), uint32(len(img)), true, 0x00030000, size, size, w32.LR_DEFAULTCOLOR)
}

// bindWindowControls exposes drag, minimise and close to the page.
//
// WebView2 has no equivalent of Electron's `-webkit-app-region: drag`, so a
// frameless window cannot be moved without help from the host. The standard
// approach is to tell Windows the click actually landed on a caption:
// ReleaseCapture, then post WM_NCLBUTTONDOWN with HTCAPTION, and Windows runs
// its own drag loop from there. That gives real OS behaviour - snap, aero
// shake, multi-monitor - rather than a hand-rolled imitation that moves the
// window by mouse deltas.
//
// PostMessage rather than SendMessage: the drag loop is modal and blocks until
// the mouse is released, and SendMessage would hold the WebView2 callback (and
// therefore the renderer) for the whole drag.
func bindWindowControls(w webview2.WebView, hwnd w32.HWND) {
	_ = w.Bind("codexDragWindow", func() {
		w32.ReleaseCapture()
		w32.PostMessage(hwnd, w32.WM_NCLBUTTONDOWN, w32.HTCAPTION, 0)
	})
	_ = w.Bind("codexMinimize", func() {
		w32.ShowWindow(hwnd, w32.SW_MINIMIZE)
	})
	// Close HIDES rather than destroys. The client keeps capturing with the
	// window shut, and hiding means reopening from the tray is instant and keeps
	// scroll position, instead of paying WebView2 startup again. The renderer is
	// still released when the process exits or the user quits from the tray.
	_ = w.Bind("codexClose", func() {
		w32.ShowWindow(hwnd, w32.SW_HIDE)
	})
}

// focusWindow restores and raises an existing panel window.
//
// SW_RESTORE unconditionally, before raising: a minimised or hidden window
// ignores SetForegroundWindow, so without it, clicking "Open Codex Client"
// while the window sat minimised would appear to do nothing - the exact
// confusion that made people relaunch the app and spawn duplicates. On a window
// that is already visible SW_RESTORE just activates it, so this is safe either
// way and avoids querying the window state first.
func focusWindow(hwnd w32.HWND) {
	if hwnd == 0 {
		return
	}
	w32.ShowWindow(hwnd, w32.SW_RESTORE)
	w32.SetForegroundWindow(hwnd)
}

// PanelWindowOpen reports whether a panel window currently exists. The tray
// reads it to label its menu item.
func PanelWindowOpen() bool {
	panelWinMu.Lock()
	defer panelWinMu.Unlock()
	return panelWin != nil
}
