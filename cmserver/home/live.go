package home

import (
	"encoding/json"
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
// This is the first step: quest settings and one daily quest pool, to see
// whether the client takes them. The quests are our own, made of the
// game's quest kinds and reward chests.

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
			"MaxConcurrentQuests":       3,
			"MaxConcurrentDailyQuests":  3,
			"MaxTotalDailyQuests":       3,
			"DailyQuestRerollLimit":     1,
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

// Reward chests of chests.json (quest_*).
const (
	chestQuestDailySmall int32 = 127_000_061
	chestQuestDailyMini  int32 = 127_000_062
	chestQuestGold100    int32 = 127_000_269
)

const (
	questMetaUpgrade  int32 = 9
	questWinRumble    int32 = 19
	questPlayGameMode int32 = 26
)

// The pool has one quest more than a day hands out: the client rerolls a
// quest by itself, picking at random among the pool's unused quests
// (0x785fac), so with exactly one left over the server knows its pick.
// Only quests the server can count from what it sees (battle results).
var dailyQuests = []questDef{
	{"PlayGameMode", questPlayGameMode, "TID_CHRONOS_QUESTS_PLAY_DUEL", 3,
		[]int32{chestQuestDailyMini}, map[string]any{"GameMode": "2_PLAYER"}},
	{"WinRumble", questWinRumble, "TID_CHRONOS_QUESTS_WINRUMBLE", 1,
		[]int32{chestQuestDailySmall}, nil},
	{"PlayGameMode", questPlayGameMode, "TID_CHRONOS_QUESTS_PLAY_RUMBLE", 2,
		[]int32{chestQuestDailyMini}, map[string]any{"GameMode": "RUMBLE"}},
	{"PlayGameMode", questPlayGameMode, "TID_CHRONOS_QUESTS_PLAY_DUEL", 5,
		[]int32{chestQuestGold100}, map[string]any{"GameMode": "2_PLAYER"}},
}

// questsPerDay: how many of the pool a day hands out.
const questsPerDay = 3

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
	for _, q := range dailyQuests {
		list = append(list, q.json())
	}
	pool := map[string]any{
		"Pool":             list,
		"CycleCategory":    "Daily",
		"DailyLimit":       questsPerDay,
		"CycleLimit":       questsPerDay + 1,
		"PoolPriority":     1,
		"DuplicateAllowed": false,
		"LevelLimitLow":    0,
		"LevelLimitHigh":   999,
	}
	b, _ := json.Marshal(pool)
	return []LiveEvent{{
		ID: dailyPoolEvent, Name: "daily_quests", Type: EventQuestPool,
		Start: start, End: end, VisibleStart: start, VisibleEnd: end,
		Parameters: string(b),
	}}
}
