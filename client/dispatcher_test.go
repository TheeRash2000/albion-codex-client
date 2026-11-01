package client

import "testing"

// TestCreateUploaders_ReusesUploaderForSameTarget guards against the
// memory leak where every upload call constructed brand new uploaders
// (a fresh NATS connection that was never closed, a fresh http.Transport
// whose idle connections were never reclaimed) instead of reusing one
// per ingest target. Ported from upstream albiondata-client 8fe0886.
func TestCreateUploaders_ReusesUploaderForSameTarget(t *testing.T) {
	target := "https://example.com/TestCreateUploaders_ReusesUploaderForSameTarget"

	first := createUploaders([]string{target})
	second := createUploaders([]string{target})

	if len(first) != 1 || len(second) != 1 {
		t.Fatalf("got %d and %d uploaders, want 1 and 1", len(first), len(second))
	}
	if first[0] != second[0] {
		t.Fatal("createUploaders returned a different uploader instance for the same target")
	}
}
