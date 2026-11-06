package home

import (
	"encoding/json"
	"math/rand"
	"time"
)

// Boards. Duel and Rumble play on a board (locations.csv) the battle picks
// from its game mode's LocationData (game_modes.csv). The plain modes
// (Autochess_2_Player, Autochess_8_Player, 0x787b34) are what the client
// uses without a server; Season 1 had its own modes by league:
//
//	Autochess_{2,8}_Season_1_Silver  ElectroValley, HealingTemple
//	Autochess_{2,8}_Season_1_Gold+   VampireCastle (Banquet Hall),
//	                                 HealingTemple, ElectroValley
//
// The live server chose them with a GAME_MODE_MAPPING event
// (LogicLiveGameModeMappingData 0x34cd28): "SoloMode" (mode 2) and
// "TournamentMode" (mode 3), each {"Status": "OverrideGameMode",
// "OverrideGameMode": <game_modes.csv name>} (0x862cb4; also "Default" and
// "Disabled", optional "LocationsByRank" 0x817b4c). The client looks the
// mapping up when it builds a battle (0x787b34 via 0x7e9d58), the offline
// bot battle too (home update 0x467e78 → 0x940664).
//
// Bronze keeps the plain modes (the Basic modes have no boards).
//
// The mapping reaches the client (StartMatchmake carries its event id), but
// a bot battle is a PvE battle: it plays on its npc's board (npcs.csv
// OfflineMatchNPC: WindyPlains). So the server also picks one of the mode's
// boards for every bot battle and the hook puts it on the npc (BoardFor).

// gameModeEvent: the chronos event id of the mapping.
const gameModeEvent int32 = 4001

// seasonGameModes: the Season 1 modes for a league, "" for none.
func seasonGameModes(trophies int32) (solo, tournament string) {
	switch l := LeagueIndex(trophies); {
	case l >= 7: // Gold 3 and up
		return "Autochess_2_Season_1_Gold+", "Autochess_8_Season_1_Gold+"
	case l >= 4: // Silver 3..1
		return "Autochess_2_Season_1_Silver", "Autochess_8_Season_1_Silver"
	}
	return "", ""
}

// gameModeLiveEvent: the GAME_MODE_MAPPING event for these trophies, false
// in Bronze.
func gameModeLiveEvent(trophies int32) (LiveEvent, bool) {
	solo, tournament := seasonGameModes(trophies)
	if !Live || solo == "" {
		return LiveEvent{}, false
	}
	override := func(mode string) map[string]any {
		return map[string]any{"Status": "OverrideGameMode", "OverrideGameMode": mode}
	}
	b, _ := json.Marshal(map[string]any{
		"SoloMode":       override(solo),
		"TournamentMode": override(tournament),
		"LevelLimitLow":  0,
		"LevelLimitHigh": 999,
	})
	y, m, d := now().Date()
	day := time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
	start := int32(day.AddDate(0, 0, -1).Unix())
	end := int32(day.AddDate(0, 0, 30).Unix())
	return LiveEvent{
		ID: gameModeEvent, Name: "game_modes", Type: EventGameModeMapping,
		Start: start, End: end, VisibleStart: start, VisibleEnd: end,
		Parameters: string(b),
	}, true
}

// GameModeSummary for the log: the modes the battles use now.
func (a *Account) GameModeSummary() string {
	a.Lock()
	defer a.Unlock()
	solo, tournament := seasonGameModes(a.Trophies)
	if solo == "" {
		return "the plain ones (Bronze: Windy Plains)"
	}
	return solo + ", " + tournament
}

// The boards' items in the locations table (the CSV's order; each board
// spans two CSV lines).
const (
	boardWindyPlains   int32 = 0
	boardVampireCastle int32 = 1 // Banquet Hall
	boardElectroValley int32 = 2
	boardHealingTemple int32 = 3
)

var boardNames = map[int32]string{
	boardWindyPlains: "Windy Plains", boardVampireCastle: "Banquet Hall",
	boardElectroValley: "Electro Valley", boardHealingTemple: "Healing Temple",
}

// seasonBoards: the boards of the league's Season 1 modes (game_modes.csv
// LocationData), none in Bronze.
func seasonBoards(trophies int32) []int32 {
	switch l := LeagueIndex(trophies); {
	case l >= 7:
		return []int32{boardVampireCastle, boardHealingTemple, boardElectroValley}
	case l >= 4:
		return []int32{boardElectroValley, boardHealingTemple}
	}
	return nil
}

// BoardFor picks the next bot battle's board: its locations item + 1, 0
// for the npc's own (Windy Plains), and its name for the log.
func BoardFor(trophies int32, rng *rand.Rand) (int32, string) {
	pool := seasonBoards(trophies)
	if !Live || len(pool) == 0 {
		return 0, boardNames[boardWindyPlains]
	}
	b := pool[rng.Intn(len(pool))]
	return b + 1, boardNames[b]
}

// NextBoard: the board of the next bot battle (BoardFor). The caller holds
// the lock.
func (a *Account) NextBoard() (int32, string) {
	return BoardFor(a.Trophies, a.random())
}
