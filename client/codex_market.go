package client

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/ao-data/albiondata-client/lib"
	"github.com/ao-data/albiondata-client/log"
)

// marketStream is the capture-health name of the market-price uploads. The
// check-in heartbeat reads this stream's numbers (codex_version.go).
const marketStream = "Market prices"

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

// recordMarketActivity logs each uploaded price to the live panel feed, so the
// user sees EXACTLY what left their PC (item + price), not just a count. A single
// market scan is usually one item across a few qualities, so per-line is readable;
// a large batch is capped with a "...and N more" tail so the feed never floods.
func recordMarketActivity(prices []codexPrice) {
	const maxLines = 15
	shown := len(prices)
	if shown > maxLines {
		shown = maxLines
	}
	if extra := len(prices) - shown; extra > 0 {
		Status.addActivity("...and " + strconv.Itoa(extra) + " more prices sent to Codex")
	}
	for i := 0; i < shown; i++ {
		Status.addActivity(formatPriceLine(prices[i]))
	}
}

// formatPriceLine renders one uploaded price like "Rare Ashenbark Planks - sell
// 1,234" or "Adept's Bag, Good - sell 980 / buy 900".
func formatPriceLine(p codexPrice) string {
	label := itemLabel(p.ItemID)
	if q := qualityName(p.Quality); q != "" {
		label += ", " + q
	}
	var parts []string
	if p.SellMin != nil {
		parts = append(parts, "sell "+formatSilver(*p.SellMin))
	}
	if p.BuyMax != nil {
		parts = append(parts, "buy "+formatSilver(*p.BuyMax))
	}
	if len(parts) == 0 {
		return label + " sent to Codex"
	}
	return label + " - " + strings.Join(parts, " / ")
}

// humanizeItemID turns a raw Albion item id ("T7_PLANKS_LEVEL2@2",
// "T8_2H_AXE_KEEPER") into a readable "T7.2 Planks" / "T8 2h Axe Keeper" label.
// It keeps the tier + enchant and title-cases the rest; no item-name database is
// bundled, so the wording stays close to the id but is far clearer than the raw id.
func humanizeItemID(id string) string {
	s := id
	ench := ""
	if i := strings.IndexByte(s, '@'); i >= 0 {
		ench = s[i+1:]
		s = s[:i]
	}
	// Resource enchant can also appear as a "_LEVELn" suffix.
	if idx := strings.Index(s, "_LEVEL"); idx >= 0 {
		if ench == "" {
			ench = s[idx+len("_LEVEL"):]
		}
		s = s[:idx]
	}
	tier := ""
	if len(s) >= 2 && s[0] == 'T' {
		j := 1
		for j < len(s) && s[j] >= '0' && s[j] <= '9' {
			j++
		}
		if j > 1 && j < len(s) && s[j] == '_' {
			tier = s[1:j]
			s = s[j+1:]
		}
	}
	name := titleWords(strings.ReplaceAll(s, "_", " "))
	prefix := ""
	if tier != "" {
		prefix = "T" + tier
		if ench != "" && ench != "0" {
			prefix += "." + ench
		}
		prefix += " "
	}
	out := strings.TrimSpace(prefix + name)
	if out == "" {
		return id
	}
	return out
}

// titleWords lower-cases then capitalises the first letter of each word.
func titleWords(s string) string {
	words := strings.Fields(strings.ToLower(s))
	for i, w := range words {
		r := []rune(w)
		if len(r) > 0 {
			r[0] = unicode.ToUpper(r[0])
		}
		words[i] = string(r)
	}
	return strings.Join(words, " ")
}

// formatSilver adds thousands separators to a silver amount (1234567 -> "1,234,567").
func formatSilver(n int) string {
	s := strconv.Itoa(n)
	neg := strings.HasPrefix(s, "-")
	if neg {
		s = s[1:]
	}
	var b []byte
	for i := 0; i < len(s); i++ {
		if i > 0 && (len(s)-i)%3 == 0 {
			b = append(b, ',')
		}
		b = append(b, s[i])
	}
	if neg {
		return "-" + string(b)
	}
	return string(b)
}

// codexServerName maps the client's detected server id (state.GetServer) to the
// region label. Returns "" when undetected (e.g. offline replay) so the server
// upserts don't overwrite a known region with a placeholder.
func codexServerName(id int) string {
	name := ""
	switch id {
	case 1:
		name = "americas"
	case 2:
		name = "asia"
	case 3:
		name = "europe"
	}
	// Record it for the panel. Every capture path routes through here, so this
	// one line covers all of them without a new call site; the empty case is
	// skipped so an offline replay cannot blank a realm we already detected.
	if name != "" {
		Status.setServer(name)
	}
	return name
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
	// Market spot prices are re-observable on the next browse, so they are low-value
	// (shed first if the queue ever overflows).
	n := len(prices)
	enqueueCodexUpload(marketStream, "/api/client/market", body, n, true, func() {
		log.Infof("Codex: uploaded %d market prices.", n)
		recordMarketActivity(prices)
	})
}
