package home

import "testing"

func TestExpLevel(t *testing.T) {
	for _, c := range []struct{ exp, level, points int32 }{
		{0, 1, 0}, {4, 1, 4}, {5, 2, 0}, {20, 5, 0}, {44, 9, 4}, {45, 10, 0},
		{105, 16, 0}, {354, 25, 24}, {355, 25, 25}, {9999, 25, 25},
	} {
		l, p := ExpLevel(c.exp)
		if l != c.level || p != c.points {
			t.Errorf("%d exp: level %d (%d), want %d (%d)", c.exp, l, p, c.level, c.points)
		}
	}
}

func TestStatsInHome(t *testing.T) {
	a := NewAccount()
	a.BattleResult(ModeDuel, 1, true)   // win, ranked streak 1
	a.BattleResult(ModeDuel, 1, false)  // casual win: streak 2, ranked stays 1
	a.BattleResult(ModeRumble, 1, true) // 1st
	a.BattleResult(ModeRumble, 3, true) // top 4 = win
	a.BattleResult(ModeRumble, 5, true) // out
	s := a.Stats
	if s.Played != [2]int32{2, 3} || s.Wins != [2]int32{2, 2} || s.Losses != [2]int32{0, 1} ||
		s.Firsts != [2]int32{2, 1} || s.Streak != -1 || s.RankedStreak != -1 {
		t.Fatalf("stats %+v", s)
	}
	if a.Trophies != 5+10+(4+3)+1 { // third ranked win in a row: streak bonus
		t.Fatalf("trophies %d", a.Trophies)
	}
	a.Exp, a.Gems = 12, 7
	h := a.ownHome()
	h.home.skinStore = false // the test decoder skips a fixed-size home end
	m, err := decode(h.encode())
	if err != nil {
		t.Fatal(err)
	}
	av := m.avatar
	if av.trophies != 23 || av.seasonBest != 23 || av.expLevel != 3 || av.expPoints != 2 ||
		av.battleCounts != s.Played || av.battleWins != s.Wins || av.battleLosses != s.Losses ||
		av.battleFirsts != s.Firsts || av.streakAll != -1 || av.streakRanked != -1 || av.gems != 7 {
		t.Fatalf("home avatar %+v", av)
	}
}

func TestSavedAccountNotInTutorial(t *testing.T) {
	path := t.TempDir() + "/accounts.json"
	a, _ := LoadAccount(path) // new: tutorial
	if !a.TutorialPending {
		t.Fatal("new account without tutorial")
	}
	a.TutorialPending = false
	a.Save()
	b, _ := LoadAccount(path)
	if b.TutorialPending {
		t.Fatal("saved account back in the tutorial")
	}
}
