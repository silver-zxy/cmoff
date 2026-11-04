package home

import (
	"fmt"
	"strings"

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
	Index    int32 `json:"index"` // in dailyQuests
	Progress int32 `json:"progress"`
	Claimed  bool  `json:"claimed,omitempty"`
}

const (
	questCategoryDaily int32 = 2
	sourceQuest        int32 = 2
)

// dailyQuestsFor: today's quests, new ones when the day changed.
func (a *Account) refreshQuests() {
	if !Live {
		return
	}
	day := now().Format("2006-01-02")
	pool := questPoolSignature()
	if a.QuestDay == day && a.QuestPool == pool && len(a.Quests) > 0 {
		return
	}
	// a changed pool makes new quests too: the saved ones point into it
	// by index
	a.QuestDay = day
	a.QuestPool = pool
	a.QuestRerolls = 0
	a.Quests = a.Quests[:0]
	for i := 0; i < questsPerDay; i++ {
		a.QuestID++
		a.Quests = append(a.Quests, Quest{ID: a.QuestID, Index: int32(i)})
	}
}

// questPoolSignature names the pool's quests, to see when it changed.
func questPoolSignature() string {
	var b strings.Builder
	for _, d := range dailyQuests {
		gm, _ := d.Own["GameMode"].(string)
		fmt.Fprintf(&b, "%s/%s/%d/%v;", d.Kind, gm, d.Target, d.Rewards)
	}
	return b.String()
}

// RerollQuest is LogicRerollQuestCommand 502 (base, VInt quest id): the
// client replaces the quest by an unused one of the pool itself; with one
// left over that is the pool's last quest. The new quest takes the next id
// (+0xb0 counts them).
func (a *Account) RerollQuest(id int32) (from, to string, ok bool) {
	a.refreshQuests()
	used := map[int32]bool{}
	for _, q := range a.Quests {
		used[q.Index] = true
	}
	free := int32(-1)
	for i := range dailyQuests {
		if !used[int32(i)] {
			free = int32(i)
			break
		}
	}
	for i := range a.Quests {
		q := &a.Quests[i]
		if q.ID != id || q.Claimed || free < 0 || a.QuestRerolls >= questRerollsPerDay {
			continue
		}
		from = dailyQuests[q.Index].Text
		a.QuestID++
		*q = Quest{ID: a.QuestID, Index: free}
		a.QuestRerolls++
		return from, dailyQuests[free].Text, true
	}
	return "", "", false
}

// questRerollsPerDay: QUEST_SETTINGS DailyQuestRerollLimit.
const questRerollsPerDay = 1

// questProgress counts for the quests of a kind; match says whether this
// quest counts it.
func (a *Account) questProgress(kind int32, n int32, match func(questDef) bool) {
	a.refreshQuests()
	for i := range a.Quests {
		q := &a.Quests[i]
		d := dailyQuests[q.Index]
		if q.Claimed || d.TypeID != kind || q.Progress >= d.Target || (match != nil && !match(d)) {
			continue
		}
		q.Progress = min(q.Progress+n, d.Target)
	}
}

// questsAfterBattle: a finished battle counts for PlayGameMode and, won,
// for WinRumble.
func (a *Account) questsAfterBattle(mode, place int32) {
	a.questProgress(questPlayGameMode, 1, func(d questDef) bool {
		gm, _ := d.Own["GameMode"].(string)
		return (gm == "2_PLAYER" && mode == ModeDuel) || (gm == "RUMBLE" && mode == ModeRumble)
	})
	if mode == ModeRumble && place == 1 { // "Finish as the Champion"
		a.questProgress(questWinRumble, 1, nil)
	}
}

// ClaimQuest is 503: the quest's reward chest, made by the client.
func (a *Account) ClaimQuest(r *Reward, id int32) bool {
	for i := range a.Quests {
		q := &a.Quests[i]
		d := dailyQuests[q.Index]
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
	w.VInt(int32(len(active)))
	for _, q := range active {
		def := dailyQuests[q.Index]
		w.VInt(dailyPoolEvent)
		w.VInt(q.Index)
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
		w.VInt(questCategoryDaily)
		intList(w, def.Rewards)
		w.String("") // +0x58
		w.VInt(0)    // +0x18
		w.VInt(0)    // +0x1c
	}
	// the pools' state (+0x0 event, +0x10 handed out, +0x20 the taken
	// indices; encoder 0x31b864): a reroll (0x785fac) takes one back and
	// picks among the free indices, so with the taken ones listed its pick
	// is the server's
	if len(quests) > 0 {
		w.VInt(1)
		w.Int(dailyPoolEvent)
		w.Int(int32(len(quests)))
		var taken []int32
		for _, q := range quests {
			taken = append(taken, q.Index)
		}
		intList(w, taken)
	} else {
		w.VInt(0)
	}
	vints(w, 3) // the other lists
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
	for _, q := range a.Quests {
		d := dailyQuests[q.Index]
		st := ""
		if q.Claimed {
			st = " claimed"
		}
		l = append(l, fmt.Sprintf("#%d %s %d/%d%s", q.ID, d.Kind, q.Progress, d.Target, st))
	}
	return strings.Join(l, ", ")
}

func (d questDef) encodeState(w *proto.Writer) {
	switch d.TypeID {
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
}
