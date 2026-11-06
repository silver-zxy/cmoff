package home

import (
	"encoding/json"
	"math/rand"
	"time"

	"cmserver/proto"
)

// Live content (docs/LIVE_CONTENT.md): what the live server sent instead of
// keeping it in the APK. Two parts:
//
//   - global configurations in the home (+0x148, 0x4f61d8 / decoder
//     0x4fa778): three optional strings, each a JSON object {"ID": ...,
//     "Params": {...}} for LogicLiveGlobalManager (0x4bfe7c): KILL_SWITCH,
//     QUEST_SETTINGS, ROTATING_SHOP;
//   - Chronos events, sent with LogicUpdateChronosEventsCommand 417.
//
// The quests are made of the game's quest kinds, texts and reward chests.

// Live: send the live content (server option live=on|off).
var Live = true

// LiveGlobals: the home's global configurations, at most three.
func LiveGlobals() []string {
	if !Live {
		return nil
	}
	cfg := map[string]any{
		"ID": "QUEST_SETTINGS",
		"Params": map[string]any{
			"Enabled":                   true,
			"MaxConcurrentQuests":       questsConcurrent + miniQuestsShown, // room for them when on
			"MaxConcurrentDailyQuests":  questsConcurrent,
			"MaxTotalDailyQuests":       questsPerDay,
			"DailyQuestRerollLimit":     questRerollsPerDay,
			"MaxConcurrentWeeklyQuests": 0,
			"MaxTotalWeeklyQuests":      0,
			"WeeklyQuestRerollLimit":    0,
		},
	}
	b, _ := json.Marshal(cfg)
	return []string{string(b)}
}

// Event categories (0x4ccac0).
const (
	EventShopLayout      int32 = 0
	EventQuestPool       int32 = 1
	EventQuestChain      int32 = 2
	EventGameModeMapping int32 = 3
	EventSeasonData      int32 = 4
	EventLinkPopup       int32 = 5
	EventQuestGroup      int32 = 6
)

// LiveEvent is one Chronos event (decoder 0x4915fc; the offline JSON of
// 0x4ca1c0 names the fields).
type LiveEvent struct {
	ID         int32
	Name       string
	Type       int32 // +0x18
	Start, End int32 // +0x1c, +0x20, unix seconds (0x6170cc)
	// +0x24, +0x28: visibleDateTime, visibleEndDateTime
	VisibleStart, VisibleEnd int32
	Parameters               string // +0x60, the category's own JSON
}

func (e LiveEvent) encode(w *proto.Writer) {
	w.VInt(e.ID)
	w.String(e.Name)
	w.VInt(e.Type)
	w.VInt(e.Start)
	w.VInt(e.End)
	w.VInt(e.VisibleStart)
	w.VInt(e.VisibleEnd)
	w.String("") // +0x30
	w.String("") // +0x40
	w.String("") // +0x50
	w.String(e.Parameters)
	w.VInt(0)     // +0x70
	w.Bool(false) // +0x74
}

// EncodeEvents writes an event collection (decoder 0x295f9c): VInt (+0),
// VInt count, the events.
func EncodeEvents(w *proto.Writer, events []LiveEvent) {
	w.VInt(0)
	w.VInt(int32(len(events)))
	for _, e := range events {
		e.encode(w)
	}
}

// questDef is one quest of our daily pool: a quest kind of the game
// (LogicLiveQuestData 0x5887fc; "Type" picks the kind, 0x3807a8, whose own
// keys sit in an object named like the kind, e.g. 0x8650e8 reads
// "PlayGameMode": {"GameMode": "2_PLAYER"}).
type questDef struct {
	Kind    string
	TypeID  int32 // 0x3807a8: MetaUpgrade 9, WinRumble 19, PlayGameMode 26
	Text    string
	Target  int32
	Rewards []int32 // chests.json ids
	Own     map[string]any
}

// req: the unit the quest names (Own "RequirementMini", a characters.csv
// name) as a table 6 reference: its row + 1; 0 for none.
func (q questDef) req() int32 {
	name, _ := q.Own["RequirementMini"].(string)
	if name == "" {
		return 0
	}
	if row, ok := Data.CharRow(name); ok {
		return row + 1
	}
	return 0
}

// Reward chests of chests.json. quest_Daily_Small is the daily quest's
// "1 Pony Fuel, 10-30 gold" of the screenshots; the "100 gold" quests give
// quest_gold_100 and a Pony Fuel (SeasonPoint_1).
const (
	chestQuestDailySmall int32 = 127_000_061
	chestQuestDailyMini  int32 = 127_000_062
	chestQuestGold100    int32 = 127_000_269
	chestSeasonPoint1    int32 = 127_000_178
)

var (
	rewardSmall = []int32{chestQuestDailySmall}
	rewardBig   = []int32{chestQuestGold100, chestSeasonPoint1}
)

// Quest kinds (0x3807a8).
const (
	questWinWithUnit      int32 = 1
	questPlayWithUnit     int32 = 2
	questUpgradeUnit      int32 = 3
	questTotalKOs         int32 = 4
	questKeywordClash     int32 = 5
	questEndRoundAlive    int32 = 7
	questWinDuelPerfect   int32 = 18
	questWinGameNoReroll  int32 = 20
	questWinRoundNoDeath  int32 = 21
	questMetaUpgrade      int32 = 9
	questWinRumble        int32 = 19
	questSeasonRoadLevel  int32 = 24
	questResourceGained   int32 = 25
	questPlayGameMode     int32 = 26
	resourceRankPoints    int32 = 12 // resources.csv row of the trophies
	resourceRankPointsKey       = "RankPoints"
)

// The quests a day can come from: only kinds the server can count from
// what it sees (battle results, trophies, the road). The kinds, texts and
// reward chests are the game's; the amounts are ours (docs/ROADMAP.md B5).
var questCatalog = []questDef{
	playDuels(3, rewardSmall),
	playRumbles(2, rewardSmall),
	{"PlayWithUnit", questPlayWithUnit, "TID_CHRONOS_QUESTS_PLAYWITHUNIT", 4,
		rewardSmall, map[string]any{}},
	{"WinWithUnit", questWinWithUnit, "TID_CHRONOS_QUESTS_WINWITHUNIT", 2,
		rewardSmall, map[string]any{"UseRoundWins": false}},
	{"MetaSeasonRoadLevel", questSeasonRoadLevel, "TID_CHRONOS_QUESTS_SEASONPASSLEVELUP", 2,
		rewardSmall, nil},
	playDuels(5, rewardBig),
	playRumbles(3, rewardBig),
	{"WinWithUnit", questWinWithUnit, "TID_CHRONOS_QUESTS_WINWITHUNIT", 4,
		rewardBig, map[string]any{"UseRoundWins": false}},
	{"WinRumble", questWinRumble, "TID_CHRONOS_QUESTS_WINRUMBLE", 1,
		rewardBig, nil},
	// Battle kinds: the client counts them on the battle's copies of the
	// quests (they need an end time, quests.go), the hook reports the
	// progress after the battle (QuestsFromClient). Rounds and 3:0 the
	// server also counts from the hook's rounds (BattleRounds). On the
	// screenshots: win 9 rounds (100 gold), knock out 15 (10-30), win 1
	// match without rerolling (10-30).
	{"WinWithUnit", questWinWithUnit, "TID_CHRONOS_QUESTS_WINROUNDSWITHUNIT", 9,
		rewardBig, map[string]any{"UseRoundWins": true}},
	{"WinDuelPerfect", questWinDuelPerfect, "TID_CHRONOS_QUESTS_WINDUELPERFECT", 1,
		rewardBig, nil},
	{"TotalKOs", questTotalKOs, "TID_CHRONOS_QUESTS_TOTALKOS", 15,
		rewardSmall, map[string]any{}},
	{"WinGameNoReroll", questWinGameNoReroll, "TID_CHRONOS_QUESTS_WINGAMENOREROLL", 1,
		rewardSmall, nil},
	{"WinRoundNoDeath", questWinRoundNoDeath, "TID_CHRONOS_QUESTS_WINROUNDNODEATH", 2,
		rewardSmall, nil},
	{"UseKeywordClash", questKeywordClash, "TID_CHRONOS_QUESTS_USEKEYWORDCLASH", 10,
		rewardSmall, map[string]any{}},
	{"UpgradeUnit", questUpgradeUnit, "TID_CHRONOS_QUESTS_UPGRADEUNIT", 5,
		rewardSmall, map[string]any{}},
	// Not "Win N Trophies" (MetaResourceGained): a quest of that kind the
	// client made from the pool crashed the quest screen (its hint,
	// 0x834d64, found no resource), 0075.
}

func playDuels(n int32, rewards []int32) questDef {
	return questDef{"PlayGameMode", questPlayGameMode, "TID_CHRONOS_QUESTS_PLAY_DUEL", n,
		rewards, map[string]any{"GameMode": "2_PLAYER"}}
}

func playRumbles(n int32, rewards []int32) questDef {
	return questDef{"PlayGameMode", questPlayGameMode, "TID_CHRONOS_QUESTS_PLAY_RUMBLE", n,
		rewards, map[string]any{"GameMode": "RUMBLE"}}
}

// questPoolSize: the quests of a day's pool: a day's quests and one for
// the reroll.
const questPoolSize = questsPerDay + questRerollsPerDay

// todaysQuests: today's pool (UTC day), seven of the catalog, the same all
// day. The saved quests point into it by index.
func todaysQuests() []questDef {
	if len(questCatalog) <= questPoolSize {
		return questCatalog
	}
	day := now().UTC().Unix() / 86400
	rng := rand.New(rand.NewSource(day*104729 + 17))
	var out []questDef
	for _, i := range rng.Perm(len(questCatalog))[:questPoolSize] {
		out = append(out, questCatalog[i])
	}
	return out
}

// A day: six daily quests, all of them shown at once ("Quests completed:
// 0/6"); a claimed one leaves the list.
const (
	questsPerDay     = 6
	questsConcurrent = 6
)

func (q questDef) json() map[string]any {
	m := map[string]any{
		"Type":         q.Kind,
		"QuestTextTID": q.Text,
		"MinProgress":  q.Target,
		"MaxProgress":  q.Target,
		"Rewards":      chestNames(q.Rewards),
	}
	if q.Own != nil {
		m[q.Kind] = q.Own
		// also at the quest's top level: the kinds' readers may take
		// their keys from there
		for k, v := range q.Own {
			m[k] = v
		}
	}
	return m
}

// chestNames: a quest's rewards for the pool JSON. The client takes them by
// name (0x5887fc: each through 0x865888, a lookup by name, then the id
// +0x18); a number finds nothing and the quest it makes on a reroll gets no
// reward. The names come from the APK's chests.json.
func chestNames(ids []int32) []any {
	var out []any
	for _, id := range ids {
		if c, ok := Data.Chest(id); ok && c.Name != "" {
			out = append(out, c.Name)
		} else {
			out = append(out, id)
		}
	}
	return out
}

// dailyPoolEvent: the chronos event of the daily quest pool.
const dailyPoolEvent int32 = 1001

// LiveEvents: the events of now, a day back to 30 days ahead.
func LiveEvents() []LiveEvent {
	if !Live {
		return nil
	}
	// whole days, so the events stay the same through a day
	y, m, d := now().Date()
	day := time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
	start := int32(day.AddDate(0, 0, -1).Unix())
	end := int32(day.AddDate(0, 0, 30).Unix())
	var list []any
	for _, q := range todaysQuests() {
		list = append(list, q.json())
	}
	pool := map[string]any{
		"Pool":             list,
		"CycleCategory":    "Daily",
		"DailyLimit":       questsPerDay,
		"CycleLimit":       questPoolSize,
		"PoolPriority":     1,
		"DuplicateAllowed": false,
		"LevelLimitLow":    0,
		"LevelLimitHigh":   999,
	}
	b, _ := json.Marshal(pool)
	events := []LiveEvent{{
		ID: dailyPoolEvent, Name: "daily_quests", Type: EventQuestPool,
		Start: start, End: end, VisibleStart: start, VisibleEnd: end,
		Parameters: string(b),
	}, seasonLiveEvent(), shopLiveEvent()}
	return events
}
