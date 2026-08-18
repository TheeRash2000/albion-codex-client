package client

import "testing"

// A 401 on any upload means the device was revoked on the website: the panel
// must expose tokenRejected so it can offer re-pairing even though a token file
// still exists (the "Linked but every upload fails, no way to re-link" trap).
func TestTokenRejectedLifecycle(t *testing.T) {
	Status = &ClientStatus{}
	defer func() { Status = &ClientStatus{} }()

	if Status.snapshot().TokenRejected {
		t.Fatal("tokenRejected should start false")
	}

	recordUploadOutcome("Destiny board", 10, 401, nil)
	snap := Status.snapshot()
	if !snap.TokenRejected {
		t.Fatal("a 401 upload must set tokenRejected")
	}
	// The transition is also surfaced as an issue (once), on top of the
	// per-stream failure entry.
	tokenIssues := 0
	for _, is := range snap.Issues {
		if is.Kind == "token" {
			tokenIssues++
		}
	}
	if tokenIssues != 1 {
		t.Fatalf("expected exactly 1 token issue, got %d", tokenIssues)
	}

	// A second 401 must not spam another token issue.
	recordUploadOutcome("Market prices", 5, 401, nil)
	tokenIssues = 0
	for _, is := range Status.snapshot().Issues {
		if is.Kind == "token" {
			tokenIssues++
		}
	}
	if tokenIssues != 1 {
		t.Fatalf("expected the token issue not to repeat, got %d", tokenIssues)
	}

	// An accepted upload proves the token works again (e.g. after re-pairing).
	recordUploadOutcome("Destiny board", 10, 200, nil)
	if Status.snapshot().TokenRejected {
		t.Fatal("a 2xx upload must clear tokenRejected")
	}
}
