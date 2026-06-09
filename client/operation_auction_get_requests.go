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
	log.Debug("Got response to AuctionGetOffers operation...")

	if !state.IsValidLocation() {
		return
	}

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
	Status.addActivity("Read " + strconv.Itoa(len(orders)) + " market buy orders")

	upload := lib.MarketUpload{
		Orders: orders,
	}

	srv, _ := state.GetServer()
	uploadMarketToCodex(orders, codexServerName(srv))

	identifier, _ := uuid.NewV4()
	log.Infof("Sending %d live market buy orders to ingest (Identifier: %s)", len(orders), identifier)
	sendMsgToPublicUploaders(upload, lib.NatsMarketOrdersIngest, state, identifier.String())
}
