package home

import (
	"os"
	"testing"

	"cmserver/proto"
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
	own := func(name string) {
		row, _ := Data.CharRow(name)
		for i, r := range miniCharRows {
			if r == row {
				a.entry(miniID(miniSeasonalRows[i])).Level = 1
			}
		}
	}
	// no tutorial chain before the player has one of step 1's minis
	a.Collection = map[int32]*Owned{}
	a.ownHome()
	if a.chain(tutorialChainEvent) != nil {
		t.Fatal("tutorial chain without Magic Archer or Shield Skeleton")
	}
	// it starts during the session once the player has one (the events
	// again with 417, as for a new mini's quests)
	own("ShieldSkeletonB1")
	if _, ok := a.NewMiniQuests(); !ok || a.chain(tutorialChainEvent) == nil {
		t.Fatal("tutorial chain not started during the session")
	}
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
	// step 1's mini stays the one picked, Shield Skeleton, after the
	// player gets Magic Archer too; Magic Archer in the deck does not do it
	if tut.Variant != 2 {
		t.Fatalf("variant %d", tut.Variant)
	}
	own("EliteArcherB1")
	archer, _ := Data.CharRow("EliteArcherB1")
	for i, r := range miniCharRows {
		if r == archer {
			a.Swap(miniSeasonalRows[i], 0, -1)
		}
	}
	if len(a.TakeQuestUpdates()) != 0 || tutorialSteps(a)[0].Own["RequirementMini"] != "ShieldSkeletonB1" {
		t.Fatal("Magic Archer did Shield Skeleton's step")
	}
	// step 1: done when the deck gets its mini, at once (351); a battle
	// alone does not do it
	a.questsAfterBattle(ModeDuel, 2)
	if a.ClaimQuest(&Reward{}, tut.Quest.ID) {
		t.Fatal("step 1 done without its mini")
	}
	name := tutorialSteps(a)[0].Own["RequirementMini"].(string)
	row, _ := Data.CharRow(name)
	for i, r := range miniCharRows {
		if r == row {
			a.Swap(miniSeasonalRows[i], 0, -1)
		}
	}
	l := a.TakeQuestUpdates()
	if len(l) != 1 || l[0].q.ID != tut.Quest.ID || tut.Quest.Progress != 1 {
		t.Fatalf("351 %+v %+v", l, *tut)
	}
	w := proto.NewWriter()
	a.EncodeQuestUpdate(w, l)
	r := proto.NewReader(w.Bytes())
	for i := 0; i < 5; i++ {
		r.VInt()
	}
	if !r.Bool() || r.VInt() != 1 {
		t.Fatal("351 head")
	}
	SkipQuest(r)
	if r.Err() != nil || r.Remaining() != 0 {
		t.Fatalf("351 quest %v", r.Err())
	}
	// step 2: a match using Wizard, counted by the client; the next step
	// with the next id
	for step := int32(1); step <= 2; step++ {
		first := tut.Quest.ID
		if step == 2 {
			a.QuestsFromClient([][2]int32{{first, 1}})
		}
		if !a.ClaimQuest(&Reward{}, first) || tut.Step != step || tut.Quest.ID != a.QuestID || tut.Quest.ID == first {
			t.Fatalf("step %d %+v", step, *tut)
		}
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
	if !a.ClaimQuest(&Reward{}, id) || comp.Done || comp.Quest.ID != a.QuestID || comp.Step != 0 || comp.Quest.Progress != 0 {
		t.Fatalf("competitive %+v", *comp)
	}
	// a finished chain's event goes
	for _, e := range a.chainLiveEvents() {
		if e.ID == tutorialChainEvent {
			t.Fatal("tutorial chain still sent")
		}
	}
}
