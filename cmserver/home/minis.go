package home

import (
	"encoding/json"
	"fmt"
	"time"
)

// Mini quests (the quest screen's MINI QUESTS): for every mini the player
// owns, win 1, 3 and 5 matches using it, one after the other ("unlocking a
// mini gives a set of quests", digistatement); their rewards are bigger than
// the daily ones (quest_Daily_Mini: 1 Pony Fuel and a mini fragment, as on
// the screenshots). In the client a QUEST_CHAIN of category Tutorial (3,
// 0x71d454). Ours: one chain event with the quests of all the client's
// minis; the home hands out the next step of three owned minis at a time
// (deck minis first). The unit is a table 6 reference: table 6 holds the 55
// rows of characters.csv, then the 14 of characters_heroes.csv (the hook
// logged 69 items, rows 0..), so a mini is its characters.csv row + 1.

// miniQuestsOn: the quest screen lists only quests not yet ended
// (0x753e88: +0x1c end time > now), the button's count does not look at
// that; with end time 0 (until 0079) the button showed 3 done mini quests
// the screen did not list. With the end time they are on again.
var miniQuestsOn = true

// miniChainEvent: the chronos event of a mini's quest chain (win 1, 3, 5
// with it), one per mini with an open quest. After a claim the client
// hands out the chain's next step itself, as the server does
// (claimMiniQuest).
func miniChainEvent(mini int) int32 { return 1100 + int32(mini) }

// miniOfQuest: a mini quest's mini (index into miniCharRows) and step.
func miniOfQuest(q Quest) (int, int32) {
	return int(q.Index) / len(miniQuestWins), q.Index % int32(len(miniQuestWins))
}

// miniQuestsShown: mini quests in the list at a time.
const miniQuestsShown = 3

var miniQuestWins = []int32{1, 3, 5}

// miniChain: the chain's quests, three per mini of the client (by
// miniCharRows order); empty without game data (the names).
func miniChain() []questDef {
	if Data.CharName(miniCharRows[0]) == "" {
		return nil
	}
	var out []questDef
	for _, row := range miniCharRows {
		for _, n := range miniQuestWins {
			out = append(out, questDef{
				Kind: "WinWithUnit", TypeID: questWinWithUnit,
				Text: "TID_CHRONOS_QUESTS_WINWITHUNITSPECIFIC", Target: n,
				Rewards: []int32{chestQuestDailyMini},
				Own:     map[string]any{"UseRoundWins": false, "RequirementMini": Data.CharName(row)},
			})
		}
	}
	return out
}

// miniChainLiveEvent: the QUEST_CHAIN event of one mini.
func miniChainLiveEvent(mini int) LiveEvent {
	var list []any
	for _, q := range miniChain()[mini*len(miniQuestWins) : (mini+1)*len(miniQuestWins)] {
		list = append(list, q.json())
	}
	b, _ := json.Marshal(map[string]any{
		"Chain":          list,
		"Category":       "Tutorial",
		"LoopWhenDone":   false,
		"LevelLimitLow":  0,
		"LevelLimitHigh": 999,
	})
	y, m, d := now().Date()
	day := time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
	start := int32(day.AddDate(0, 0, -1).Unix())
	end := int32(day.AddDate(0, 0, 30).Unix())
	return LiveEvent{
		ID: miniChainEvent(mini), Name: fmt.Sprintf("mini_quests_%s", Data.CharName(miniCharRows[mini])),
		Type: EventQuestChain, Start: start, End: end, VisibleStart: start, VisibleEnd: end,
		Parameters: string(b),
	}
}

// miniChainEvents: the chain events of the minis with an open quest.
func (a *Account) miniChainEvents() []LiveEvent {
	var out []LiveEvent
	if !miniQuestsOn || len(miniChain()) == 0 {
		return nil
	}
	for _, q := range a.MiniQuests {
		if !q.Claimed {
			mini, _ := miniOfQuest(q)
			out = append(out, miniChainLiveEvent(mini))
		}
	}
	return out
}

// refreshMiniQuests hands out the next step of owned minis until three are
// open: the selected deck's minis first, then by characters.csv row.
func (a *Account) refreshMiniQuests() {
	if !miniQuestsOn || !Live || !a.Progression || len(miniChain()) == 0 {
		return
	}
	keep := a.MiniQuests[:0]
	open := map[int32]bool{}
	for _, q := range a.MiniQuests {
		if !q.Claimed {
			keep = append(keep, q)
			open[q.Index/int32(len(miniQuestWins))] = true
		}
	}
	a.MiniQuests = keep
	// Mini quests come with a newly unlocked mini, as in the original
	// (Deconstructor of Fun, January 2022: "triggered when a new unit is
	// unlocked"; decided 0097): only the minis unlocked since the account
	// was first seen (MiniNew), the earliest first. Quests open from
	// before stay until claimed.
	a.noteNewMinis()
	var order []int
	for _, r := range a.MiniNew {
		for i, sr := range miniSeasonalRows {
			if sr == r {
				order = append(order, i)
			}
		}
	}
	for _, i := range order {
		if len(a.MiniQuests) >= miniQuestsShown {
			break
		}
		if open[int32(i)] || a.owned(miniID(miniSeasonalRows[i])).Level == 0 {
			continue
		}
		step := a.MiniSteps[miniCharRows[i]]
		if int(step) >= len(miniQuestWins) {
			continue
		}
		a.QuestID++
		a.MiniQuests = append(a.MiniQuests, Quest{ID: a.QuestID, Index: int32(i*len(miniQuestWins)) + step})
	}
}

// miniQuestsWon: a won match counts for the mini quests of the minis in the
// selected deck.
func (a *Account) miniQuestsWon() {
	chain := miniChain()
	inDeck := map[int32]bool{}
	for _, r := range a.Decks[a.Selected] {
		inDeck[r] = true
	}
	for i := range a.MiniQuests {
		q := &a.MiniQuests[i]
		mini := int(q.Index) / len(miniQuestWins)
		if q.Claimed || int(q.Index) >= len(chain) || !inDeck[miniSeasonalRows[mini]] {
			continue
		}
		q.Progress = min(q.Progress+1, chain[q.Index].Target)
	}
}

// claimMiniQuest claims a mini quest (503): its chest, the mini's next step.
func (a *Account) claimMiniQuest(r *Reward, id int32) bool {
	chain := miniChain()
	for i := range a.MiniQuests {
		q := &a.MiniQuests[i]
		if q.ID != id || q.Claimed || int(q.Index) >= len(chain) || q.Progress < chain[q.Index].Target {
			continue
		}
		d := chain[q.Index]
		q.Claimed = true
		if a.MiniSteps == nil {
			a.MiniSteps = map[int32]int32{}
		}
		a.MiniSteps[miniCharRows[int(q.Index)/len(miniQuestWins)]]++
		var c ChestContent
		for _, chest := range d.Rewards {
			if cc, err := rollChest(a.random(), chest); err == nil {
				c.add(cc)
			}
		}
		// the chain's next step right away, with the next quest id, as
		// the client hands it out after the claim
		if _, step := miniOfQuest(*q); int(step)+1 < len(miniQuestWins) {
			a.QuestID++
			a.MiniQuests = append(a.MiniQuests, Quest{ID: a.QuestID, Index: q.Index + 1})
		}
		a.pushChest(r, d.Rewards[0], sourceQuest, c)
		if n := len(a.Chests); ChestsInHome && n > 0 && a.Chests[n-1].Index == a.ChestIndex {
			a.Chests[n-1].SourceValue = id
		}
		a.refreshMiniQuests()
		return true
	}
	return false
}

// noteNewMinis: owned minis the account has not seen yet are newly
// unlocked (any way: a chest, the shop, the client's own upgrade); the
// first time, every owned mini is just seen. MiniNew keeps the unlock
// order; a mini whose quests are all claimed leaves it.
func (a *Account) noteNewMinis() {
	first := len(a.MiniSeen) == 0
	seen := map[int32]bool{}
	for _, r := range a.MiniSeen {
		seen[r] = true
	}
	for _, r := range miniSeasonalRows {
		if seen[r] || a.owned(miniID(r)).Level == 0 {
			continue
		}
		a.MiniSeen = append(a.MiniSeen, r)
		if !first {
			a.MiniNew = append(a.MiniNew, r)
		}
	}
	keep := a.MiniNew[:0]
	for _, r := range a.MiniNew {
		for i, sr := range miniSeasonalRows {
			if sr == r && int(a.MiniSteps[miniCharRows[i]]) < len(miniQuestWins) {
				keep = append(keep, r)
			}
		}
	}
	a.MiniNew = keep
}

// events: the account's live events as its home has them: the global ones,
// the game modes of its league, its mini quest chains and the Clashmas
// group. The caller holds the lock and has refreshed the quests.
func (a *Account) events() []LiveEvent {
	events := LiveEvents()
	if e, ok := gameModeLiveEvent(a.Trophies); ok {
		events = append(events, e)
	}
	if Live {
		events = append(events, a.miniChainEvents()...)
		events = append(events, a.chainLiveEvents()...)
		if clashmasOn() && a.Progression {
			events = append(events, clashmasLiveEvent())
		}
	}
	return events
}

// NewMiniQuests hands out the mini quests of minis unlocked since the last
// look (a chest, the shop, an upgrade during the session) and, if there
// are any, returns the account's events for LogicUpdateChronosEventsCommand
// 417: on an events update the client starts every QUEST_CHAIN event it has
// no state for (0x682b80 -> 0x7b5e50 -> 0x52e6d0 -> 0x529788) with the
// chain's first quest and the next quest id, as the server does here; so
// the new mini's quest shows at once, not after the next home. The caller
// holds the lock.
func (a *Account) NewMiniQuests() ([]LiveEvent, bool) {
	if !Live || !miniQuestsOn || !a.Progression {
		return nil, false
	}
	before := a.QuestID
	a.refreshMiniQuests()
	if a.QuestID == before {
		return nil, false
	}
	return a.events(), true
}
