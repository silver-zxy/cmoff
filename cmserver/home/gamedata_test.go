package home

import (
	"archive/zip"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// A tiny stand-in APK with the two files in the game's format.
func TestGameDataGold(t *testing.T) {
	apk := filepath.Join(t.TempDir(), "test.apk")
	f, _ := os.Create(apk)
	z := zip.NewWriter(f)
	w, _ := z.Create("assets/csv_logic/leagues.json")
	w.Write([]byte(`{"leagues":[{"name":"Tutorial","GameModeConfigs":{
		"SoloMode":{"ChestRewardsForRanking":[{"RankReq":0,"Rewards":[127000002],"RewardsCompetitive":[127000001]},
		                                      {"RankReq":1,"Rewards":[127000001]}]},
		"TournamentMode":{"ChestRewardsForRanking":[{"RankReq":2,"Rewards":[127000002,127000002]}]}}}]}`))
	w, _ = z.Create("assets/chests/chests.json")
	w.Write([]byte(`{"chests":[{"resources":[]},
		{"resources":[{"name":"RankPoints","min":3}]},
		{"resources":[{"name":"Gold","min":12}]}]}`))
	z.Close()
	f.Close()

	d, err := LoadGameData(apk)
	if err != nil {
		t.Fatal(err)
	}
	Data = d
	defer func() { Data = nil }()
	if g := rankReward(0, ModeDuel, 1, true, "Gold"); g != 12 {
		t.Fatalf("duel win %d", g)
	}
	if g := rankReward(0, ModeDuel, 2, true, "Gold"); g != 0 {
		t.Fatalf("duel loss %d", g)
	}
	if g := rankReward(0, ModeRumble, 3, false, "Gold"); g != 24 {
		t.Fatalf("rumble 3rd %d", g)
	}
	if n := rankReward(0, ModeDuel, 1, true, "RankPoints"); n != 3 {
		t.Fatalf("rank points %d", n)
	}
	if n := rankReward(0, ModeDuel, 1, false, "RankPoints"); n != 0 {
		t.Fatalf("casual rank points %d", n)
	}
	if _, ok := Data.RankReward(5, ModeDuel, 0, false, "Gold"); ok {
		t.Fatal("unknown league")
	}
}

// The tutorial's end: the base deck at level 1 and the chosen hero.
func TestStartWithHero(t *testing.T) {
	a := NewAccount()
	a.StartProgression()
	a.entry(miniID(16)).Level = 3
	r, err := a.StartWithHero(13)
	if err != nil {
		t.Fatal(err)
	}
	if a.Gold != starterGold || a.Level(heroID(13)) != 1 || a.Level(heroID(12)) != 0 || a.Level(miniID(16)) != 0 {
		t.Fatalf("gold %d, queen %d, king %d, collection %+v", a.Gold, a.Level(heroID(13)), a.Level(heroID(12)), a.Collection)
	}
	for _, row := range starterMinis {
		if a.Level(miniID(row)) != 1 || a.owned(miniID(row)).Fragments != 0 {
			t.Fatalf("mini %d: %+v", row, a.owned(miniID(row)))
		}
	}
	if a.Heroes[0] != 13 || a.Decks[0] != [5]int32{0, 1, 4, 5, 6} || len(r.Unlocked) != 6 {
		t.Fatalf("deck %v hero %d (%s)", a.Decks[0], a.Heroes[0], r.String())
	}
	if _, err := a.StartWithHero(14); err == nil {
		t.Fatal("hero 14 is no starting choice")
	}
}

// Daily battle gold limit and the ranked win streak bonus (no game data:
// the fallbacks).
func TestDailyGoldAndStreakBonus(t *testing.T) {
	day := time.Date(2026, 11, 3, 12, 0, 0, 0, time.Local)
	now = func() time.Time { return day }
	defer func() { now = time.Now }()
	a := NewAccount()
	a.StartProgression()
	r := &Reward{}
	a.matchGold(r, 150)
	a.matchGold(r, 80)
	if a.GoldToday != 200 || r.Gold != 200 || !r.GoldCapped {
		t.Fatalf("today %d, got %d, capped %v", a.GoldToday, r.Gold, r.GoldCapped)
	}
	r = &Reward{}
	a.matchGold(r, 10)
	if r.Gold != 0 {
		t.Fatalf("over the limit: %d", r.Gold)
	}
	day = day.Add(24 * time.Hour)
	r = &Reward{}
	a.matchGold(r, 10)
	if r.Gold != 10 || a.GoldToday != 10 {
		t.Fatalf("next day: %d, today %d", r.Gold, a.GoldToday)
	}

	var bonus []int32
	for i := 0; i < 4; i++ {
		bonus = append(bonus, a.BattleResult(ModeDuel, 1, true).StreakBonus)
	}
	a.BattleResult(ModeDuel, 2, true)
	bonus = append(bonus, a.BattleResult(ModeDuel, 1, true).StreakBonus)
	if fmt.Sprint(bonus) != "[0 0 3 3 0]" {
		t.Fatalf("streak bonus %v", bonus)
	}
}

// The pool JSON names its reward chests (the client looks them up by name);
// without game data the ids stay.
func TestQuestRewardNames(t *testing.T) {
	if got := fmt.Sprint(chestNames([]int32{chestQuestDailyMini})); got != "[127000062]" {
		t.Fatalf("without data %s", got)
	}
	d := &GameData{chests: make([]chestDef, 63)}
	d.chests[62].Name = "quest_Daily_Mini"
	Data = d
	defer func() { Data = nil }()
	if got := fmt.Sprint(chestNames([]int32{chestQuestDailyMini})); got != "[quest_Daily_Mini]" {
		t.Fatalf("names %s", got)
	}
}

// With live content the home's tail carries the server time, the client's
// time base for the events' windows.
func TestHomeServerTime(t *testing.T) {
	day := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	now = func() time.Time { return day }
	defer func() { now = time.Now }()
	a := NewAccount()
	if m := a.ownHome(); m.tail[1] != int32(day.Unix()) {
		t.Fatalf("tail %v", m.tail)
	}
}
