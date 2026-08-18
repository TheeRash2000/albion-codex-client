package client

import (
	"reflect"
	"testing"
)

func TestParseVersionNums(t *testing.T) {
	cases := []struct {
		in   string
		want []int
	}{
		{"alpha-0.1.10", []int{0, 1, 10}},
		{"alpha-0.2.0", []int{0, 2, 0}},
		{"1.2.3", []int{1, 2, 3}},
		{"dev", nil},
		{"", nil},
	}
	for _, c := range cases {
		if got := parseVersionNums(c.in); !reflect.DeepEqual(got, c.want) {
			t.Errorf("parseVersionNums(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

// TestVersionIsNewer locks the rule that gates self-update: only a strictly higher
// build triggers an update, and unparseable versions (incl. dev builds) never do -
// this prevents update loops and accidental downgrades.
func TestVersionIsNewer(t *testing.T) {
	cases := []struct {
		candidate string
		current   string
		want      bool
	}{
		{"alpha-0.1.11", "alpha-0.1.10", true},   // patch bump
		{"alpha-0.2.0", "alpha-0.1.10", true},    // minor bump beats higher patch
		{"alpha-0.1.10", "alpha-0.1.10", false},  // same version, no loop
		{"alpha-0.1.9", "alpha-0.1.10", false},   // never downgrade
		{"alpha-0.1.10.1", "alpha-0.1.10", true}, // extra group is newer
		{"dev", "alpha-0.1.10", false},           // dev candidate never wins
		{"alpha-0.1.11", "dev", false},           // dev current never updates
	}
	for _, c := range cases {
		if got := versionIsNewer(c.candidate, c.current); got != c.want {
			t.Errorf("versionIsNewer(%q, %q) = %v, want %v", c.candidate, c.current, got, c.want)
		}
	}
}
