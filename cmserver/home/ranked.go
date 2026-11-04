package home

// Ranked play: trophies (the client calls them RankPoints, avatar +0x70),
// leagues from csv_logic/leagues.json and the bot strength that goes with
// them. Only ranked battles (StartMatchmake 582 with ranked = true) count.

// Battle modes as in StartMatchmake / LogicChooseGameModeCommand.
const (
	ModeDuel   int32 = 2
	ModeRumble int32 = 3
)

// leagues.json: name and the RankPoints a league starts at.
var leagues = []struct {
	name   string
	points int32
}{
	{"Tutorial", 0}, {"Bronze 3", 5}, {"Bronze 2", 25}, {"Bronze 1", 50},
	{"Silver 3", 100}, {"Silver 2", 200}, {"Silver 1", 300},
	{"Gold 3", 400}, {"Gold 2", 500}, {"Gold 1", 600},
	{"Diamond 3", 700}, {"Diamond 2", 800}, {"Diamond 1", 900},
	{"Master", 1000}, {"Grandmaster", 1100}, {"Legendary", 1200},
}

// LeagueIndex is the leagues.json Index for these trophies (as 0x483e40:
// the last league whose RankPoints are reached).
func LeagueIndex(trophies int32) int {
	i := len(leagues) - 1
	for i > 0 && leagues[i].points > trophies {
		i--
	}
	return i
}

// LeagueName is the league's display name.
func LeagueName(trophies int32) string { return leagues[LeagueIndex(trophies)].name }

// BotConfig is the bot_configs.csv config for a player with these trophies.
// Config n plays with the decks of league n-1 (config 2: Bronze 3..1, config
// 5: Silver 3..1, config 10: Diamond 3..1, config 13: Master); the low ones
// also use the beginner behaviours (BotNewBie*, dynamic difficulty).
func BotConfig(trophies int32) int32 {
	l := int32(LeagueIndex(trophies))
	switch {
	case l >= 13:
		return 13
	case l >= 9:
		return 10
	case l < 1:
		return 2
	}
	return l + 1
}

// Outcome is what one battle changed.
type Outcome struct {
	Trophies int32  // change
	Reward   Reward // gold and fragments (progression only)
	// StreakBonus: trophies of the win streak bonus, part of Trophies.
	StreakBonus int32
}

// BattleResult records a bot battle: statistics and streaks for every
// battle, rank points (trophies) for ranked ones, as the league's rank reward
// chests give them (economy.go). No experience: the game gave none for
// battles. The caller holds the lock.
func (a *Account) BattleResult(mode, place int32, ranked bool) (o Outcome) {
	league := LeagueIndex(a.Trophies)
	i := 0
	if mode == ModeRumble {
		i = 1
	}
	w := won(mode, place)
	if place != 0 {
		a.questsAfterBattle(mode, place)
	}
	s := &a.Stats
	s.Played[i]++
	if w {
		s.Wins[i]++
	} else {
		s.Losses[i]++
	}
	if place == 1 {
		s.Firsts[i]++
	}
	streak(&s.Streak, w)

	if a.Progression {
		a.battleRewards(a.random(), &o.Reward, mode, place, league, ranked)
	}
	if !ranked {
		return o
	}
	streak(&s.RankedStreak, w)
	d := rankReward(league, mode, place, true, "RankPoints")
	if w && s.RankedStreak >= winStreakBonusRequirement {
		o.StreakBonus = winStreakBonus()
		d += o.StreakBonus
	}
	if a.Trophies+d < 0 {
		d = -a.Trophies
	}
	a.Trophies += d
	if a.Trophies > a.BestTrophies {
		a.BestTrophies = a.Trophies
	}
	o.Trophies = d
	return o
}

// PlaceName is a placement for the log.
func PlaceName(mode, place int32) string {
	if mode != ModeRumble {
		switch place {
		case 1:
			return "win"
		case 2:
			return "loss"
		}
		return "left (loss)"
	}
	switch place {
	case 1:
		return "1st"
	case 2:
		return "2nd"
	case 3:
		return "3rd-4th"
	case 5:
		return "5th-8th"
	}
	return "left (5th-8th)"
}
