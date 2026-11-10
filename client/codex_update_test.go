package client

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

// The updater reads the first manifest source that serves one and falls back to
// the next when a source is missing, so builds keep updating while releases
// move from one host to another.
func TestFetchManifestUsesFirstSourceThatAnswers(t *testing.T) {
	t.Setenv("CODEX_UPDATE_MANIFEST_URL", "")
	var hits [2]int
	missing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits[0]++
		http.NotFound(w, r)
	}))
	defer missing.Close()
	serves := func(i int, version string) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			hits[i]++
			fmt.Fprintf(w, `{"version":%q,"url":"https://example.com/c.exe","sha256":"00"}`, version)
		}))
	}
	primary := serves(0, "alpha-0.2.0")
	defer primary.Close()
	fallback := serves(1, "alpha-0.1.18")
	defer fallback.Close()

	prev := updateManifestURLs
	defer func() { updateManifestURLs = prev }()

	// Primary has no release yet: the fallback's manifest is used.
	updateManifestURLs = []string{missing.URL, fallback.URL}
	m, err := fetchManifest()
	if err != nil || m.Version != "alpha-0.1.18" {
		t.Fatalf("fallback: got %+v, err %v", m, err)
	}
	if hits != [2]int{1, 1} {
		t.Errorf("fallback: requests = %v, want one to each source", hits)
	}

	// Primary answers: the fallback is never asked.
	hits = [2]int{}
	updateManifestURLs = []string{primary.URL, fallback.URL}
	m, err = fetchManifest()
	if err != nil || m.Version != "alpha-0.2.0" {
		t.Fatalf("primary: got %+v, err %v", m, err)
	}
	if hits != [2]int{1, 0} {
		t.Errorf("primary: requests = %v, want only the first source", hits)
	}

	// Nothing answers: an error, not a nil manifest.
	updateManifestURLs = []string{missing.URL}
	if m, err := fetchManifest(); err == nil {
		t.Fatalf("all sources missing: got %+v, want an error", m)
	}
}

// An explicit override replaces the built-in sources entirely.
func TestManifestURLOverride(t *testing.T) {
	t.Setenv("CODEX_UPDATE_MANIFEST_URL", " https://staging.example.test/version.json ")
	if got := manifestURLs(); !reflect.DeepEqual(got, []string{"https://staging.example.test/version.json"}) {
		t.Errorf("manifestURLs() = %q", got)
	}
}

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
