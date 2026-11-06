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
	// the home: the dailies in the active list, the mini quests in the
	// chains' state (event, step, quest), one chain event per open mini
	h := a.ownHome()
	h.home.skinStore = false
	m, err := decode(h.encode())
	if err != nil || len(m.home.quests.quests) != questsPerDay || len(lastChains) != miniQuestsShown {
		t.Fatalf("%v %d quests, chains %+v", err, len(m.home.quests.quests), lastChains)
	}
	events := map[int32]LiveEvent{}
	for _, e := range m.home.events {
		if e.Type == EventQuestChain {
			events[e.ID] = e
		}
	}
	for _, c := range lastChains {
		// the decoded quest's index is its index in the chain: the step
		var q Quest
		for _, mq := range a.MiniQuests {
			if mq.ID == c.quest.ID {
				q = mq
			}
		}
		mini, _ := miniOfQuest(q)
		e, ok := events[c.event]
		if !ok || q.ID == 0 || c.event != miniChainEvent(q) || c.step != 0 || c.quest.Index != 0 {
			t.Fatalf("chain %+v (mini %d) events %v", c, mini, len(events))
		}
		var p struct {
			Chain    []map[string]any
			Category string
		}
		if err := json.Unmarshal([]byte(e.Parameters), &p); err != nil || len(p.Chain) != 1 || p.Category != "Tutorial" ||
			p.Chain[0]["RequirementMini"] != Data.CharName(miniCharRows[mini]) {
			t.Fatalf("%v %s", err, e.Parameters)
		}
	}
	if len(events) != miniQuestsShown {
		t.Fatalf("%d chain events", len(events))
	}
}
