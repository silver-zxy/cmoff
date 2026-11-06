package home

import (
	"encoding/json"
	"fmt"
	"time"

	"cmserver/proto"
)

// The Clashmas quests: a QUEST_GROUP event (LogicLiveQuestGroupData
// 0x608d94: TitleTID, Quests, PickLimit, PickRandomly, Milestones with
// QuestsDoneReq and Chest, LevelLimitLow/High). Clashmas was a time in
// which the group was on: a few of its quests at a time, finishing 12
// unlocked the Candy Elf skin (the milestone's chest), and after that the
// group went on with Crystal Coins (screenshots: 12/12 with quests still
// open). Our snapshot keeps Clashmas forever, so the group never ends; its
// list starts over once every quest was handed out.
//
// The quests are those of the screenshots; where a reward is not known
// (a "Musketeer fragment", "1 fragment") the game's random-fragments quest
// chest stands in. "Win 10 trophies" (MetaResourceGained) crashed the
// quest screen (0075) and "Prince charges 3 enemies" (AbilityHitInOneGame)
// has no known state; both are left out.
//
// The home holds the group's quests apart from the daily and chain ones
// (quest manager +0xa0, decoder 0x4bd6d0) and per group its state (+0x60
// event id, +0x70 quests done, +0x80 the indices handed out, +0x90 the
// milestones claimed; encoder 0x31b9b4, decoder 0x4bd7d4). A battle does
// not copy the group's quests (0x4f6854 takes +0x30 and +0x50 only), so
// the server counts them. A claim (503) counts as done (+0x70, 0x5ca20c)
// and the client hands out the next quest (0x5ca2a4 -> 0x9a5554); the
// milestone is claimed with LogicClaimQuestMilestoneCommand 508 (event,
// milestone; 0x7d7428).

const (
	clashmasEvent   int32 = 3001
	clashmasPick          = 3  // quests open at a time (PickLimit)
	clashmasDoneReq int32 = 12 // for the Candy Elf
	// chests.json
	chestCandyElf    int32 = 127_000_264 // shop_skin_SpearGoblinElf
	chestCoins5      int32 = 127_000_240 // shop_ChallengeCoin_5: 5 Crystal Coins
	chestMonk25      int32 = 127_000_075 // shop_miniHero_Monk_25
	chestMystery     int32 = 127_000_182 // quest_daily_Mystery: 1-3 random fragments
	supportRowChamp  int32 = 17          // support_heroes: Royal Champion
	clashmasCycleLen       = 3           // the list repeats this often in the event
)

// groupQuest is a group quest with the unit it needs in battle: a mini in
// the selected deck (characters.csv name) or the selected hero (support
// heroes row).
type groupQuest struct {
	def  questDef
	mini string
	hero int32
}

func coins(n int) []int32 {
	out := make([]int32, n)
	for i := range out {
		out[i] = chestCoins5
	}
	return out
}

// clashmasBase: the group's quests, in the order they are handed out.
func clashmasBase() []groupQuest {
	duels := func(n int32, r []int32) groupQuest { return groupQuest{def: playDuels(n, r)} }
	rumbles := func(n int32, r []int32) groupQuest { return groupQuest{def: playRumbles(n, r)} }
	return []groupQuest{
		duels(6, []int32{chestMystery}),
		rumbles(3, []int32{chestMystery}),
		{def: questDef{"WinDuelPerfect", questWinDuelPerfect, "TID_CHRONOS_QUESTS_WINDUELPERFECT", 3,
			[]int32{chestMystery}, nil}},
		{def: questDef{"PlayWithUnit", questPlayWithUnit, "TID_CHRONOS_QUESTS_PLAYWITHUNITSPECIFIC", 6,
			coins(3), map[string]any{"RequirementMini": "HeroRoyalChampion"}}, hero: supportRowChamp},
		{def: questDef{"MetaSeasonRoadLevel", questSeasonRoadLevel, "TID_CHRONOS_QUESTS_SEASONPASSLEVELUP", 5,
			[]int32{chestCoins5, chestMonk25}, map[string]any{}}},
		duels(20, coins(3)),
		{def: questDef{"WinWithUnit", questWinWithUnit, "TID_CHRONOS_QUESTS_WINWITHUNITSPECIFIC", 3,
			coins(2), map[string]any{"UseRoundWins": false, "RequirementMini": "MusketeerB1"}}, mini: "MusketeerB1"},
		rumbles(10, coins(3)),
	}
}

// clashmasQuests: the event's list, the base list a few times over.
func clashmasQuests() []groupQuest {
	var out []groupQuest
	for i := 0; i < clashmasCycleLen; i++ {
		out = append(out, clashmasBase()...)
	}
	return out
}

// clashmasLiveEvent: the QUEST_GROUP event.
func clashmasLiveEvent() LiveEvent {
	var list []any
	for _, g := range clashmasQuests() {
		list = append(list, g.def.json())
	}
	b, _ := json.Marshal(map[string]any{
		"TitleTID":     "TID_CHRONOS_CLASHMAS_QUEST_GROUP",
		"Quests":       list,
		"PickLimit":    clashmasPick,
		"PickRandomly": false,
		"Milestones": []any{map[string]any{
			"QuestsDoneReq": clashmasDoneReq,
			"Chest":         "shop_skin_SpearGoblinElf",
		}},
		"LevelLimitLow":  0,
		"LevelLimitHigh": 999,
	})
	y, m, d := now().Date()
	day := time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
	// two weeks ahead, again with every home: the clock shows about 14
	// days, as on the screenshots, and never runs out
	start := int32(day.AddDate(0, 0, -1).Unix())
	end := int32(day.AddDate(0, 0, 15).Unix())
	return LiveEvent{
		ID: clashmasEvent, Name: "clashmas_quests", Type: EventQuestGroup,
		Start: start, End: end, VisibleStart: start, VisibleEnd: end,
		Parameters: string(b),
	}
}

// clashmasOn: the group needs the game data (the quests' units) and the
// quests on.
func clashmasOn() bool { return Live && Data != nil }

// refreshClashmas keeps three of the group's quests open: the claimed
// ones go, the next ones are the lowest indices not yet handed out (as the
// client picks with PickRandomly false); once all were handed out the list
// starts over.
func (a *Account) refreshClashmas() {
	if !clashmasOn() || !a.Progression {
		return
	}
	list := clashmasQuests()
	keep := a.Clashmas.Quests[:0]
	for _, q := range a.Clashmas.Quests {
		if !q.Claimed && int(q.Index) < len(list) {
			keep = append(keep, q)
		}
	}
	a.Clashmas.Quests = keep
	for len(a.Clashmas.Quests) < clashmasPick {
		i := a.clashmasNext(len(list))
		if i < 0 {
			// all handed out: the list starts over (the open ones stay
			// taken)
			a.Clashmas.Taken = nil
			for _, q := range a.Clashmas.Quests {
				a.Clashmas.Taken = append(a.Clashmas.Taken, q.Index)
			}
			if i = a.clashmasNext(len(list)); i < 0 {
				return
			}
		}
		a.QuestID++
		a.Clashmas.Quests = append(a.Clashmas.Quests, Quest{ID: a.QuestID, Index: i})
		a.Clashmas.Taken = append(a.Clashmas.Taken, i)
	}
}

func (a *Account) clashmasNext(n int) int32 {
	taken := map[int32]bool{}
	for _, i := range a.Clashmas.Taken {
		taken[i] = true
	}
	for i := int32(0); i < int32(n); i++ {
		if !taken[i] {
			return i
		}
	}
	return -1
}

// clashmasProgress counts for the group's open quests of a kind whose
// unit, if any, was in the battle.
func (a *Account) clashmasProgress(kind, n int32, match func(questDef) bool) {
	if !clashmasOn() {
		return
	}
	list := clashmasQuests()
	inDeck := map[string]bool{}
	for _, r := range a.Decks[a.Selected] {
		for i, sr := range miniSeasonalRows {
			if sr == r {
				inDeck[Data.CharName(miniCharRows[i])] = true
			}
		}
	}
	for i := range a.Clashmas.Quests {
		q := &a.Clashmas.Quests[i]
		if q.Claimed || int(q.Index) >= len(list) {
			continue
		}
		g := list[q.Index]
		if g.def.TypeID != kind || q.Progress >= g.def.Target || (match != nil && !match(g.def)) {
			continue
		}
		if (g.mini != "" && !inDeck[g.mini]) || (g.hero != 0 && a.Heroes[a.Selected] != g.hero) {
			continue
		}
		q.Progress = min(q.Progress+n, g.def.Target)
	}
}

// claimClashmasQuest claims a group quest (503): its chests, one more
// done, the next quest.
func (a *Account) claimClashmasQuest(r *Reward, id int32) bool {
	list := clashmasQuests()
	for i := range a.Clashmas.Quests {
		q := &a.Clashmas.Quests[i]
		if q.ID != id || q.Claimed || int(q.Index) >= len(list) || q.Progress < list[q.Index].def.Target {
			continue
		}
		d := list[q.Index].def
		q.Claimed = true
		a.Clashmas.Done++
		var c ChestContent
		for _, chest := range d.Rewards {
			if cc, err := rollChest(a.random(), chest); err == nil {
				c.add(cc)
			}
		}
		a.pushChest(r, d.Rewards[0], sourceQuest, c)
		if n := len(a.Chests); ChestsInHome && n > 0 && a.Chests[n-1].Index == a.ChestIndex {
			a.Chests[n-1].SourceValue = id
		}
		a.refreshClashmas()
		return true
	}
	return false
}

// ClaimMilestone is 508: the group's milestone (the Candy Elf) once its
// quests are done.
func (a *Account) ClaimMilestone(r *Reward, event, milestone int32) error {
	if event != clashmasEvent || milestone != 0 {
		return fmt.Errorf("no milestone %d in event %d", milestone, event)
	}
	if a.Clashmas.Done < clashmasDoneReq {
		return fmt.Errorf("%d of %d quests done", a.Clashmas.Done, clashmasDoneReq)
	}
	for _, m := range a.Clashmas.Claimed {
		if m == milestone {
			return fmt.Errorf("already claimed")
		}
	}
	a.Clashmas.Claimed = append(a.Clashmas.Claimed, milestone)
	c, err := rollChest(a.random(), chestCandyElf)
	if err != nil {
		return err
	}
	a.pushChest(r, chestCandyElf, sourceQuest, c)
	return nil
}

// ClashmasState: the group's quests and state as the home holds them.
type ClashmasState struct {
	Quests  []Quest `json:"quests,omitempty"`
	Done    int32   `json:"done,omitempty"`
	Taken   []int32 `json:"taken,omitempty"`
	Claimed []int32 `json:"claimed,omitempty"`
}

// encodeGroups writes the group quests (+0xa0) and the groups' state.
func encodeGroups(w *proto.Writer, s ClashmasState, on bool, end int32) {
	if !on {
		w.VInt(0)
		w.VInt(0)
		return
	}
	list := clashmasQuests()
	var open []Quest
	for _, q := range s.Quests {
		if !q.Claimed && int(q.Index) < len(list) {
			open = append(open, q)
		}
	}
	w.VInt(int32(len(open)))
	for _, q := range open {
		writeQuest(w, q, list[q.Index].def, clashmasEvent, q.Index, questCategoryGroup, end)
	}
	w.VInt(1)
	w.VInt(clashmasEvent)
	w.VInt(s.Done)
	intList(w, s.Taken)
	intList(w, s.Claimed)
}

// clashmasSummary: the group's quests for the log.
func (a *Account) clashmasSummary() []string {
	list := clashmasQuests()
	var l []string
	for _, q := range a.Clashmas.Quests {
		if !q.Claimed && int(q.Index) < len(list) {
			g := list[q.Index]
			l = append(l, fmt.Sprintf("clashmas #%d %s %d/%d", q.ID, g.def.Kind, q.Progress, g.def.Target))
		}
	}
	if len(l) > 0 {
		l = append(l, fmt.Sprintf("clashmas done %d/%d", a.Clashmas.Done, clashmasDoneReq))
	}
	return l
}
