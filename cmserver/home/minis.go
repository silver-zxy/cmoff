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

// miniChainEvent: the chronos event of one mini quest: a QUEST_CHAIN of
// just that quest (1100 + its index, mini * 3 + step). A longer chain made
// the client hand out the next step itself after a claim, and that quest
// showed "<name>" (its mini was missing) until the home was loaded again.
// With one quest per chain a claim ends the chain; the next step comes
// with the next home, that is after the next match (the only thing that
// moves a mini quest).
func miniChainEvent(q Quest) int32 { return 1100 + q.Index }

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

// miniChainLiveEvent: the QUEST_CHAIN event of one open mini quest.
func miniChainLiveEvent(q Quest) LiveEvent {
	b, _ := json.Marshal(map[string]any{
		"Chain":          []any{miniChain()[q.Index].json()},
		"Category":       "Tutorial",
		"LoopWhenDone":   false,
		"LevelLimitLow":  0,
		"LevelLimitHigh": 999,
	})
	y, m, d := now().Date()
	day := time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
	start := int32(day.AddDate(0, 0, -1).Unix())
	end := int32(day.AddDate(0, 0, 30).Unix())
	mini, step := miniOfQuest(q)
	return LiveEvent{
		ID: miniChainEvent(q), Name: fmt.Sprintf("mini_quest_%s_%d", Data.CharName(miniCharRows[mini]), step+1),
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
			out = append(out, miniChainLiveEvent(q))
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
	var order []int
	inDeck := map[int32]bool{}
	for _, r := range a.Decks[a.Selected] {
		inDeck[r] = true
	}
	for i, r := range miniSeasonalRows {
		if inDeck[r] {
			order = append(order, i)
		}
	}
	for i, r := range miniSeasonalRows {
		if !inDeck[r] {
			order = append(order, i)
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
		a.pushChest(r, d.Rewards[0], sourceQuest, c)
		if n := len(a.Chests); ChestsInHome && n > 0 && a.Chests[n-1].Index == a.ChestIndex {
			a.Chests[n-1].SourceValue = id
		}
		a.refreshMiniQuests()
		return true
	}
	return false
}
