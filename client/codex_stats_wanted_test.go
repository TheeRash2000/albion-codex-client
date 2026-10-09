package client

import (
	"encoding/json"
	"testing"
)

// The site added name/tier/copyText to each wanted entry. They must survive the
// decode (the panel is fed from this struct re-encoded), and entries from an
// older response without them must still decode cleanly.
func TestClientStatsWantedNewAndOldFields(t *testing.T) {
	raw := `{"bounties":{"server":"Europe","wanted":[
		{"id":"T5_ARTEFACT_2H_BOW_CRYSTAL","kind":"item","blocks":3,"name":"Expert's Windborne Crystal","tier":5,"copyText":"Windborne Crystal"},
		{"id":"T4_ORE","kind":"item","blocks":1}
	]}}`
	var s ClientStats
	if err := json.Unmarshal([]byte(raw), &s); err != nil {
		t.Fatal(err)
	}
	w := s.Bounties.Wanted
	if len(w) != 2 {
		t.Fatalf("want 2 entries, got %d", len(w))
	}
	if w[0].Name != "Expert's Windborne Crystal" || w[0].Tier != 5 || w[0].CopyText != "Windborne Crystal" {
		t.Errorf("new fields lost: %+v", w[0])
	}
	if w[1].Name != "" || w[1].Tier != 0 || w[1].CopyText != "" {
		t.Errorf("old entry should have empty new fields: %+v", w[1])
	}
	out, _ := json.Marshal(s)
	var back map[string]interface{}
	_ = json.Unmarshal(out, &back)
	first := back["bounties"].(map[string]interface{})["wanted"].([]interface{})[0].(map[string]interface{})
	if first["copyText"] != "Windborne Crystal" || first["name"] == nil {
		t.Errorf("fields not re-served to the panel: %v", first)
	}
}
