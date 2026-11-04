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
	if h, m := a.BotLevels(); h != 1 || m != 1 {
		t.Fatalf("bot levels %d %d", h, m)
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
	if err := a.upgradeErr(heroID(12), 1); err != nil || a.Gold != 0 || a.Level(heroID(12)) != 2 {
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
		a.Gems != 10 || a.Gold != starterGold+100 {
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
	if levels[miniID(0)] != 1 || levels[heroID(12)] != 1 || levels[miniID(9)] != 0 || levels[globalID(classSkin, 59)] != 1 {
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
	if a.Gold != gold+100 || a.Gems != 10 || len(a.Chests) != 0 || a.ChestIndex != 3 {
		t.Fatalf("gold %d gems %d chests %+v", a.Gold, a.Gems, a.Chests)
	}
}

// Daily quests: handed out per day, counted by battles and upgrades, the
// claim pushes the reward chest the client makes (source 2, the quest id).
func TestDailyQuests(t *testing.T) {
	day := time.Date(2026, 11, 4, 12, 0, 0, 0, time.Local)
	now = func() time.Time { return day }
	defer func() { now = time.Now }()
	a := NewAccount()
	a.StartProgression()
	a.refreshQuests()
	if len(a.Quests) != 3 {
		t.Fatalf("quests %+v", a.Quests)
	}
	for i := 0; i < 3; i++ {
		a.BattleResult(ModeDuel, 2, false) // played, lost: counts for PlayGameMode
	}
	a.BattleResult(ModeRumble, 2, false) // not the champion
	a.BattleResult(ModeRumble, 1, false)
	if a.Quests[0].Progress != 3 || a.Quests[1].Progress != 1 || a.Quests[2].Progress != 2 {
		t.Fatalf("progress %+v", a.Quests)
	}
	a.Quests[2].Progress = 1
	if a.ClaimQuest(&Reward{}, a.Quests[2].ID) {
		t.Fatal("claimed an unfinished quest")
	}
	// reroll: the pool's fourth quest, a new id; only once a day
	old := a.Quests[2].ID
	if _, _, ok := a.RerollQuest(old); !ok || a.Quests[2].Index != 3 || a.Quests[2].ID != a.QuestID || a.Quests[2].Progress != 0 {
		t.Fatalf("reroll %+v", a.Quests)
	}
	if _, _, ok := a.RerollQuest(a.Quests[1].ID); ok {
		t.Fatal("second reroll")
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
	if a.ClaimQuest(r, a.Quests[0].ID) {
		t.Fatal("claimed twice")
	}
	// the home: two active quests left; the pool state lists all three
	// taken indices (incl. the rerolled one's new and the claimed one)
	h := a.ownHome()
	h.home.skinStore = false
	m, err := decode(h.encode())
	if err != nil || len(m.home.quests.quests) != 2 || len(m.home.events) != 1 || m.home.events[0].Type != EventQuestPool {
		t.Fatalf("%v quests %+v events %d", err, m.home.quests.quests, len(m.home.events))
	}
	if fmt.Sprint(lastPoolState) != fmt.Sprint([]int32{dailyPoolEvent, 3, 0, 1, 3}) {
		t.Fatalf("pool state %v", lastPoolState)
	}
	// a changed pool: the saved quests point into it by index, new ones
	saved := a.Quests[1].ID
	a.QuestPool = "an older pool"
	a.refreshQuests()
	if a.Quests[1].ID <= saved || a.Quests[1].Progress != 0 || a.QuestPool != questPoolSignature() {
		t.Fatalf("pool changed %+v", a.Quests)
	}
	// next day: new quests
	day = day.AddDate(0, 0, 1)
	a.refreshQuests()
	if a.Quests[0].Claimed || a.Quests[0].Progress != 0 || a.Quests[0].ID <= 3 {
		t.Fatalf("next day %+v", a.Quests)
	}
}
