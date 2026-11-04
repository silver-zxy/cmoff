package home

// Player level and battle statistics.

// expToNext is exp_levels.csv ExpToNextLevel for levels 1..25: the client
// keeps the points within the current level (avatar +0xc0) and moves up when
// they reach this (0x8363a0). Level 25 is the last.
var expToNext = []int32{
	5, 5, 5, 5, 5, 5, 5, 5, 5, // 1-9
	10, 10, 10, 10, 10, 10, // 10-15
	25, 25, 25, 25, 25, 25, 25, 25, 25, 25, // 16-25
}

// MaxExpLevel is the last row of exp_levels.csv.
const MaxExpLevel = 25

// ExpLevel turns total experience into (level, points within the level).
// At the last level the points stop at that level's ExpToNextLevel.
func ExpLevel(exp int32) (level, points int32) {
	level = 1
	for level < MaxExpLevel && exp >= expToNext[level-1] {
		exp -= expToNext[level-1]
		level++
	}
	if level == MaxExpLevel && exp > expToNext[level-1] {
		exp = expToNext[level-1]
	}
	return level, exp
}

// Stats are the profile's battle numbers ([0] duel, [1] Rumble) and the
// win/loss streaks (> 0 wins in a row, < 0 losses in a row).
type Stats struct {
	Played       [2]int32 `json:"played"`
	Wins         [2]int32 `json:"wins"`
	Losses       [2]int32 `json:"losses"`
	Firsts       [2]int32 `json:"firsts"` // 1st places (duel: = wins)
	Streak       int32    `json:"streak"`
	RankedStreak int32    `json:"ranked_streak"`
}

func streak(s *int32, won bool) {
	switch {
	case won && *s > 0:
		*s++
	case won:
		*s = 1
	case *s < 0:
		*s--
	default:
		*s = -1
	}
}

// won: a duel win, or a Rumble in the top 4 (the places that gain trophies).
func won(mode, place int32) bool {
	if mode == ModeRumble {
		return place >= 1 && place <= 4
	}
	return place == 1
}
