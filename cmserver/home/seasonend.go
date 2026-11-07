package home

import "fmt"

// The season's end (every two months here, when the season key changes;
// in dev mode also once at every start). As in the original:
//   - the road's tiers reached and not claimed come by themselves, both
//     lanes (LogicClaimSeasonRoadRewardsOnSeasonEndCommand 418 in the
//     original);
//   - the star exchange: the minis' stars (a level each after the first)
//     go, every mini back to level 1 ("Star Upgrades are exchanged for
//     rewards at the end of each season", TID_HINT_SEASON_RESET); for 6 or
//     more stars the tier's chests of season_rewards.csv: the three
//     EndStars_Min_2/3/4_Cost chests (mini fragments back) and the tier's
//     own (6 stars season0_EndStars_1 .. 50 stars season0_EndStars_9:
//     500-4000 gold, 10-100 gems, up to 5 Crystal Coins, fragments);
//   - trophies from Diamond 3 up go back (leagues.json RankPointsReset,
//     RankPointsResetValue: Diamond 3 and 2 to 700, Diamond 1 to 800,
//     Master 900, Grandmaster 1000, Legendary 1100); below nothing changes.
// From March 2022 the exchange was a choice; our client has no screen for
// it (only the hint and the popup "Season has ended!" / "Total Mini
// Stars"), so it is forced as in December 2021 (decided). The new season's
// popup (LogicSeasonEndCommand 451) is not sent yet: the chests wait in
// the home.

// endStarRefunds: season_rewards.csv, the chests every tier has first
// (season0_EndStars_Min_2_Cost, _3_Cost, _4_Cost: mini fragments).
var endStarRefunds = []int32{127_000_127, 127_000_128, 127_000_129}

// endStarTiers: season_rewards.csv, StarCount and the tier's own chest.
var endStarTiers = []struct{ stars, chest int32 }{
	{6, 127_000_138}, {10, 127_000_137}, {15, 127_000_136}, {20, 127_000_135},
	{25, 127_000_134}, {30, 127_000_133}, {35, 127_000_132}, {40, 127_000_131},
	{50, 127_000_130},
}

// trophyResets: leagues.json, from the top: at RankPoints or more, the
// trophies go back to RankPointsResetValue.
var trophyResets = []struct{ from, to int32 }{
	{1200, 1100}, {1100, 1000}, {1000, 900}, {900, 800}, {800, 700}, {700, 700},
}

// miniStars: the stars of the minis, a level each after the first.
func (a *Account) miniStars() int32 {
	var n int32
	for _, r := range miniSeasonalRows {
		if l := a.owned(miniID(r)).Level; l > 1 {
			n += l - 1
		}
	}
	return n
}

// endSeason ends a.Season (before the next one starts) and says what it
// gave. The caller holds the lock.
func (a *Account) endSeason() string {
	if !a.Progression {
		return ""
	}
	r := &Reward{}
	old := a.Season
	tiers := 0
	if Live {
		tiers = a.claimSeason(r, &old)
	}
	stars := a.miniStars()
	starChest := int32(0)
	for _, t := range endStarTiers {
		if stars >= t.stars {
			starChest = t.chest
		}
	}
	if starChest != 0 {
		var c ChestContent
		for _, id := range append(append([]int32{}, endStarRefunds...), starChest) {
			if cc, err := rollChest(a.random(), id); err == nil {
				c.add(cc)
			}
		}
		a.pushChest(r, starChest, sourceSeason, c)
	}
	// the stars go: every mini back to level 1 (its fragments stay)
	for _, row := range miniSeasonalRows {
		if o := a.Collection[miniID(row)]; o != nil && o.Level > 1 {
			o.Level = 1
		}
	}
	before := a.Trophies
	for _, t := range trophyResets {
		if a.Trophies >= t.from {
			a.Trophies = t.to
			break
		}
	}
	return fmt.Sprintf("season %d ended: %d road tiers claimed, %d stars (chest %d), trophies %d -> %d: %s",
		old.Key, tiers, stars, starChest, before, a.Trophies, r.String())
}

// EndSeasonNow ends the season at once and starts it again (dev mode).
func (a *Account) EndSeasonNow() string {
	a.Lock()
	a.season()
	msg := a.endSeason()
	a.Season = SeasonState{Key: a.Season.Key}
	a.Unlock()
	if err := a.Save(); err != nil {
		msg += fmt.Sprintf(" (not saved: %v)", err)
	}
	return msg
}

// TakeSeasonEnd: what the last season's end gave, once (for the log).
func (a *Account) TakeSeasonEnd() string {
	msg := a.LastSeasonEnd
	a.LastSeasonEnd = ""
	return msg
}
