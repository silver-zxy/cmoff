package home

import (
	"fmt"
	"strings"
	"time"

	"cmserver/proto"
)

// Daily quests. The client does not pick its quests from the pool: the
// server hands them out in the home's quest manager (+0x180, encoder
// 0x31b774, decoder 0x4bccd8), which the client shows and counts.
//
// Quest manager: bool +0xc8, VInt +0xb0 +0xb4 +0xe4 +0xb8 +0xe8, timers
// +0xcc and +0xd8 (0x4a47a4, daily and weekly reset), then five lists: the
// active quests (+0x30, each 0x951a38), (int, int, int list) +0x0/+0x10/
// +0x20, (long, optional quest) +0x40/+0x50, quests +0xa0, (VInt, VInt, int
// list, int list) +0x60.. .
//
// One quest (0x951a38 with the full tail; offline JSON keys of 0x6e10c4 in
// brackets): VInt +0 [ce] chronos event, VInt +4 [cx] index in the pool,
// VInt +8 [t] quest kind, bool + kind state (+0x10, made by 0x506b4c from
// the kind), string +0x20 [tx] text, VInt +0x38 [id], +0x3c [pc] progress,
// +0x40 [pcm] target (claimable at progress >= target, 0x5fc0c8), +0x30
// [p], +0x34 [cat] category (Daily 2, Weekly 4: 0x6b2d5c), VInt list
// +0x48 [r] reward chests, string +0x58 [itag], VInt +0x18 [tg], +0x1c
// [te].
//
// Claiming is LogicClaimQuestRewardCommand 503 (base, VInt quest id, bool):
// the client makes one chest of the quest's rewards (0x6e1a5c, source 2,
// value the quest id, 0x5c9f14) whose content the server pushes with 420.

// Quest is one daily quest of the account.
type Quest struct {
	ID       int32 `json:"id"`
	Index    int32 `json:"index"` // in todaysQuests
	Progress int32 `json:"progress"`
	Claimed  bool  `json:"claimed,omitempty"`
}

const (
	questCategoryMini  int32 = 3 // "Tutorial", the quest screen's MINI QUESTS (0x71d454)
	questCategoryDaily int32 = 2
	// questCategoryGroup: 6, the timed event's block under the group's
	// header (the quest screen's section by category, 0x5b5750: 1
	// season_quest, 2 daily_quest, 3 tutorial_quest, 4 weekly_quest, 5
	// competitive_quest, 6 timed_event_quest; anything else is the
	// tutorial's). 1 put the Clashmas quests under SEASONAL QUEST (0087), 0
	// under a second MINI QUESTS (0088).
	questCategoryGroup int32 = 6
	sourceQuest        int32 = 2
)

// refreshQuests: today's quests, new ones when the day changed, and the
// next ones while fewer than questsConcurrent are open.
func (a *Account) refreshQuests() {
	if !Live {
		return
	}
	day := now().Format("2006-01-02")
	pool := questPoolSignature()
	if a.QuestDay != day || a.QuestPool != pool {
		// a changed pool makes new quests too: the saved ones point into
		// it by index
		a.QuestDay = day
		a.QuestPool = pool
		a.QuestRerolls = 0
		a.QuestDropped = nil
		a.Quests = a.Quests[:0]
	}
	a.topUpQuests()
}

// questRerollIndex: the pool index kept for the reroll (the pool's last).
const questRerollIndex = questPoolSize - 1

// topUpQuests hands out the pool's next quests while fewer than questsConcurrent are
// open and fewer than six were handed out today. The next is the lowest
// index not handed out, not rerolled away, and not the reroll's.
func (a *Account) topUpQuests() {
	for {
		open := 0
		for _, q := range a.Quests {
			if !q.Claimed {
				open++
			}
		}
		if open >= questsConcurrent || len(a.Quests) >= questsPerDay {
			return
		}
		next := a.nextQuestIndex()
		if next < 0 {
			return
		}
		a.QuestID++
		a.Quests = append(a.Quests, Quest{ID: a.QuestID, Index: next})
	}
}

func (a *Account) nextQuestIndex() int32 {
	taken := a.questIndicesTaken()
	for i := int32(0); i < questPoolSize; i++ {
		if !taken[i] && i != questRerollIndex {
			return i
		}
	}
	return -1
}

// questIndicesTaken: the pool indices handed out today or rerolled away.
func (a *Account) questIndicesTaken() map[int32]bool {
	taken := map[int32]bool{}
	for _, q := range a.Quests {
		taken[q.Index] = true
	}
	for _, i := range a.QuestDropped {
		taken[i] = true
	}
	return taken
}

// questPoolTaken: the taken indices the home tells the client. Its reroll
// (0x785fac) picks at random among the indices not listed, so every index
// but the reroll's is listed, the ones still to come included; after the
// reroll only the one rerolled away is free (the client frees it itself).
func (a *Account) questPoolTaken() []int32 {
	var out []int32
	for i := int32(0); i < questPoolSize; i++ {
		if a.QuestRerolls < questRerollsPerDay && i == questRerollIndex {
			continue
		}
		dropped := false
		for _, d := range a.QuestDropped {
			dropped = dropped || d == i
		}
		if !dropped {
			out = append(out, i)
		}
	}
	return out
}

// questPoolSignature names the pool's quests, to see when it changed.
func questPoolSignature() string {
	var b strings.Builder
	for _, d := range todaysQuests() {
		gm, _ := d.Own["GameMode"].(string)
		fmt.Fprintf(&b, "%s/%s/%d/%v;", d.Kind, gm, d.Target, d.Rewards)
	}
	return b.String()
}

// RerollQuest is LogicRerollQuestCommand 502 (base, VInt quest id): the
// client replaces the quest by an unused one of the pool itself; the home
// leaves only the reroll's index free, so that is its pick. The new quest
// takes the next id (+0xb0 counts them).
func (a *Account) RerollQuest(id int32) (from, to string, ok bool) {
	a.refreshQuests()
	pool := todaysQuests()
	if a.QuestRerolls >= questRerollsPerDay || a.questIndicesTaken()[questRerollIndex] {
		return "", "", false
	}
	for i := range a.Quests {
		q := &a.Quests[i]
		if q.ID != id || q.Claimed {
			continue
		}
		from = pool[q.Index].Text
		a.QuestDropped = append(a.QuestDropped, q.Index)
		a.QuestID++
		*q = Quest{ID: a.QuestID, Index: questRerollIndex}
		a.QuestRerolls++
		return from, pool[questRerollIndex].Text, true
	}
	return "", "", false
}

// questRerollsPerDay: QUEST_SETTINGS DailyQuestRerollLimit.
const questRerollsPerDay = 1

// questProgress counts for the quests of a kind; match says whether this
// quest counts it.
func (a *Account) questProgress(kind int32, n int32, match func(questDef) bool) {
	a.refreshQuests()
	pool := todaysQuests()
	for i := range a.Quests {
		q := &a.Quests[i]
		d := pool[q.Index]
		if q.Claimed || d.TypeID != kind || q.Progress >= d.Target || (match != nil && !match(d)) {
			continue
		}
		// a quest with a unit (the hero quests) only the client can
		// count: it knows the battle's units (QuestsFromClient)
		if name, _ := d.Own["RequirementMini"].(string); name != "" {
			continue
		}
		q.Progress = min(q.Progress+n, d.Target)
	}
	a.clashmasProgress(kind, n, match)
}

// questsAfterBattle: a finished battle counts for PlayGameMode and
// PlayWithUnit, won for WinWithUnit (a duel won, a Rumble in the top 4)
// and, first in a Rumble, WinRumble.
func (a *Account) questsAfterBattle(mode, place int32) {
	a.questProgress(questPlayGameMode, 1, func(d questDef) bool {
		gm, _ := d.Own["GameMode"].(string)
		return (gm == "2_PLAYER" && mode == ModeDuel) || (gm == "RUMBLE" && mode == ModeRumble)
	})
	a.questProgress(questPlayWithUnit, 1, nil)
	if won(mode, place) {
		a.questProgress(questWinWithUnit, 1, func(d questDef) bool {
			r, _ := d.Own["UseRoundWins"].(bool)
			return !r // rounds: the client counts them
		})
	}
	if won(mode, place) {
		a.miniQuestsWon()
	}
	if mode == ModeRumble && place == 1 { // "Finish as the Champion"
		a.questProgress(questWinRumble, 1, nil)
	}
}

// BattleRounds counts the rounds the hook read from a finished battle:
// rounds won for "win N rounds", a duel won without a round lost for 3:0
// (WinDuelPerfect). lost < 0: not known. The caller holds the lock.
func (a *Account) BattleRounds(mode, place, wonRounds, lost int32) {
	a.refreshQuests()
	if wonRounds > 0 {
		a.questProgress(questWinWithUnit, wonRounds, func(d questDef) bool {
			r, _ := d.Own["UseRoundWins"].(bool)
			return r
		})
	}
	if mode == ModeDuel && place == 1 && lost == 0 {
		a.questProgress(questWinDuelPerfect, 1, nil)
	}
	// "Use Prince to Charge 3 enemies in a single match": the client counts
	// the hits in battle, but a battle copies no group quests (0x4f6854),
	// and the hook does not see the hits. Ours: the Prince charges once a
	// round (prince_prebattle_rush, his start-up ability), so a match of 3
	// rounds or more with him in the deck does it.
	if wonRounds+lost >= 3 {
		a.clashmasProgress(questAbilityHit, 1, nil)
	}
}

// QuestsFromClient takes the progress the client counted itself (the hook
// reads it after a battle: battle kinds such as knock outs, rounds or 3:0
// only the client sees). A quest only goes up, to its target at most. The
// caller holds the lock. It returns what changed, for the log.
func (a *Account) QuestsFromClient(progress [][2]int32) (changed []string) {
	a.refreshQuests()
	pool := todaysQuests()
	chain := miniChain()
	for _, p := range progress {
		for i := range a.MiniQuests {
			q := &a.MiniQuests[i]
			if q.ID != p[0] || q.Claimed || int(q.Index) >= len(chain) {
				continue
			}
			d := chain[q.Index]
			if v := min(p[1], d.Target); v > q.Progress {
				q.Progress = v
				changed = append(changed, fmt.Sprintf("#%d mini %d/%d", q.ID, v, d.Target))
			}
		}
		for i := range a.Quests {
			q := &a.Quests[i]
			if q.ID != p[0] || q.Claimed || int(q.Index) >= len(pool) {
				continue
			}
			d := pool[q.Index]
			if v := min(p[1], d.Target); v > q.Progress {
				q.Progress = v
				changed = append(changed, fmt.Sprintf("#%d %s %d/%d", q.ID, d.Kind, v, d.Target))
			}
		}
	}
	return changed
}

// questsTrophies: trophies won count for "Win N Trophies".
func (a *Account) questsTrophies(n int32) {
	if n > 0 {
		a.questProgress(questResourceGained, n, func(d questDef) bool {
			return d.Own["ResourceData"] == resourceRankPointsKey
		})
	}
}

// questsRoadTiers: tiers reached on the road count for "Advance on the
// Adventure Road N times".
func (a *Account) questsRoadTiers(n int32) {
	if n > 0 {
		a.questProgress(questSeasonRoadLevel, n, nil)
	}
}

// ClaimQuest is 503: the quest's reward chest, made by the client.
func (a *Account) ClaimQuest(r *Reward, id int32) bool {
	if a.claimMiniQuest(r, id) || a.claimClashmasQuest(r, id) {
		return true
	}
	a.refreshQuests()
	pool := todaysQuests()
	for i := range a.Quests {
		q := &a.Quests[i]
		d := pool[q.Index]
		if q.ID != id || q.Claimed || q.Progress < d.Target {
			continue
		}
		q.Claimed = true
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
		// the next quest takes the claimed one's place (in the next home)
		a.topUpQuests()
		return true
	}
	return false
}

func intList(w *proto.Writer, xs []int32) {
	w.VInt(int32(len(xs)))
	for _, x := range xs {
		w.VInt(x)
	}
}

// encodeQuests writes the home's quest manager with the active quests
// (claimed ones are left out).
// questsEnd: when today's quests end, the next daily reset.
func questsEnd(d dailyGold) int32 {
	if d.ticksLeft > 0 {
		return int32(now().Unix()) + d.ticksLeft/20
	}
	y, m, dd := now().UTC().Date()
	return int32(time.Date(y, m, dd+1, 0, 0, 0, 0, time.UTC).Unix())
}

func encodeQuests(w *proto.Writer, s questState, d dailyGold) {
	quests := s.quests
	done := int32(0)
	for _, q := range quests {
		if q.Claimed {
			done++
		}
	}
	w.Bool(Live)        // +0xc8: quests on; off, 502/503/508/351 fail with 3 (0x5c53dc)
	w.VInt(s.lastID)    // +0xb0: the last quest id (the client counts on)
	w.VInt(s.rerolls)   // +0xb4: daily rerolls used (0x4974c4)
	w.VInt(done)        // +0xe4: daily quests done ("Quests completed", 0x90d7c0)
	w.VInt(0)           // +0xb8: weekly rerolls used
	w.VInt(0)           // +0xe8: weekly quests done
	w.VInt(d.ticksLeft) // daily reset timer (0x4a47a4)
	w.VInt(d.ticksLeft)
	if d.ticksLeft > 0 {
		w.VInt(-1)
	} else {
		w.VInt(0)
	}
	vints(w, 3) // weekly reset timer
	var active []Quest
	for _, q := range quests {
		if !q.Claimed {
			active = append(active, q)
		}
	}
	pool := todaysQuests()
	type shown struct {
		q        Quest
		def      questDef
		event    int32
		category int32
	}
	var list []shown
	for _, q := range active {
		list = append(list, shown{q, pool[q.Index], dailyPoolEvent, questCategoryDaily})
	}
	w.VInt(int32(len(list)))
	for _, e := range list {
		writeQuest(w, e.q, e.def, e.event, e.q.Index, e.category, questsEnd(d))
	}
	// the pools' state (+0x0 event, +0x10 handed out, +0x20 the taken
	// indices; encoder 0x31b864): a reroll (0x785fac) takes one back and
	// picks among the free indices, so with all but one listed its pick is
	// the server's (questPoolTaken)
	if len(quests) > 0 {
		w.VInt(1)
		w.Int(dailyPoolEvent)
		w.Int(int32(len(quests)))
		intList(w, s.taken)
	} else {
		w.VInt(0)
	}
	// the chains' state (+0x40 a LogicLong per chain: its event id, ours
	// the step; +0x50 the chain's current quest or none; encoder 0x31b91c,
	// decoder 0x4bd2d8). The quest screen shows a chain's quest from here
	// (0x7a3250 looks the chain's event up by the key's id), not from the
	// active list: mini quests there showed in the quest button's count
	// but not on the screen.
	chain := miniChain()
	var open []Quest
	for _, q := range s.mini {
		if miniQuestsOn && !q.Claimed && int(q.Index) < len(chain) {
			open = append(open, q)
		}
	}
	w.VInt(int32(len(open)))
	for _, q := range open {
		mini, step := miniOfQuest(q)
		w.Int(miniChainEvent(mini))
		w.Int(step)
		w.Bool(true)
		writeQuest(w, q, chain[q.Index], miniChainEvent(mini), step, questCategoryMini, questsEnd(d))
	}
	encodeGroups(w, s.clashmas, s.groups, questsEnd(d))
}

// writeQuest writes one quest (decoder 0x3b3af4, the short form).
func writeQuest(w *proto.Writer, q Quest, def questDef, event, index, category, end int32) {
	w.VInt(event)
	w.VInt(index)
	w.VInt(def.TypeID)
	// the kind's state (0x506b4c makes it by kind; its vtable+0x18
	// writes it): MetaUpgrade two bools (0xea0478), WinRumble nothing
	// (0xeb3fe0), PlayGameMode the mode, 2_PLAYER 2 / RUMBLE 3
	// (0xec3568, 0x86517c)
	w.Bool(true)
	def.encodeState(w)
	w.String(def.Text)
	w.VInt(q.ID)
	w.VInt(q.Progress)
	w.VInt(def.Target)
	w.VInt(0) // +0x30
	w.VInt(category)
	intList(w, def.Rewards)
	w.String("") // +0x58
	w.VInt(0)    // +0x18
	// +0x1c: the end time (unix seconds, as the events'). A battle copies
	// only the quests not yet ended (0x4f6854: +0x1c > now), and the quest
	// screen lists only those (0x753e88); with 0 both had none.
	w.VInt(end)
}

// QuestSummary: today's quests for the log ("" without live content).
func (a *Account) QuestSummary() string {
	a.Lock()
	defer a.Unlock()
	if !Live {
		return ""
	}
	a.refreshQuests()
	var l []string
	pool := todaysQuests()
	for _, q := range a.Quests {
		d := pool[q.Index]
		st := ""
		if q.Claimed {
			st = " claimed"
		}
		l = append(l, fmt.Sprintf("#%d %s %d/%d%s", q.ID, d.Kind, q.Progress, d.Target, st))
	}
	chain := miniChain()
	for _, q := range a.MiniQuests {
		if !q.Claimed && int(q.Index) < len(chain) {
			mini, _ := miniOfQuest(q)
			l = append(l, fmt.Sprintf("mini #%d %s %d/%d", q.ID, Data.CharName(miniCharRows[mini]), q.Progress, chain[q.Index].Target))
		}
	}
	l = append(l, a.clashmasSummary()...)
	return strings.Join(l, ", ")
}

func (d questDef) encodeState(w *proto.Writer) {
	switch d.TypeID {
	case questWinWithUnit:
		// UseRoundWins, RequirementMini (a characters reference, 0x37cc70:
		// row + 1, 0 for none) (0x4c7400 / 0x3d85dc). A Bool +0x11 follows
		// only in the other form: the home's quests are read with flag 0
		// (0x4bce10 → 0x3b3af4 → the state's reader), which reads the
		// quest's tail and not that Bool. Writing it shifted the rest of
		// the home (the client crashed in the next component).
		r, _ := d.Own["UseRoundWins"].(bool)
		w.Bool(r)
		w.VInt(d.req())
	case questPlayWithUnit, questUpgradeUnit, questTotalKOs, questKeywordClash, questEndRoundAlive:
		w.VInt(d.req()) // RequirementMini, a table 6 reference: none (0x5190c8, 0x4709ec, 0x4c8eac, 0x5d031c, 0x946038)
	case questWinDuelPerfect, questWinGameNoReroll, questWinRoundNoDeath:
		// nothing in the short form (a Bool only in the other, 0x8f7258,
		// 0x92fd04; 0x792064 none)
	case questResourceGained:
		// ResourceData: a resources reference, row + 1 (0x473414). The live
		// data's key is ResourceData (0x546b0c); 0075 sent "Resource", the
		// client's quest had no resource and its hint crashed (0x834d64)
		w.VInt(resourceRankPoints + 1)
	case questAbilityHit:
		// Unit (table 6, row + 1), AbilityMask, HitsReqInOneGame (state
		// 0xea24a8: writer 0x960238, JSON 0x4708a0; the live data's bools
		// CountAttack 1, CountSuper 2, CountStartUp 4, CountDeath 8,
		// 0x80a300)
		unit := int32(0)
		if name, _ := d.Own["Unit"].(string); name != "" {
			if row, ok := Data.CharRow(name); ok {
				unit = row + 1
			}
		}
		mask := int32(0)
		for bit, k := range []string{"CountAttack", "CountSuper", "CountStartUp", "CountDeath"} {
			if b, _ := d.Own[k].(bool); b {
				mask |= 1 << bit
			}
		}
		hits, _ := d.Own["HitsReqInOneGame"].(int)
		w.VInt(unit)
		w.VInt(mask)
		w.VInt(int32(hits))
	case questMetaUpgrade:
		u, _ := d.Own["UnitUpgrades"].(bool)
		h, _ := d.Own["HeroUpgrades"].(bool)
		w.Bool(u)
		w.Bool(h)
	case questPlayGameMode:
		gm, _ := d.Own["GameMode"].(string)
		m := int32(0)
		switch gm {
		case "2_PLAYER":
			m = 2
		case "RUMBLE":
			m = 3
		}
		w.VInt(m)
	}
}

// questState: what the home's quest manager shows.
type questState struct {
	quests  []Quest
	lastID  int32
	rerolls int32
	taken   []int32 // the pool's taken indices (questPoolTaken)
	mini    []Quest // the mini quests (minis.go)
	// clashmas: the Clashmas group (clashmas.go), sent if groups
	clashmas ClashmasState
	groups   bool
}
