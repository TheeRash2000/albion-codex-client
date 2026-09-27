package icon

import (
	"bytes"
	"testing"
)

func TestICOImagePicksClosestEntry(t *testing.T) {
	// The shipped icon has 16, 32 and 48 px entries.
	cases := []struct{ ask, want int }{
		{16, 16}, {20, 32}, {32, 32}, {40, 48}, {48, 48}, {64, 48}, {8, 16},
	}
	for _, c := range cases {
		img, w, err := ICOImage(ICO, c.ask)
		if err != nil {
			t.Fatalf("size %d: %v", c.ask, err)
		}
		if w != c.want {
			t.Errorf("size %d: got %d px entry, want %d", c.ask, w, c.want)
		}
		if len(img) == 0 {
			t.Errorf("size %d: empty image bytes", c.ask)
		}
	}
}

func TestICOImageEntriesAreImages(t *testing.T) {
	for _, s := range []int{16, 32, 48} {
		img, _, err := ICOImage(ICO, s)
		if err != nil {
			t.Fatal(err)
		}
		isPNG := bytes.HasPrefix(img, []byte("\x89PNG"))
		isDIB := len(img) >= 4 && img[0] == 40 && img[1] == 0 // BITMAPINFOHEADER size
		if !isPNG && !isDIB {
			t.Errorf("size %d: entry is neither PNG nor DIB", s)
		}
	}
}

func TestICOImageRejectsGarbage(t *testing.T) {
	for _, b := range [][]byte{nil, []byte("nope"), {0, 0, 1, 0, 5, 0}} {
		if _, _, err := ICOImage(b, 32); err == nil {
			t.Errorf("expected an error for %q", b)
		}
	}
}
