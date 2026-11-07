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
	// the starter minis count as unlocked after the first home (only new
	// minis get mini quests)
	a.MiniSeen = []int32{-1}
	a.refreshMiniQuests()
	if len(a.MiniQuests) != miniQuestsShown {
		t.Fatalf("mini quests %+v", a.MiniQuests)
	}
	chain := miniChain()
	first := chain[a.MiniQuests[0].Index]
	if first.Target != 1 || first.Own["RequirementMini"] == "" || first.req() == 0 {
		t.Fatalf("first %+v", first)
	}
	// in the order they were unlocked: Knight (0), Barbarian (1), Archer (3)
	if a.MiniQuests[0].Index != 0 || a.MiniQuests[1].Index != 1*3 || a.MiniQuests[2].Index != 3*3 {
		t.Fatalf("order %+v", a.MiniQuests)
	}
	// counted by the client (the units played), reported by the hook
	a.BattleResult(ModeDuel, 1, false)
	if a.MiniQuests[0].Progress != 0 {
		t.Fatalf("counted by the deck %+v", a.MiniQuests)
	}
	a.QuestsFromClient([][2]int32{{a.MiniQuests[0].ID, 1}})
	if a.MiniQuests[0].Progress != 1 {
		t.Fatalf("not counted %+v", a.MiniQuests)
	}
	id, last := a.MiniQuests[0].ID, a.QuestID
	if !a.ClaimQuest(&Reward{}, id) || a.MiniSteps[miniCharRows[0]] != 1 {
		t.Fatalf("claim %+v %v", a.MiniQuests, a.MiniSteps)
	}
	// its next step (win 3) took its place, with the next id, as the
	// client hands it out
	found := false
	for _, q := range a.MiniQuests {
		found = found || (q.Index == 1 && !q.Claimed && q.ID == last+1)
	}
	if !found || len(a.MiniQuests) != miniQuestsShown {
		t.Fatalf("next step %+v", a.MiniQuests)
	}
	// the home: the dailies in the active list, the mini quests in the
	// chains' state (event, step, quest), one chain event per open mini
	h := a.ownHome()
	h.home.skinStore = false
	m, err := decode(h.encode())
	var miniChains []chainState // the tutorial and competitive chains apart (chains.go)
	for _, c := range lastChains {
		if c.event >= 1100 && c.event < 1200 {
			miniChains = append(miniChains, c)
		}
	}
	if err != nil || len(m.home.quests.quests) != questsPerDay || len(miniChains) != miniQuestsShown {
		t.Fatalf("%v %d quests, chains %+v", err, len(m.home.quests.quests), lastChains)
	}
	events := map[int32]LiveEvent{}
	for _, e := range m.home.events {
		if e.Type == EventQuestChain && e.ID >= 1100 && e.ID < 1200 {
			events[e.ID] = e
		}
	}
	for _, c := range miniChains {
		// the decoded quest's index is its index in the chain: the step
		var q Quest
		for _, mq := range a.MiniQuests {
			if mq.ID == c.quest.ID {
				q = mq
			}
		}
		mini, step := miniOfQuest(q)
		e, ok := events[c.event]
		if !ok || q.ID == 0 || c.event != miniChainEvent(mini) || c.step != step || c.quest.Index != step {
			t.Fatalf("chain %+v (mini %d step %d) events %v", c, mini, step, len(events))
		}
		var p struct {
			Chain    []map[string]any
			Category string
		}
		if err := json.Unmarshal([]byte(e.Parameters), &p); err != nil || len(p.Chain) != 3 || p.Category != "Tutorial" ||
			p.Chain[0]["PieceRequirementType"] != "MiniFromList" ||
			p.Chain[0]["WinWithUnit"].(map[string]any)["UnitList"].([]any)[0] != Data.CharName(miniCharRows[mini]) {
			t.Fatalf("%v %s", err, e.Parameters)
		}
	}
	if len(events) != miniQuestsShown {
		t.Fatalf("%d chain events", len(events))
	}
}

// A mini unlocked after the first home gets its quests first.
func TestMiniQuestsNewFirst(t *testing.T) {
	Data = miniData()
	defer func() { Data = nil }()
	a := NewAccount()
	a.StartProgression()
	a.refreshMiniQuests()
	var row int32 = -1
	for _, r := range miniSeasonalRows {
		if a.owned(miniID(r)).Level == 0 {
			row = r
			break
		}
	}
	if row < 0 {
		t.Skip("every mini owned")
	}
	a.entry(miniID(row)).Level = 1
	// the open ones stay; once a slot is free, the new mini is first
	a.MiniQuests = nil
	a.refreshMiniQuests()
	if q := a.MiniQuests[0]; q.Index != int32(func() int {
		for i, r := range miniSeasonalRows {
			if r == row {
				return i
			}
		}
		return -1
	}()*len(miniQuestWins)) {
		t.Fatalf("not first: %+v", a.MiniQuests)
	}
	found := false
	for _, q := range a.MiniQuests {
		mini, _ := miniOfQuest(q)
		found = found || (!q.Claimed && miniSeasonalRows[mini] == row)
	}
	if !found || len(a.MiniNew) != 1 || a.MiniNew[0] != row {
		t.Fatalf("new mini %d: %+v new %v", row, a.MiniQuests, a.MiniNew)
	}
}

// The day's hero quests name a hero, and the server leaves them to the
// client.
func TestHeroQuests(t *testing.T) {
	for _, d := range todaysQuests() {
		if d.Own["RequirementMini"] == heroOfDay {
			t.Fatal("hero not set")
		}
	}
	catalog := questCatalog
	defer func() { questCatalog = catalog }()
	r := playRumbles(2, rewardSmall)
	questCatalog = []questDef{catalog[0], catalog[1], r, r, r, r, r}
	a := NewAccount()
	a.StartProgression()
	a.refreshQuests()
	a.questsAfterBattle(ModeDuel, 1)
	a.BattleRounds(ModeDuel, 1, 3, 0)
	for _, q := range a.Quests {
		if q.Index < 2 && q.Progress != 0 {
			t.Fatalf("server counted a hero quest %+v", a.Quests)
		}
	}
	if n := todaysQuests()[0].Own["RequirementMini"]; n == heroOfDay || n == "" {
		t.Fatalf("hero %v", n)
	}
}

// Minis owned at the first home get no mini quests; one unlocked later does.
func TestMiniQuestsOnlyNew(t *testing.T) {
	Data = miniData()
	defer func() { Data = nil }()
	a := NewAccount()
	a.StartProgression()
	a.refreshMiniQuests()
	if len(a.MiniQuests) != 0 {
		t.Fatalf("starter minis got quests %+v", a.MiniQuests)
	}
}

// A mini unlocked during the session: its quest at once, and the events
// to send carry its chain.
func TestNewMiniQuestsEvents(t *testing.T) {
	Data = miniData()
	defer func() { Data = nil }()
	a := NewAccount()
	a.StartProgression()
	a.refreshMiniQuests()
	if _, ok := a.NewMiniQuests(); ok {
		t.Fatal("events without a new mini")
	}
	var row int32 = -1
	for _, r := range miniSeasonalRows {
		if a.owned(miniID(r)).Level == 0 {
			row = r
			break
		}
	}
	a.entry(miniID(row)).Level = 1
	events, ok := a.NewMiniQuests()
	if !ok || len(a.MiniQuests) != 1 {
		t.Fatalf("%v %+v", ok, a.MiniQuests)
	}
	mini, _ := miniOfQuest(a.MiniQuests[0])
	found := false
	for _, e := range events {
		found = found || e.ID == miniChainEvent(mini)
	}
	if !found || miniSeasonalRows[mini] != row || a.MiniQuests[0].ID != a.QuestID {
		t.Fatalf("chain event missing %+v", a.MiniQuests)
	}
}
