package client

import (
	"encoding/json"
	"strconv"

	"github.com/ao-data/albiondata-client/lib"
	"github.com/ao-data/albiondata-client/log"
	uuid "github.com/nu7hatch/gouuid"
)

type operationAuctionGetRequestsResponse struct {
	MarketOrders []string `mapstructure:"0"`
}

func (op operationAuctionGetRequestsResponse) Process(state *albionState) {
	log.Debug("Got response to AuctionGetRequests operation...")

	orders := decodeRequestOrders(op.MarketOrders, state.LocationId)

	if len(orders) < 1 {
		return
	}

	srv, _ := state.GetServer()
	// This buy-order path used to drop captures SILENTLY before the location was
	// known. Buffer them instead and flush once the zone resolves.
	if !state.locationKnown() {
		stashMarketOrders(orders, codexServerName(srv))
		Status.addActivity("Saw market buy orders before your zone loaded - holding " + strconv.Itoa(len(orders)) + " to send once it does")
		return
	}

	Status.addActivity("Read " + strconv.Itoa(len(orders)) + " market buy orders")

	upload := lib.MarketUpload{
		Orders: orders,
	}

	uploadMarketToCodex(orders, codexServerName(srv))

	identifier, _ := uuid.NewV4()
	log.Infof("Sending %d live market buy orders to ingest (Identifier: %s)", len(orders), identifier)
	sendMsgToPublicUploaders(upload, lib.NatsMarketOrdersIngest, state, identifier.String())
}

// decodeRequestOrders turns the raw buy-request strings into orders at the
// player's location. An order that is not valid JSON is skipped: it used to be
// kept as an EMPTY order (no item, no price) and forwarded to the public
// uploaders. One that is valid JSON with a mistyped field is kept, as before.
func decodeRequestOrders(raw []string, playerLocation string) []*lib.MarketOrder {
	var orders []*lib.MarketOrder
	skipped := 0

	for _, v := range raw {
		if !json.Valid([]byte(v)) {
			skipped++
			continue
		}
		order := &lib.MarketOrder{}

		err := json.Unmarshal([]byte(v), order)
		if err != nil {
			log.Errorf("Problem converting market order to internal struct: %v", err)
		}

		order.LocationID = playerLocation
		orders = append(orders, order)
	}

	if skipped > 0 {
		log.Errorf("Skipped %d market buy orders that were not valid JSON.", skipped)
	}
	return orders
}
