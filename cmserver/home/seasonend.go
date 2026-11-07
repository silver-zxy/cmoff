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
// Stars"), so it is forced as in December 2021 (decided).
//
// As in the original the client does the exchange itself, with the popup:
// LogicSeasonEndCommand 451 (decoder 0x4a96e0: VInt +0x24, Int +0x20 the
// new season key, Int +0x28 the new season's event, Long +0x30, Long
// +0x38, the server command base; execute 0x95918c) runs only while the
// avatar's season key (+0x44, 0x4c382c) is lower than the new one. Then
// 0x48a4dc sets the avatar's key (0x3d7ec0), keeps the trophies (+0x4c)
// and the season's best (+0x50), counts the minis' stars (characters.csv
// ModifierLevel1-3 = 2, 3, 4 reached: level - 1, 0x47af8c), takes the
// highest season_rewards.csv tier with StarCount <= stars (0x84770c) and
// the league's SeasonResetChestIds (none in leagues.json), sets every mini
// back to level 1 and takes its fragments (0x48aaa0), resets the trophies by the league (0x483e40),
// shows the popup (listener +0x198) and, for a tier, makes one chest of
// the tier's chests (0x6e1a5c, source 9, value the new key) whose content
// the server pushes (420). The road's manager is left alone when its key
// is the new one already (0x8fbefc).
//
// So at the season's change all is due: the home still has the avatar's
// old key, the minis' levels and the trophies; right after it 451 (the
// popup first), then the star chest's content, and the server does the
// same as the client (TakeSeasonEnd). The road's tiers reached and not
// claimed go into the star chest; without one (under 6 stars) they come
// as a chest with the next home. The original's
// LogicClaimSeasonRoadRewardsOnSeasonEndCommand 418 (decoder 0x69fda4:
// VInt list, VInt season, VInt source, VInt value, Bool + content; execute
// 0x53338c, the season as a second road manager entry) froze the client
// after 451 (0103). Not while chests wait in the
// home: their popups ran into the season end's screens (0101).

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

// sourceSeasonEnd: the star exchange's chest (0x48a7e4).
const sourceSeasonEnd int32 = 9

// endSeason ends a.Season (before the next one starts): all due for 451
// and, for the road's tiers reached and not claimed, 418. The caller holds
// the lock.
func (a *Account) endSeason() string {
	if !a.Progression {
		return ""
	}
	old := a.Season
	if a.SeasonEndKey == 0 || a.SeasonEndKey > old.Key {
		a.SeasonEndKey = old.Key
	}
	a.SeasonEndDue = true
	a.SeasonEndRoad = nil
	if Live && seasonUnclaimed(&old) {
		a.SeasonEndRoad = &old
	}
	return fmt.Sprintf("season %d ended, road to claim %v", old.Key, a.SeasonEndRoad != nil)
}

// starTier: the season_rewards.csv tier the stars reach, its chests (the
// refunds first, as in the file), or nil.
func starTier(stars int32) []int32 {
	var chest int32
	for _, t := range endStarTiers {
		if stars >= t.stars {
			chest = t.chest
		}
	}
	if chest == 0 {
		return nil
	}
	return append(append([]int32{}, endStarRefunds...), chest)
}

// SeasonEnd is LogicSeasonEndCommand 451's content.
type SeasonEnd struct {
	Key   int32 // +0x20 the new season
	Event int32 // +0x28 its event
	Log   string
}

// TakeSeasonEnd: a season's end due, once no chest waits in the home (the
// road's chest first): what 451 says, and the server
// does what the client does with it (the star chest as a push, so its
// content goes with 420 right after). The caller holds the lock.
func (a *Account) TakeSeasonEnd() (SeasonEnd, bool) {
	if !a.SeasonEndDue || (ChestsInHome && a.popupChests() > 0) {
		return SeasonEnd{}, false // the home's chests first
	}
	a.SeasonEndDue = false
	key := a.season().Key
	if !a.Progression || a.SeasonEndKey >= key {
		a.SeasonEndKey = key
		return SeasonEnd{}, false
	}
	r := &Reward{}
	stars := a.miniStars()
	list := starTier(stars)
	tiers := 0
	if list != nil {
		var c ChestContent
		for _, id := range list {
			if cc, err := rollChest(a.random(), id); err == nil {
				c.add(cc)
			}
		}
		// the road's rewards in the same chest, after the popup (ours:
		// 418, the original's, froze the client after 451, 0103)
		if old := a.SeasonEndRoad; old != nil {
			a.SeasonEndRoad = nil
			var rc ChestContent
			rc, _, tiers = a.claimSeasonContent(old)
			c.add(rc)
		}
		a.pushChest(r, list[len(list)-1], sourceSeasonEnd, c)
		if n := len(a.Chests); ChestsInHome && n > 0 && a.Chests[n-1].Index == a.ChestIndex {
			ch := &a.Chests[n-1]
			ch.List, ch.SourceValue, ch.Show = list, key, true
		}
	}
	// the stars go: every mini unlocked back to level 1, and its fragments
	// go too, as the client does (0x48aaa0: for each mini of level 1 or
	// more the +0x200 and the fragment map's +0x228 counts taken off, 0x7cc704
	// and 0x732868, then level 1, 0x95b240). Kept here until 0109, the
	// server then upgraded once more than the client and every chest it
	// made after had the next one's index: the level chest got an exp
	// chest's content and the level-up popup hung.
	for _, row := range miniSeasonalRows {
		if o := a.Collection[miniID(row)]; o != nil && o.Level >= 1 {
			o.Level, o.Fragments = 1, 0
		}
	}
	before := a.Trophies
	a.SeasonEndTrophies, a.SeasonEndBest = a.Trophies, a.BestTrophies
	for _, t := range trophyResets {
		if a.Trophies >= t.from {
			a.Trophies = t.to
			break
		}
	}
	a.SeasonEndKey = key
	return SeasonEnd{Key: key, Event: seasonEvent,
		Log: fmt.Sprintf("season %d: %d stars (tier %v), trophies %d -> %d, %d road tiers in the chest: %s",
			key, stars, list, before, a.Trophies, tiers, r.String())}, true
}

// popupChests: the chests waiting in the home that open with a popup. The
// silent ones (an upgrade's exp) do not get in the way, and the client
// tells of their opening only with its next command, so after a restart
// they may still be listed (0105: they kept dev mode's season end off).
func (a *Account) popupChests() int {
	n := 0
	for _, c := range a.Chests {
		if c.Show {
			n++
		}
	}
	return n
}

// giveSeasonEndRoad: the ended season's road tiers not claimed, when no
// star chest took them (under 6 stars), as a chest in the home after 451.
// The caller holds the lock.
func (a *Account) giveSeasonEndRoad() {
	if a.SeasonEndDue || a.SeasonEndRoad == nil {
		return
	}
	old := a.SeasonEndRoad
	a.SeasonEndRoad = nil
	// a chest the home brings (not a push: the client gets it whole)
	c, first, tiers := a.claimSeasonContent(old)
	if tiers == 0 || first == 0 {
		return
	}
	a.queueChestFrom(nil, first, sourceSeason, c)
	if n := len(a.Chests); ChestsInHome && n > 0 && a.Chests[n-1].Index == a.ChestIndex {
		a.Chests[n-1].SourceValue = old.Key
	}
}

// EndSeasonNow ends the season at once and starts it again (dev mode): the
// avatar's key one lower, so 451 runs again.
func (a *Account) EndSeasonNow() string {
	a.Lock()
	key := a.season().Key
	msg := a.endSeason()
	if a.SeasonEndDue {
		// the ended season one lower, for 451
		a.SeasonEndKey = key - 1
		if a.SeasonEndRoad != nil {
			a.SeasonEndRoad.Key = key - 1
		}
	}
	a.Season = SeasonState{Key: key}
	a.Unlock()
	if err := a.Save(); err != nil {
		msg += fmt.Sprintf(" (not saved: %v)", err)
	}
	return msg
}

// TakeSeasonEndLog: what the last season's end gave, once (for the log).
func (a *Account) TakeSeasonEndLog() string {
	msg := a.LastSeasonEnd
	a.LastSeasonEnd = ""
	return msg
}
