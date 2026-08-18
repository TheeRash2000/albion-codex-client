package client

import (
	"strconv"
	"sync"
	"time"

	"github.com/ao-data/albiondata-client/lib"
	"github.com/ao-data/albiondata-client/log"
	uuid "github.com/nu7hatch/gouuid"
)

// pendingOrders holds market orders captured before the player's LOCATION was
// known. A market browse right after login or a relog can arrive (AuctionGetOffers
// / AuctionGetRequests) before operation_join sets the location - and that browse
// used to be DROPPED (silently, for buy orders) because IsValidLocation() was
// false. Instead we stash the orders here and flush them the moment the location
// resolves, so no browse is ever lost. Mirrors pending_board.go.
var (
	pendingMarketMu sync.Mutex
	pendingOrders   []*lib.MarketOrder
	pendingServer   string
	pendingAt       time.Time
)

// maxPendingOrders caps the buffer so a long location-less window can never grow
// it without bound (a single wide market view is a few hundred orders).
const maxPendingOrders = 5000

// stashMarketOrders remembers orders to upload once the location is known.
func stashMarketOrders(orders []*lib.MarketOrder, server string) {
	if len(orders) == 0 {
		return
	}
	pendingMarketMu.Lock()
	defer pendingMarketMu.Unlock()
	pendingOrders = append(pendingOrders, orders...)
	if len(pendingOrders) > maxPendingOrders {
		pendingOrders = pendingOrders[len(pendingOrders)-maxPendingOrders:]
	}
	if server != "" {
		pendingServer = server
	}
	pendingAt = time.Now()
}

// flushPendingMarket uploads stashed market orders once a valid location is known,
// stamping any unlocated order with it. Called from the join + cluster handlers
// right after the location is set. No-op if nothing is pending or the location is
// still unknown. A buffer that has gone stale (the location took too long, so the
// player probably changed zones) is dropped rather than attributed to the wrong
// zone.
func flushPendingMarket(state *albionState) {
	pendingMarketMu.Lock()
	if len(pendingOrders) == 0 || !state.locationKnown() {
		pendingMarketMu.Unlock()
		return
	}
	orders := pendingOrders
	server := pendingServer
	at := pendingAt
	pendingOrders = nil
	pendingServer = ""
	pendingMarketMu.Unlock()

	if time.Since(at) > 90*time.Second {
		log.Debugf("Dropping %d market orders buffered before location (too stale)", len(orders))
		return
	}

	for _, o := range orders {
		if o.LocationID == "" {
			o.LocationID = state.LocationId
		}
	}
	if server == "" {
		srv, _ := state.GetServer()
		server = codexServerName(srv)
	}

	log.Infof("Flushing %d market orders that were buffered before the location was known.", len(orders))
	Status.addActivity("Sent " + strconv.Itoa(len(orders)) + " market prices captured before your zone loaded")
	uploadMarketToCodex(orders, server)

	upload := lib.MarketUpload{Orders: orders}
	identifier, _ := uuid.NewV4()
	sendMsgToPublicUploaders(upload, lib.NatsMarketOrdersIngest, state, identifier.String())
}
