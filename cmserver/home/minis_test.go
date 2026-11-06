package home

import (
	"encoding/json"
	"fmt"
	"testing"
)

func miniData() *GameData {
	d := &GameData{chests: make([]chestDef, 280), charName: map[int32]string{}}
	for _, r := range miniCharRows {
		d.charName[r] = fmt.Sprintf("Mini%dB1", r)
	}
	return d
}

// Mini quests: the next step of three owned minis (deck first), win 1/3/5
// using the mini, its characters.csv row + 1 in the home, category 3; a won
// match counts for the deck's minis; a claim moves the mini on.
func TestMiniQuests(t *testing.T) {
	Data = miniData()
	defer func() { Data = nil }()
	a := NewAccount()
	a.StartProgression()
	a.Decks[a.Selected] = [5]int32{4, 0, 1, 5, 6}
	a.refreshMiniQuests()
	if len(a.MiniQuests) != miniQuestsShown {
		t.Fatalf("mini quests %+v", a.MiniQuests)
	}
	chain := miniChain()
	first := chain[a.MiniQuests[0].Index]
	if first.Target != 1 || first.Own["RequirementMini"] == "" || first.req() == 0 {
		t.Fatalf("first %+v", first)
	}
	// the deck's minis, in miniSeasonalRows order: Knight (0), Barbarian (1), Archer (3)
	if a.MiniQuests[0].Index != 0 || a.MiniQuests[1].Index != 1*3 || a.MiniQuests[2].Index != 3*3 {
		t.Fatalf("order %+v", a.MiniQuests)
	}
	a.BattleResult(ModeDuel, 1, false)
	if a.MiniQuests[0].Progress != 1 {
		t.Fatalf("not counted %+v", a.MiniQuests)
	}
	id := a.MiniQuests[0].ID
	if !a.ClaimQuest(&Reward{}, id) || a.MiniSteps[miniCharRows[0]] != 1 {
		t.Fatalf("claim %+v %v", a.MiniQuests, a.MiniSteps)
	}
	// its next step (win 3) took its place
	found := false
	for _, q := range a.MiniQuests {
		found = found || (q.Index == 1 && !q.Claimed)
	}
	if !found || len(a.MiniQuests) != miniQuestsShown {
		t.Fatalf("next step %+v", a.MiniQuests)
	}
	// the home: dailies and mini quests decode; the chain event is there
	h := a.ownHome()
	h.home.skinStore = false
	m, err := decode(h.encode())
	if err != nil || len(m.home.quests.quests) != questsPerDay+miniQuestsShown {
		t.Fatalf("%v %d quests", err, len(m.home.quests.quests))
	}
	var chainEv *LiveEvent
	for i := range m.home.events {
		if m.home.events[i].Type == EventQuestChain {
			chainEv = &m.home.events[i]
		}
	}
	if chainEv == nil {
		t.Fatal("no chain event")
	}
	var p struct {
		Chain    []map[string]any
		Category string
	}
	if err := json.Unmarshal([]byte(chainEv.Parameters), &p); err != nil || len(p.Chain) != 3*len(miniCharRows) || p.Category != "Tutorial" ||
		p.Chain[0]["RequirementMini"] != "Mini0B1" {
		t.Fatalf("%v %s", err, chainEv.Parameters[:200])
	}
}
