package client

import (
	"os"
	"strconv"
	"strings"

	"github.com/ao-data/albiondata-client/log"
)

// codex_diag.go - TEMPORARY reverse-engineering aid for the Crafting P&L lifecycle.
// It dumps the raw decoded Photon param map for chosen op/event codes so an offline
// replay reveals the field layout of currently-unhandled packets.
//
// Codes are the WIRE codes the decoder resolves (NOT the source line numbers, and
// NOT the enum value where it drifts). The still-unhandled lifecycle packets:
//   - craft finished  = EVENT 71   (evCraftItemFinished; wire 71, enum drifts to 80)
//   - list (sell)     = OP    79   (opAuctionCreateOffer;  qty + price)
//   - buy order       = OP    80   (opAuctionCreateRequest)
//   - instant buy     = OP    83   (opAuctionBuyOffer)
// (Realized sales AND purchases are already captured via marketplace mail, so they
// no longer need diag; the four above are what B2/B3 need a sample of.)
//
// Inert unless CODEX_DIAG is set, so it is safe to leave in a working tree:
//   CODEX_DIAG=1                          -> dump EVERY op/event code
//   CODEX_DIAG=1 CODEX_DIAG_CODES=71,79,80,83 -> dump only the lifecycle codes above
//
// Usage (offline, no game needed - replay an admin-mode .gob recording):
//   set CODEX_DIAG=1
//   set CODEX_DIAG_CODES=71,79,80,83
//   codex-client.exe -d -o C:\path\to\lifecycle.gob   (logs to stderr)
//
// REMOVE this file + the three codexDiag* calls in decode.go once the capture is decoded.

var (
	codexDiagOn    = os.Getenv("CODEX_DIAG") != ""
	codexDiagCodes = parseDiagCodes(os.Getenv("CODEX_DIAG_CODES"))
)

func parseDiagCodes(s string) map[uint16]bool {
	if strings.TrimSpace(s) == "" {
		return nil // nil = dump every code
	}
	m := map[uint16]bool{}
	for _, part := range strings.Split(s, ",") {
		if n, err := strconv.ParseUint(strings.TrimSpace(part), 10, 16); err == nil {
			m[uint16(n)] = true
		}
	}
	return m
}

func codexDiagWanted(code uint16) bool {
	if !codexDiagOn {
		return false
	}
	if codexDiagCodes == nil {
		return true
	}
	return codexDiagCodes[code]
}

func codexDiagReq(params map[uint8]interface{}) {
	if !codexDiagOn {
		return
	}
	if code, ok := resolveOperationCode(params); ok && codexDiagWanted(code) {
		log.Infof("[CODEXDIAG] REQ op=%d %s %s", code, OperationType(code), formatDebugPhotonParams(params))
	}
}

func codexDiagResp(params map[uint8]interface{}) {
	if !codexDiagOn {
		return
	}
	if code, ok := resolveOperationCode(params); ok && codexDiagWanted(code) {
		log.Infof("[CODEXDIAG] RESP op=%d %s %s", code, OperationType(code), formatDebugPhotonParams(params))
	}
}

func codexDiagEvent(params map[uint8]interface{}) {
	if !codexDiagOn {
		return
	}
	if code, ok := resolveEventCode(params); ok && codexDiagWanted(code) {
		log.Infof("[CODEXDIAG] EVENT ev=%d %s %s", code, EventType(code), formatDebugPhotonParams(params))
	}
}
