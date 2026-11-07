package home

import (
	"fmt"
	"math/rand"
	"testing"
	"time"
)

func TestProgressionStartAndUpgrade(t *testing.T) {
	a := NewAccount()
	a.Trophies, a.Exp = 300, 50
	if msg := a.SetProgression("fresh"); msg == "" || !a.Progression {
		t.Fatalf("fresh: %q", msg)
	}
	if a.Trophies != 0 || a.Exp != 0 || a.Gold != starterGold {
		t.Fatalf("not reset: %+v", a)
	}
	if a.Level(miniID(0)) != 1 || a.Level(miniID(3)) != 0 || a.Level(heroID(12)) != 1 || a.Level(heroID(11)) != 0 {
		t.Fatal("starter collection wrong")
	}
	if h, m := a.BotLevels(); h != 0 || m != 0 {
		t.Fatalf("bot levels %d %d, want the decks' own (0 0)", h, m)
	}

	// Mini: unlock needs 1 fragment, no gold.
	if err := a.upgradeErr(miniID(3), 0); err == nil {
		t.Fatal("unlock without fragments")
	}
	a.entry(miniID(3)).Fragments = 1
	if err := a.upgradeErr(miniID(3), 0); err != nil || a.Level(miniID(3)) != 1 {
		t.Fatalf("unlock: %v", err)
	}
	if err := a.upgradeErr(miniID(3), 0); err == nil {
		t.Fatal("same level twice")
	}
	// Hero 1 -> 2: 10 fragments and 100 gold.
	a.entry(heroID(12)).Fragments = 10
	if err := a.upgradeErr(heroID(12), 1); err != nil || a.Gold != starterGold-100 || a.Level(heroID(12)) != 2 {
		t.Fatalf("hero upgrade: %v gold %d", err, a.Gold)
	}

	// The client unlocks a hero by itself and then asks for level 1 -> 2:
	// both levels are paid here.
	a.entry(heroID(14)).Fragments = 30
	a.Gold = 100
	if err := a.upgradeErr(heroID(14), 1); err != nil || a.Level(heroID(14)) != 2 ||
		a.owned(heroID(14)).Fragments != 10 || a.Gold != 0 {
		t.Fatalf("catch-up: %v %+v gold %d", err, a.owned(heroID(14)), a.Gold)
	}

	// switching back and forth keeps the progress
	a.SetProgression("unlocked")
	if a.Level(miniID(3)) != maxMiniLevel {
		t.Fatal("unlocked mode not maxed")
	}
	if msg := a.SetProgression("fresh"); msg != "progression resumed" || a.Level(heroID(12)) != 2 {
		t.Fatalf("resume: %q", msg)
	}
}

func TestRewards(t *testing.T) {
	a := NewAccount()
	a.StartProgression()
	a.rng = rand.New(rand.NewSource(1))
	// the original: a win brings gold only
	if o := a.BattleResult(ModeDuel, 1, false); o.Reward.Gold != 12 || len(o.Reward.Chests) != 0 || len(a.Chests) != 0 {
		t.Fatalf("original win reward %+v", o.Reward)
	}
	a.Gold -= 12
	a.GoldToday -= 12
	WinFragments = true
	defer func() { WinFragments = false }()
	o := a.BattleResult(ModeDuel, 1, false) // Bronze rank 0: 12 gold
	if o.Reward.Gold != 12 || len(o.Reward.Chests) != 1 || len(a.Chests) != 1 ||
		len(a.Chests[0].Content.Fragments) == 0 || a.Gold != starterGold+12 {
		t.Fatalf("win reward %+v gold %d", o.Reward, a.Gold)
	}
	// the fragments come when the chest is opened (550)
	r, chest, ok := a.OpenPending(a.Chests[0].Index)
	if !ok || chest != winChest || len(r.Fragments) == 0 {
		t.Fatalf("open %v %d %+v", ok, chest, r)
	}
	if _, _, ok := a.OpenPending(99); ok {
		t.Fatal("opened an unknown chest")
	}
	o = a.BattleResult(ModeDuel, 2, true)
	if o.Reward.Gold != 5 || len(o.Reward.Chests) != 0 || o.Trophies != 1 {
		t.Fatalf("loss reward %+v", o)
	}
	if a.Exp != 0 {
		t.Fatalf("battles gave exp: %d", a.Exp)
	}
	// fragments never exceed what is still needed
	for i := 0; i < 2000; i++ {
		a.BattleResult(ModeRumble, 1, false)
		if i%3 == 0 { // some chests stay unopened for a while
			for len(a.Chests) > 0 {
				a.OpenPending(a.Chests[0].Index)
			}
		}
	}
	for len(a.Chests) > 0 {
		a.OpenPending(a.Chests[0].Index)
	}
	for _, id := range collectibleIDs() {
		if a.fragmentsMissing(id) < 0 {
			t.Fatalf("%d over-filled", id)
		}
	}
}

func TestUpgradeExpAndLevelChest(t *testing.T) {
	a := NewAccount()
	a.StartProgression()
	openAll := func() *Reward {
		all := &Reward{}
		for len(a.Chests) > 0 {
			r, _, _ := a.OpenPending(a.Chests[0].Index)
			all.Exp += r.Exp
			all.Gold += r.Gold
			all.Gems += r.Gems
			all.LevelUps = append(all.LevelUps, r.LevelUps...)
		}
		return all
	}
	// five mini upgrades 1 -> 2: five exp chests; opened, 5 exp = player
	// level 2, whose chest comes when the fifth is opened
	for _, r := range starterMinis {
		a.entry(miniID(r)).Fragments = 1
		rw, err := a.Upgrade(miniID(r), 1)
		if err != nil || rw.Exp != 0 || len(rw.Chests) != 1 || rw.Chests[0] != upgradeRewardChest {
			t.Fatalf("upgrade: %v %+v", err, rw)
		}
	}
	if a.Exp != 0 || len(a.Chests) != 5 {
		t.Fatalf("exp before opening %d, chests %d", a.Exp, len(a.Chests))
	}
	all := openAll()
	if l, _ := ExpLevel(a.Exp); l != 2 || all.Exp != 5 || len(all.LevelUps) != 1 ||
		a.Gems != starterGems+10 || a.Gold != starterGold+100 {
		t.Fatalf("level %d %+v gems %d gold %d", l, all, a.Gems, a.Gold)
	}
	// the unlock gives no exp
	a.entry(miniID(3)).Fragments = 1
	if rw, _ := a.Upgrade(miniID(3), 0); len(rw.Chests) != 0 {
		t.Fatalf("unlock chest %+v", rw)
	}
}

// Chests and the daily gold state in the home, as the client decodes them.
func TestChestsAndDailyGoldInHome(t *testing.T) {
	day := time.Date(2026, 11, 3, 23, 0, 0, 0, time.Local)
	now = func() time.Time { return day }
	defer func() { now = time.Now }()
	a := NewAccount()
	a.StartProgression()
	a.matchGold(&Reward{}, 30)
	a.queueChest(nil, winChest, ChestContent{Fragments: map[int32]int32{miniID(7): 2}})
	a.queueChestFrom(nil, 127_000_184, sourcePlayerLevel, ChestContent{Resources: map[string]int32{"Gold": 100, "Diamonds": 10}})
	h := a.ownHome()
	h.home.skinStore = false
	m, err := decode(h.encode())
	if err != nil {
		t.Fatal(err)
	}
	d := m.home.daily
	if d.left != 170 || d.ticksLeft != 3600*20 || d.weekday != d.day%7+1 || m.home.chestIndex != 2 {
		t.Fatalf("daily %+v, chest index %d", d, m.home.chestIndex)
	}
	if len(m.home.chests) != 2 || m.home.chests[0].Index != 1 || m.home.chests[0].Content.Fragments[miniID(7)] != 2 ||
		m.home.chests[1].Chest != 127_000_184 || m.home.chests[1].Content.Resources["Diamonds"] != 10 ||
		!m.home.chests[0].Show || !m.home.chests[1].Show || m.home.chests[1].Source != sourcePlayerLevel {
		t.Fatalf("chests %+v", m.home.chests)
	}
}

func TestProgressionHome(t *testing.T) {
	a := NewAccount()
	a.StartProgression()
	a.entry(miniID(9)).Fragments = 1
	h := a.ownHome()
	h.home.skinStore = false
	m, err := decode(h.encode())
	if err != nil {
		t.Fatal(err)
	}
	if len(m.home.minis) != 5 || len(m.home.heroes) != 1 {
		t.Fatalf("owned: %d minis, %d heroes", len(m.home.minis), len(m.home.heroes))
	}
	if len(m.home.map228) != 1 || m.home.map228[0] != [2]int32{miniID(9), 1} {
		t.Fatalf("fragments %v", m.home.map228)
	}
	levels := map[int32]int32{}
	for _, kv := range m.home.collectibleLevels {
		levels[kv[0]] = kv[1]
	}
	// skins: only the owned ones (at the start the four older ones, 0095)
	if levels[miniID(0)] != 1 || levels[heroID(12)] != 1 || levels[miniID(9)] != 0 || levels[globalID(classSkin, 73)] != 0 || levels[globalID(classSkin, 59)] != 1 {
		t.Fatalf("levels %v", levels)
	}
	if g := m.avatar.slots[0]; len(g) != 1 || g[0] != (dataSlot{classRESOURCES, resourceGold, starterGold}) {
		t.Fatalf("gold slot %v", g)
	}
}

func TestResetToken(t *testing.T) {
	a := NewAccount()
	if _, err := a.ApplyProgression("reset:1"); err != nil || !a.Progression {
		t.Fatal("reset")
	}
	a.Gold = 999
	a.ApplyProgression("reset:1") // same build started again
	if a.Gold != 999 {
		t.Fatal("reset twice with the same token")
	}
	a.ApplyProgression("reset:2")
	if a.Gold != starterGold {
		t.Fatal("new token did not reset")
	}
}

func (a *Account) upgradeErr(id, level int32) error {
	_, err := a.Upgrade(id, level)
	return err
}

// Fragments are spread evenly, the selected deck gets no extra share (5 of
// the 22 minis).
func TestFragmentsEven(t *testing.T) {
	a := NewAccount()
	a.StartProgression()
	a.rng = rand.New(rand.NewSource(2))
	deck, other := 0, 0
	for i := 0; i < 200; i++ {
		frag := map[int32]int32{}
		a.giveFragments(a.rng, frag, 2, false, 1)
		for id, n := range frag {
			isDeck := false
			for _, row := range a.Decks[a.Selected] {
				isDeck = isDeck || miniID(row) == id
			}
			if isDeck {
				deck += int(n)
			} else {
				other += int(n)
			}
		}
		// keep the deck needing fragments
		for _, row := range a.Decks[a.Selected] {
			a.entry(miniID(row)).Fragments = 0
		}
		for id := range a.Collection {
			if a.Collection[id].Level == 0 {
				a.Collection[id].Fragments = 0
			}
		}
	}
	if share := float64(deck) / float64(deck+other); share < 0.15 || share > 0.32 {
		t.Fatalf("deck %d, other %d", deck, other)
	}
}

// Opening a chest upgrades what its fragments pay for, as the client does;
// the client makes the exp chest of each level and the level chest of each
// player level itself, with the next indexes, and gets their content pushed
// (420).
func TestChestAutoUpgrade(t *testing.T) {
	a := NewAccount()
	a.StartProgression()
	a.entry(heroID(13)).Fragments = 25 // unlock 10, then level 2 costs gold
	a.Exp = 4                          // one more is player level 2
	a.queueChest(nil, winChest, ChestContent{Fragments: map[int32]int32{miniID(0): 2, miniID(7): 1}})
	if len(a.TakePushes()) != 0 || a.Chests[0].Index != 1 {
		t.Fatalf("win chest %+v", a.Chests)
	}
	r, _, _ := a.OpenPending(1)
	// knight 1 -> 2 (1 fragment; 2 -> 3 needs 2), mini 7 and the queen unlocked
	if a.Level(miniID(0)) != 2 || a.owned(miniID(0)).Fragments != 1 || a.Level(miniID(7)) != 1 ||
		a.Level(heroID(13)) != 1 || a.owned(heroID(13)).Fragments != 15 || len(r.Upgraded) != 1 {
		t.Fatalf("%s", r)
	}
	p := a.TakePushes()
	if len(p) != 1 || p[0].Index != 2 || p[0].Chest != upgradeRewardChest || p[0].Source != sourceUpgrade {
		t.Fatalf("pushes %+v", p)
	}
	gold := a.Gold
	r, _, _ = a.OpenPending(2) // exp 5: level 2; its chest now, the client made it
	p = a.TakePushes()
	if a.Exp != 5 || len(r.LevelUps) != 1 || len(p) != 1 || p[0].Index != 3 ||
		p[0].Source != sourcePlayerLevel || p[0].SourceValue != 2 || !p[0].Show {
		t.Fatalf("%s; pushes %+v", r, p)
	}
	a.OpenPending(3)
	if a.Gold != gold+100 || a.Gems != starterGems+10 || len(a.Chests) != 0 || a.ChestIndex != 3 {
		t.Fatalf("gold %d gems %d chests %+v", a.Gold, a.Gems, a.Chests)
	}
}

// Daily quests: six a day, all shown at once, counted by battles and the
// road; the claim pushes the reward chest the client makes (source 2, the
// quest id); one reroll takes the pool's last quest.
func TestDailyQuests(t *testing.T) {
	catalog := questCatalog
	defer func() { questCatalog = catalog }()
	win := questDef{"WinWithUnit", questWinWithUnit, "TID_CHRONOS_QUESTS_WINWITHUNIT", 2, rewardSmall, map[string]any{"UseRoundWins": false}}
	road := questDef{"MetaSeasonRoadLevel", questSeasonRoadLevel, "TID_CHRONOS_QUESTS_SEASONPASSLEVELUP", 2, rewardSmall, nil}
	play := questDef{"PlayWithUnit", questPlayWithUnit, "TID_CHRONOS_QUESTS_PLAYWITHUNIT", 4, rewardSmall, map[string]any{}}
	var winRumble questDef
	for _, d := range catalog {
		if d.TypeID == questWinRumble {
			winRumble = d
		}
	}
	questCatalog = []questDef{playDuels(3, rewardSmall), winRumble, win, road, playRumbles(2, rewardSmall), play, playDuels(5, rewardBig)}
	day := time.Date(2026, 11, 4, 12, 0, 0, 0, time.Local)
	now = func() time.Time { return day }
	defer func() { now = time.Now }()
	a := NewAccount()
	a.StartProgression()
	a.refreshQuests()
	if len(a.Quests) != questsPerDay || a.Quests[0].Index != 0 || a.Quests[5].Index != 5 {
		t.Fatalf("quests %+v", a.Quests)
	}
	for i := 0; i < 3; i++ {
		a.BattleResult(ModeDuel, 2, false) // played, lost
	}
	a.BattleResult(ModeRumble, 2, false) // won (top 4), not the champion
	a.BattleResult(ModeRumble, 1, false)
	// duels 3/3, champion 1/1, win 2/2, rumbles 2/2, play 4/4 (5 battles)
	if a.Quests[0].Progress != 3 || a.Quests[1].Progress != 1 || a.Quests[2].Progress != 2 ||
		a.Quests[4].Progress != 2 || a.Quests[5].Progress != 4 {
		t.Fatalf("progress %+v", a.Quests)
	}
	// the road: 2 tiers
	a.applyContent(&Reward{}, ChestContent{Resources: map[string]int32{"SeasonPoints": 2}})
	if a.Quests[3].Progress != 2 {
		t.Fatalf("road %+v", a.Quests[3])
	}
	a.Quests[2].Progress = 1
	if a.ClaimQuest(&Reward{}, a.Quests[2].ID) {
		t.Fatal("claimed an unfinished quest")
	}
	// the home lists every index but the reroll's as taken
	if fmt.Sprint(a.questPoolTaken()) != "[0 1 2 3 4 5]" {
		t.Fatalf("taken %v", a.questPoolTaken())
	}
	// reroll: the pool's last quest, a new id; only once a day
	old := a.Quests[2]
	if _, _, ok := a.RerollQuest(old.ID); !ok || a.Quests[2].Index != questRerollIndex || a.Quests[2].ID != a.QuestID || a.Quests[2].Progress != 0 {
		t.Fatalf("reroll %+v", a.Quests)
	}
	if _, _, ok := a.RerollQuest(a.Quests[1].ID); ok {
		t.Fatal("second reroll")
	}
	if fmt.Sprint(a.questPoolTaken()) != "[0 1 3 4 5 6]" {
		t.Fatalf("taken after the reroll %v", a.questPoolTaken())
	}
	a.TakePushes()
	r := &Reward{}
	if !a.ClaimQuest(r, a.Quests[0].ID) || !a.Quests[0].Claimed {
		t.Fatal("claim failed")
	}
	p := a.TakePushes()
	if len(p) != 1 || p[0].Source != sourceQuest || p[0].SourceValue != a.Quests[0].ID || p[0].Index != a.ChestIndex {
		t.Fatalf("pushes %+v", p)
	}
	if a.ClaimQuest(r, a.Quests[0].ID) || len(a.Quests) != questsPerDay {
		t.Fatalf("claimed twice or a seventh quest %+v", a.Quests)
	}
	// the home: five open quests; the pool state as questPoolTaken
	h := a.ownHome()
	h.home.skinStore = false
	m, err := decode(h.encode())
	if err != nil || len(m.home.quests.quests) != 5 || m.home.events[0].Type != EventQuestPool || m.home.events[1].Type != EventSeasonData || m.home.events[2].Type != EventShopLayout {
		t.Fatalf("%v quests %+v events %d", err, m.home.quests.quests, len(m.home.events))
	}
	if fmt.Sprint(lastPoolState) != fmt.Sprint([]int32{dailyPoolEvent, 6, 0, 1, 3, 4, 5, 6}) {
		t.Fatalf("pool state %v", lastPoolState)
	}
	// a changed pool: the saved quests point into it by index, new ones
	a.QuestPool = "an older pool"
	a.refreshQuests()
	if len(a.Quests) != questsPerDay || a.Quests[0].Claimed || a.QuestPool != questPoolSignature() {
		t.Fatalf("pool changed %+v", a.Quests)
	}
	// next day: new quests
	day = day.AddDate(0, 0, 1)
	a.refreshQuests()
	if a.Quests[0].Claimed || a.Quests[0].Progress != 0 || a.QuestRerolls != 0 {
		t.Fatalf("next day %+v", a.Quests)
	}
}

// The client's own progress (battle kinds) comes from the hook: up only, to
// the target at most; claimed and unknown quests stay as they are.
func TestQuestsFromClient(t *testing.T) {
	catalog := questCatalog
	defer func() { questCatalog = catalog }()
	ko := questDef{"TotalKOs", questTotalKOs, "TID_CHRONOS_QUESTS_TOTALKOS", 15, rewardSmall, map[string]any{}}
	rounds := questDef{"WinWithUnit", questWinWithUnit, "TID_CHRONOS_QUESTS_WINROUNDSWITHUNIT", 9, rewardBig, map[string]any{"UseRoundWins": true}}
	questCatalog = []questDef{ko, rounds, playDuels(3, rewardSmall), playRumbles(2, rewardSmall),
		{"WinRoundNoDeath", questWinRoundNoDeath, "TID_CHRONOS_QUESTS_WINROUNDNODEATH", 2, rewardSmall, nil},
		{"WinGameNoReroll", questWinGameNoReroll, "TID_CHRONOS_QUESTS_WINGAMENOREROLL", 1, rewardSmall, nil},
		{"WinDuelPerfect", questWinDuelPerfect, "TID_CHRONOS_QUESTS_WINDUELPERFECT", 1, rewardBig, nil}}
	day := time.Date(2026, 11, 4, 12, 0, 0, 0, time.Local)
	now = func() time.Time { return day }
	defer func() { now = time.Now }()
	a := NewAccount()
	a.StartProgression()
	a.refreshQuests()
	ids := map[string]int32{}
	for _, q := range a.Quests {
		ids[questCatalog[q.Index].Text] = q.ID
	}
	// a won duel does not count for "win 9 rounds" on the server
	a.BattleResult(ModeDuel, 1, false)
	if q := a.Quests[1]; q.Progress != 0 {
		t.Fatalf("rounds counted by the server %+v", q)
	}
	changed := a.QuestsFromClient([][2]int32{{ids["TID_CHRONOS_QUESTS_TOTALKOS"], 7}, {ids["TID_CHRONOS_QUESTS_WINROUNDSWITHUNIT"], 99}, {999, 5},
		{ids["TID_CHRONOS_QUESTS_PLAY_DUEL"], 0}})
	if len(changed) != 2 || a.Quests[0].Progress != 7 || a.Quests[1].Progress != 9 || a.Quests[2].Progress != 1 {
		t.Fatalf("%v %+v", changed, a.Quests)
	}
	// claimed stays claimed and unchanged
	a.ClaimQuest(&Reward{}, a.Quests[1].ID)
	if c := a.QuestsFromClient([][2]int32{{a.Quests[1].ID, 3}}); len(c) != 0 || a.Quests[1].Progress != 9 {
		t.Fatalf("claimed changed %v", c)
	}
	// the home writes all the kinds
	h := a.ownHome()
	h.home.skinStore = false
	if m, err := decode(h.encode()); err != nil || len(m.home.quests.quests) != 5 {
		t.Fatalf("%v %+v", err, m.home.quests.quests)
	}
}

// Rounds the hook read: won rounds count for "win N rounds", a duel won
// 3:0 for WinDuelPerfect; a lost round or an unknown count (-1) does not.
func TestBattleRounds(t *testing.T) {
	catalog := questCatalog
	defer func() { questCatalog = catalog }()
	rounds := questDef{"WinWithUnit", questWinWithUnit, "TID_CHRONOS_QUESTS_WINROUNDSWITHUNIT", 9, rewardBig, map[string]any{"UseRoundWins": true}}
	win := questDef{"WinWithUnit", questWinWithUnit, "TID_CHRONOS_QUESTS_WINWITHUNIT", 2, rewardSmall, map[string]any{"UseRoundWins": false}}
	perfect := questDef{"WinDuelPerfect", questWinDuelPerfect, "TID_CHRONOS_QUESTS_WINDUELPERFECT", 1, rewardBig, nil}
	questCatalog = []questDef{rounds, win, perfect, playDuels(3, rewardSmall), playDuels(5, rewardBig), playRumbles(2, rewardSmall), playRumbles(3, rewardBig)}
	day := time.Date(2026, 11, 4, 12, 0, 0, 0, time.Local)
	now = func() time.Time { return day }
	defer func() { now = time.Now }()
	a := NewAccount()
	a.StartProgression()
	a.refreshQuests()
	a.BattleRounds(ModeDuel, 1, 3, 1) // won 3:1
	a.BattleRounds(ModeDuel, 1, 3, -1)
	a.BattleRounds(ModeDuel, 2, 2, 3)
	if a.Quests[0].Progress != 8 || a.Quests[1].Progress != 0 || a.Quests[2].Progress != 0 {
		t.Fatalf("progress %+v", a.Quests)
	}
	a.BattleRounds(ModeDuel, 1, 3, 0) // 3:0
	if a.Quests[0].Progress != 9 || a.Quests[2].Progress != 1 {
		t.Fatalf("progress %+v", a.Quests)
	}
}
