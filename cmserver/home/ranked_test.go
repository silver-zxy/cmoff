package home

import "testing"

func TestLeaguesAndBots(t *testing.T) {
	for _, c := range []struct {
		trophies int32
		league   string
		bot      int32
	}{
		{0, "Tutorial", 2}, {5, "Bronze 3", 2}, {99, "Bronze 1", 4},
		{100, "Silver 3", 5}, {450, "Gold 3", 8}, {650, "Gold 1", 10},
		{900, "Diamond 1", 10}, {1000, "Master", 13}, {5000, "Legendary", 13},
	} {
		if got := LeagueName(c.trophies); got != c.league {
			t.Errorf("%d trophies: league %q, want %q", c.trophies, got, c.league)
		}
		if got := BotConfig(c.trophies); got != c.bot {
			t.Errorf("%d trophies: bot config %d, want %d", c.trophies, got, c.bot)
		}
	}
}

func TestBattleResult(t *testing.T) {
	a := NewAccount()
	if o := a.BattleResult(ModeDuel, 1, false); o.Trophies != 0 || a.Trophies != 0 {
		t.Fatalf("casual counted: %+v", o)
	}
	// Bronze (fallback data): duel won 5, lost 1; Rumble 1st 10, 5th-8th 1
	a.BattleResult(ModeDuel, 1, true)
	a.BattleResult(ModeDuel, 2, true)
	a.BattleResult(ModeRumble, 1, true)
	a.BattleResult(ModeRumble, 5, true)
	if a.Trophies != 17 || a.BestTrophies != 17 {
		t.Fatalf("trophies %d best %d", a.Trophies, a.BestTrophies)
	}
}
