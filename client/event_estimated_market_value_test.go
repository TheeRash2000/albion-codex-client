package client

import "testing"

// The EMV event's item array arrives as []uint8 for small ids and []int16 for
// larger ones; numericSlice must normalise both (and other numeric kinds).
func TestNumericSlice(t *testing.T) {
	cases := []struct {
		name string
		in   interface{}
		want []int64
	}{
		{"uint8 (small item ids)", []uint8{10, 11}, []int64{10, 11}},
		{"int16 (larger item ids)", []int16{900, 901}, []int64{900, 901}},
		{"int32 (values)", []int32{5313203, 78369101}, []int64{5313203, 78369101}},
		{"int", []int{1, 2, 3}, []int64{1, 2, 3}},
		{"non-slice", "nope", nil},
		{"nil", nil, nil},
	}
	for _, c := range cases {
		got := numericSlice(c.in)
		if len(got) != len(c.want) {
			t.Errorf("%s: len=%d want %d", c.name, len(got), len(c.want))
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("%s: [%d]=%d want %d", c.name, i, got[i], c.want[i])
			}
		}
	}
}
