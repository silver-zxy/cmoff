package home

import (
	"encoding/json"
	"testing"
	"time"
)

// Pony Fuel from quest chests fills the road; a claim takes every tier
// reached, both lanes, once; the home carries the season; a new two-month
// window starts anew.
func TestAdventureRoad(t *testing.T) {
	day := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)
	now = func() time.Time { return day }
	defer func() { now = time.Now }()
	a := NewAccount()
	a.StartProgression()
	for i := 0; i < 10; i++ {
		r := &Reward{}
		a.applyContent(r, ChestContent{Resources: map[string]int32{"SeasonPoints": 1}})
		if r.SeasonPoints != 1 {
			t.Fatalf("reward %+v", r)
		}
	}
	if a.Season.Points != 10 || a.Season.Key != seasonKey(day) {
		t.Fatalf("season %+v", a.Season)
	}
	chests := len(a.Chests)
	if n := a.ClaimSeasonRewards(&Reward{}); n != 10 || a.Season.Claimed != 10 || a.Season.PaidClaimed != 10 {
		t.Fatalf("claimed %d %+v", n, a.Season)
	}
	if len(a.Chests) != chests+1 || a.Chests[len(a.Chests)-1].Source != sourceSeason || a.Chests[len(a.Chests)-1].SourceValue != a.Season.Key {
		t.Fatalf("chest %+v", a.Chests)
	}
	if n := a.ClaimSeasonRewards(&Reward{}); n != 0 {
		t.Fatalf("claimed again %d", n)
	}
	a.SeeSeasonTier()
	if a.Season.Seen != 10 {
		t.Fatalf("seen %d", a.Season.Seen)
	}
	h := a.ownHome()
	h.home.skinStore = false
	m, err := decode(h.encode())
	if err != nil || m.home.season == nil || *m.home.season != (SeasonState{Key: a.Season.Key, Points: 10, Claimed: 10, PaidClaimed: 10}) {
		t.Fatalf("%v home season %+v", err, m.home.season)
	}
	if h.home.seasonSeen != 9 {
		t.Fatalf("seen in the home %d", h.home.seasonSeen)
	}
	// the event: 90 tiers within this window
	ev := seasonLiveEvent()
	var p struct {
		VisualConfig    map[string]any
		HasInfoPage     bool
		SeasonIndex     int32
		SeasonPassTiers []struct {
			PointsReq  int32
			FreeChests []any
			PaidChests []any
		}
	}
	if err := json.Unmarshal([]byte(ev.Parameters), &p); err != nil || len(p.SeasonPassTiers) != 90 || p.SeasonIndex != a.Season.Key ||
		p.SeasonPassTiers[89].PointsReq != 180 || len(p.SeasonPassTiers[0].FreeChests) != 1 || p.VisualConfig["TierExportName"] == nil || !p.HasInfoPage {
		t.Fatalf("%v event %s", err, ev.Parameters)
	}
	if start := time.Unix(int64(ev.Start), 0).UTC(); start.Day() != 1 || start.Month() != time.November || ev.End <= int32(day.Unix()) {
		t.Fatalf("window %d %d", ev.Start, ev.End)
	}
	// December is the same season, January a new one
	day = time.Date(2026, 12, 20, 0, 0, 1, 0, time.UTC)
	if s := a.season(); s.Points != 10 {
		t.Fatalf("December %+v", s)
	}
	day = time.Date(2027, 1, 1, 0, 0, 1, 0, time.UTC)
	if s := a.season(); s.Points != 0 || s.Claimed != 0 || s.PaidClaimed != 0 || s.Key != seasonKey(day) {
		t.Fatalf("new season %+v", s)
	}
}

// The road as known: 90 tiers, the skins on the pass lane at 26 and 87,
// the pass's 200 gems and 2000 gold.
func TestSeasonOneRoad(t *testing.T) {
	if len(seasonTiers) != 90 {
		t.Fatalf("%d tiers", len(seasonTiers))
	}
	has := func(xs []int32, x int32) bool {
		for _, y := range xs {
			if y == x {
				return true
			}
		}
		return false
	}
	if !has(seasonTiers[25].Paid, chestSeasonMiniSkin) || !has(seasonTiers[86].Paid, chestSeasonHeroSkin) {
		t.Fatal("skins not at 26 and 87")
	}
	gems, gold := map[int32]int32{chestSeasonGems20: 20, chestSeasonGems40: 40, chestSeasonGems100: 100}, int32(0)
	var g int32
	for _, tier := range seasonTiers {
		for _, c := range tier.Paid {
			g += gems[c]
			if c == chestSeasonGold250 {
				gold += 250
			}
		}
		if f := seasonTierFuel(1); f != 1 {
			t.Fatal(f)
		}
	}
	if g != 200 || gold != 2000 {
		t.Fatalf("pass gems %d gold %d", g, gold)
	}
}

// A road saved when seasons were one month long goes on in this month.
func TestSeasonMonthlyKeyKept(t *testing.T) {
	day := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)
	now = func() time.Time { return day }
	defer func() { now = time.Now }()
	a := NewAccount()
	a.Season = SeasonState{Key: legacySeasonKey(day), Points: 12, Claimed: 9}
	if s := a.season(); s.Key != seasonKey(day) || s.Points != 12 || s.Claimed != 9 {
		t.Fatalf("%+v", s)
	}
}

// Crystal Coins from a chest count, up to 9999, and show in the home.
func TestCrystalCoins(t *testing.T) {
	a := NewAccount()
	a.StartProgression()
	r := &Reward{}
	a.applyContent(r, ChestContent{Resources: map[string]int32{"PromotionToken": 5}})
	if a.CrystalCoins != 5 || r.CrystalCoins != 5 {
		t.Fatalf("coins %d %+v", a.CrystalCoins, r)
	}
	a.CrystalCoins = 9998
	a.applyContent(r, ChestContent{Resources: map[string]int32{"PromotionToken": 5}})
	if a.CrystalCoins != 9999 {
		t.Fatalf("cap %d", a.CrystalCoins)
	}
	m := a.ownHome()
	found := false
	for _, s := range m.avatar.slots[0] {
		found = found || (s.class == classRESOURCES && s.instance == resourcePromotionToken && s.count == 9999)
	}
	if !found {
		t.Fatalf("slots %+v", m.avatar.slots[0])
	}
}
