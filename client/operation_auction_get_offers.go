package client

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/ao-data/albiondata-client/lib"
	"github.com/ao-data/albiondata-client/log"
	uuid "github.com/nu7hatch/gouuid"
)

type operationAuctionGetOffers struct {
	Category         string   `mapstructure:"1"`
	SubCategory      string   `mapstructure:"2"`
	Quality          string   `mapstructure:"5"`
	Enchantment      uint32   `mapstructure:"6"`
	EnchantmentLevel string   `mapstructure:"10"`
	ItemIds          []uint16 `mapstructure:"8"`
	MaxResults       uint32   `mapstructure:"12"`
	IsAscendingOrder bool     `mapstructure:"14"`
}

func (op operationAuctionGetOffers) Process(state *albionState) {
	log.Debug("Got AuctionGetOffers operation...")
	state.WaitingForMarketData = true
}

type operationAuctionGetOffersResponse struct {
	MarketOrders []string `mapstructure:"0"`
}

func (op operationAuctionGetOffersResponse) Process(state *albionState) {
	log.Debug("Got response to AuctionGetOffers operation...")
	state.WaitingForMarketData = false

	orders := decodeOfferOrders(op.MarketOrders, state.LocationId)

	if len(orders) < 1 {
		return
	}

	srv, _ := state.GetServer()
	// A market browse right after login/relog can arrive before the location is
	// set. Rather than drop those prices, buffer them and flush once the zone
	// resolves (codex_market_pending.go) so no browse is ever lost.
	if !state.locationKnown() {
		stashMarketOrders(orders, codexServerName(srv))
		Status.addActivity("Saw the market before your zone loaded - holding " + strconv.Itoa(len(orders)) + " prices to send once it does")
		return
	}

	// This response also answers buy-order requests and "my open orders" (the
	// game sends them all as op 81; seen live 2026-11-10), so say which sides
	// were actually read. Each order carries its own AuctionType, and the Codex
	// upload below already sorts on it.
	read := ordersReadLabel(orderSides(orders))
	Status.addActivity("Read " + read)

	upload := lib.MarketUpload{
		Orders: orders,
	}

	// Codex first, so a failure in the third-party (public) uploader can never
	// prevent our own upload.
	uploadMarketToCodex(orders, codexServerName(srv))

	identifier, _ := uuid.NewV4()
	log.Infof("Sending %s to ingest (Identifier: %s)", read, identifier)
	sendMsgToPublicUploaders(upload, lib.NatsMarketOrdersIngest, state, identifier.String())
}

// orderSides counts a batch by each order's own type: "offer" is a sell order,
// anything else (the game sends "request") is a buy order. The same rule the
// Codex upload uses (codex_market.go).
func orderSides(orders []*lib.MarketOrder) (sell, buy int) {
	for _, o := range orders {
		if o == nil {
			continue
		}
		if strings.EqualFold(o.AuctionType, "offer") {
			sell++
		} else {
			buy++
		}
	}
	return sell, buy
}

// ordersReadLabel words those counts for the feed and the log: "38 sell orders
// and 12 buy orders", or just the side that was read.
func ordersReadLabel(sell, buy int) string {
	side := func(n int, kind string) string {
		if n == 1 {
			return "1 " + kind + " order"
		}
		return strconv.Itoa(n) + " " + kind + " orders"
	}
	switch {
	case sell > 0 && buy > 0:
		return side(sell, "sell") + " and " + side(buy, "buy")
	case sell > 0:
		return side(sell, "sell")
	case buy > 0:
		return side(buy, "buy")
	default:
		return "0 orders"
	}
}

// decodeOfferOrders turns the raw sell-offer strings into orders. City orders
// carry a null location and take the player's (playerLocation); Smuggler's Den and rest
// orders carry their own.
//
// An order that is not valid JSON is skipped. It used to hit log.Fatal, which
// exits the whole client, so one bad order stopped every capture (board, market,
// sales) until the next launch. An order that is valid JSON but has a field of an
// unexpected type is kept with the fields that did decode, as before.
func decodeOfferOrders(raw []string, playerLocation string) []*lib.MarketOrder {
	var orders []*lib.MarketOrder
	skipped := 0
	finished := 0

	for _, v := range raw {
		// Unmarshal market order data to map
		var marketOrder map[string]interface{}
		err2 := json.Unmarshal([]byte(v), &marketOrder)
		if err2 != nil {
			skipped++
			continue
		}

		// "Finished auctions" (completed trades waiting for pickup) arrive through
		// this same response. A finished auction is not a live price, so it is
		// never uploaded as one. Live orders carry IsFinished=false.
		if done, _ := marketOrder["IsFinished"].(bool); done {
			finished++
			continue
		}

		// Pull the location
		location, _ := marketOrder["LocationId"].(string)

		// if the location has @, it is either a rest or smugglers den
		if strings.Contains(location, "@") {

			// Set the location in the market order
			marketOrder["LocationId"] = location

			// Marshal the map back to json
			newJson, _ := json.Marshal(marketOrder)

			// Set the new json back to the market order
			v = string(newJson)
		}

		order := &lib.MarketOrder{}

		err := json.Unmarshal([]byte(v), order)
		if err != nil {
			log.Errorf("Problem converting market order to internal struct: %v", err)
		}

		// Set the location only if its string(nil). Smugglers Dens pull locations directly from the market data (above)
		// while the orignal cities have a null location ID and is pulled from the client state.
		if order.LocationID == "" {
			order.LocationID = playerLocation
		}

		orders = append(orders, order)
	}

	if skipped > 0 {
		log.Errorf("Skipped %d market orders that were not valid JSON.", skipped)
	}
	if finished > 0 {
		log.Debugf("Left out %d finished auctions (not live prices).", finished)
	}
	return orders
}
