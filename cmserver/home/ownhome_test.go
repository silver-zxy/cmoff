package home

import (
	"reflect"
	"testing"

	"cmserver/proto"
)

// fixedLen is the byte length of a fixed encoder section, used both to assert
// the layout and to skip the section on decode (as the client's own decoder
// does). Each fixed section begins and ends on a byte boundary, so its length
// in isolation equals its length in the stream.
func fixedLen(f func(*proto.Writer)) int {
	w := proto.NewWriter()
	f(w)
	return len(w.Bytes())
}

func decodeCollection(h *clientHome, r *proto.Reader) {
	for n := r.VInt(); n > 0; n-- {
		row := r.VInt() - 1
		h.minis = append(h.minis, [2]int32{row, r.VInt()})
	}
	ns := r.VInt()
	present := make([]bool, ns)
	for i := range present {
		present[i] = r.Bool()
	}
	for _, p := range present {
		if p {
			row := r.VInt() - 1
			slot := [2]int32{row, r.VInt()}
			h.deckSlots = append(h.deckSlots, &slot)
		} else {
			h.deckSlots = append(h.deckSlots, nil)
		}
	}
	for n := r.VInt(); n > 0; n-- {
		h.deckHeroes = append(h.deckHeroes, r.VInt())
	}
	for n := r.VInt(); n > 0; n-- {
		_ = r.VInt() // record length, always 5
		var d [5]int32
		for j := range d {
			d[j] = r.VInt()
		}
		h.decks = append(h.decks, d)
	}
	h.selected = r.VInt()
	for n := r.VInt(); n > 0; n-- {
		h.heroes = append(h.heroes, r.VInt()-1)
	}
	h.heroExtra = r.VInt()
}

// decode mirrors OwnHomeData::decode: it reads the parts encode fills and skips
// the fixed empty sections by their length.
func decode(body []byte) (ownHomeData, error) {
	r := proto.NewReader(body)
	var m ownHomeData

	m.home.homeID[0], m.home.homeID[1] = r.Long()
	m.home.chestIndex = r.VInt() // +0xb0
	decodeCollection(&m.home, r)
	r.VInt()
	r.Bool()
	m.home.events = decodeEvents(r)
	// the rest starts with a bool, which shares the byte with the last
	// event's bool
	if len(m.home.events) == 0 {
		r.Skip(fixedLen(encodeHomeHeadRest))
	} else {
		r.Skip(fixedLen(encodeHomeHeadRest) - 1)
	}
	d := &m.home.daily
	d.weekday, d.day, d.unknownC0 = r.VInt(), r.VInt(), r.VInt()
	d.ticksLeft = r.VInt()
	r.VInt()
	r.VInt()
	d.left = r.VInt()
	r.Skip(fixedLen(encodeHomeHead2))
	m.home.flags130 = r.VInt()
	r.Skip(fixedLen(encodeHomeTail))
	m.home.quests.quests = decodeQuests(r)
	for mp := 0; mp < 3; mp++ {
		for n := r.VInt(); n > 0; n-- {
			k, v := r.VInt(), r.VInt()
			switch mp {
			case 0:
				m.home.map200 = append(m.home.map200, [2]int32{k, v})
			case 1:
				m.home.map228 = append(m.home.map228, [2]int32{k, v})
			default:
				m.home.collectibleLevels = append(m.home.collectibleLevels, [2]int32{k, v})
			}
		}
	}
	r.Skip(fixedLen(func(w *proto.Writer) { encodeHomeEnd(w, false, nil) }))
	m.home.chests = decodeChests(r)
	// The manager's bool and the bool after it share a byte, with the last
	// chest's last bool when there are chests.
	if len(m.home.chests) == 0 {
		r.Skip(fixedLen(func(w *proto.Writer) { w.Bool(false); encodeHomeEnd2(w) }))
	} else {
		r.Skip(fixedLen(encodeHomeEnd2) - 1)
	}

	m.avatar.accountID[0], m.avatar.accountID[1] = r.VLong()
	m.avatar.homeID[0], m.avatar.homeID[1] = r.VLong()
	m.avatar.id2[0], m.avatar.id2[1] = r.VLong()
	m.avatar.name = r.String()
	m.avatar.nameChanges = r.VInt()
	m.avatar.trophies = r.VInt()
	m.avatar.lastTrophies = r.VInt()
	m.avatar.seasonBest = r.VInt()
	for i := 0; i < 7; i++ {
		r.VInt()
	}
	r.Long() // long90
	if n := r.VInt(); n != 8 {
		return m, errSlots(n)
	}
	for i := range m.avatar.slots {
		for c := r.VInt(); c > 0; c-- {
			class, instance := r.DataRef()
			m.avatar.slots[i] = append(m.avatar.slots[i], dataSlot{class, instance, r.VInt()})
		}
	}
	// encode_mid, field by field. Arrays are VInt deltas (0x720dac).
	for _, p := range []*[2]int32{&m.avatar.battleCounts, &m.avatar.battleWins,
		&m.avatar.battleLosses, &m.avatar.stats118, &m.avatar.battleFirsts} {
		p[0] = r.VInt()
		p[1] = p[0] + r.VInt()
	}
	m.avatar.streakAll = r.VInt()
	m.avatar.streakRanked = r.VInt()
	r.VInt()
	r.Bool()
	m.avatar.gems = r.VInt()
	r.VInt()
	m.avatar.expPoints = r.VInt()
	m.avatar.expLevel = r.VInt()
	r.VInt()
	m.avatar.nameSet = r.Bool()
	r.Bool()
	r.VInt()
	r.VInt()
	r.Bool()
	r.Bool()
	m.avatar.tutorialFlags = r.VInt()
	r.Skip(fixedLen(avatarEncodeTail))
	for i := range m.tail {
		m.tail[i] = r.VInt()
	}
	return m, r.Err()
}

type errSlots int32

func (e errSlots) Error() string { return "avatar slot-list count not 8" }

func TestEmptyHomeLength(t *testing.T) {
	Live = false // the structure without live content
	defer func() { Live = true }()
	got := fixedLen(func(w *proto.Writer) { (&clientHome{}).encode(w) })
	if got != 199 {
		t.Fatalf("empty ClientHome encodes to %d bytes, want 199", got)
	}
}

func TestStarterRoundTrip(t *testing.T) {
	m := ownHomeData{home: starterHome()}
	got, err := decode(m.encode())
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !reflect.DeepEqual(got.home, m.home) {
		t.Fatalf("starter home did not survive round trip:\n got %+v\nwant %+v", got.home, m.home)
	}
}

func TestFullRoundTrip(t *testing.T) {
	m := ownHomeData{home: starterHome()}
	m.home.homeID = [2]int32{0, 5}
	m.home.flags130 = 3
	m.home.collectibleLevels = [][2]int32{{107_000_012, 13}, {36_000_001, 10}}
	m.avatar = clientAvatar{
		accountID:     [2]int32{0, 5},
		homeID:        [2]int32{0, 5},
		name:          "Player",
		tutorialFlags: 0x100,
		expLevel:      7,
		expPoints:     300,
		nameSet:       true,
		nameChanges:   1,
	}
	m.avatar.slots[2] = []dataSlot{{class: 5, instance: 1, count: 100}, {class: 0, instance: 0, count: 0}}
	m.tail = [4]int32{1, 0, 1_700_000_000, -1}

	got, err := decode(m.encode())
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !reflect.DeepEqual(got, m) {
		t.Fatalf("full round trip mismatch:\n got %+v\nwant %+v", got, m)
	}
}

func TestBuildStarterNonEmpty(t *testing.T) {
	if b := BuildStarter(); len(b) < 199 {
		t.Fatalf("BuildStarter returned %d bytes", len(b))
	}
}

// The account's home must carry the skin store (+0x1c0); without it every
// skin shows as locked.
func TestAccountSendsSkinStore(t *testing.T) {
	m := starterOwnHome()
	without := len(m.encode())
	if got := len(NewAccount().OwnHomeData()); got <= without {
		t.Fatalf("account home %d bytes, starter %d: skin store missing", got, without)
	}
}

func TestSkinStoreIndex(t *testing.T) {
	for _, s := range skinTable {
		if s.extra && defaultSkin(s.owner) == 0 {
			t.Errorf("owner %d has no default skin", s.owner)
		}
	}
	read := func(sel map[int32]int32) map[int32][]int32 { // owner -> [idx, skins...]
		w := proto.NewWriter()
		encodeSkinStore(w, sel)
		r := proto.NewReader(w.Bytes())
		out := map[int32][]int32{}
		for n := r.VInt(); n > 0; n-- {
			o, idx, c := r.VInt(), r.VInt(), r.VInt()
			e := []int32{idx}
			for ; c > 0; c-- {
				e = append(e, r.VInt())
			}
			out[o] = e
		}
		return out
	}
	archer := read(nil)[36000004] // Archer: default, Sparkle
	if archer[0] != 0 || archer[1] != 44000000 || archer[2] != 44000095 {
		t.Fatalf("archer default entry %v", archer)
	}
	if e := read(map[int32]int32{36000004: 44000095})[36000004]; e[0] != 1 {
		t.Fatalf("archer sparkle entry %v", e)
	}
}

// decodeChests mirrors encodeChests (the client's decoder 0x556140).
func decodeChests(r *proto.Reader) []PendingChest {
	var out []PendingChest
	list := func() []int32 {
		var xs []int32
		for n := r.VInt(); n > 0; n-- {
			xs = append(xs, r.VInt())
		}
		return xs
	}
	for n := r.VInt(); n > 0 && r.Err() == nil; n-- {
		var c PendingChest
		r.Bool()
		r.Bool()
		r.Bool()
		list()
		c.Chest = r.VInt()
		c.Index = r.VInt()
		c.SourceValue = r.VInt()
		c.Source = r.VInt()
		_ = r.String()
		r.VInt()
		if r.Bool() {
			for e := r.VInt(); e > 0; e-- {
				ids, ns := list(), list()
				fids, fns := list(), list()
				list()
				r.Bool()
				r.VInt()
				for i, id := range ids {
					for name, row := range resourceRows {
						if id == globalID(classRESOURCES, row) {
							if c.Content.Resources == nil {
								c.Content.Resources = map[string]int32{}
							}
							c.Content.Resources[name] += ns[i]
						}
					}
				}
				for i, id := range fids {
					if c.Content.Fragments == nil {
						c.Content.Fragments = map[int32]int32{}
					}
					c.Content.Fragments[id] += fns[i]
				}
			}
			c.Show = r.Bool()
		}
		for i := 0; i < 5; i++ {
			r.VInt()
		}
		r.Bool()
		out = append(out, c)
	}
	return out // the manager's bool shares a byte with what follows
}

// decodeQuests mirrors encodeQuests (the client's 0x4bccd8 / 0x3b3af4).
// lastPoolState: the pool state decodeQuests read last (event, handed
// out, taken indices...).
var lastPoolState []int32

func decodeQuests(r *proto.Reader) []Quest {
	if !r.Bool() {
		panic("quest manager +0xc8 off: the client refuses 502/503")
	}
	for i := 0; i < 5+6; i++ {
		r.VInt()
	}
	var out []Quest
	for n := r.VInt(); n > 0 && r.Err() == nil; n-- {
		var q Quest
		r.VInt() // event
		q.Index = r.VInt()
		kind := r.VInt()
		if r.Bool() {
			switch kind {
			case questMetaUpgrade:
				r.Bool()
				r.Bool()
			case questPlayGameMode:
				r.VInt()
			}
		}
		_ = r.String()
		q.ID = r.VInt()
		q.Progress = r.VInt()
		r.VInt()
		r.VInt()
		r.VInt()
		for k := r.VInt(); k > 0; k-- {
			r.VInt()
		}
		_ = r.String()
		r.VInt()
		r.VInt()
		out = append(out, q)
	}
	// pool state: event, handed out, taken indices
	lastPoolState = nil
	for n := r.VInt(); n > 0 && r.Err() == nil; n-- {
		lastPoolState = append(lastPoolState, r.Int(), r.Int())
		for k := r.VInt(); k > 0; k-- {
			lastPoolState = append(lastPoolState, r.VInt())
		}
	}
	for i := 0; i < 3; i++ {
		r.VInt()
	}
	return out
}

// decodeEvents mirrors EncodeEvents (0x295f9c, 0x4915fc).
func decodeEvents(r *proto.Reader) []LiveEvent {
	r.VInt()
	var out []LiveEvent
	for n := r.VInt(); n > 0 && r.Err() == nil; n-- {
		var e LiveEvent
		e.ID = r.VInt()
		e.Name = r.String()
		e.Type = r.VInt()
		e.Start, e.End, e.VisibleStart, e.VisibleEnd = r.VInt(), r.VInt(), r.VInt(), r.VInt()
		_, _, _ = r.String(), r.String(), r.String()
		e.Parameters = r.String()
		r.VInt()
		r.Bool()
		out = append(out, e)
	}
	return out
}
