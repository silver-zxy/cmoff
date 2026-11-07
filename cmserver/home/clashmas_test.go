package home

import (
	"encoding/json"
	"os"
	"testing"
	"time"
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
	// the list: the known quests, the first ones again up to 12, then the
	// Crystal Coin ones; then it ends
	list := clashmasQuests()
	if len(list) <= int(clashmasDoneReq) || list[len(clashmasBase())].def.Kind != list[0].def.Kind ||
		list[clashmasDoneReq].def.Rewards[0] != chestCoins5 || list[len(list)-1].def.Rewards[0] != chestCoins5 {
		t.Fatalf("list of %d", len(list))
	}
	for len(a.Clashmas.Quests) > 0 {
		q := &a.Clashmas.Quests[0]
		q.Progress = list[q.Index].def.Target
		if !a.ClaimQuest(&Reward{}, q.ID) {
			t.Fatalf("claim %+v", a.Clashmas.Quests)
		}
	}
	if a.Clashmas.Done != int32(len(list)) {
		t.Fatalf("done %d of %d", a.Clashmas.Done, len(list))
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

// The season's end: the road's tiers reached come, the minis' stars are
// exchanged (back to level 1, the tier's chests), trophies from Diamond 3
// up go back; a new season starts.
func TestSeasonEnd(t *testing.T) {
	Data = shopData()
	defer func() { Data = nil }()
	day := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	now = func() time.Time { return day }
	defer func() { now = time.Now }()
	a := NewAccount()
	a.StartProgression()
	a.season().Points = 10
	for i, r := range miniSeasonalRows {
		if i < 4 {
			a.entry(miniID(r)).Level = 4 // 3 stars each: 12
		}
	}
	a.Trophies = 950
	chests := len(a.Chests)
	day = day.AddDate(0, 3, 0)
	s := a.season()
	if s.Points != 0 || a.Trophies != 800 || len(a.Chests) != chests+2 || a.miniStars() != 0 {
		t.Fatalf("points %d trophies %d chests %d stars %d: %s", s.Points, a.Trophies, len(a.Chests)-chests, a.miniStars(), a.LastSeasonEnd)
	}
	if a.owned(miniID(miniSeasonalRows[0])).Level != 1 {
		t.Fatal("stars kept")
	}
	// below Diamond 3 the trophies stay; under 6 stars no star chest
	a.Trophies = 650
	if msg := a.EndSeasonNow(); a.Trophies != 650 || msg == "" {
		t.Fatalf("trophies %d: %s", a.Trophies, msg)
	}
}
