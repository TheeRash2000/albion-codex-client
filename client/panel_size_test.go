package client

import "testing"

func TestPanelSize(t *testing.T) {
	cases := []struct {
		name             string
		savedW, savedH   int
		scale            float64
		screenW, screenH int
		wantW, wantH     int
	}{
		{"first run opens wide", 0, 0, 1, 1920, 1080, 1180, 700},
		{"default follows display scaling", 0, 0, 1.5, 2880, 1620, 1770, 1050},
		{"saved size is kept as it is", 900, 700, 1.5, 2880, 1620, 900, 700},
		{"half a saved size is no saved size", 900, 0, 1, 1920, 1080, 1180, 700},
		{"never larger than the screen", 0, 0, 1, 1024, 600, 1024, 600},
		{"a saved size from a bigger screen is cut down", 2400, 1300, 1, 1366, 768, 1366, 768},
		{"never smaller than the minimum", 120, 90, 1, 1920, 1080, 400, 420},
		{"unknown screen does not shrink it", 0, 0, 1, 0, 0, 1180, 700},
		{"a nonsense scale is treated as 100%", 0, 0, 0, 1920, 1080, 1180, 700},
	}
	for _, c := range cases {
		w, h := panelSize(c.savedW, c.savedH, c.scale, c.screenW, c.screenH)
		if w != c.wantW || h != c.wantH {
			t.Errorf("%s: got %dx%d, want %dx%d", c.name, w, h, c.wantW, c.wantH)
		}
	}
}
