package home

// OwnHomeData (25091) body, ported verbatim from the client's own encoders
// (LogicClientHome::encode 0x596f24 + LogicClientAvatar::encode 0x5cb380 + the
// four trailing VInts of OwnHomeData 0x94f060). Each write below mirrors one
// write of the client, in order, so the client's decoder consumes the whole
// body without the heap corruption a hand-built body caused. This replaces the
// Unicorn-emulated ownhome.bin: the client's encoder is simple enough to
// reproduce directly once every field order is known.
//
// Stream vtable slots used by the client there: 0x48 writeInt, 0x80 writeVInt,
// 0x40 writeBoolean, 0x20/0x28/0x38 writeString, 0x90/0x98 LogicLong as two
// ints. Helpers: 0x9052f4 LogicLong as two VInts (low then high), 0x37cc70 row
// reference as one VInt (row+1, 0=none), 0x8e9860 (class, instance) reference
// (null = one VInt 0, else two VInts).

import "cmserver/proto"

// CSV class ids of build 1.1143.2 (LogicDataTables loader 0x98d49c). A global
// data id is class*1_000_000 + row.
const (
	classRESOURCES   int32 = 5
	classCHARACTERS  int32 = 34
	classSEASONAL    int32 = 36 // seasonal_character_mapping
	classSupportHero int32 = 107
	classSkin        int32 = 44 // skin_confs
)

func globalID(class, row int32) int32 { return class*1_000_000 + row }

// dataSlot is one entry of an avatar data-slot list (0x6ab360).
type dataSlot struct {
	class, instance int32 // (CSV class id, row instance id)
	count           int32
}

type clientHome struct {
	homeID [2]int32 // (field0, field1) exactly as the client's LogicLong

	minis      [][2]int32  // +0x28: (characters.csv row, level)
	deckSlots  []*[2]int32 // +0x38: current deck positions, nil = empty
	deckHeroes []int32     // +0x90: hero of each saved deck (support_heroes global id)
	decks      [][5]int32  // +0x80: saved decks, five seasonal global ids each
	selected   int32       // +0xa0: selected deck index
	heroes     []int32     // +0x20: owned support heroes (support_heroes rows)
	heroExtra  int32       // VInt after the hero list

	collectibleLevels [][2]int32      // +0x250: (global id, level)
	map200            [][2]int32      // +0x200
	map228            [][2]int32      // +0x228
	flags130          int32           // +0x130, 0 for a new account
	skinStore         bool            // write the owned skins (skins.go)
	skinSel           map[int32]int32 // selected skin per owner

	chestIndex int32          // +0xb0: the last chest index handed out
	chests     []PendingChest // +0x1b0: chests to open (chests_pending.go)
	daily      dailyGold      // +0xb4..+0xc4: battle gold left today
	quests     questState     // +0x180: the daily quests (quests.go)
	events     []LiveEvent    // +0x140/+0x10: the chronos events (live.go)
}

// dailyGold is the home's daily battle gold state (0x59a380, 0x54a9a4):
// +0xb8 a day number, +0xb4 the weekday (day % 7 + 1), +0xc0 unknown, the
// timer +0xc4 to the next day (0x4a47a4: ticks left and total at 20 a
// second, then an end time, -1 = set by the client), +0xbc the gold that
// battles may still give today. When the timer runs out the client counts
// the day up and refills +0xbc with DAILY_GOLD_GAIN_LIMIT_FROM_MATCH.
type dailyGold struct {
	weekday, day, unknownC0 int32
	ticksLeft               int32
	left                    int32
}

func (d dailyGold) encode(w *proto.Writer) {
	w.VInt(d.weekday)
	w.VInt(d.day)
	w.VInt(d.unknownC0)
	w.VInt(d.ticksLeft) // 0x4a47a4: left, total, end
	w.VInt(d.ticksLeft)
	if d.ticksLeft > 0 {
		w.VInt(-1)
	} else {
		w.VInt(0)
	}
	w.VInt(d.left) // +0xbc
}

type clientAvatar struct {
	accountID [2]int32
	homeID    [2]int32
	id2       [2]int32
	name      string
	slots     [8][]dataSlot

	tutorialFlags int32 // +0x130: bit 0x100 skips the tutorial battle
	expLevel      int32 // +0xbc: must be >=1 (HUD looks up level-1)
	expPoints     int32 // +0xc0
	nameSet       bool  // +0x135: else the client asks for a name every start
	nameChanges   int32 // +0x138
	trophies      int32 // +0x70 "score": RankPoints (profile 0x305dd4)
	gems          int32 // +0xc4: diamonds (HUD 0x4d24a0)
	lastTrophies  int32 // +0x74 "lastScore"
	seasonBest    int32 // +0x34 "seasonBestScore"

	// Battle statistics, [0] duel, [1] Rumble (avatar JSON 0x392f00; the
	// profile's battle_status 0x49cd10 reads counts, wins and rank-top[1]).
	battleCounts [2]int32 // +0x100
	battleWins   [2]int32 // +0x108
	battleLosses [2]int32 // +0x110
	stats118     [2]int32 // +0x118, unknown
	battleFirsts [2]int32 // +0x120 "battleRankTopCounts"
	streakAll    int32    // +0x128 "allTypeGameWinLoseStreak": >0 wins, <0 losses
	streakRanked int32    // +0x12c "promoteGameWinLoseStreak": the league screen's streak
}

type ownHomeData struct {
	home   clientHome
	avatar clientAvatar
	tail   [4]int32 // +0x98..+0xa4, probably timestamps
}

// --- helpers mirroring the client's stream helpers ---

func vints(w *proto.Writer, n int) {
	for i := 0; i < n; i++ {
		w.VInt(0)
	}
}

// rowRef writes a present row as VInt(row+1) (0x37cc70; 0 = none).
func rowRef(w *proto.Writer, row int32) { w.VInt(row + 1) }

// --- ClientHome ---

func (h *clientHome) encode(w *proto.Writer) {
	w.Long(h.homeID[0], h.homeID[1])
	w.VInt(h.chestIndex) // +0xb0
	h.encodeCollection(w)
	encodeHomeHead(w, h.events)
	h.daily.encode(w)
	encodeHomeHead2(w)
	w.VInt(h.flags130)
	encodeHomeTail(w)
	encodeQuests(w, h.quests, h.daily)
	h.encodeMaps(w)
	encodeHomeEnd(w, h.skinStore, h.skinSel)
	encodeChests(w, h.chests)
	encodeHomeEnd2(w)
}

func (h *clientHome) encodeCollection(w *proto.Writer) {
	w.VInt(int32(len(h.minis)))
	for _, m := range h.minis {
		rowRef(w, m[0])
		w.VInt(m[1])
	}
	w.VInt(int32(len(h.deckSlots)))
	for _, s := range h.deckSlots {
		w.Bool(s != nil)
	}
	for _, s := range h.deckSlots {
		if s != nil {
			rowRef(w, s[0])
			w.VInt(s[1])
		}
	}
	w.VInt(int32(len(h.deckHeroes)))
	for _, x := range h.deckHeroes {
		w.VInt(x)
	}
	w.VInt(int32(len(h.decks)))
	for _, d := range h.decks {
		w.VInt(5)
		for _, id := range d {
			w.VInt(id)
		}
	}
	w.VInt(h.selected)
	w.VInt(int32(len(h.heroes)))
	for _, row := range h.heroes {
		rowRef(w, row)
	}
	w.VInt(h.heroExtra)
}

func (h *clientHome) encodeMaps(w *proto.Writer) {
	for _, m := range [][][2]int32{h.map200, h.map228} {
		w.VInt(int32(len(m)))
		for _, kv := range m {
			w.VInt(kv[0])
			w.VInt(kv[1])
		}
	}
	w.VInt(int32(len(h.collectibleLevels)))
	for _, kv := range h.collectibleLevels {
		w.VInt(kv[0])
		w.VInt(kv[1])
	}
}

// encodeHomeHead mirrors the client from 0x8197fc up to +0x130. Boolean runs
// pack across the internal sub-encoder boundaries (0x8197fc's last flag shares
// a byte with 0x4f61d8's three), which proto.Writer.Bool reproduces.
func encodeHomeHead(w *proto.Writer, events []LiveEvent) {
	w.VInt(0) // +0x140
	// +0x10 present: home init (0x60add4 -> 0x476b88) reads its first field
	// without a null check. 0x7ea4f4: the chronos events, as in 417
	// (live.go).
	w.Bool(true)
	EncodeEvents(w, events)
	encodeHomeHeadRest(w)
}

// encodeHomeHeadRest continues encodeHomeHead after the events.
func encodeHomeHeadRest(w *proto.Writer) {
	w.Bool(false) // +0x20
	vints(w, 4)   // four LogicLong lists
	w.VInt(0)     // map of bool-flagged entries
	vints(w, 6)
	w.VInt(0)
	w.VInt(0) // two VInt lists
	w.VInt(0)
	w.VInt(0)     // (vint, bool) and (vint, vint) lists
	w.Bool(false) // shares a byte with the three below
	// 0x4f61d8 (+0x148): three optional strings, the live global
	// configurations (live.go)
	g := LiveGlobals()
	for i := 0; i < 3; i++ {
		if i < len(g) {
			w.Bool(true)
			w.String(g[i])
		} else {
			w.Bool(false)
		}
	}
	w.VInt(0) // +0x158
	w.VInt(0) // +0x70
	w.DataRef(0, 0)
	w.DataRef(0, 0) // +0x110, +0x118
	// then the daily gold state (dailyGold.encode)
}

// encodeHomeHead2 continues encodeHomeHead after the daily gold state.
func encodeHomeHead2(w *proto.Writer) {
	vints(w, 9) // 0x4a47a4 x3
	w.VInt(0)   // list of 0x7f5b90 records
	w.VInt(0)   // +0x108
}

// encodeHomeTail mirrors the client after +0x130, up to the maps at +0x200.
func encodeHomeTail(w *proto.Writer) {
	w.VInt(0)
	w.VInt(0)     // +0x134 +0x138
	w.VInt(0)     // VInt list +0x120
	w.VInt(0)     // +0x13c
	w.Bool(false) // +0xa5
	w.VInt(0)
	w.VInt(0)     // +0x150 +0x154
	w.Bool(false) // optional object +0x78
	w.VInt(0)     // list of virtual objects +0x168
	w.Int(0)      // 0x39dd54: 10-bit bitset, one word
	// then the quest manager 0x31b774 (+0x180, quests.go)
}

// encodeHomeEnd mirrors the client after the maps at +0x200.
func encodeHomeEnd(w *proto.Writer, skinStore bool, skinSel map[int32]int32) {
	vints(w, 4)   // +0x188..+0x194
	w.Bool(false) // +0xac
	w.Bool(false)
	w.Bool(false)
	w.Bool(false) // optional longs +0x1a0 +0x198 +0x1a8
	w.VInt(0)
	w.VInt(0) // 0x7ec5fc (+0x1b8)
	for i := 0; i < 4; i++ {
		w.Long(0, 0)
	}
	vints(w, 2) // two lists
	vints(w, 2)
	w.VInt(0)      // VInt list
	if skinStore { // 0x7295d8 (+0x1c0), skins.go
		encodeSkinStore(w, skinSel)
	} else {
		w.VInt(0)
	}
	vints(w, 4) // 0x31e77c (+0x1c8)
	// then the chest manager (encodeChests)
}

// encodeHomeEnd2 continues encodeHomeEnd after the chest manager.
func encodeHomeEnd2(w *proto.Writer) {
	w.Bool(false) // optional object +0x1d0
	w.String("")  // +0x1e8
	w.VInt(0)
	w.VInt(0)     // +0x1f8 +0x1d8
	w.Bool(false) // +0x1dc
	w.VInt(0)     // +0x1e0
	w.String("")  // +0x280
	w.VInt(0)     // +0x290
	vints(w, 2)   // 0x7ee56c (+0x278)
	w.VInt(0)     // pair list
	vints(w, 2)
	vints(w, 20) // 0x720dac x2, 10 each
	vints(w, 3)
	w.VInt(0)   // list
	w.VInt(0)   // +0x90
	vints(w, 5) // five lists
	w.Bool(false)
	vints(w, 3)
	w.VInt(0) // list
	w.Bool(false)
	w.VInt(0) // +0x294
}

// --- ClientAvatar ---

func (a *clientAvatar) encode(w *proto.Writer) {
	a.encodeHead(w)
	a.encodeMid(w)
	w.VInt(a.tutorialFlags)
	avatarEncodeTail(w)
}

func (a *clientAvatar) encodeHead(w *proto.Writer) {
	w.VLong(a.accountID[0], a.accountID[1])
	w.VLong(a.homeID[0], a.homeID[1])
	w.VLong(a.id2[0], a.id2[1])
	w.String(a.name)
	w.VInt(a.nameChanges)  // +0x138
	w.VInt(a.trophies)     // +0x70
	w.VInt(a.lastTrophies) // +0x74
	w.VInt(a.seasonBest)   // +0x34
	vints(w, 7)            // +0x38..+0x50
	w.Long90(0, 0)         // +0x58
	w.VInt(int32(len(a.slots)))
	for _, list := range a.slots {
		w.VInt(int32(len(list)))
		for _, s := range list {
			w.DataRef(s.class, s.instance)
			w.VInt(s.count)
		}
	}
}

// pair mirrors 0x720dac: an int array as VInt deltas (first value, then each
// value minus the one before).
func pair(w *proto.Writer, v [2]int32) {
	w.VInt(v[0])
	w.VInt(v[1] - v[0])
}

func (a *clientAvatar) encodeMid(w *proto.Writer) {
	pair(w, a.battleCounts) // +0x100
	pair(w, a.battleWins)   // +0x108
	pair(w, a.battleLosses) // +0x110
	pair(w, a.stats118)     // +0x118
	pair(w, a.battleFirsts) // +0x120
	w.VInt(a.streakAll)     // +0x128
	w.VInt(a.streakRanked)  // +0x12c
	w.VInt(0)               // +0x13c
	w.Bool(false)           // +0x140
	w.VInt(a.gems)          // +0xc4
	w.VInt(0)               // +0xc8
	w.VInt(a.expPoints)
	w.VInt(a.expLevel) // +0xc0 +0xbc
	w.VInt(0)          // +0xcc
	w.Bool(a.nameSet)
	w.Bool(false) // +0x135 +0x134
	w.VInt(0)     // +0x144
	w.VInt(0)     // +0xb8
	w.Bool(false) // optional block +0xd8
	w.Bool(false) // +0x136
}

func avatarEncodeTail(w *proto.Writer) {
	w.Long90(0, 0) // +0x10
	w.VInt(0)      // +0x18
}

// --- OwnHomeData ---

func (m *ownHomeData) encode() []byte {
	w := proto.NewWriter()
	m.home.encode(w)
	m.avatar.encode(w)
	for _, v := range m.tail {
		w.VInt(v)
	}
	return w.Bytes()
}

// Real collection rows from the decrypted 1.1143.2 game CSVs, with the
// obfuscation rows (random name, empty cells) excluded. 22 minis
// (seasonal_character_mapping class 36, with their characters class 34 rows)
// and 6 support heroes (class 107). An account that owns the whole collection
// at max looks like a real, active account (unlock_all, .gnoks's default).
var (
	miniSeasonalRows = []int32{0, 1, 3, 4, 5, 6, 7, 8, 9, 10, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23}
	miniCharRows     = []int32{0, 1, 3, 4, 5, 6, 7, 8, 9, 10, 12, 13, 14, 15, 17, 18, 19, 20, 21, 22, 23, 24}
	heroRows         = []int32{11, 12, 13, 14, 15, 17}
	// Skins present both in collectibles.json and as real (non-obfuscated)
	// rows of skin_confs.csv: MagicArcher_Celtic, ElectroWizard_Scientist,
	// Miner_Gravedigger, Pekka_Plushy, shieldmaiden_Victory, monk_Luchador,
	// Prince_Fabulous, BarbarianKing_Jolly, MiniPekka_Helpful,
	// GiantSkeleton_Gift, SpearGoblin_CandyElf, Bowler_Snowler, Archer_Sparkle.
	// The other skins of collectibles.json were stripped from this build.
	skinRows = []int32{59, 65, 70, 73, 80, 81, 89, 90, 91, 92, 93, 94, 95}
)

// MiniLevel is the value written after each owned mini (mini object +0x10,
// encoder 0x7e8034), probably its collection level. Experiment knob.
var MiniLevel int32 = 1

// starterHome builds a fully-unlocked account: every mini and hero owned, all
// collectible levels maxed, with the Barbarian King starter deck selected.
func starterHome() clientHome {
	const barbarianKing int32 = 12 // support_heroes row, the deck's hero

	var h clientHome
	h.homeID = [2]int32{1, 0} // matches the account id the LoginOk assigns

	// Own every mini (by its characters.csv row) at level 1.
	for _, cr := range miniCharRows {
		h.minis = append(h.minis, [2]int32{cr, MiniLevel})
	}

	// A valid 5-card deck: Knight, Barbarian, Archer, Spear Goblin, Wizard
	// (seasonal rows), with the Barbarian King as hero in all three slots.
	deckSeasonal := []int32{0, 1, 4, 5, 6}
	var deck [5]int32
	for i, row := range deckSeasonal {
		deck[i] = globalID(classSEASONAL, row)
		slot := [2]int32{row, 1}
		h.deckSlots = append(h.deckSlots, &slot)
	}
	h.decks = [][5]int32{deck, deck, deck}
	h.deckHeroes = []int32{
		globalID(classSupportHero, barbarianKing),
		globalID(classSupportHero, barbarianKing),
		globalID(classSupportHero, barbarianKing),
	}
	h.selected = 0

	// Own every support hero.
	h.heroes = append(h.heroes, heroRows...)

	// collectible_levels unlock the collection (SERVER.md: without an entry a
	// mini shows as locked). Minis (class 36) maxed at 4, heroes (class 107) at
	// 10.
	for _, row := range miniSeasonalRows {
		h.collectibleLevels = append(h.collectibleLevels, [2]int32{globalID(classSEASONAL, row), 4})
	}
	for _, row := range heroRows {
		h.collectibleLevels = append(h.collectibleLevels, [2]int32{globalID(classSupportHero, row), 10})
	}
	// Skins are collectibles too (chests/collectibles.json "Skin Character",
	// class 44 = skin_confs, one upgrade fragment): owning one = level 1.
	for _, row := range skinRows {
		h.collectibleLevels = append(h.collectibleLevels, [2]int32{globalID(classSkin, row), 1})
	}
	return h
}

// BuildStarter returns the OwnHomeData body for a fresh unlocked account: the
// starter collection and decks, an avatar past the tutorial with a set name and
// a valid exp level. IDs match the LoginOk (account/home id 1).
func BuildStarter() []byte {
	m := starterOwnHome()
	return m.encode()
}

func starterOwnHome() ownHomeData {
	// A populated avatar carries the account past the early HUD/home-scene
	// init: a set name (so the client stops asking for one), exp level >= 1
	// (the HUD looks up level-1 and crashes on 0), the tutorial-skip flag, and
	// a starting resource. This config is what reaches ~87%.
	m := ownHomeData{
		home: starterHome(),
		avatar: clientAvatar{
			accountID:     [2]int32{1, 0},
			homeID:        [2]int32{1, 0},
			name:          "Player",
			tutorialFlags: 0x100, // skip the tutorial battle -> home screen
			expLevel:      1,     // >=1 or the HUD crashes on the level-1 lookup
			expPoints:     0,
			nameSet:       true, // else the client prompts for a name each start
			nameChanges:   1,
		},
		// Tail: all zero, matching .gnoks's known-working body dump. (A non-zero
		// tail, especially slot 3 = -1, is a prime suspect for the client's
		// "account scheduled for deletion" warning; his working body uses 0s.)
		tail: [4]int32{0, 0, 0, 0},
	}
	// Seed the avatar's resource slot (class 5 = RESOURCES), as .gnoks's slot
	// example does; a big count for the unlocked account.
	m.avatar.slots[2] = []dataSlot{
		{class: classRESOURCES, instance: 1, count: 1_000_000},
	}
	return m
}
