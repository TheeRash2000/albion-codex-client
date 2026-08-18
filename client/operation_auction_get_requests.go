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

	var orders []*lib.MarketOrder

	for _, v := range op.MarketOrders {
		order := &lib.MarketOrder{}

		err := json.Unmarshal([]byte(v), order)
		if err != nil {
			log.Errorf("Problem converting market order to internal struct: %v", err)
		}

		order.LocationID = state.LocationId
		orders = append(orders, order)
	}

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
