package home

import (
	"fmt"
	"math/rand"
)

// Opening a chest of chests.json (rules read off its fields):
//   - resources: min, plus a bonus and a random 0..spread from a tier picked
//     by tierWeights (Gold, Diamonds, ExpPoints; RankPoints and the like are
//     not resources of the account and are skipped here);
//   - fragments: a tier (fragTier by fragTierWeight) says how many picks; each
//     pick takes a collectible of fragList by fragWeight (each at most once
//     unless duplicatesAllowed) and gives it max(1, total / picks), total =
//     fragMin + bonus + 0..spread. So "starterchest_..._Deck" (fragMin 5,
//     5 picks) gives one fragment to each of its five minis and
//     "shop_miniHero_..._25" (fragMin 25, 1 pick) 25 to its hero; with as
//     many picks as entries, every entry gets its share once;
//   - collectiblePicks of collectibleList (by weight): owned at level 1.

func pickWeighted(rng *rand.Rand, w []int32) int {
	var sum int32
	for _, x := range w {
		if x > 0 {
			sum += x
		}
	}
	if sum <= 0 {
		return 0
	}
	r := rng.Int31n(sum)
	for i, x := range w {
		if x <= 0 {
			continue
		}
		if r < x {
			return i
		}
		r -= x
	}
	return 0
}

func pickTier(rng *rand.Rand, t []tier, w []int32) tier {
	if len(t) == 0 {
		return tier{}
	}
	i := 0
	if len(w) == len(t) {
		i = pickWeighted(rng, w)
	}
	return t[i]
}

func roll(rng *rand.Rand, t tier) int32 {
	n := t.Bonus
	if t.Spread > 0 {
		n += rng.Int31n(t.Spread + 1)
	}
	return n
}

// OpenChest opens a chest into the account and adds what it gave to r.
func (a *Account) OpenChest(rng *rand.Rand, r *Reward, id int32) error {
	c, ok := Data.Chest(id)
	if !ok {
		return fmt.Errorf("chest %d unknown (game data not loaded?)", id)
	}
	for _, res := range c.Resources {
		n := res.Min + roll(rng, pickTier(rng, res.Tiers, res.TierWeights))
		switch res.Name {
		case "Gold":
			a.give(r, n)
		case "Diamonds":
			a.Gems += n
			r.Gems += n
		case "ExpPoints":
			a.addExp(r, n)
		}
	}
	t := pickTier(rng, c.FragTier, c.FragTierWeight)
	if len(c.FragList) > 0 && t.Picks > 0 {
		total := c.FragMin + roll(rng, t)
		each := total / t.Picks
		if each < 1 {
			each = 1
		}
		weights := append([]int32(nil), c.FragWeight...)
		for len(weights) < len(c.FragList) {
			weights = append(weights, 1)
		}
		p := int32(0)
		// As many picks as entries (the starter "_Deck" chests): each entry
		// once, even where duplicates are allowed; that is what the chest is
		// for (its five deck minis).
		if int(t.Picks) >= len(c.FragList) {
			for _, id := range c.FragList {
				a.addFragments(r, id, each)
			}
			p = int32(len(c.FragList))
		}
		for ; p < t.Picks; p++ {
			i := pickWeighted(rng, weights)
			if weights[i] <= 0 {
				break // every entry taken
			}
			if !c.DuplicatesAllowed {
				weights[i] = 0
			}
			a.addFragments(r, c.FragList[i], each)
		}
	}
	weights := append([]int32(nil), c.CollectibleWeight...)
	for len(weights) < len(c.CollectibleList) {
		weights = append(weights, 1)
	}
	for p := int32(0); p < c.CollectiblePicks && len(c.CollectibleList) > 0; p++ {
		i := pickWeighted(rng, weights)
		weights[i] = 0
		id := c.CollectibleList[i]
		if o := a.entry(id); o.Level == 0 {
			o.Level = 1
			r.Unlocked = append(r.Unlocked, id)
		}
	}
	return nil
}

// addFragments gives a unit fragments as the client does (0x7d7550): only
// as many as it still needs up to its max level; every fragment beyond is
// one Crystal Coin (collectibles.json fragmentToRes: 1 PromotionToken, the
// same for every mini and hero).
func (a *Account) addFragments(r *Reward, id, n int32) {
	if class := id / 1_000_000; (class == classSEASONAL || class == classSupportHero) && n > 0 {
		if missing := max(a.fragmentsMissing(id), 0); n > missing {
			a.addCrystalCoins(r, n-missing)
			n = missing
		}
	}
	if n <= 0 {
		return
	}
	if id/1_000_000 == classSkin {
		a.ownSkin(id)
		r.Skins = append(r.Skins, id)
		return
	}
	a.entry(id).Fragments += n
	if r.Fragments == nil {
		r.Fragments = map[int32]int32{}
	}
	r.Fragments[id] += n
}

// --- the new player's starting choice ---

// startingChoice: starting_choices.csv, the hero a new player picks at the
// end of the tutorial and the chests it opens (FirstMatchChestId).
type startingChoice struct {
	hero   int32 // support_heroes row
	chests []int32
}

var startingChoices = []startingChoice{
	{12, []int32{127000065, 127000066, 127000077, 127000032, 127000032, 127000032, 127000017, 127000031, 127000036, 127000036, 127000036, 127000036}}, // Barbarian King
	{13, []int32{127000067, 127000068, 127000077, 127000032, 127000032, 127000032, 127000028, 127000035, 127000036, 127000036, 127000036, 127000036}}, // Archer Queen
	{15, []int32{127000069, 127000070, 127000077, 127000032, 127000032, 127000032, 127000017, 127000019, 127000036, 127000036, 127000036, 127000036}}, // Shield Maiden
}

// StartWithHero is the end of the tutorial: the base collection (the starter
// deck's five minis, Knight, Barbarian, Archer, Spear Goblin and Wizard, at
// level 1, and the starter gold) with the chosen hero at level 1 instead of
// the Barbarian King. The hero's starting chests (startingChoices) are not
// opened: the player starts with the base deck only.
func (a *Account) StartWithHero(heroRow int32) (*Reward, error) {
	known := false
	for _, c := range startingChoices {
		known = known || c.hero == heroRow
	}
	if !known {
		return nil, fmt.Errorf("hero row %d is no starting choice: keeping the Barbarian King", heroRow)
	}
	r := &Reward{}
	a.Collection = map[int32]*Owned{}
	for _, row := range starterMinis {
		a.entry(miniID(row)).Level = 1
		r.Unlocked = append(r.Unlocked, miniID(row))
	}
	a.entry(heroID(heroRow)).Level = 1
	r.Unlocked = append(r.Unlocked, heroID(heroRow))
	for i := range a.Decks {
		copy(a.Decks[i][:], starterMinis)
		a.Heroes[i] = heroRow
	}
	a.Selected = 0
	a.Gold, a.Gems = starterGold, starterGems
	r.Gold, r.Gems = starterGold, starterGems
	// a new pick replaces the chests of the one before
	a.Chests = nil
	a.StarterChests = false
	a.giveStarterChests(r, heroRow)
	return r, nil
}

// giveStarterChests puts the chosen hero's starting chests into the home
// as the client does it (0x687ccc): one chest made of the hero's chest list
// (starting_choices.csv FirstMatchChestId, the row's +0x48, 0x8c570c;
// chest 0x71e890: id -1, the list at +0x10), source 8 (0x687e50). Its
// content is all of them: the hero's chest, its deck's chest, 500 gold,
// single mini fragments. The hero itself, in the hero chest's
// collectibleList, the pick has unlocked already.
func (a *Account) giveStarterChests(r *Reward, heroRow int32) bool {
	var chests []int32
	for _, c := range startingChoices {
		if c.hero == heroRow {
			chests = c.chests
		}
	}
	if chests == nil || Data == nil {
		return false
	}
	var all ChestContent
	for _, id := range chests {
		c, err := rollChest(a.random(), id)
		if err != nil {
			return false
		}
		all.add(c)
	}
	a.queueChestFrom(r, chests[0], sourceStarter, all)
	if n := len(a.Chests); n > 0 {
		a.Chests[n-1].List = chests
		a.Chests[n-1].Show = true
	}
	a.StarterChests = true
	return true
}

// catchUpStarter: an account that began before the starting chests and
// values (0055) gets them now, once: the chests of its starting hero, the
// gold the start lacked, the starting gems.
func (a *Account) catchUpStarter(r *Reward) bool {
	if !a.Progression || a.TutorialPending || a.StarterChests || !ChestsInHome {
		return false
	}
	// the starting hero: the selected deck's first, then the others'
	hero := int32(-1)
	heroes := append([]int32{a.Heroes[a.Selected]}, a.Heroes[:]...)
	for _, h := range heroes {
		o := a.Collection[heroID(h)]
		if hero >= 0 || o == nil || o.Level == 0 {
			continue
		}
		for _, c := range startingChoices {
			if c.hero == h {
				hero = h
			}
		}
	}
	if hero < 0 || !a.giveStarterChests(r, hero) {
		return false
	}
	a.Gold += starterGold - oldStarterGold
	a.Gems += starterGems
	r.Gold += starterGold - oldStarterGold
	r.Gems += starterGems
	return true
}

// autoUpgrade does what the client does after it applies a chest's content
// (0x823924 -> 0x823d1c -> 0x4abb74): every collectible goes up as many
// levels as its fragments pay for (0x935380, then the upgrade itself,
// 0x7eadc4, without a command). Only steps without gold: that is the unlock
// and every mini level; hero levels cost gold and stay with the player
// (602). It returns the levels after the unlock (each brings an exp chest,
// openContent).
func (a *Account) autoUpgrade(r *Reward) (levels int32) {
	for _, id := range collectibleIDs() {
		o := a.entry(id)
		for {
			frag, gold, ok := upgradeCost(id, o.Level)
			if !ok || gold > 0 || o.Fragments < frag {
				break
			}
			o.Fragments -= frag
			o.Level++
			if o.Level == 1 {
				r.Unlocked = append(r.Unlocked, id)
				continue
			}
			r.Upgraded = append(r.Upgraded, id)
			levels++
			a.questsUpgraded(id)
		}
		if o.Level == 0 && o.Fragments == 0 {
			delete(a.Collection, id)
		}
	}
	return levels
}

// CatchUp runs autoUpgrade on the saved account, for fragments a version
// before it left unspent (the client had upgraded them itself), and puts
// their exp into a chest. It says what changed ("" = nothing) and saves.
func (a *Account) CatchUp() string {
	a.Lock()
	r := &Reward{}
	if a.Progression {
		// one level-up at most, as with pushed exp chests (0129: 20 exp
		// chests in a home gave two level-ups at once)
		a.upgradeExpChests(r, a.autoUpgrade(r), false)
	}
	coins := int32(0)
	if a.Progression {
		coins = a.excessToCoins(r)
	}
	starter := a.catchUpStarter(r)
	locked := a.lockSkins()
	a.Unlock()
	if len(r.Unlocked) == 0 && len(r.Upgraded) == 0 && !starter && coins == 0 && !locked {
		return ""
	}
	if err := a.Save(); err != nil {
		return "caught up, but not saved: " + err.Error()
	}
	if starter {
		return "caught up with the start (starting chests, gold, gems): " + r.String()
	}
	if locked && len(r.Unlocked) == 0 && len(r.Upgraded) == 0 && coins == 0 {
		return fmt.Sprintf("caught up: skins locked as in the original, %d owned", len(a.OwnedSkins))
	}
	if coins > 0 && len(r.Unlocked) == 0 && len(r.Upgraded) == 0 {
		return fmt.Sprintf("caught up: %d fragments past max level became Crystal Coins (%d now)", coins, a.CrystalCoins)
	}
	return "caught up with the client's own upgrades: " + r.String()
}

// excessToCoins turns fragments a unit holds beyond what it needs up to its
// max level into Crystal Coins, as the client does when they come (saved
// before the server did the same, 0074).
func (a *Account) excessToCoins(r *Reward) (coins int32) {
	for id, o := range a.Collection {
		if class := id / 1_000_000; class != classSEASONAL && class != classSupportHero {
			continue
		}
		if extra := -a.fragmentsMissing(id); extra > 0 && o.Fragments >= extra {
			o.Fragments -= extra
			before := a.CrystalCoins
			a.addCrystalCoins(r, extra)
			coins += a.CrystalCoins - before
		}
	}
	return coins
}
