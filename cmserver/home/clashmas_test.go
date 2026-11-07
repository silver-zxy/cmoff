package home

import (
	"encoding/json"
	"os"
	"testing"
)

// The Clashmas group: three quests open, sent in the group list with the
// group's state and event; a duel counts for its quests; a claim counts as
// done and hands out the next index; the Candy Elf after 12; the list
// starts over once all were handed out.
func TestClashmas(t *testing.T) {
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
	// the units of the quests are in table 6
	for _, g := range clashmasBase() {
		if name, _ := g.def.Own["RequirementMini"].(string); name != "" && g.def.req() == 0 {
			t.Fatalf("%s not in table 6", name)
		}
	}
	a := NewAccount()
	a.StartProgression()
	h := a.ownHome()
	h.home.skinStore = false
	events := h.home.events
	if _, err := decode(h.encode()); err != nil {
		t.Fatal(err)
	}
	if len(lastGroupQuests) != clashmasPick || len(lastGroups) != 1 || lastGroups[0].event != clashmasEvent ||
		len(lastGroups[0].taken) != clashmasPick {
		t.Fatalf("group quests %+v groups %+v", lastGroupQuests, lastGroups)
	}
	var ev *LiveEvent
	for i := range events {
		if events[i].ID == clashmasEvent {
			ev = &events[i]
		}
	}
	var p struct {
		Quests     []map[string]any
		PickLimit  int
		Milestones []map[string]any
	}
	if ev == nil || ev.Type != EventQuestGroup || json.Unmarshal([]byte(ev.Parameters), &p) != nil ||
		len(p.Quests) != len(clashmasQuests()) || p.PickLimit != clashmasPick || p.Milestones[0]["Chest"] != "shop_skin_SpearGoblinElf" {
		t.Fatalf("event %+v", ev)
	}

	// six duels finish "play 6 duels"
	for i := 0; i < 6; i++ {
		a.questsAfterBattle(ModeDuel, 2)
	}
	q := a.Clashmas.Quests[0]
	if q.Index != 0 || q.Progress != 6 {
		t.Fatalf("%+v", a.Clashmas.Quests)
	}
	if !a.ClaimQuest(&Reward{}, q.ID) || a.Clashmas.Done != 1 {
		t.Fatalf("claim: %+v", a.Clashmas)
	}
	if n := len(a.Clashmas.Quests); n != clashmasPick || a.Clashmas.Quests[n-1].Index != clashmasPick || a.Clashmas.Quests[n-1].ID != a.QuestID {
		t.Fatalf("next %+v", a.Clashmas.Quests)
	}
	// the Candy Elf: not before 12
	if a.ClaimMilestone(&Reward{}, clashmasEvent, 0) == nil {
		t.Fatal("milestone with 1 done")
	}
	list := clashmasQuests()
	for k := 0; k < 2*len(list); k++ {
		q := &a.Clashmas.Quests[0]
		q.Progress = list[q.Index].def.Target
		if !a.ClaimQuest(&Reward{}, q.ID) {
			t.Fatalf("claim %d %+v", k, a.Clashmas.Quests)
		}
		if len(a.Clashmas.Quests) != clashmasPick {
			t.Fatalf("after %d claims %+v", k, a.Clashmas)
		}
	}
	if err := a.ClaimMilestone(&Reward{}, clashmasEvent, 0); err != nil {
		t.Fatal(err)
	}
	if a.ClaimMilestone(&Reward{}, clashmasEvent, 0) == nil {
		t.Fatal("milestone twice")
	}
}

// Skins as in the original: locked once for an account with progression,
// a worn locked skin back to the default; a chest's skin is owned, can be
// worn and is in the home's skin store.
func TestSkinsLocked(t *testing.T) {
	a := NewAccount()
	a.StartProgression()
	crystal := globalID(classSkin, 35)
	a.Skins = map[int32]int32{36000017: crystal}
	if !a.lockSkins() || len(a.Skins) != 0 || a.lockSkins() {
		t.Fatalf("lock %+v", a.Skins)
	}
	a.SelectSkin(crystal)
	if len(a.Skins) != 0 {
		t.Fatal("a locked skin was worn")
	}
	a.addFragments(&Reward{}, crystal, 1)
	a.SelectSkin(crystal)
	if a.Skins[36000017] != crystal || !a.ownedSkinSet()[crystal] || len(a.ownedSkinSet()) != 1+len(betaSkins) {
		t.Fatalf("owned %+v worn %+v", a.OwnedSkins, a.Skins)
	}
}

// Dev mode: the currencies at their top, the road's fuel, every quest
// done, with every home.
func TestDevMode(t *testing.T) {
	Dev = true
	defer func() { Dev = false }()
	a := NewAccount()
	a.StartProgression()
	a.Gold = 5
	a.ownHome()
	if a.Gold != devGold || a.Gems != devGems || a.CrystalCoins != devCoins ||
		a.Season.Points != seasonTiers[len(seasonTiers)-1].Points {
		t.Fatalf("gold %d gems %d coins %d fuel %d", a.Gold, a.Gems, a.CrystalCoins, a.Season.Points)
	}
	pool := todaysQuests()
	for _, q := range a.Quests {
		if q.Progress != pool[q.Index].Target {
			t.Fatalf("quest not done %+v", q)
		}
	}
	if AccountFile(true) == AccountFile(false) {
		t.Fatal("dev mode shares the account")
	}
}
