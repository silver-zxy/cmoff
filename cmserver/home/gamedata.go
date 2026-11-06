package home

import (
	"archive/zip"
	"bufio"
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

// Game data read at runtime from the player's own APK (nothing of it is in
// this repository): the end screen shows the gold of the league's rank
// reward chests (0x945098 sums the Gold of csv_logic/leagues.json
// ChestRewardsForRanking chests, chests/chests.json), so the server gives the
// same.

type tier struct {
	Picks  int32 `json:"picks"`
	Bonus  int32 `json:"bonus"`
	Spread int32 `json:"spread"`
}

type chestResource struct {
	Name        string  `json:"name"`
	Min         int32   `json:"min"`
	Tiers       []tier  `json:"tiers"`
	TierWeights []int32 `json:"tierWeights"`
}

// chestDef is one chest of chests.json.
type chestDef struct {
	Name              string          `json:"name"`
	Resources         []chestResource `json:"resources"`
	FragMin           int32           `json:"fragMin"`
	FragTier          []tier          `json:"fragTier"`
	FragTierWeight    []int32         `json:"fragTierWeight"`
	FragList          []int32         `json:"fragList"`
	FragWeight        []int32         `json:"fragWeight"`
	DuplicatesAllowed bool            `json:"duplicatesAllowed"`
	CollectiblePicks  int32           `json:"collectiblePicks"`
	CollectibleList   []int32         `json:"collectibleList"`
	CollectibleWeight []int32         `json:"collectibleWeight"`
	ShowChestOpening  bool            `json:"showChestOpening"`
}

type rankChests struct {
	RankReq     int32   `json:"RankReq"`
	Rewards     []int32 `json:"Rewards"`
	Competitive []int32 `json:"RewardsCompetitive"`
}

type leagueData struct {
	Name    string `json:"name"`
	Configs map[string]struct {
		Ranking []rankChests `json:"ChestRewardsForRanking"`
	} `json:"GameModeConfigs"`
}

// GameData is what the server uses from the APK.
type GameData struct {
	leagues []leagueData
	chests  []chestDef // by chest id - 127000000
	// miniCost: a mini's elixir cost by seasonal row (characters.csv
	// ManaCost of the row's Season1 character)
	miniCost map[int32]int32
}

// chestBase: chest ids are 127000000 + their index in chests.json.
const chestBase = 127_000_000

// Data is the loaded game data, nil until LoadGameData succeeds.
var Data *GameData

// LoadGameData reads leagues.json and chests.json from the APK.
func LoadGameData(apk string) (*GameData, error) {
	z, err := zip.OpenReader(apk)
	if err != nil {
		return nil, err
	}
	defer z.Close()
	read := func(name string, v any) error {
		f, err := z.Open(name)
		if err != nil {
			return err
		}
		defer f.Close()
		b, err := io.ReadAll(f)
		if err != nil {
			return err
		}
		return json.Unmarshal(b, v)
	}
	var l struct {
		Leagues []leagueData `json:"leagues"`
	}
	if err := read("assets/csv_logic/leagues.json", &l); err != nil {
		return nil, fmt.Errorf("leagues.json: %w", err)
	}
	var c struct {
		Chests []chestDef `json:"chests"`
	}
	if err := read("assets/chests/chests.json", &c); err != nil {
		return nil, fmt.Errorf("chests.json: %w", err)
	}
	d := &GameData{leagues: l.Leagues, chests: c.Chests}
	// the tables below are optional: a stand-in APK may lack them
	chars, err1 := readCSV(z, "assets/csv_logic/characters.csv")
	seas, err2 := readCSV(z, "assets/csv_logic/seasonal_character_mapping.csv")
	if err1 == nil && err2 == nil {
		d.miniCost = miniCosts(chars, seas)
	}
	return d, nil
}

// csvTable is a csv_logic table: the header, then the rows after the
// types row.
type csvTable struct {
	cols map[string]int
	rows [][]string
}

func (t csvTable) get(row []string, col string) string {
	i, ok := t.cols[col]
	if !ok || i >= len(row) {
		return ""
	}
	return row[i]
}

// readCSV reads a csv_logic table from the APK (LZMA compressed,
// sclzma.go).
func readCSV(z *zip.ReadCloser, name string) (csvTable, error) {
	f, err := z.Open(name)
	if err != nil {
		return csvTable{}, err
	}
	defer f.Close()
	b, err := io.ReadAll(f)
	if err != nil {
		return csvTable{}, err
	}
	if b, err = scDecompress(b); err != nil {
		return csvTable{}, fmt.Errorf("%s: %w", name, err)
	}
	r := csv.NewReader(bytes.NewReader(b))
	r.FieldsPerRecord = -1
	r.LazyQuotes = true
	all, err := r.ReadAll()
	if err != nil {
		return csvTable{}, fmt.Errorf("%s: %w", name, err)
	}
	if len(all) < 2 {
		return csvTable{}, fmt.Errorf("%s: no rows", name)
	}
	t := csvTable{cols: map[string]int{}, rows: all[2:]}
	for i, c := range all[0] {
		t.cols[strings.TrimSpace(c)] = i
	}
	return t, nil
}

// miniCosts: the elixir cost of every seasonal row that maps to a
// character.
func miniCosts(chars, seas csvTable) map[int32]int32 {
	cost := map[string]int32{}
	for _, r := range chars.rows {
		if n, err := strconv.Atoi(chars.get(r, "ManaCost")); err == nil {
			cost[chars.get(r, "Name")] = int32(n)
		}
	}
	out := map[int32]int32{}
	for i, r := range seas.rows {
		if c, ok := cost[seas.get(r, "Season1")]; ok {
			out[int32(i)] = c
		}
	}
	return out
}

// MiniCost: a mini's elixir cost by seasonal row (0 if unknown).
func (d *GameData) MiniCost(seasonalRow int32) int32 {
	if d == nil {
		return 0
	}
	return d.miniCost[seasonalRow]
}

// FindOwnAPK returns the APK this process runs from (/proc/self/maps), for
// the in-game server.
func FindOwnAPK() string {
	f, err := os.Open("/proc/self/maps")
	if err != nil {
		return ""
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if i := strings.Index(line, "/"); i >= 0 && strings.HasSuffix(line, "/base.apk") {
			return line[i:]
		}
	}
	return ""
}

func (d *GameData) chestResource(id int32, name string) int32 {
	i := int(id - chestBase)
	if d == nil || i < 0 || i >= len(d.chests) {
		return 0
	}
	var n int32
	for _, r := range d.chests[i].Resources {
		if r.Name == name {
			n += r.Min
		}
	}
	return n
}

// ChestResource: the amount of a resource ("Gold", "RankPoints",
// "ExpPoints", "Diamonds") in a chest of chests.json.
func (d *GameData) ChestResource(id int32, name string) (int32, bool) {
	i := int(id - chestBase)
	if d == nil || i < 0 || i >= len(d.chests) {
		return 0, false
	}
	return d.chestResource(id, name), true
}

// RankReward: a resource summed over a battle's rank reward chests: league
// index (as LeagueIndex), mode, rank (duel 0 won / 1 lost; Rumble place - 1);
// ranked battles also get the "RewardsCompetitive" chests (rank points).
func (d *GameData) RankReward(league int, mode, rank int32, ranked bool, name string) (int32, bool) {
	if d == nil || league < 0 || league >= len(d.leagues) {
		return 0, false
	}
	cfg := "SoloMode"
	if mode == ModeRumble {
		cfg = "TournamentMode"
	}
	for _, r := range d.leagues[league].Configs[cfg].Ranking {
		if r.RankReq != rank {
			continue
		}
		var n int32
		for _, id := range r.Rewards {
			n += d.chestResource(id, name)
		}
		if ranked {
			for _, id := range r.Competitive {
				n += d.chestResource(id, name)
			}
		}
		return n, true
	}
	return 0, false
}

// ChestByName returns a chest's id by its name in chests.json.
func (d *GameData) ChestByName(name string) (int32, bool) {
	if d == nil {
		return 0, false
	}
	for i, c := range d.chests {
		if c.Name == name {
			return chestBase + int32(i), true
		}
	}
	return 0, false
}

// Chest returns a chest's definition.
func (d *GameData) Chest(id int32) (chestDef, bool) {
	i := int(id - chestBase)
	if d == nil || i < 0 || i >= len(d.chests) {
		return chestDef{}, false
	}
	return d.chests[i], true
}
