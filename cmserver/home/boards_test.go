package home

import (
	"encoding/json"
	"math/rand"
	"testing"
)

// Bronze plays the plain modes; Silver and Gold+ get Season 1's modes in
// the home's events.
func TestGameModeMapping(t *testing.T) {
	if _, ok := gameModeLiveEvent(50); ok {
		t.Fatal("Bronze got a mapping")
	}
	for trophies, want := range map[int32]string{100: "Autochess_2_Season_1_Silver", 399: "Autochess_2_Season_1_Silver",
		400: "Autochess_2_Season_1_Gold+", 1300: "Autochess_2_Season_1_Gold+"} {
		e, ok := gameModeLiveEvent(trophies)
		if !ok || e.Type != EventGameModeMapping {
			t.Fatalf("%d: %+v", trophies, e)
		}
		var p struct {
			SoloMode, TournamentMode struct{ Status, OverrideGameMode string }
		}
		if err := json.Unmarshal([]byte(e.Parameters), &p); err != nil || p.SoloMode.OverrideGameMode != want ||
			p.SoloMode.Status != "OverrideGameMode" || p.TournamentMode.OverrideGameMode[len("Autochess_")] != '8' {
			t.Fatalf("%d: %v %s", trophies, err, e.Parameters)
		}
	}
	a := NewAccount()
	a.StartProgression()
	a.Trophies = 450
	h := a.ownHome()
	h.home.skinStore = false
	m, err := decode(h.encode())
	if err != nil || len(m.home.events) != 4 || m.home.events[3].Type != EventGameModeMapping {
		t.Fatalf("%v events %d", err, len(m.home.events))
	}
}

// Bot battles: Bronze keeps the npc's board, Silver gets Electro Valley and
// Healing Temple, Gold+ also Banquet Hall (rows + 1 for the hook).
func TestBoardFor(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	if b, name := BoardFor(50, rng); b != 0 || name != "Windy Plains" {
		t.Fatalf("Bronze %d %s", b, name)
	}
	for trophies, want := range map[int32]map[int32]bool{
		150: {boardElectroValley + 1: true, boardHealingTemple + 1: true},
		500: {boardElectroValley + 1: true, boardHealingTemple + 1: true, boardVampireCastle + 1: true},
	} {
		seen := map[int32]bool{}
		for i := 0; i < 200; i++ {
			b, _ := BoardFor(trophies, rng)
			if !want[b] {
				t.Fatalf("%d trophies: board %d", trophies, b)
			}
			seen[b] = true
		}
		if len(seen) != len(want) {
			t.Fatalf("%d trophies: only %v", trophies, seen)
		}
	}
}
