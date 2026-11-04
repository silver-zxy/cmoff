package home

import (
	"encoding/hex"
	"strings"
	"testing"
)

// gnoksBody is a known-working OwnHomeData body (1.1143.2, body only, no
// frame) from .gnoks's server: new account, starter deck, home id 0, 371 bytes.
const gnoksBodyHex = `
00 00 00 00 00 00 00 00 00 05 01 01 02 01 05 01
06 01 07 01 05 1f 01 01 02 01 05 01 06 01 07 01
03 8c c3 85 66 8c c3 85 66 8c c3 85 66 03 05 80
c4 aa 22 81 c4 aa 22 84 c4 aa 22 85 c4 aa 22 86
c4 aa 22 05 80 c4 aa 22 81 c4 aa 22 84 c4 aa 22
85 c4 aa 22 86 c4 aa 22 05 80 c4 aa 22 81 c4 aa
22 84 c4 aa 22 85 c4 aa 22 86 c4 aa 22 00 01 0d
00 00 01 00 00 00 00 00 00 00 00 00 00 00 00 00
00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00
00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00
00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00
00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00
00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00
00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00
00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00
00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00
00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00
00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00
00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00
00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00
00 00 00 00 00 08 00 00 00 00 00 00 00 00 00 00
00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00
00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00
00 00 00`

// gnoksConfig reproduces exactly the account .gnoks's dump encodes: starter
// collection, home id 0, empty collectibles/maps, an all-zero avatar and tail.
func gnoksConfig() ownHomeData {
	var h clientHome // home id (0,0)
	for _, row := range []int32{0, 1, 4, 5, 6} {
		h.minis = append(h.minis, [2]int32{row, 1})
		slot := [2]int32{row, 1}
		h.deckSlots = append(h.deckSlots, &slot)
	}
	h.deckHeroes = []int32{globalID(classSupportHero, 12), globalID(classSupportHero, 12), globalID(classSupportHero, 12)}
	deck := [5]int32{
		globalID(classSEASONAL, 0), globalID(classSEASONAL, 1), globalID(classSEASONAL, 4),
		globalID(classSEASONAL, 5), globalID(classSEASONAL, 6),
	}
	h.decks = [][5]int32{deck, deck, deck}
	h.heroes = []int32{12}
	return ownHomeData{home: h} // avatar + tail all zero
}

func TestDiffAgainstGnoks(t *testing.T) {
	Live = false // the structure without live content
	defer func() { Live = true }()
	want, err := hex.DecodeString(strings.Join(strings.Fields(gnoksBodyHex), ""))
	if err != nil {
		t.Fatal(err)
	}
	m := gnoksConfig()
	got := m.encode()
	t.Logf("lengths: mine=%d gnoks=%d", len(got), len(want))

	n := len(got)
	if len(want) < n {
		n = len(want)
	}
	first := -1
	for i := 0; i < n; i++ {
		if got[i] != want[i] {
			first = i
			break
		}
	}
	if first == -1 && len(got) == len(want) {
		t.Logf("IDENTICAL: my encoder reproduces .gnoks's working body exactly")
		// Note: BuildStarter() is intentionally richer than this minimal body
		// (populated avatar + collectible levels), so it is NOT byte-identical;
		// this test only pins down that the encoder itself is correct.
		return
	}
	if first == -1 {
		t.Fatalf("one is a prefix of the other; lengths differ (mine=%d gnoks=%d)", len(got), len(want))
	}
	lo := first - 6
	if lo < 0 {
		lo = 0
	}
	hi := first + 10
	if hi > n {
		hi = n
	}
	t.Fatalf("first diff at offset 0x%x (%d):\n  mine : % x\n  gnoks: % x", first, first, got[lo:hi], want[lo:hi])
}
