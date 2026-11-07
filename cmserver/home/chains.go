package home

import (
	"encoding/json"
	"fmt"
	"time"

	"cmserver/proto"
)

// Two quest chains of the client besides the mini quests (0114):
//
//   - The tutorial chain for new players (TID_CHRONOS_QUESTS_TUTORIAL_CHAIN_
//     1-4): change the deck to include Magic Archer (or Shield Skeleton),
//     play a match using Wizard, upgrade a hero, play a ranked match.
//     Category Tutorial: the quest screen's MINI QUESTS section. The deck
//     and upgrade kinds have no state of their own (MetaChangeDeck state
//     0xec8ba0, MetaUsePromotionToken 0xea9b90: writers that write
//     nothing); the unit is in the text only. Ours: the rewards (random
//     fragments, quest_daily_Mystery) and who gets it (an account at
//     player level 3 or lower, once).
//   - The competitive chain (TID_CHRONOS_QUESTS_COMPETITIVEQUESTCHAIN "Win
//     N rounds in Duel or Rumble", quest_Competitive_5W: 1 Crystal Coin),
//     Category Competitive (the quest screen's section 5), again and again
//     (LoopWhenDone). Ours: 5 rounds (the chest's name).
//
// A chain is a QUEST_CHAIN event (LogicLiveQuestChainData 0x71d454:
// Chain, Category, LoopWhenDone, LevelLimitLow/High) and in the home the
// chains' state (+0x40 the event and step, +0x50 its current quest). A
// battle copies the chains' quests (0x4f6854), so the client counts the
// battle kinds and the hook reports them (QuestsFromClient); the meta kinds
// (deck, upgrade) the server counts from the commands. After a claim the
// client hands out the next step itself, as the server does.

const (
	tutorialChainEvent    int32 = 1201
	competitiveChainEvent int32 = 1202
	questMetaChangeDeck   int32 = 17
	questMetaUseCoins     int32 = 11
	// questCategoryCompetitive: the quest screen's section 5 (0x5b5750)
	questCategoryCompetitive int32 = 5
	// tutorialChainMaxLevel: who gets the tutorial chain (ours)
	tutorialChainMaxLevel int32 = 3
)

// ChainState: one chain of the account: the step and its quest.
type ChainState struct {
	Event int32 `json:"event"`
	Step  int32 `json:"step"`
	Quest Quest `json:"quest"`
	Done  bool  `json:"done,omitempty"`
	// Variant: the tutorial chain's first step, Magic Archer (1) or
	// Shield Skeleton (0), picked at its start
	Variant int32 `json:"variant,omitempty"`
}

type chainDef struct {
	event    int32
	name     string
	category string
	catID    int32
	loop     bool
	steps    func(a *Account) []questDef
}

func tutorialSteps(a *Account) []questDef {
	// Magic Archer if the player has it and it is not in the deck, else
	// Shield Skeleton (the two texts of step 1)
	text := "TID_CHRONOS_QUESTS_TUTORIAL_CHAIN_1_SHIELDSKELETONSWAP"
	if s := a.chain(tutorialChainEvent); s != nil && s.Variant == 1 {
		text = "TID_CHRONOS_QUESTS_TUTORIAL_CHAIN_1_MAGICARCHERSWAP"
	}
	r := []int32{chestMystery}
	return []questDef{
		{"MetaChangeDeck", questMetaChangeDeck, text, 1, r, map[string]any{}},
		{"PlayWithUnit", questPlayWithUnit, "TID_CHRONOS_QUESTS_TUTORIAL_CHAIN_2", 1, r,
			map[string]any{"RequirementMini": "WizardB1"}},
		{"MetaUpgrade", questMetaUpgrade, "TID_CHRONOS_QUESTS_TUTORIAL_CHAIN_3", 1, r,
			map[string]any{"UnitUpgrades": false, "HeroUpgrades": true}},
		{"PlayGameMode", questPlayGameMode, "TID_CHRONOS_QUESTS_TUTORIAL_CHAIN_4", 1, r,
			map[string]any{"GameMode": "2_PLAYER"}},
	}
}

func competitiveSteps(*Account) []questDef {
	chest, ok := Data.ChestByName("quest_Competitive_5W")
	if !ok {
		return nil
	}
	return []questDef{{"WinWithUnit", questWinWithUnit, "TID_CHRONOS_QUESTS_COMPETITIVEQUESTCHAIN", 5,
		[]int32{chest}, map[string]any{"UseRoundWins": true}}}
}

var chainDefs = []chainDef{
	{tutorialChainEvent, "tutorial_chain", "Tutorial", questCategoryMini, false, tutorialSteps},
	{competitiveChainEvent, "competitive_chain", "Competitive", questCategoryCompetitive, true, competitiveSteps},
}

func chainByEvent(event int32) (chainDef, bool) {
	for _, c := range chainDefs {
		if c.event == event {
			return c, true
		}
	}
	return chainDef{}, false
}

// chainsOn: the chains need the game data (names, chests) and live quests.
func chainsOn() bool { return Live && Data != nil }

func (a *Account) ownsChar(row int32) bool {
	for i, r := range miniCharRows {
		if r == row {
			return a.owned(miniID(miniSeasonalRows[i])).Level > 0
		}
	}
	return false
}

func (a *Account) deckHasChar(row int32) bool {
	for _, s := range a.Decks[a.Selected] {
		for i, sr := range miniSeasonalRows {
			if sr == s && miniCharRows[i] == row {
				return true
			}
		}
	}
	return false
}

func (a *Account) chain(event int32) *ChainState {
	for i := range a.Chains {
		if a.Chains[i].Event == event {
			return &a.Chains[i]
		}
	}
	return nil
}

// refreshChains starts the chains the account gets (the caller holds the
// lock): the competitive one always, the tutorial one once for a new
// player.
func (a *Account) refreshChains() {
	if !chainsOn() || !a.Progression {
		return
	}
	for _, c := range chainDefs {
		if a.chain(c.event) != nil || len(c.steps(a)) == 0 {
			continue
		}
		if c.event == tutorialChainEvent && a.expLevel() > tutorialChainMaxLevel {
			continue
		}
		s := ChainState{Event: c.event}
		if row, ok := Data.CharRow("EliteArcherB1"); ok && c.event == tutorialChainEvent && a.ownsChar(row) && !a.deckHasChar(row) {
			s.Variant = 1
		}
		a.QuestID++
		s.Quest = Quest{ID: a.QuestID}
		a.Chains = append(a.Chains, s)
	}
}

// openChains: the chains with a quest open, their definitions and steps.
func (a *Account) openChains() []struct {
	c    chainDef
	s    *ChainState
	step questDef
} {
	var out []struct {
		c    chainDef
		s    *ChainState
		step questDef
	}
	if !chainsOn() {
		return nil
	}
	for i := range a.Chains {
		s := &a.Chains[i]
		c, ok := chainByEvent(s.Event)
		steps := c.steps(a)
		if !ok || s.Done || int(s.Step) >= len(steps) {
			continue
		}
		out = append(out, struct {
			c    chainDef
			s    *ChainState
			step questDef
		}{c, s, steps[s.Step]})
	}
	return out
}

// chainsProgress counts n for the open chain quests of a kind (match: the
// quest's own keys; a unit quest only with the unit in the deck).
func (a *Account) chainsProgress(kind, n int32, match func(questDef) bool) {
	for _, o := range a.openChains() {
		d := o.step
		if d.TypeID != kind || o.s.Quest.Progress >= d.Target || (match != nil && !match(d)) {
			continue
		}
		if name, _ := d.Own["RequirementMini"].(string); name != "" {
			row, ok := Data.CharRow(name)
			if !ok || !a.deckHasChar(row) {
				continue
			}
		}
		o.s.Quest.Progress = min(o.s.Quest.Progress+n, d.Target)
	}
}

// chainsFromClient: the client's counts after a battle (the hook).
func (a *Account) chainsFromClient(id, v int32) (string, bool) {
	for _, o := range a.openChains() {
		if o.s.Quest.ID != id {
			continue
		}
		if v = min(v, o.step.Target); v > o.s.Quest.Progress {
			o.s.Quest.Progress = v
			return fmt.Sprintf("#%d %s %d/%d", id, o.c.name, v, o.step.Target), true
		}
	}
	return "", false
}

// claimChainQuest claims a chain quest (503): its chest and the next step
// (or the first again for a loop), with the next quest id, as the client
// hands it out.
func (a *Account) claimChainQuest(r *Reward, id int32) bool {
	for _, o := range a.openChains() {
		s, d := o.s, o.step
		if s.Quest.ID != id || s.Quest.Progress < d.Target {
			continue
		}
		var c ChestContent
		for _, chest := range d.Rewards {
			if cc, err := rollChest(a.random(), chest); err == nil {
				c.add(cc)
			}
		}
		s.Step++
		if int(s.Step) >= len(o.c.steps(a)) {
			if o.c.loop {
				s.Step = 0
			} else {
				s.Done = true
			}
		}
		if !s.Done {
			a.QuestID++
			s.Quest = Quest{ID: a.QuestID}
		}
		a.pushChest(r, d.Rewards[0], sourceQuest, c)
		if n := len(a.Chests); ChestsInHome && n > 0 && a.Chests[n-1].Index == a.ChestIndex {
			a.Chests[n-1].SourceValue = id
		}
		return true
	}
	return false
}

// chainLiveEvents: the QUEST_CHAIN events of the open chains (a finished
// chain's event goes: the client would start it again, 0x529788).
func (a *Account) chainLiveEvents() []LiveEvent {
	var out []LiveEvent
	y, m, d := now().Date()
	day := time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
	start := int32(day.AddDate(0, 0, -1).Unix())
	end := int32(day.AddDate(0, 0, 30).Unix())
	for _, o := range a.openChains() {
		var list []any
		for _, q := range o.c.steps(a) {
			list = append(list, q.json())
		}
		b, _ := json.Marshal(map[string]any{
			"Chain":          list,
			"Category":       o.c.category,
			"LoopWhenDone":   o.c.loop,
			"LevelLimitLow":  0,
			"LevelLimitHigh": 999,
		})
		out = append(out, LiveEvent{ID: o.c.event, Name: o.c.name, Type: EventQuestChain,
			Start: start, End: end, VisibleStart: start, VisibleEnd: end, Parameters: string(b)})
	}
	return out
}

// chainQuest: one open chain quest for the home.
type chainQuest struct {
	event, step, category int32
	q                     Quest
	def                   questDef
}

func (a *Account) chainQuests() []chainQuest {
	var out []chainQuest
	for _, o := range a.openChains() {
		out = append(out, chainQuest{o.c.event, o.s.Step, o.c.catID, o.s.Quest, o.step})
	}
	return out
}

func encodeChainQuests(w *proto.Writer, l []chainQuest, end int32) {
	for _, c := range l {
		w.Int(c.event)
		w.Int(c.step)
		w.Bool(true)
		writeQuest(w, c.q, c.def, c.event, c.step, c.category, end)
	}
}

// chainsSummary: the open chain quests for the log.
func (a *Account) chainsSummary() []string {
	var l []string
	for _, o := range a.openChains() {
		l = append(l, fmt.Sprintf("%s #%d %s %d/%d", o.c.name, o.s.Quest.ID, o.step.Kind, o.s.Quest.Progress, o.step.Target))
	}
	return l
}
