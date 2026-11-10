package client

import (
	"math"
	"strconv"
	"time"
)

// ClientContribution is the owner's Codex+ contribution as the site reports it
// in the stats response: the score that earns Codex+ free, and where it stands.
// The site computes status and pointsToEarn with the same rules as its daily
// recompute, so the client only presents them.
type ClientContribution struct {
	Score        *float64 `json:"score"`
	Threshold    float64  `json:"threshold"`
	KeepAt       float64  `json:"keepAt"`
	Status       string   `json:"status"` // earned | keeping | below | none
	PointsToEarn float64  `json:"pointsToEarn"`
	Earned       bool     `json:"earned"`    // held through the contributor grant
	CodexPlus    bool     `json:"codexPlus"` // held at all (earned, paid or gifted)
	UpdatedAt    *string  `json:"updatedAt"` // the score is recomputed once a day
}

// PlusRow is what the panel's Codex+ row shows.
type PlusRow struct {
	Title   string `json:"title"`
	Detail  string `json:"detail"`
	Chip    string `json:"chip"`
	Percent int    `json:"percent"` // progress to the threshold, 0-100; -1 = no bar
	Held    bool   `json:"held"`    // Codex+ is active on the account
	Updated string `json:"updated,omitempty"`
}

// plusBarFrom is the share of the threshold (in percent) from which the row
// shows a progress bar and the points still needed.
const plusBarFrom = 10

// codexPlusRow turns the site's contribution into the panel row. With no
// contribution (an older site, or the read failed) it is a plain pointer to
// Codex+ with no numbers, never a zero score.
func codexPlusRow(c *ClientContribution, now time.Time) PlusRow {
	if c == nil || c.Threshold <= 0 {
		return PlusRow{Title: "Codex+", Detail: "Earn it free by contributing with this client", Chip: "Learn more", Percent: -1}
	}
	score := 0.0
	if c.Score != nil && *c.Score > 0 {
		score = *c.Score
	}
	pct := int(math.Floor(score / c.Threshold * 100))
	if pct > 100 {
		pct = 100
	}
	row := PlusRow{Percent: pct, Updated: scoreUpdated(c.UpdatedAt, now)}
	// Far from the threshold a bar is a sliver and the gap a five-figure number:
	// accurate, but it reads as unreachable. Below plusBarFrom the row shows the
	// points without the bar or the gap. Earned rows always show the bar.
	far := pct < plusBarFrom
	if far && !c.Earned {
		row.Percent = -1
	}

	switch {
	case c.Earned && c.Status == "keeping":
		row.Title, row.Chip, row.Held = "Codex+ earned", "Slipping", true
		row.Detail = commas(score) + " points - slipping: you keep it above " + commas(c.KeepAt)
	case c.Earned:
		row.Title, row.Chip, row.Held = "Codex+ earned", "Earned", true
		row.Detail = commas(score) + " points - yours while you stay above " + commas(c.KeepAt)
	case c.CodexPlus:
		row.Title, row.Chip, row.Held = "Codex+ active", "Active", true
		row.Detail = "Contribution: " + commas(score) + " / " + commas(c.Threshold) + " points"
	case c.PointsToEarn <= 0:
		// Over the threshold, but the grant is only written by the daily recompute.
		row.Title, row.Chip = "Codex+", strconv.Itoa(pct)+"%"
		row.Detail = commas(score) + " / " + commas(c.Threshold) + " points - granted at the next daily update"
	case score <= 0:
		row.Title, row.Chip = "Codex+", "Learn more"
		row.Detail = "Earn it free by contributing with this client"
	case far:
		row.Title, row.Chip = "Codex+", "Learn more"
		row.Detail = commas(score) + " contribution points - top contributors earn Codex+ free"
	default:
		row.Title, row.Chip = "Codex+", strconv.Itoa(pct)+"%"
		row.Detail = commas(score) + " / " + commas(c.Threshold) + " points - " + commas(c.PointsToEarn) + " to earn it free"
	}
	return row
}

// scoreUpdated says how old the score is ("Score updated 14 hours ago"), since
// it only moves once a day and would otherwise look stuck. Empty when unknown.
func scoreUpdated(at *string, now time.Time) string {
	if at == nil {
		return ""
	}
	t, err := time.Parse(time.RFC3339, *at)
	if err != nil {
		return ""
	}
	d := now.Sub(t)
	if d < 0 {
		d = 0
	}
	plural := func(n int, unit string) string {
		if n == 1 {
			return "Score updated 1 " + unit + " ago"
		}
		return "Score updated " + strconv.Itoa(n) + " " + unit + "s ago"
	}
	switch {
	case d < time.Hour:
		return "Score updated in the last hour"
	case d < 48*time.Hour:
		return plural(int(d/time.Hour), "hour")
	default:
		return plural(int(d/(24*time.Hour)), "day")
	}
}

// commas formats a non-negative whole number with thousands separators.
func commas(v float64) string {
	if v < 0 || math.IsNaN(v) || math.IsInf(v, 0) {
		v = 0
	}
	s := strconv.FormatInt(int64(math.Round(v)), 10)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}

// panelStatsBody is the panel's /api/stats response: the cached stats plus the
// Codex+ row derived from them.
type panelStatsBody struct {
	*ClientStats
	Plus PlusRow `json:"plus"`
}

// panelStats builds that response. Nil until stats have been fetched, which the
// page reads as "show this session's numbers only".
func panelStats(s *ClientStats, now time.Time) *panelStatsBody {
	if s == nil {
		return nil
	}
	return &panelStatsBody{ClientStats: s, Plus: codexPlusRow(s.Contribution, now)}
}

// CodexPlusURL is the Codex page that explains Codex+ and how it is earned.
func CodexPlusURL() string {
	return codexBase() + "/pricing"
}
