package home

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"math/rand"
	"os"
	"strings"
	"sync"
	"time"
)

// Account is everything the player can change that has to survive a restart.
// The collection itself (every mini and hero, maxed) is always unlocked and
// not stored. Saved as JSON (accounts.json next to the server).
type Account struct {
	Name     string      `json:"name"`
	Selected int32       `json:"selected_deck"` // 0..2
	Decks    [3][5]int32 `json:"decks"`         // seasonal_character_mapping rows, -1 = empty slot
	Heroes   [3]int32    `json:"deck_heroes"`   // support_heroes row per deck
	// NameSet: the player picked a name. Until then the client shows its
	// first-start name dialog (avatar +0x135 = 0).
	NameSet bool `json:"name_set"`
	// Skins: selected skin (global id) per mini/hero (collectible id).
	Skins map[int32]int32 `json:"skins,omitempty"`
	// OwnedSkins: the extra skins the player has (global ids, skins.go);
	// SkinsLocked: the skins were locked as in the original once (0088)
	OwnedSkins []int32 `json:"owned_skins,omitempty"`
	// LastSeasonEnd: what the last season's end gave (seasonend.go), until
	// logged
	LastSeasonEnd string `json:"-"`
	// SeasonEndKey: the avatar's season key (+0x44), the last season whose
	// end the client did; SeasonEndDue: a season ended and 451 is still to
	// come (seasonend.go); SeasonEndTrophies, SeasonEndBest: the avatar's
	// +0x4c and +0x50 the client keeps then
	SeasonEndKey      int32 `json:"season_end_key,omitempty"`
	SeasonEndDue      bool  `json:"season_end_due,omitempty"`
	SeasonEndTrophies int32 `json:"season_end_trophies,omitempty"`
	SeasonEndBest     int32 `json:"season_end_best,omitempty"`
	// SeasonEndRoad: the ended season's road with tiers not claimed, given
	// after 451 (seasonend.go)
	SeasonEndRoad *SeasonState `json:"season_end_road,omitempty"`
	SkinsLocked   bool         `json:"skins_locked,omitempty"`
	// Trophies (RankPoints) from ranked battles, see ranked.go.
	Trophies     int32 `json:"trophies"`
	BestTrophies int32 `json:"best_trophies"`
	// Exp: total experience; the level follows from exp_levels.csv.
	Exp int32 `json:"exp"`
	// Stats of every bot battle, ranked or not ([0] duel, [1] Rumble).
	Stats Stats `json:"stats"`
	// Progression (collection.go): false = everything unlocked. Collection
	// holds the earned minis and heroes by collectible id; it is kept when
	// switching to unlocked, so progress can be resumed.
	Progression bool  `json:"progression"`
	Gold        int32 `json:"gold"`
	Gems        int32 `json:"gems"`
	// CrystalCoins: the resource PromotionToken
	CrystalCoins int32 `json:"crystal_coins,omitempty"`
	// Battle gold earned on GoldDay (daily limit, economy.go).
	GoldDay   string `json:"gold_day,omitempty"`
	GoldToday int32  `json:"gold_today,omitempty"`
	// Chests waiting in the home to be opened (chests_pending.go), and the
	// last chest index given out.
	Chests     []PendingChest `json:"chests,omitempty"`
	ChestIndex int32          `json:"chest_index,omitempty"`
	// WithheldExp: upgrade exp held back for the next level chests
	// (pushUpgradeExp)
	WithheldExp int32 `json:"withheld_exp,omitempty"`
	// Chains: the tutorial and competitive quest chains (chains.go)
	Chains []ChainState `json:"chains,omitempty"`
	// LevelChests: player levels whose level chest was given.
	LevelChests []int32 `json:"level_chests,omitempty"`
	// Daily quests (quests.go): today's, the day, the last quest id.
	Quests       []Quest `json:"quests,omitempty"`
	QuestDay     string  `json:"quest_day,omitempty"`
	QuestID      int32   `json:"quest_id,omitempty"`
	QuestRerolls int32   `json:"quest_rerolls,omitempty"`
	QuestPool    string  `json:"quest_pool,omitempty"`
	// QuestDropped: today's pool indices rerolled away (not handed out again)
	QuestDropped []int32 `json:"quest_dropped,omitempty"`
	// MiniQuests: the mini quests handed out (minis.go); MiniSteps: per
	// characters.csv row, how many of a mini's quests were claimed
	MiniQuests []Quest         `json:"mini_quests,omitempty"`
	MiniSteps  map[int32]int32 `json:"mini_steps,omitempty"`
	// MiniSeen: the owned minis seen (seasonal rows); MiniNew: those
	// unlocked since, in order, first for the mini quests (minis.go)
	MiniSeen []int32 `json:"mini_seen,omitempty"`
	MiniNew  []int32 `json:"mini_new,omitempty"`
	// Clashmas: the Clashmas quest group (clashmas.go)
	Clashmas  ClashmasState `json:"clashmas"`
	Season    SeasonState   `json:"season"`
	Purchases []Purchase    `json:"purchases,omitempty"`
	// StarterChests: the starting chests have been given (chests.go)
	StarterChests bool             `json:"starter_chests,omitempty"`
	Collection    map[int32]*Owned `json:"collection,omitempty"`
	// ResetToken: the last "reset:<token>" applied, so a build that resets
	// does it once and not on every start.
	ResetToken string `json:"reset_token,omitempty"`
	// TutorialPending: a brand-new account has not done the tutorial yet.
	// Cleared when the hook reports the tutorial done (the client sets the
	// avatar's bit 0x100), or by LogicSkipTutorialCommand 607,
	// LogicCompleteTutorialHomeCommand 988 or the name.
	TutorialPending bool `json:"tutorial_pending,omitempty"`
	// TutorialFlags: the avatar's tutorial flags reached so far (0x200 = the
	// battle part is done, the home part with the hero choice is next).
	TutorialFlags int32 `json:"tutorial_flags,omitempty"`

	path string
	mu   sync.Mutex
	rng  *rand.Rand
}

func (a *Account) random() *rand.Rand {
	if a.rng == nil {
		a.rng = rand.New(rand.NewSource(time.Now().UnixNano()))
	}
	return a.rng
}

// NewAccount is the starter account: Knight, Barbarian, Archer, Spear Goblin,
// Wizard with the Barbarian King in all three decks.
func NewAccount() *Account {
	a := &Account{Name: "Player", TutorialPending: true}
	for i := range a.Decks {
		a.Decks[i] = [5]int32{0, 1, 4, 5, 6}
		a.Heroes[i] = 12
	}
	return a
}

// LoadAccount reads path, or returns a new starter account if it is missing.
func LoadAccount(path string) (*Account, error) {
	a := NewAccount()
	a.path = path
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return a, a.Save()
	}
	if err != nil {
		return nil, err
	}
	// a saved account: not new, so no tutorial unless the file says so
	// (tutorial_pending is omitted when false)
	a.TutorialPending = false
	if err := json.Unmarshal(b, a); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	a.sanitize()
	return a, nil
}

func (a *Account) sanitize() {
	if a.Selected < 0 || a.Selected > 2 {
		a.Selected = 0
	}
	if a.Name == "" {
		a.Name = "Player"
	}
	if a.Trophies < 0 {
		a.Trophies = 0
	}
	if a.BestTrophies < a.Trophies {
		a.BestTrophies = a.Trophies
	}
	if a.Exp < 0 {
		a.Exp = 0
	}
}

// Save writes the account atomically. The caller must not hold the lock.
func (a *Account) Save() error {
	if a.path == "" {
		return nil
	}
	a.mu.Lock()
	b, err := json.MarshalIndent(a, "", "  ")
	a.mu.Unlock()
	if err != nil {
		return err
	}
	tmp := a.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, a.path)
}

// ApplyProgression applies the progression option (SetProgression) at server
// start and saves. Before the first fresh start it keeps a copy of the
// unlocked account next to the file (accounts.json.unlocked-backup).
func (a *Account) ApplyProgression(mode string) (string, error) {
	if mode == "" {
		return "", nil
	}
	// reset:<token> resets once per token (each build makes a new one).
	if m, token, ok := strings.Cut(mode, ":"); ok && m == "reset" {
		if token == a.ResetToken {
			mode = "fresh"
		} else {
			a.ResetToken = token
			mode = "reset"
		}
	}
	if !a.Progression && a.Collection == nil && a.path != "" && (mode == "fresh" || mode == "reset") {
		backup := a.path + ".unlocked-backup"
		if _, err := os.Stat(backup); errors.Is(err, fs.ErrNotExist) {
			if b, err := os.ReadFile(a.path); err == nil {
				_ = os.WriteFile(backup, b, 0o644)
			}
		}
	}
	a.Lock()
	msg := a.SetProgression(mode)
	a.Unlock()
	if msg == "" {
		return "", nil
	}
	return msg, a.Save()
}

// Lock/Unlock guard the account while commands are applied.
func (a *Account) Lock()   { a.mu.Lock() }
func (a *Account) Unlock() { a.mu.Unlock() }

// Tutorial: send new accounts into the game's tutorial (experimental).
var Tutorial bool

// DeckSlotValue is the int written after each current-deck slot (slot object
// +8, encoder 0x6ab6c0). Its meaning is unknown: SwapChess creates slots with
// 0 (0x7a0d1c). 1 emptied the battle hand after five deploys
// (TID_BATTLE_DECK_EMPTY); 0 fixes it.
var DeckSlotValue int32 = 0

// OwnHomeData builds the 25091 body for this account.
func (a *Account) OwnHomeData() []byte {
	a.Lock()
	defer a.Unlock()
	m := a.ownHome()
	return m.encode()
}

// ownHome fills the message from the account. The caller holds the lock.
func (a *Account) ownHome() ownHomeData {
	a.devTopUp()
	m := starterOwnHome()
	h := &m.home
	h.selected = a.Selected
	h.deckSlots = nil
	for _, row := range a.Decks[a.Selected] {
		if row < 0 {
			h.deckSlots = append(h.deckSlots, nil)
			continue
		}
		slot := [2]int32{row, DeckSlotValue}
		h.deckSlots = append(h.deckSlots, &slot)
	}
	h.decks = nil
	h.deckHeroes = nil
	for i, d := range a.Decks {
		var g [5]int32
		for j, row := range d {
			if row < 0 {
				g[j] = -1
			} else {
				g[j] = globalID(classSEASONAL, row)
			}
		}
		h.decks = append(h.decks, g)
		h.deckHeroes = append(h.deckHeroes, globalID(classSupportHero, a.Heroes[i]))
	}
	// The hero shown and used in battle is the hero collection's selected
	// entry (+0x10, the VInt after the hero list; set by 583's execute
	// 0x5a6888 -> 0x7561b4 -> 0x933fb8 to the hero's global id), not the
	// per-deck hero list. 0 made the client fall back to the first hero.
	h.heroExtra = globalID(classSupportHero, a.Heroes[a.Selected])
	h.skinStore = true
	h.skinSel = a.Skins
	h.skinOwned = a.ownedSkinSet()
	m.avatar.name = a.Name
	m.avatar.nameSet = a.NameSet
	// New players play the tutorial when Tutorial is on (tutorial flags 0).
	// At init the client goes straight into the tutorial's NPC battle
	// (LogicTutorialManager::onInitStateCheckIfGoToBattle 0x878a38), which
	// it only builds itself offline; the server signals the hook for that at
	// login (server.go), and the hook reports the end.
	if a.TutorialPending && Tutorial {
		m.avatar.tutorialFlags = a.TutorialFlags
	}
	// Name changes are counted by the client (+0x138) and the first one is
	// free; always sending 0 keeps renaming free.
	m.avatar.nameChanges = 0
	m.avatar.trophies = a.Trophies
	m.avatar.gems = a.Gems
	m.avatar.lastTrophies = a.Trophies
	m.avatar.seasonBest = a.BestTrophies
	m.avatar.expLevel, m.avatar.expPoints = ExpLevel(a.Exp)
	s := &a.Stats
	m.avatar.battleCounts = s.Played
	m.avatar.battleWins = s.Wins
	m.avatar.battleLosses = s.Losses
	m.avatar.battleFirsts = s.Firsts
	m.avatar.streakAll = s.Streak
	m.avatar.streakRanked = s.RankedStreak
	if a.Progression {
		a.giveSeasonEndRoad()
		a.fillCollection(&m)
		h.chests = homeChests(a.Chests, a.expLevel())
	}
	h.chestIndex = a.ChestIndex
	h.daily = a.dailyGold()
	if Live {
		a.refreshQuests()
		a.refreshMiniQuests()
		a.refreshClashmas()
		a.refreshChains()
	}
	h.events = a.events()
	if Live {
		groups := clashmasOn() && a.Progression
		h.quests = questState{quests: a.Quests, lastID: a.QuestID, rerolls: a.QuestRerolls, taken: a.questPoolTaken(), mini: a.MiniQuests,
			clashmas: a.Clashmas, groups: groups, chains: a.chainQuests()}
		// the server time, unix seconds: the tail's second VInt (msg +0x9c,
		// 0x48b2e4) becomes the logic's time base (+0x1d4 via 0x85f100,
		// 0x327c54; now = ticks/20 + base, 0x4df510). At 0 the events'
		// windows lie in the future: the quest pool is inactive and a reroll
		// (0x5e0478 finds no pool) rebuilds the old quest (0x8c8c8c).
		m.tail[1] = int32(now().Unix())
		h.season = a.season()
		h.purchases = a.shopPurchases()
		h.seasonSeen = h.season.Seen - 1
		if a.SeasonEndKey == 0 && a.Progression {
			a.SeasonEndKey = h.season.Key // no 451 for a season before
		}
	}
	m.avatar.seasonEnd = [3]int32{a.SeasonEndKey, a.SeasonEndTrophies, a.SeasonEndBest}
	return m
}

// --- commands (PORTING.md §6b; applied by the server in commands.go) ---

// SetName: LogicChangeAvatarNameByUserCommand 549.
func (a *Account) SetName(name string) {
	a.Name = name
	a.NameSet = true
	a.TutorialPending = false
}

// SelectSkin: LogicSelectSkinCommand 592 (character global id, skin global
// id). A default skin clears the selection.
func (a *Account) SelectSkin(skin int32) {
	owner, extra, ok := SkinOwner(skin)
	if !ok {
		return
	}
	if a.Skins == nil {
		a.Skins = map[int32]int32{}
	}
	if extra {
		if !a.ownsSkin(skin) {
			return
		}
		a.Skins[owner] = skin
	} else {
		delete(a.Skins, owner)
	}
}

// SelectDeck: LogicSelectDeckCommand 597.
func (a *Account) SelectDeck(i int32) {
	if i >= 0 && i <= 2 {
		a.Selected = i
	}
}

// SelectHero: LogicSelectSupportHeroCommand 583 (global id, class 107).
func (a *Account) SelectHero(global int32) {
	if global/1_000_000 == classSupportHero {
		a.Heroes[a.Selected] = global % 1_000_000
	}
}

// Swap: LogicSwapChessCommand 500. mini = seasonal row or -1, slot 0..4,
// slot2 = second slot or -1: put a mini in, swap two slots, or clear one.
func (a *Account) Swap(mini, slot, slot2 int32) {
	d := &a.Decks[a.Selected]
	if slot < 0 || slot > 4 {
		return
	}
	switch {
	case mini >= 0:
		for j := range d {
			if d[j] == mini && int32(j) != slot {
				d[j] = d[slot] // mini moves: the old occupant takes its place
			}
		}
		d[slot] = mini
	case slot2 >= 0 && slot2 <= 4:
		d[slot], d[slot2] = d[slot2], d[slot]
	default:
		d[slot] = -1
	}
	a.questProgress(questMetaChangeDeck, 1, nil)
}

// AddTroop: LogicAddTroopToDeckCommand 595 (global mini id, add or remove).
func (a *Account) AddTroop(global int32, add bool) {
	row := global % 1_000_000
	d := &a.Decks[a.Selected]
	for j := range d {
		if (add && d[j] < 0) || (!add && d[j] == row) {
			if add {
				d[j] = row
			} else {
				d[j] = -1
			}
			a.questProgress(questMetaChangeDeck, 1, nil)
			return
		}
	}
}
