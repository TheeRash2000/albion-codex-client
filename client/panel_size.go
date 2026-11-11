package client

// The panel window's size.
//
// Wide by default so the cards sit side by side: three columns at the default
// width, and tall enough that a full Most wanted list still leaves the activity
// feed several lines, with nothing below the fold on a typical account. The player can
// resize it, and the size they leave it at is what it opens with next time.

const (
	// Default size, in pixels at 100% display scaling.
	panelDefaultWidth  = 1180
	panelDefaultHeight = 780

	// Smaller than this the single-column layout stops being readable.
	panelMinWidth  = 400
	panelMinHeight = 420
)

// panelSize decides the size to open the window at. saved is the size the
// player last left it at (0,0 when never resized), scale is the display scaling
// (1.0 at 100%), and screen is the size of the primary display (0,0 when
// unknown). A saved size is used as it is; the default is scaled. Either way
// the result is at least the minimum and never larger than the screen.
func panelSize(savedW, savedH int, scale float64, screenW, screenH int) (int, int) {
	if scale < 1 {
		scale = 1
	}
	w, h := savedW, savedH
	if w <= 0 || h <= 0 {
		w = int(float64(panelDefaultWidth) * scale)
		h = int(float64(panelDefaultHeight) * scale)
	}
	minW, minH := panelMinSize(scale)
	if w < minW {
		w = minW
	}
	if h < minH {
		h = minH
	}
	if screenW > 0 && w > screenW {
		w = screenW
	}
	if screenH > 0 && h > screenH {
		h = screenH
	}
	return w, h
}

// panelMinSize is the smallest the window may be dragged to.
func panelMinSize(scale float64) (int, int) {
	if scale < 1 {
		scale = 1
	}
	return int(float64(panelMinWidth) * scale), int(float64(panelMinHeight) * scale)
}

// rememberPanelSize saves the size the player left the window at. Ignores
// sizes that are not a real window (a minimised window reports almost nothing).
func rememberPanelSize(w, h int) {
	if w < panelMinWidth || h < panelMinHeight {
		return
	}
	s := LoadCodexSettings()
	if s.PanelWidth == w && s.PanelHeight == h {
		return
	}
	s.PanelWidth, s.PanelHeight = w, h
	_ = SaveCodexSettings(s)
}
