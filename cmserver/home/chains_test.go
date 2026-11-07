package home

import (
	"os"
	"testing"
)

// The tutorial chain for a new player, step by step; the competitive chain
// again and again; both in the home's chains.
func TestChains(t *testing.T) {
	apk := os.Getenv("CMSERVER_TEST_APK")
	if apk == "" {
		t.Skip("CMSERVER_TEST_APK not set")
	}
	d, err := LoadGameData(apk)
	if err != nil {
		t.Skip(err)
	}
	Data = d
	defer func() { Data = nil }()
	if _, ok := Data.ChestByName("quest_Competitive_5W"); !ok {
		t.Fatal("quest_Competitive_5W not in chests.json")
	}
	a := NewAccount()
	a.StartProgression()
	h := a.ownHome()
	h.home.skinStore = false
	if _, err := decode(h.encode()); err != nil {
		t.Fatal(err)
	}
	tut, comp := a.chain(tutorialChainEvent), a.chain(competitiveChainEvent)
	if tut == nil || comp == nil {
		t.Fatalf("chains %+v", a.Chains)
	}
	found := 0
	for _, c := range lastChains {
		if c.event == tutorialChainEvent || c.event == competitiveChainEvent {
			found++
		}
	}
	if found != 2 {
		t.Fatalf("home chains %+v", lastChains)
	}
	// step 1: a deck change
	a.Swap(a.Decks[a.Selected][1], 0, -1)
	if !a.ClaimQuest(&Reward{}, tut.Quest.ID) || tut.Step != 1 {
		t.Fatalf("step 1 %+v", *tut)
	}
	// step 2: a match with the Wizard in the deck
	row, _ := Data.CharRow("WizardB1")
	for i, r := range miniCharRows {
		if r == row {
			a.Decks[a.Selected][0] = miniSeasonalRows[i]
		}
	}
	a.questsAfterBattle(ModeDuel, 2)
	if !a.ClaimQuest(&Reward{}, tut.Quest.ID) || tut.Step != 2 {
		t.Fatalf("step 2 %+v", *tut)
	}
	// step 3: a hero upgrade (a mini's does not count)
	a.questsUpgraded(miniID(0))
	if a.ClaimQuest(&Reward{}, tut.Quest.ID) {
		t.Fatal("a mini upgrade counted as a hero's")
	}
	a.questsUpgraded(heroID(0))
	if !a.ClaimQuest(&Reward{}, tut.Quest.ID) {
		t.Fatalf("step 3 %+v", *tut)
	}
	a.questsAfterBattle(ModeDuel, 1)
	if !a.ClaimQuest(&Reward{}, tut.Quest.ID) || !tut.Done {
		t.Fatalf("step 4 %+v", *tut)
	}
	// the competitive chain: 5 rounds, then again
	a.BattleRounds(ModeDuel, 1, 3, 0)
	a.BattleRounds(ModeDuel, 1, 2, 1)
	id := comp.Quest.ID
	if !a.ClaimQuest(&Reward{}, id) || comp.Done || comp.Quest.ID == id || comp.Step != 0 {
		t.Fatalf("competitive %+v", *comp)
	}
	// a finished chain's event goes
	for _, e := range a.chainLiveEvents() {
		if e.ID == tutorialChainEvent {
			t.Fatal("tutorial chain still sent")
		}
	}
}
