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

// The season's end: the road's tiers reached come at once; the rest is due
// for 451: the minis' stars are exchanged (back to level 1, the tier's
// chests in one chest), trophies from Diamond 3 up go back.
func TestSeasonEnd(t *testing.T) {
	Data = shopData()
	defer func() { Data = nil }()
	day := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	now = func() time.Time { return day }
	defer func() { now = time.Now }()
	a := NewAccount()
	a.StartProgression()
	old := a.season()
	old.Points = 10
	oldKey := old.Key
	for i, r := range miniSeasonalRows {
		if i < 4 {
			a.entry(miniID(r)).Level = 4 // 3 stars each: 12
			a.entry(miniID(r)).Fragments = 7
		}
	}
	a.Trophies = 950
	a.ownHome() // the avatar's season key
	chests := len(a.Chests)
	day = day.AddDate(0, 3, 0)
	m := a.ownHome()
	s := a.season()
	// the home: the road anew and the old one to claim, the rest as before
	if s.Points != 0 || a.Trophies != 950 || len(a.Chests) != chests || a.miniStars() != 12 ||
		!a.SeasonEndDue || m.avatar.seasonEnd[0] != oldKey || a.SeasonEndRoad == nil || a.SeasonEndRoad.Key != oldKey {
		t.Fatalf("points %d trophies %d chests %d stars %d due %v avatar %v: %s", s.Points, a.Trophies,
			len(a.Chests)-chests, a.miniStars(), a.SeasonEndDue, m.avatar.seasonEnd, a.LastSeasonEnd)
	}
	a.Chests, chests = nil, 0
	end, ok := a.TakeSeasonEnd()
	if !ok || end.Key != s.Key || a.Trophies != 800 || len(a.Chests) != 1 || a.SeasonEndRoad != nil || a.miniStars() != 0 {
		t.Fatalf("451 %v %+v trophies %d stars %d chests %d", ok, end, a.Trophies, a.miniStars(), len(a.Chests)-chests)
	}
	ch := a.Chests[0]
	if len(ch.List) != 4 || ch.List[3] != 127_000_137 || ch.Source != sourceSeasonEnd || ch.SourceValue != s.Key || !ch.Push {
		t.Fatalf("star chest %+v", ch)
	}
	if o := a.owned(miniID(miniSeasonalRows[0])); o.Level != 1 || o.Fragments != 0 {
		t.Fatalf("mini after the reset %+v (the client takes the fragments too)", o)
	}
	if m := a.ownHome(); m.avatar.seasonEnd != [3]int32{s.Key, 950, a.BestTrophies} {
		t.Fatalf("avatar %v", m.avatar.seasonEnd)
	}
	if _, ok := a.TakeSeasonEnd(); ok {
		t.Fatal("451 twice")
	}
	// below Diamond 3 the trophies stay; under 6 stars no star chest
	a.Trophies = 650
	a.Chests = nil
	chests = len(a.Chests)
	if msg := a.EndSeasonNow(); msg == "" {
		t.Fatal("no season end")
	}
	if _, ok := a.TakeSeasonEnd(); !ok || a.Trophies != 650 || len(a.Chests) != chests {
		t.Fatalf("451 %v trophies %d chests %d", ok, a.Trophies, len(a.Chests)-chests)
	}
}

// Dev mode's start: Diamond 1, then the season's end takes it to 800.
func TestDevStart(t *testing.T) {
	a := NewAccount()
	a.StartProgression()
	msg := a.DevStart()
	if _, ok := a.TakeSeasonEnd(); !ok || a.Trophies != 800 {
		t.Fatalf("trophies %d: %s", a.Trophies, msg)
	}
}

// 451 waits for the home's chests; dev mode does not pile season ends.
func TestSeasonEndAfterChests(t *testing.T) {
	a := NewAccount()
	a.StartProgression()
	a.Chests = append(a.Chests, PendingChest{Index: 1, Chest: chestSeasonGold50, Show: true})
	if msg := a.DevStart(); a.SeasonEndDue {
		t.Fatalf("season end with chests: %s", msg)
	}
	a.Chests = nil
	a.DevStart()
	a.Chests = append(a.Chests, PendingChest{Index: 2, Chest: chestSeasonGold50, Show: true})
	if _, ok := a.TakeSeasonEnd(); ok || !a.SeasonEndDue {
		t.Fatal("451 before the chests")
	}
	// silent exp chests do not hold it back
	a.Chests = []PendingChest{{Index: 3, Chest: upgradeRewardChest}}
	if _, ok := a.TakeSeasonEnd(); !ok {
		t.Fatal("no 451")
	}
}

// Under 6 stars the ended road's rewards come with the next home.
func TestSeasonEndRoadNextHome(t *testing.T) {
	Data = shopData()
	defer func() { Data = nil }()
	a := NewAccount()
	a.StartProgression()
	a.Chests = nil
	a.season().Points = 10
	a.EndSeasonNow()
	if a.SeasonEndRoad == nil {
		t.Fatal("no road to give")
	}
	a.ownHome()
	if len(a.Chests) != 0 {
		t.Fatal("road chest before 451")
	}
	if _, ok := a.TakeSeasonEnd(); !ok || a.SeasonEndRoad == nil {
		t.Fatal("451")
	}
	a.ownHome()
	// (the test data may not roll the road's chests: then no chest)
	if a.SeasonEndRoad != nil || (len(a.Chests) > 0 && (a.Chests[0].Source != sourceSeason || a.Chests[0].Push)) {
		t.Fatalf("road chest %+v", a.Chests)
	}
}

// "Win 10 trophies" and "Use Prince to Charge 3 enemies": open, in the
// home, counted (trophies from ranked wins; the Prince by a match of 3
// rounds with him in the deck).
func TestClashmasTrophiesAndPrince(t *testing.T) {
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
	base := clashmasBase()
	tro, prince := int32(-1), int32(-1)
	for i, g := range base {
		switch g.def.TypeID {
		case questResourceGained:
			tro = int32(i)
		case questAbilityHit:
			prince = int32(i)
		}
	}
	if tro < 0 || prince < 0 {
		t.Fatal("quests missing")
	}
	if _, ok := Data.CharRow("PrinceB1"); !ok {
		t.Fatal("PrinceB1 not in table 6")
	}
	a := NewAccount()
	a.StartProgression()
	a.Clashmas = ClashmasState{Quests: []Quest{{ID: 900, Index: tro}, {ID: 901, Index: prince}}, Taken: []int32{tro, prince}}
	h := a.ownHome()
	h.home.skinStore = false
	if _, err := decode(h.encode()); err != nil {
		t.Fatal(err)
	}
	a.questsTrophies(10)
	// the Prince in the deck
	for i, r := range miniSeasonalRows {
		if Data.CharName(miniCharRows[i]) == "PrinceB1" {
			a.Decks[a.Selected][0] = r
		}
	}
	a.BattleRounds(ModeDuel, 2, 1, 2)
	if a.Clashmas.Quests[0].Progress != 10 || a.Clashmas.Quests[1].Progress != 1 {
		t.Fatalf("%+v", a.Clashmas.Quests)
	}
	j := base[prince].def.json()
	if j["HitsReqInOneGame"] != 3 || j["Unit"] != "PrinceB1" || base[tro].def.json()["ResourceData"] != "RankPoints" {
		t.Fatalf("json %v", j)
	}
}
