package client

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/ao-data/albiondata-client/lib"
	"github.com/ao-data/albiondata-client/log"
)

// codexPrice is one aggregated market row in the ingest payload.
type codexPrice struct {
	ItemID      string `json:"item_id"`
	City        string `json:"city"`
	Quality     int    `json:"quality"`
	SellMin     *int   `json:"sell_min,omitempty"`
	SellMinDate string `json:"sell_min_date,omitempty"`
	BuyMax      *int   `json:"buy_max,omitempty"`
	BuyMaxDate  string `json:"buy_max_date,omitempty"`
}

type codexMarketPayload struct {
	Server string       `json:"server,omitempty"`
	Prices []codexPrice `json:"prices"`
}

// codexServerName maps the client's detected server id (state.GetServer) to the
// region label. Returns "" when undetected (e.g. offline replay) so the server
// upserts don't overwrite a known region with a placeholder.
func codexServerName(id int) string {
	switch id {
	case 1:
		return "americas"
	case 2:
		return "asia"
	case 3:
		return "europe"
	default:
		return ""
	}
}

// uploadMarketToCodex aggregates a batch of captured market orders into per-item
// sell-min / buy-max and posts them to Codex. Best-effort and opt-in: no-op unless
// market upload is enabled (--codex-market) and the client is paired. Orders arrive
// as either sell offers OR buy requests, so a batch usually carries only one side;
// the server merges them.
func uploadMarketToCodex(orders []*lib.MarketOrder, server string) {
	if !ConfigGlobal.CodexMarketUpload {
		return
	}
	token := strings.TrimSpace(ConfigGlobal.CodexToken)
	if token == "" || len(orders) == 0 {
		return
	}

	type agg struct {
		city    string
		quality int
		itemID  string
		sellMin *int
		buyMax  *int
	}
	byKey := map[string]*agg{}
	for _, o := range orders {
		if o.ItemID == "" || o.LocationID == "" {
			continue
		}
		// The packet's ItemTypeId already carries the enchant suffix (e.g.
		// "T7_PLANKS_LEVEL2@2") for enchanted items; only append it if absent.
		itemID := o.ItemID
		if o.EnchantmentLevel > 0 && !strings.Contains(itemID, "@") {
			itemID = itemID + "@" + strconv.Itoa(o.EnchantmentLevel)
		}
		key := itemID + "|" + o.LocationID + "|" + strconv.Itoa(o.QualityLevel)
		a := byKey[key]
		if a == nil {
			a = &agg{city: o.LocationID, quality: o.QualityLevel, itemID: itemID}
			byKey[key] = a
		}
		// Packet UnitPriceSilver is real silver x10000 (matches what AODP divides out).
		price := o.Price / 10000
		// AuctionType "offer" = sell order; anything else (request) = buy order.
		if strings.EqualFold(o.AuctionType, "offer") {
			if a.sellMin == nil || price < *a.sellMin {
				a.sellMin = &price
			}
		} else {
			if a.buyMax == nil || price > *a.buyMax {
				a.buyMax = &price
			}
		}
	}

	now := time.Now().UTC().Format(time.RFC3339)
	prices := make([]codexPrice, 0, len(byKey))
	for _, a := range byKey {
		cp := codexPrice{ItemID: a.itemID, City: a.city, Quality: a.quality}
		if a.sellMin != nil {
			cp.SellMin = a.sellMin
			cp.SellMinDate = now
		}
		if a.buyMax != nil {
			cp.BuyMax = a.buyMax
			cp.BuyMaxDate = now
		}
		prices = append(prices, cp)
	}
	if len(prices) == 0 {
		return
	}

	body, err := json.Marshal(codexMarketPayload{Server: server, Prices: prices})
	if err != nil {
		log.Errorf("Codex market: could not encode prices: %v", err)
		return
	}
	base := strings.TrimRight(strings.TrimSpace(ConfigGlobal.CodexBaseURL), "/")
	if base == "" {
		base = "https://albioncodex.com"
	}
	req, err := http.NewRequest(http.MethodPost, base+"/api/client/market", bytes.NewReader(body))
	if err != nil {
		log.Errorf("Codex market: could not build request: %v", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("User-Agent", codexClientVersion)

	resp, err := (&http.Client{Timeout: 20 * time.Second}).Do(req)
	if err != nil {
		log.Errorf("Codex market: upload failed: %v", err)
		return
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		log.Infof("Codex: uploaded %d market prices.", len(prices))
	case resp.StatusCode == http.StatusUnauthorized:
		log.Error("Codex market: device token rejected (401).")
	default:
		log.Errorf("Codex market: rejected with status %d.", resp.StatusCode)
	}
}
