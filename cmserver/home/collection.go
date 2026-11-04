package home

import (
	"fmt"
	"math/rand"
	"sort"
)

// Progression: instead of the fully unlocked collection, the account owns
// what it has earned. Minis and heroes are collectibles (chests/
// collectibles.json) with levels; going from level L to L+1 ("unlock" at 0)
// takes fragments and, for heroes, gold. The client does it with
// LogicUnlockCollectableCommand 602 (id, current level; execute 0x6c39d4,
// apply 0x7eadc4) and the server repeats it on the saved account.
//
// In the home, the collection object (+0x200) keeps two maps: fragments per
// collectible (+0x228) and levels (+0x250). Gold is the avatar's first data
// slot list (resource Gold, read by 0x598d98 for the HUD).

// Owned is one collectible's progress. Level 0 = not owned yet.
type Owned struct {
	Level     int32 `json:"level"`
	Fragments int32 `json:"fragments"`
}

const (
	maxMiniLevel = 4
	maxHeroLevel = 10
)

// Upgrade costs per current level (collectibles.json: every mini has the
// same table, every hero too).
var (
	miniFragments = []int32{1, 1, 2, 3}
	heroFragments = []int32{10, 10, 25, 50, 75, 100, 150, 200, 250, 300}
	heroGold      = []int32{0, 100, 200, 400, 800, 1200, 1600, 2000, 2500, 3000}
)

// Starter collection: the starter deck's minis (Knight, Barbarian, Archer,
// Spear Goblin, Wizard) and the Barbarian King, all at level 1.
var (
	starterMinis       = []int32{0, 1, 4, 5, 6} // seasonal rows
	starterHero  int32 = 12                     // support_heroes row
	starterGold  int32 = 100
)

func miniID(seasonalRow int32) int32 { return globalID(classSEASONAL, seasonalRow) }
func heroID(row int32) int32         { return globalID(classSupportHero, row) }

// collectibleIDs: every mini and hero the server knows, minis first.
func collectibleIDs() []int32 {
	var ids []int32
	for _, r := range miniSeasonalRows {
		ids = append(ids, miniID(r))
	}
	for _, r := range heroRows {
		ids = append(ids, heroID(r))
	}
	return ids
}

func isHero(id int32) bool { return id/1_000_000 == classSupportHero }

func maxLevel(id int32) int32 {
	if isHero(id) {
		return maxHeroLevel
	}
	return maxMiniLevel
}

// upgradeCost: fragments and gold to go from level to level+1.
func upgradeCost(id, level int32) (fragments, gold int32, ok bool) {
	if level < 0 || level >= maxLevel(id) {
		return 0, 0, false
	}
	if isHero(id) {
		return heroFragments[level], heroGold[level], true
	}
	return miniFragments[level], 0, true
}

// fragmentsMissing: how many fragments it still needs to reach max level.
func (a *Account) fragmentsMissing(id int32) int32 {
	o := a.owned(id)
	var n int32
	for l := o.Level; l < maxLevel(id); l++ {
		f, _, _ := upgradeCost(id, l)
		n += f
	}
	return n - o.Fragments
}

func (a *Account) owned(id int32) Owned {
	if o := a.Collection[id]; o != nil {
		return *o
	}
	return Owned{}
}

func (a *Account) entry(id int32) *Owned {
	if a.Collection == nil {
		a.Collection = map[int32]*Owned{}
	}
	o := a.Collection[id]
	if o == nil {
		o = &Owned{}
		a.Collection[id] = o
	}
	return o
}

// Level of a collectible: in the unlocked mode everything is maxed.
func (a *Account) Level(id int32) int32 {
	if !a.Progression {
		return maxLevel(id)
	}
	return a.owned(id).Level
}

// StartProgression begins a fresh account: the starter collection, starter
// decks and gold; trophies, experience and statistics from zero. The name
// and the skin choices stay.
func (a *Account) StartProgression() {
	a.Progression = true
	a.Collection = map[int32]*Owned{}
	for _, r := range starterMinis {
		a.entry(miniID(r)).Level = 1
	}
	a.entry(heroID(starterHero)).Level = 1
	for i := range a.Decks {
		copy(a.Decks[i][:], starterMinis)
		a.Heroes[i] = starterHero
	}
	a.Selected = 0
	a.Gold = starterGold
	a.Chests, a.LevelChests = nil, nil
	a.Trophies, a.BestTrophies, a.Exp, a.Gems = 0, 0, 0, 0
	a.Stats = Stats{}
}

// SetProgression applies the server option progression=fresh|unlocked|reset
// and says what it did ("" = nothing changed). fresh starts progression once
// and afterwards keeps it; a collection from before is resumed. reset always
// starts over. unlocked switches to the full collection and keeps the
// progress in the file for later.
func (a *Account) SetProgression(mode string) string {
	switch mode {
	case "fresh":
		if a.Progression {
			return ""
		}
		if a.Collection != nil {
			a.Progression = true
			return "progression resumed"
		}
		a.StartProgression()
		return "progression started: starter collection"
	case "reset":
		// a new player: no name yet, so the first start shows the tutorial
		// and the name dialog again
		a.StartProgression()
		a.Name, a.NameSet, a.TutorialPending, a.TutorialFlags = "Player", false, true, 0
		return "progression reset: new player with the starter collection (tutorial, name dialog)"
	case "unlocked":
		if !a.Progression {
			return ""
		}
		a.Progression = false
		return "everything unlocked (progress kept for later)"
	}
	return ""
}

// Upgrade is LogicUnlockCollectableCommand 602: collectible id at its current
// level goes one level up, paid with fragments and gold.
//
// The client can be ahead of the server: it unlocks a hero by itself once
// it has the fragments (no command; seen with the Old Monk: 30 fragments,
// level 0 here, then "602 from level 1"). So when the command's level is
// above the saved one, the missing levels are paid and taken first, as the
// client did.
func (a *Account) Upgrade(id, level int32) (*Reward, error) {
	r := &Reward{}
	if !a.Progression {
		return r, nil
	}
	known := false
	for _, k := range collectibleIDs() {
		known = known || k == id
	}
	if !known {
		return r, fmt.Errorf("unknown collectible %d", id)
	}
	o := a.entry(id)
	if o.Level > level {
		return r, fmt.Errorf("collectible %d is level %d, not %d", id, o.Level, level)
	}
	for o.Level <= level {
		frag, gold, ok := upgradeCost(id, o.Level)
		if !ok {
			return r, fmt.Errorf("collectible %d is maxed", id)
		}
		if o.Fragments < frag || a.Gold < gold {
			return r, fmt.Errorf("collectible %d level %d: needs %d fragments and %d gold, has %d and %d",
				id, o.Level, frag, gold, o.Fragments, a.Gold)
		}
		o.Fragments -= frag
		a.Gold -= gold
		o.Level++
		if o.Level > 1 { // an upgrade, not the unlock: its reward chest (+1 exp)
			a.pushChest(r, upgradeRewardChest, sourceUpgrade, upgradeExpContent())
			a.questProgress(questMetaUpgrade, 1, nil)
		}
	}
	return r, nil
}

// --- rewards ---

// Reward is what the account got, for the log.
type Reward struct {
	Gold      int32
	Gems      int32
	Exp       int32
	LevelUps  []int32 // player levels reached
	Unlocked  []int32 // collectibles now owned
	Upgraded  []int32 // collectibles one level up (by the client itself)
	Fragments map[int32]int32
	Chests    []int32 // chests put into the home to be opened
	// GoldCapped: today's battle gold limit was reached.
	GoldCapped bool
}

func (r *Reward) String() string {
	s := fmt.Sprintf("%d gold", r.Gold)
	if r.GoldCapped {
		s += " (daily battle gold limit reached)"
	}
	if r.Gems > 0 {
		s += fmt.Sprintf(", %d gems", r.Gems)
	}
	if r.Exp > 0 {
		s += fmt.Sprintf(", %d exp", r.Exp)
	}
	for _, l := range r.LevelUps {
		s += fmt.Sprintf(", LEVEL %d", l)
	}
	for _, id := range r.Unlocked {
		s += fmt.Sprintf(", unlocked %d", id)
	}
	for _, id := range r.Upgraded {
		s += fmt.Sprintf(", upgraded %d", id)
	}
	ids := make([]int, 0, len(r.Fragments))
	for id := range r.Fragments {
		ids = append(ids, int(id))
	}
	sort.Ints(ids)
	for _, id := range ids {
		s += fmt.Sprintf(", %d× %d", r.Fragments[int32(id)], id)
	}
	for _, id := range r.Chests {
		s += fmt.Sprintf(", chest %d to open", id)
	}
	return s
}

func (a *Account) give(r *Reward, gold int32) {
	a.Gold += gold
	r.Gold += gold
}

// giveFragments picks n fragments, one pick at a time, for minis (or
// heroes) that still need some (counting what is already in out and in the
// chests waiting), all equally likely, as the game's own chests weigh them
// (chests.json fragWeight 1).
func (a *Account) giveFragments(rng *rand.Rand, out map[int32]int32, n int32, heroes bool, chunk int32) {
	missing := func(id int32) int32 {
		m := a.fragmentsMissing(id) - out[id]
		for _, c := range a.Chests {
			m -= c.Content.Fragments[id]
		}
		return m
	}
	for ; n > 0; n -= chunk {
		var pool []int32
		for _, id := range collectibleIDs() {
			if isHero(id) != heroes || missing(id) <= 0 {
				continue
			}
			pool = append(pool, id)
		}
		if len(pool) == 0 {
			return
		}
		id := pool[rng.Intn(len(pool))]
		out[id] += min(chunk, n, missing(id))
	}
}

// WinFragments: a won battle also brings fragments in a chest (server
// option winfragments=on). Not the original: there a battle gave gold and
// rank points only (leagues.json, sources.json "Battle rewards"
// gameMode_Ranked_1W..5W); fragments came from quest chests, the shop and
// the season road.
var WinFragments = false

// battleRewards: the end screen's gold for every finished battle (league at
// the start of the battle). A win (duel win, Rumble top 4) also brings mini
// fragments and sometimes hero fragments, in a chest to open in the home
// (the daily quest's mini chest): a stand-in, not the original. In
// the live game fragments came from the shop, daily quests, the season road
// and the starter chests, which this server does not have yet.
func (a *Account) battleRewards(rng *rand.Rand, r *Reward, mode, place int32, league int, ranked bool) {
	if place == 0 {
		return
	}
	a.matchGold(r, rankReward(league, mode, place, ranked, "Gold"))
	if !WinFragments || !won(mode, place) {
		return
	}
	minis := int32(2)
	if place == 1 && mode == ModeRumble {
		minis = 4
	}
	frag := map[int32]int32{}
	a.giveFragments(rng, frag, minis, false, 1)
	if rng.Intn(4) == 0 {
		a.giveFragments(rng, frag, 5, true, 5)
	}
	a.queueChest(r, winChest, ChestContent{Fragments: frag})
}

// BotLevels: the hero and mini level the bots get, the same as the player's
// selected deck (fair fights).
func (a *Account) BotLevels() (hero, mini int32) {
	if !a.Progression {
		return maxHeroLevel, maxMiniLevel
	}
	hero = a.Level(heroID(a.Heroes[a.Selected]))
	var sum, n int32
	for _, r := range a.Decks[a.Selected] {
		if r >= 0 {
			sum += a.Level(miniID(r))
			n++
		}
	}
	mini = 1
	if n > 0 {
		mini = (sum + n/2) / n
	}
	return max(hero, 1), max(mini, 1)
}

// fillCollection writes the earned collection into the home: owned minis
// (by characters row) and heroes, levels, fragments, and the gold. Skins
// stay unlocked. The caller holds the lock.
func (a *Account) fillCollection(m *ownHomeData) {
	h := &m.home
	h.minis, h.heroes, h.collectibleLevels, h.map228 = nil, nil, nil, nil
	for i, r := range miniSeasonalRows {
		o := a.owned(miniID(r))
		if o.Level > 0 {
			h.minis = append(h.minis, [2]int32{miniCharRows[i], MiniLevel})
			h.collectibleLevels = append(h.collectibleLevels, [2]int32{miniID(r), o.Level})
		}
		if o.Fragments > 0 {
			h.map228 = append(h.map228, [2]int32{miniID(r), o.Fragments})
		}
	}
	for _, r := range heroRows {
		o := a.owned(heroID(r))
		if o.Level > 0 {
			h.heroes = append(h.heroes, r)
			h.collectibleLevels = append(h.collectibleLevels, [2]int32{heroID(r), o.Level})
		}
		if o.Fragments > 0 {
			h.map228 = append(h.map228, [2]int32{heroID(r), o.Fragments})
		}
	}
	for _, r := range skinRows {
		h.collectibleLevels = append(h.collectibleLevels, [2]int32{globalID(classSkin, r), 1})
	}
	m.avatar.slots[2] = nil
	m.avatar.slots[0] = []dataSlot{{class: classRESOURCES, instance: resourceGold, count: a.Gold}}
}

// resourceGold: resources.csv row "Gold"; the HUD shows its count from the
// avatar's first data slot list (0x598d98).
const resourceGold int32 = 1
