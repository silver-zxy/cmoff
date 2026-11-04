package home

import "time"

// The original economy, as far as the game data has it. Values come from the
// player's APK at runtime (gamedata.go); the fallbacks below are the same
// data for Bronze, used when the APK cannot be read.
//
//   - Battle: the league's rank reward chests (leagues.json
//     ChestRewardsForRanking): gold for everyone, rank points (trophies) for
//     ranked battles ("RewardsCompetitive"). The end screen sums the same.
//   - Experience: none for battles (globals BATTLE_END_EXP_REWARD 0); every
//     upgrade opens the collectible's upgradeRewardChestId,
//     collectibleUpgradeReward_EXP_1 = 1 exp. The unlock gives none (chest 0).
//   - Player level: exp_levels.csv ChestWhenReached, gold and gems.

// battleRank: leagues.json RankReq for a result (duel 0 won / 1 lost; Rumble
// place - 1, place 3 = 3rd-4th, 5 = 5th-8th; leaving counts as the last).
func battleRank(mode, place int32) int32 {
	if mode == ModeRumble {
		if place < 1 {
			return 7
		}
		return place - 1
	}
	if place == 1 {
		return 0
	}
	return 1
}

// fallbackRank: Bronze 3 (leagues.json) as [mode][rank] = {gold, rank points
// of a ranked battle}.
var fallbackRank = [2][][2]int32{
	{{12, 5}, {5, 1}},
	{{12, 10}, {9, 6}, {7, 4}, {7, 4}, {5, 1}, {5, 1}, {5, 1}, {5, 1}},
}

// rankReward: gold ("Gold") or trophies ("RankPoints") for a battle in a
// league.
func rankReward(league int, mode, place int32, ranked bool, name string) int32 {
	rank := battleRank(mode, place)
	if n, ok := Data.RankReward(league, mode, rank, ranked, name); ok {
		return n
	}
	t := fallbackRank[0]
	if mode == ModeRumble {
		t = fallbackRank[1]
	}
	if int(rank) >= len(t) {
		return 0
	}
	if name == "Gold" {
		return t[rank][0]
	}
	if name == "RankPoints" && ranked {
		return t[rank][1]
	}
	return 0
}

// upgradeRewardChest: collectibles.json upgradeRewardChestId of every level
// after the unlock (collectibleUpgradeReward_EXP_1).
const upgradeRewardChest int32 = 127_000_059

func upgradeExp() int32 {
	if n, ok := Data.ChestResource(upgradeRewardChest, "ExpPoints"); ok && n > 0 {
		return n
	}
	return 1
}

// levelChest: exp_levels.csv ChestWhenReached of a level (1-based).
func levelChest(level int32) int32 {
	switch {
	case level <= 1:
		return 0
	case level == 2:
		return 127_000_184
	case level < 10:
		return 127_000_041
	case level < 20:
		return 127_000_042
	}
	return 127_000_043
}

// fallbackLevelChest: gold and gems of those chests.
var fallbackLevelChest = map[int32][2]int32{
	127_000_184: {100, 10}, 127_000_041: {100, 10},
	127_000_042: {250, 15}, 127_000_043: {500, 20},
}

// addExp adds experience and notes the levels reached (their level chests:
// openContent).
func (a *Account) addExp(r *Reward, n int32) {
	before, _ := ExpLevel(a.Exp)
	a.Exp += n
	r.Exp += n
	after, _ := ExpLevel(a.Exp)
	for l := before + 1; l <= after; l++ {
		r.LevelUps = append(r.LevelUps, l)
	}
}

// levelChestContent: the content of a player level's chest (exp_levels.csv
// ChestWhenReached).
func (a *Account) levelChestContent(level int32) ChestContent {
	id := levelChest(level)
	c, err := rollChest(a.random(), id)
	if err != nil {
		f := fallbackLevelChest[id]
		c = ChestContent{Resources: map[string]int32{"Gold": f[0], "Diamonds": f[1]}}
	}
	return c
}

// Ranked win streak bonus (globals.csv): WINSTREAK_BONUS_REQUIREMENT 3,
// WINSTREAK_BONUS_CHEST "winStreak_Trophy_3" (3 RankPoints). The end screen
// (0x9454c4) compares the ranked streak + 1 (this win) with the requirement
// (LogicGlobals +0x49c), so every ranked win from the third in a row on
// gets the chest.
const (
	winStreakBonusRequirement = 3
	winStreakBonusChest       = "winStreak_Trophy_3"
	winStreakBonusFallback    = 3
)

func winStreakBonus() int32 {
	if id, ok := Data.ChestByName(winStreakBonusChest); ok {
		n, _ := Data.ChestResource(id, "RankPoints")
		return n
	}
	return winStreakBonusFallback
}

// dailyMatchGoldLimit: globals.csv DAILY_GOLD_GAIN_LIMIT_FROM_MATCH. Gold
// from battles stops at 200 a day (the day changes at local midnight).
const dailyMatchGoldLimit int32 = 200

// now is time.Now, replaceable in tests.
var now = time.Now

// matchGold pays battle gold up to what is left of today's limit.
func (a *Account) matchGold(r *Reward, gold int32) {
	day := now().Format("2006-01-02")
	if a.GoldDay != day {
		a.GoldDay, a.GoldToday = day, 0
	}
	if left := dailyMatchGoldLimit - a.GoldToday; gold > left {
		r.GoldCapped = true
		gold = max(left, 0)
	}
	a.GoldToday += gold
	a.give(r, gold)
}

// dailyGold: the home's daily battle gold state now (ownhome.go dailyGold).
func (a *Account) dailyGold() dailyGold {
	t := now()
	y, m, d := t.Date()
	next := time.Date(y, m, d+1, 0, 0, 0, 0, t.Location())
	day := int32(time.Date(y, m, d, 0, 0, 0, 0, time.UTC).Unix() / 86400)
	left := dailyMatchGoldLimit
	if a.GoldDay == t.Format("2006-01-02") {
		left = max(dailyMatchGoldLimit-a.GoldToday, 0)
	}
	return dailyGold{
		weekday:   day%7 + 1,
		day:       day,
		ticksLeft: int32(next.Sub(t).Seconds()) * 20,
		left:      left,
	}
}
