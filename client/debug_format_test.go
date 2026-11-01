package client

import (
	"fmt"
	"strings"
	"testing"
)

func TestFormatDebugByteSlice_keepsWholePayload(t *testing.T) {
	// A dense byte slice is a payload, not padding: Destiny Board event 154
	// carries one level byte per node, parallel to the node id array, and
	// losing its tail loses those nodes. 75 bytes is 150 hex characters, which
	// the previous 128-character cap cut in half.
	p := make([]byte, 75)
	for i := range p {
		p[i] = byte(i + 1)
	}
	s := formatDebugByteSlice(p)
	if strings.Contains(s, "…") {
		t.Fatalf("expected no truncation for %d bytes, got: %s", len(p), s)
	}
	if !strings.HasSuffix(s, fmt.Sprintf("%02x", p[len(p)-1])) {
		t.Fatalf("expected the last byte to survive, got: %s", s)
	}
}

func TestFormatDebugByteSlice_stillBoundsLargePayloads(t *testing.T) {
	s := formatDebugByteSlice(make([]byte, debugFormatMaxHexBytes*2))
	if !strings.Contains(s, "…(shown=") {
		t.Fatalf("expected a truncation marker, got: %s", s)
	}
}

func TestFormatDebugPhotonParams_collapsesZeros(t *testing.T) {
	zeros := make([]int16, 50)
	zeros[0] = 1
	zeros[49] = 2
	m := map[uint8]interface{}{
		1: zeros,
		2: append(make([]byte, 40), 0xab, 0xcd),
	}
	s := formatDebugPhotonParams(m)
	if strings.Count(s, "0 ") > 15 {
		t.Fatalf("expected collapsed zeros, got: %s", s)
	}
	if !strings.Contains(s, "×") && !strings.Contains(s, "…(") {
		t.Fatalf("expected collapse marker in: %s", s)
	}
}
