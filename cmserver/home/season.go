package home

import (
	"encoding/json"
	"time"

	"cmserver/proto"
)

// The Adventure Road (season road, docs/LIVE_CONTENT.md).
//
// The road is a SEASON_DATA chronos event (type 4, LogicLiveSeasonData
// 0x49c3ac): SeasonName, SeasonIndex, SeasonPassTiers (0x5fd020; a tier,
// 0x3291ec: PointsReq, FreeChests, PaidChests by chest name, optional
// visual styles, IsKeyTier), VisualConfig (0x786afc: the road's art,
// required), the passes as shop items (BasePass, PremiumPass, optional).
//
// The home's season manager (+0x1c8, encoder 0x31e77c / decoder 0x4bb660)
// holds one entry per season: VInt event id (the client finds the
// SEASON_DATA event by it, 0x6116b8), VInt season key, and the progress
// (0x4aa038): VInt, VInt points (+0x40, the Pony Fuel), Bool pass bought
// (+0x4c), VInt highest free tier claimed (+0x44, by PointsReq: claimed
// when +0x44 >= PointsReq, 0x4ebaf8), VInt the same for the paid lane
// (+0x48), and per-tier lists the client rebuilds from the event
// (0x6eda50). The manager's +0x18 is the current season key.
//
// Claim: LogicClaimAllSeasonRewardsCommand 601 (base, Bool, VInt tier):
// the client collects the chests of every tier reached and not claimed
// (0x8ee9b0), makes one chest of them (source 5, value the season key) and
// marks the tiers claimed; the server pushes the content (420).
//
// Points come from quest chests (SeasonPoints, resources.csv row 14).

// seasonEvent: the chronos event id of the season.
const seasonEvent int32 = 2001

// sourceSeason: a chest from the season road (0x3ac51c).
const sourceSeason int32 = 5

// seasonTier is one tier of the road.
type seasonTier struct {
	Points int32
	Free   []int32 // chests.json ids
	Paid   []int32 // the Mini Pass lane, free for everyone here
	Key    bool
}

// Chests of chests.json (season1_*, seasonRewards_*, season_gems_*).
const (
	chestSeasonGold50   int32 = 127_000_047
	chestSeasonGold100  int32 = 127_000_048
	chestSeasonGold250  int32 = 127_000_049
	chestSeasonGems10   int32 = 127_000_050
	chestSeasonGems20   int32 = 127_000_051
	chestSeasonGems40   int32 = 127_000_254
	chestSeasonGems100  int32 = 127_000_255
	chestSeasonMiniAll  int32 = 127_000_219 // 1 fragment of any mini
	chestSeasonHeroS    int32 = 127_000_202 // 2-3 hero fragments
	chestSeasonHeroM    int32 = 127_000_201 // 5-6
	chestSeasonHeroL    int32 = 127_000_200 // 8+ over 2-4 heroes
	chestSeasonMini1    int32 = 127_000_195 // the season's mini (P.E.K.K.A)
	chestSeasonMiniSkin int32 = 127_000_196 // its skin, P.L.U.S.H.
	chestSeasonHero1x50 int32 = 127_000_197 // 50 fragments of the Monk
	chestSeasonHeroSkin int32 = 127_000_198 // his skin, Luchador
	chestSeasonHero2x10 int32 = 127_000_199 // 10 fragments of the Countess
)

// seasonTierCount: Season 1's road had 90 tiers of 1-3 Pony Fuel (wiki).
const seasonTierCount = 90

// seasonTierFuel: the fuel from one tier to the next: 1 on tiers 1-30, 2
// on 31-60, 3 on 61-90 (ours: the wiki says 1-3, docs/ROADMAP.md B1).
func seasonTierFuel(i int32) int32 {
	return (i-1)/30 + 1
}

// seasonTiers: Season 1's road. Known: 90 tiers; the Mini Pass gave "300+
// Hero Fragments, 2x Seasonal Skins, 200 Gems, 2000 Gold"
// (TID_SEASONPASS_SEASON1_CONTENT_INFO), the skins P.L.U.S.H. P.E.K.K.A at
// tier 26 and Luchador Monk at tier 87 (wiki). Ours: which of the game's
// season chests lies on which other tier.
var seasonTiers = func() []seasonTier {
	cycle := [][]int32{
		{chestSeasonGold50}, {chestSeasonMiniAll}, {chestSeasonHeroS},
		{chestSeasonGold100}, {chestSeasonMiniAll}, {chestSeasonGems10},
		{chestSeasonMiniAll}, {chestSeasonGold100}, {chestSeasonHeroS},
		{chestSeasonHeroM},
	}
	free := map[int32][]int32{
		10: {chestSeasonMini1}, 20: {chestSeasonHeroL},
		30: {chestSeasonMini1, chestSeasonGems20}, 40: {chestSeasonGold250},
		50: {chestSeasonHeroL}, 60: {chestSeasonMini1, chestSeasonGems20},
		70: {chestSeasonHeroL}, 80: {chestSeasonGold250},
		90: {chestSeasonHeroL, chestSeasonGold250},
	}
	// the pass lane: 2 skins, 200 gems (40 + 100 + 40 + 20), 2000 gold
	// (8 × 250), hero fragments (Monk 50, Countess 10, the rest in hero
	// chests: about 320 in all)
	paid := map[int32][]int32{
		26: {chestSeasonMiniSkin}, 87: {chestSeasonHeroSkin},
		30: {chestSeasonHero2x10}, 60: {chestSeasonHero1x50},
		15: {chestSeasonGems40}, 45: {chestSeasonGems100}, 75: {chestSeasonGems40}, 90: {chestSeasonGems20},
	}
	for _, i := range []int32{6, 18, 33, 42, 54, 66, 78, 84} {
		paid[i] = []int32{chestSeasonGold250}
	}
	for i := int32(1); i <= seasonTierCount; i++ {
		if paid[i] != nil {
			continue
		}
		switch {
		case i%3 == 0:
			paid[i] = []int32{chestSeasonHeroM}
		case i%5 == 0, i%7 == 0:
			paid[i] = []int32{chestSeasonHeroL}
		}
	}
	var out []seasonTier
	points := int32(0)
	for i := int32(1); i <= seasonTierCount; i++ {
		points += seasonTierFuel(i)
		t := seasonTier{Points: points, Free: cycle[(i-1)%10], Paid: paid[i]}
		if f := free[i]; f != nil {
			t.Free = f
		}
		t.Key = i%30 == 0 || i == 26 || i == 87
		out = append(out, t)
	}
	return out
}()

// seasonKey: the season of a time, one per two calendar months (UTC),
// counted from January 2021: every season is Season 1 again.
func seasonKey(t time.Time) int32 {
	t = t.UTC()
	return int32(((t.Year()-2021)*12+int(t.Month())-1)/2 + 1)
}

// legacySeasonKey: the key of a time when a season was one month (until
// patch 0065).
func legacySeasonKey(t time.Time) int32 {
	t = t.UTC()
	return int32((t.Year()-2021)*12 + int(t.Month()))
}

func seasonWindow(t time.Time) (start, end time.Time) {
	t = t.UTC()
	m := (int(t.Month())-1)/2*2 + 1
	start = time.Date(t.Year(), time.Month(m), 1, 0, 0, 0, 0, time.UTC)
	return start, start.AddDate(0, 2, 0)
}

// SeasonState is the account's road progress in one season.
type SeasonState struct {
	Key     int32 `json:"key"`
	Points  int32 `json:"points"`
	Claimed int32 `json:"claimed"` // highest PointsReq claimed (free lane)
	// PaidClaimed: the same for the pass lane
	PaidClaimed int32 `json:"paid_claimed,omitempty"`
	// Seen: the last tier the player has seen, its index + 1 (0: none)
	Seen int32 `json:"seen,omitempty"`
}

// season brings the account's road to the current season (points start
// anew every two months).
func (a *Account) season() *SeasonState {
	k := seasonKey(now())
	if a.Season.Key != k && a.Season.Key == legacySeasonKey(now()) {
		// saved when a season was one month: this month's road goes on
		a.Season.Key = k
	}
	if a.Season.Key != k {
		a.Season = SeasonState{Key: k}
	}
	return &a.Season
}

func (a *Account) addSeasonPoints(r *Reward, n int32) {
	s := a.season()
	before := seasonTierReached(s.Points)
	s.Points += n
	if r != nil {
		r.SeasonPoints += n
	}
	a.questsRoadTiers(seasonTierReached(s.Points) - before)
}

// seasonTierReached: how many tiers the points reach.
func seasonTierReached(points int32) int32 {
	n := int32(0)
	for _, t := range seasonTiers {
		if t.Points <= points {
			n++
		}
	}
	return n
}

// ClaimSeasonRewards claims every tier reached and not claimed (601), as
// the client does: one chest of all their chests, both lanes (the pass is
// everyone's here).
func (a *Account) ClaimSeasonRewards(r *Reward) (tiers int) {
	if !Live {
		return 0
	}
	s := a.season()
	var c ChestContent
	first := int32(0)
	take := func(chests []int32) {
		for _, chest := range chests {
			if first == 0 {
				first = chest
			}
			if cc, err := rollChest(a.random(), chest); err == nil {
				c.add(cc)
			}
		}
	}
	free, paid := s.Claimed, s.PaidClaimed
	for _, t := range seasonTiers {
		if t.Points > s.Points {
			break
		}
		claimed := false
		if t.Points > s.Claimed {
			take(t.Free)
			free = t.Points
			claimed = true
		}
		if seasonPassFree && t.Points > s.PaidClaimed {
			take(t.Paid)
			paid = t.Points
			claimed = true
		}
		if claimed {
			tiers++
		}
	}
	s.Claimed, s.PaidClaimed = free, paid
	if tiers == 0 || first == 0 {
		return tiers
	}
	a.pushChest(r, first, sourceSeason, c)
	if n := len(a.Chests); ChestsInHome && n > 0 && a.Chests[n-1].Index == a.ChestIndex {
		a.Chests[n-1].SourceValue = s.Key
	}
	return tiers
}

// seasonPassFree: the Mini Pass (real money in the original) is everyone's.
const seasonPassFree = true

// SeeSeasonTier: the player has seen the road (505); the client keeps the
// index of the highest tier reached (0x5b3bf0, -1 for none) in the home's
// +0x1e0.
func (a *Account) SeeSeasonTier() {
	s := a.season()
	s.Seen = 0
	for i, t := range seasonTiers {
		if t.Points <= s.Points {
			s.Seen = int32(i) + 1
		}
	}
}

// SeasonSummary for the log.
func (a *Account) SeasonSummary() (key, points, claimed int32) {
	s := a.season()
	return s.Key, s.Points, s.Claimed
}

// seasonLiveEvent: the road's SEASON_DATA event of the current season.
func seasonLiveEvent() LiveEvent {
	start, end := seasonWindow(now())
	var tiers []any
	for _, t := range seasonTiers {
		tiers = append(tiers, map[string]any{
			"PointsReq":  t.Points,
			"FreeChests": chestNames(t.Free),
			"PaidChests": orEmpty(chestNames(t.Paid)),
			"IsKeyTier":  t.Key,
		})
	}
	b, _ := json.Marshal(map[string]any{
		"SeasonName":      "TID_SEASONROAD_SEASON1_NAME",
		"SeasonIndex":     seasonKey(now()),
		"SeasonPassTiers": tiers,
		// the road's art; the road screen reads it without a check
		// (0x6f24c8: pass vt+0x30 = pass +0x18, 0x5fd308; null crashes in
		// 0x689b84). The game's season road file and its exports.
		"VisualConfig": map[string]any{
			"TierFileName":             "sc/season_road_01.sc",
			"TierExportName":           "road_segment",
			"ProgressMarkerFileName":   "sc/season_road_01.sc",
			"ProgressMarkerExportName": "progress_marker",
			"Decorations":              []any{},
		},
		"ShowEndTime": true,
		// the "i" button: InfoPageConfig (0x6e95a8: InfoPageSWF,
		// LayoutExportName, PagesExportName), the game's season 1 info
		// page in the road file
		"HasInfoPage": true,
		"InfoPageConfig": map[string]any{
			"InfoPageSWF":      "sc/season_road_01.sc",
			"LayoutExportName": "season_info_1_layout",
			"PagesExportName":  []string{"season_info_1_page_1"},
		},
		"LevelLimitLow":  0,
		"LevelLimitHigh": 999,
	})
	return LiveEvent{
		ID: seasonEvent, Name: "adventure_road", Type: EventSeasonData,
		Start: int32(start.Unix()), End: int32(end.Unix()),
		VisibleStart: int32(start.Unix()), VisibleEnd: int32(end.Unix()),
		Parameters: string(b),
	}
}

// encodeSeason writes the home's season manager (0x31e77c).
func encodeSeason(w *proto.Writer, s *SeasonState) {
	if s == nil {
		vints(w, 4)
		return
	}
	w.VInt(s.Key) // +0x18 the current season
	w.VInt(s.Key) // +0x20
	w.VInt(0)     // +0x1c
	w.VInt(1)     // one entry (0x53c00c)
	w.VInt(seasonEvent)
	w.VInt(s.Key)
	w.Bool(true) // progress (0x4aa038)
	w.VInt(s.Key)
	w.VInt(s.Points)       // +0x40
	w.Bool(seasonPassFree) // +0x4c pass
	w.VInt(s.Claimed)      // +0x44 free lane
	w.VInt(s.PaidClaimed)  // +0x48 paid lane
	w.VInt(0)              // +0x10 tier list, rebuilt from the event (0x6eda50)
	w.VInt(0)              // +0x20 lists
	w.VInt(0)              // +0x30 lists
}

func orEmpty(xs []any) []any {
	if xs == nil {
		return []any{}
	}
	return xs
}
