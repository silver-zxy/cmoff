package home

import (
	"encoding/json"
	"fmt"
	"strings"
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
	// Variant: the tutorial chain's first step, picked once at its start
	// and kept (0126): 1 Magic Archer, 2 Shield Skeleton (0: not yet)
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

// tutorialDeckStep: step 1's text prefix (both minis).
const tutorialDeckStep = "TID_CHRONOS_QUESTS_TUTORIAL_CHAIN_1_"

// deckChanged: the deck changed (500, 595). Step 1 of the tutorial chain
// ("Modify your deck to include ...") is done once the deck has its mini;
// the client does not count it (a PlayWithUnit, counted in battle), so the
// server tells it at once with LogicUpdateQuestProgressCommand 351
// (TakeQuestUpdates). The caller holds the lock.
func (a *Account) deckChanged() {
	for _, o := range a.openChains() {
		d := o.step
		if !strings.HasPrefix(d.Text, tutorialDeckStep) || o.s.Quest.Progress >= d.Target {
			continue
		}
		name, _ := d.Own["RequirementMini"].(string)
		if row, ok := Data.CharRow(name); ok && a.deckHasChar(row) {
			o.s.Quest.Progress = d.Target
			a.questUpdates = append(a.questUpdates, chainQuest{o.c.event, o.s.Step, o.c.catID, o.s.Quest, d})
		}
	}
}

// TakeQuestUpdates: the quests whose progress the server changed for the
// client to see now (351), once. The caller holds the lock.
func (a *Account) TakeQuestUpdates() []chainQuest {
	l := a.questUpdates
	a.questUpdates = nil
	return l
}

// EncodeQuestUpdate writes LogicUpdateQuestProgressCommand 351 (decoder
// 0x48deb4: the server command base, Bool, VInt count and the quests as
// the home has them, 0x3a0e0c -> 0x3b3af4; execute 0x7249e4 ->
// 0x750b9c "LogicQuestManager::updateQuest": the quest by its id takes
// the new one, the screen shows the progress, a quest done is done).
func (a *Account) EncodeQuestUpdate(w *proto.Writer, l []chainQuest) {
	w.VInt(0) // server command base
	w.VInt(-1)
	w.VInt(0)
	w.VInt(1)
	w.VInt(0) // LogicCommand base
	w.Bool(true)
	w.VInt(int32(len(l)))
	end := questsEnd(a.dailyGold())
	for _, c := range l {
		writeQuest(w, c.q, c.def, c.event, c.step, c.category, end)
	}
}

// SkipQuest reads past a quest as writeQuest writes it (for 351's echo).
func SkipQuest(r *proto.Reader) {
	r.VInt()
	r.VInt()
	kind := r.VInt()
	if r.Bool() {
		switch kind {
		case questMetaUpgrade:
			r.Bool()
			r.Bool()
		case questWinWithUnit:
			r.Bool()
			r.VInt()
		case questAbilityHit:
			r.VInt()
			r.VInt()
			r.VInt()
		case questPlayGameMode, questPlayWithUnit, questResourceGained,
			questUpgradeUnit, questTotalKOs, questKeywordClash, questEndRoundAlive:
			r.VInt()
		}
	}
	_ = r.String()
	for i := 0; i < 5; i++ {
		r.VInt()
	}
	for n := r.VInt(); n > 0 && r.Err() == nil; n-- {
		r.VInt()
	}
	_ = r.String()
	r.VInt()
	r.VInt()
}

func tutorialSteps(a *Account) []questDef {
	// Step 1, "Modify your deck to include Magic Archer / Shield
	// Skeleton": as MetaChangeDeck the client counted any change of the
	// deck (the kind has no unit), so it is a PlayWithUnit of that mini,
	// which the client counts only in battle; the server marks it done
	// once the deck has the mini and tells the client at once (351,
	// deckChanged; 0120). The mini is the chain's Variant, picked once
	// (tutorialVariant): picked anew with every look, it changed when the
	// player got Shield Skeleton during the session, and the quest that
	// showed Magic Archer was done with Shield Skeleton (0126).
	mini, text := "EliteArcherB1", "TID_CHRONOS_QUESTS_TUTORIAL_CHAIN_1_MAGICARCHERSWAP"
	if s := a.chain(tutorialChainEvent); s != nil && s.Variant == 2 {
		mini, text = "ShieldSkeletonB1", "TID_CHRONOS_QUESTS_TUTORIAL_CHAIN_1_SHIELDSKELETONSWAP"
	}
	r := []int32{chestMystery}
	return []questDef{
		{"PlayWithUnit", questPlayWithUnit, text, 1, r, map[string]any{"RequirementMini": mini}},
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

// tutorialVariant: step 1's mini: Magic Archer (1) if the player has it,
// else Shield Skeleton (2) if the player has that, else none yet (0).
func (a *Account) tutorialVariant() int32 {
	for i, name := range []string{"EliteArcherB1", "ShieldSkeletonB1"} {
		if row, ok := Data.CharRow(name); ok && a.ownsChar(row) {
			return int32(i + 1)
		}
	}
	return 0
}

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
	if s := a.chain(tutorialChainEvent); s != nil && s.Variant == 0 {
		s.Variant = a.tutorialVariant()
		if s.Variant == 0 {
			s.Variant = 1
		}
	}
	for _, c := range chainDefs {
		if a.chain(c.event) != nil || len(c.steps(a)) == 0 {
			continue
		}
		s := ChainState{Event: c.event}
		if c.event == tutorialChainEvent {
			if a.expLevel() > tutorialChainMaxLevel {
				continue
			}
			// it starts once the player has one of step 1's minis
			if s.Variant = a.tutorialVariant(); s.Variant == 0 {
				continue
			}
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
		// a unit quest only the client can count: it knows which units
		// were played (the hook reports it, QuestsFromClient); by the deck
		// the server counted minis that were not used (0118)
		if name, _ := d.Own["RequirementMini"].(string); name != "" {
			continue
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
		// the next step with the next quest id, as the client hands it out
		// (0x605da0: the id is the manager's +0xb0, then +0xb0 + 1)
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
