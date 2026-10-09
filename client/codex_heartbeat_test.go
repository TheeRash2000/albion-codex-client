package client

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/ao-data/albiondata-client/lib"
)

func decodeReport(t *testing.T, b []byte) map[string]interface{} {
	t.Helper()
	var m map[string]interface{}
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("report is not JSON: %v", err)
	}
	return m
}

// The check-in carries market health, so the server can tell "market upload off"
// or "failing" from a player who is not browsing a market.
func TestVersionReportCarriesMarketHealth(t *testing.T) {
	prevStatus, prevMarket := Status, ConfigGlobal.CodexMarketUpload
	defer func() { Status, ConfigGlobal.CodexMarketUpload = prevStatus, prevMarket }()

	Status = &ClientStatus{}
	ConfigGlobal.CodexMarketUpload = true
	Status.recordSent(marketStream, 12)
	Status.recordFailed(marketStream, "server returned status 500")
	Status.recordSent("Destiny board", 40) // other streams do not count

	m := decodeReport(t, versionReportBody("alpha-0.1.17"))
	if m["version"] != "alpha-0.1.17" {
		t.Errorf("version = %v", m["version"])
	}
	market, ok := m["market"].(map[string]interface{})
	if !ok {
		t.Fatalf("no market object in %v", m)
	}
	if market["on"] != true || market["failed"] != float64(1) {
		t.Errorf("market = %v, want on=true failed=1", market)
	}
	sent, err := time.Parse(time.RFC3339, market["last_sent_at"].(string))
	if err != nil || time.Since(sent) > time.Minute {
		t.Errorf("last_sent_at = %v (%v), want about now in RFC3339", market["last_sent_at"], err)
	}
}

func TestVersionReportMarketOffAndNeverSent(t *testing.T) {
	prevStatus, prevMarket := Status, ConfigGlobal.CodexMarketUpload
	defer func() { Status, ConfigGlobal.CodexMarketUpload = prevStatus, prevMarket }()

	Status = &ClientStatus{}
	ConfigGlobal.CodexMarketUpload = false

	market := decodeReport(t, versionReportBody("v"))["market"].(map[string]interface{})
	if market["on"] != false || market["failed"] != float64(0) {
		t.Errorf("market = %v, want on=false failed=0", market)
	}
	if _, has := market["last_sent_at"]; has {
		t.Errorf("last_sent_at must be omitted when nothing was sent this run: %v", market)
	}
}

// A market browse held while the location was unknown belongs to the character
// that was playing. After a switch it must be discarded: flushing it later would
// stamp the previous character's prices with the new character's zone.
func TestCharacterSwitchDiscardsHeldMarketBrowse(t *testing.T) {
	defer func() { pendingOrders, pendingServer = nil, "" }()

	state := &albionState{CharacterId: "aaaa", CharacterName: "Old"}
	stashMarketOrders([]*lib.MarketOrder{{ItemID: "T4_BAG", Price: 50000, AuctionType: "offer"}}, "europe")

	operationJoinResponse{CharacterID: "bbbb", CharacterName: "New", Location: "3008"}.Process(state)

	pendingMarketMu.Lock()
	n := len(pendingOrders)
	pendingMarketMu.Unlock()
	if n != 0 {
		t.Fatalf("%d held orders survived a character switch", n)
	}
}

// Same character re-joining (a zone change) still flushes the held browse.
func TestSameCharacterJoinStillFlushesHeldBrowse(t *testing.T) {
	prevMarket, prevPub, prevPriv := ConfigGlobal.CodexMarketUpload, ConfigGlobal.PublicIngestBaseUrls, ConfigGlobal.PrivateIngestBaseUrls
	defer func() {
		pendingOrders, pendingServer = nil, ""
		ConfigGlobal.CodexMarketUpload, ConfigGlobal.PublicIngestBaseUrls, ConfigGlobal.PrivateIngestBaseUrls = prevMarket, prevPub, prevPriv
	}()
	// No network in the test: no Codex upload and no AODP / private targets.
	ConfigGlobal.CodexMarketUpload = false
	ConfigGlobal.PublicIngestBaseUrls, ConfigGlobal.PrivateIngestBaseUrls = "", ""

	state := &albionState{CharacterId: "aaaa", CharacterName: "Same"}
	stashMarketOrders([]*lib.MarketOrder{{ItemID: "T4_BAG", Price: 50000, AuctionType: "offer"}}, "europe")

	operationJoinResponse{CharacterID: "aaaa", CharacterName: "Same", Location: "3008"}.Process(state)

	pendingMarketMu.Lock()
	n := len(pendingOrders)
	pendingMarketMu.Unlock()
	if n != 0 {
		t.Fatalf("held browse was not flushed on a same-character join (%d left)", n)
	}
}
