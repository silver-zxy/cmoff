package home

import (
	"fmt"
	"math/rand"
	"sort"

	"cmserver/proto"
)

// Chests the player opens in the home. The home's chest manager (+0x1b0,
// encoder 0x482548: VInt count, entries 0x5d1204, bool) holds chests with
// their content. In the home the client opens the first one with content
// by itself (daily update 0x59a380 -> 0x3115e0 -> 0x313ef4 -> 0x5a7154),
// shows it (chests.json showChestOpening) and sends
// LogicChestOpenFinishCommand 550 (base, VInt chest index; execute
// 0x4fcfdc), which applies the content (0x823924) on the client. The server
// applies the same content then (OpenPending). The content is the server's:
// a chest the client makes itself (the upgrade reward of command 602,
// 0x7eb554) has none and is not opened until the server's home has it.
//
// One entry (0x5d1204):
//
//	chest (0x8f21c4): bool +6, bool +4, bool +5 (1 = new, 0x5ce89c),
//	    VInt list +0x10 (chest ids; empty when +0 is set), VInt +0 chest
//	    global id, VInt +8 index (home +0xb0 counts them), VInt +0x34,
//	    VInt +0x30 (source, 0x6b2ef8), nullable string +0x20, VInt +0x38
//	bool: content present, then the content (vtable 0xea5268, 0x3d7a8c):
//	    VInt count of reward entries (vtable 0xea4fe8, 0x90c344), bool +0x18
//	    (show it: 0x9443a4; without, 0x8d9c74 sends 550 at once, no popup)
//	    reward entry: five VInt lists: +8 resource global ids, +0x18 their
//	    amounts (0x823924 -> 0x69fb0c), +0x28 collectible ids, +0x38
//	    their fragments (-> 0x7d7550 on the collection +0x200), +0x48;
//	    then bool +0x5c, VInt +0x58
//	VInt +0x10 +0x14 +0x18 +0x1c +0x20 (passed on to 0x823924), bool +0x24

// ChestContent is what a chest gives: resources by name (Gold, Diamonds,
// ExpPoints) and fragments by collectible id.
type ChestContent struct {
	Resources map[string]int32 `json:"resources,omitempty"`
	Fragments map[int32]int32  `json:"fragments,omitempty"`
}

func (c ChestContent) empty() bool {
	for _, n := range c.Resources {
		if n != 0 {
			return false
		}
	}
	for _, n := range c.Fragments {
		if n != 0 {
			return false
		}
	}
	return true
}

// add adds d's resources and fragments to c.
func (c *ChestContent) add(d ChestContent) {
	for k, n := range d.Resources {
		if c.Resources == nil {
			c.Resources = map[string]int32{}
		}
		c.Resources[k] += n
	}
	for k, n := range d.Fragments {
		if c.Fragments == nil {
			c.Fragments = map[int32]int32{}
		}
		c.Fragments[k] += n
	}
}

func (c ChestContent) String() string {
	s := ""
	add := func(p string) {
		if s != "" {
			s += ", "
		}
		s += p
	}
	for _, name := range chestResourceNames {
		if n := c.Resources[name]; n != 0 {
			add(fmt.Sprintf("%d %s", n, name))
		}
	}
	ids := make([]int, 0, len(c.Fragments))
	for id := range c.Fragments {
		ids = append(ids, int(id))
	}
	sort.Ints(ids)
	for _, id := range ids {
		add(fmt.Sprintf("%d× %d", c.Fragments[int32(id)], id))
	}
	if s == "" {
		return "nothing"
	}
	return s
}

// PendingChest is a chest waiting in the home to be opened.
type PendingChest struct {
	Index int32 `json:"index"`
	Chest int32 `json:"chest"` // chests.json global id
	// List: a chest made of several (+0x10, the chest id -1; 0x71e890),
	// as the starting chests (0x687ccc)
	List    []int32      `json:"list,omitempty"`
	Content ChestContent `json:"content"`
	// Source: chest +0x30 (0x6b2ef8): 3 upgrade reward (602, 0x7eb55c), 6
	// player level (0x8d9c1c: the level-up popup), 0 otherwise.
	Source int32 `json:"source,omitempty"`
	// SourceValue: chest +0x34, set with the source; for a level chest the
	// player level (0x82ea2c). The level-up popup sends 550 only for the
	// chest of the current level (0x45fd8c, "Invalid current level").
	SourceValue int32 `json:"source_value,omitempty"`
	// Show: the opening popup (ChestRewardPopup, 0x6bd588) instead of a
	// silent open; chests.json showChestOpening.
	Show bool `json:"show,omitempty"`
	// Push: the client made this chest itself and waits for its content
	// (420, pushChest).
	Push bool `json:"-"`
}

// Chest sources (chest +0x30).
const (
	sourceUpgrade     int32 = 3
	sourceStarter     int32 = 8 // the starting chest (0x687e50)
	sourcePlayerLevel int32 = 6
)

// resources.csv rows of the resources a chest can give here.
var resourceRows = map[string]int32{"Diamonds": 0, "Gold": 1, "ExpPoints": 11, "PromotionToken": 13, "SeasonPoints": 14}

// chestResourceNames: the resources above in a fixed order.
var chestResourceNames = []string{"Gold", "Diamonds", "ExpPoints", "PromotionToken", "SeasonPoints"}

func listInts(w *proto.Writer, xs []int32) {
	w.VInt(int32(len(xs)))
	for _, x := range xs {
		w.VInt(x)
	}
}

func (c ChestContent) encode(w *proto.Writer, show bool) {
	var resIDs, resN, fragIDs, fragN []int32
	for _, name := range chestResourceNames {
		if n := c.Resources[name]; n > 0 {
			resIDs = append(resIDs, globalID(classRESOURCES, resourceRows[name]))
			resN = append(resN, n)
		}
	}
	ids := make([]int, 0, len(c.Fragments))
	for id := range c.Fragments {
		ids = append(ids, int(id))
	}
	sort.Ints(ids)
	for _, id := range ids {
		if n := c.Fragments[int32(id)]; n > 0 {
			fragIDs = append(fragIDs, int32(id))
			fragN = append(fragN, n)
		}
	}
	w.VInt(1) // one reward entry
	listInts(w, resIDs)
	listInts(w, resN)
	listInts(w, fragIDs)
	listInts(w, fragN)
	listInts(w, nil)
	w.Bool(false) // +0x5c
	w.VInt(0)     // +0x58
	w.Bool(show)  // content +0x18: show it
}

// EncodeContent writes the chest's content as the client's content object
// encodes it (for LogicClaimRewardCommand 420).
func (c PendingChest) EncodeContent(w *proto.Writer) { c.Content.encode(w, c.Show) }

func encodeChests(w *proto.Writer, chests []PendingChest) {
	w.VInt(int32(len(chests)))
	for _, c := range chests {
		w.Bool(false) // +6
		w.Bool(false) // +4
		w.Bool(true)  // +5: new
		if len(c.List) > 0 {
			w.VInt(int32(len(c.List))) // +0x10 the chest ids
			for _, id := range c.List {
				w.VInt(id)
			}
			w.VInt(-1)
		} else {
			w.VInt(0) // chest id list
			w.VInt(c.Chest)
		}
		w.VInt(c.Index)
		w.VInt(c.SourceValue) // +0x34
		w.VInt(c.Source)
		w.StringNull()
		w.VInt(0) // +0x38
		w.Bool(true)
		c.Content.encode(w, c.Show)
		vints(w, 5)
		w.Bool(false) // +0x24
	}
	w.Bool(false) // +0x10: no chest being opened
}

// ChestsInHome: rewards come as chests in the home (true, the server
// option chests=on) or go straight into the account (chests=off).
var ChestsInHome = true

// queueChest puts a chest into the home for the player to open.
func (a *Account) queueChest(r *Reward, chest int32, c ChestContent) {
	a.queueChestFrom(r, chest, 0, c)
}

// queueChestFrom: queueChest with the chest's source.
func (a *Account) queueChestFrom(r *Reward, chest, source int32, c ChestContent) {
	a.addChest(r, chest, source, c, false)
}

// pushChest gives the content of a chest the client has just made itself
// (an upgrade's exp chest, 602 -> 0x7eb554 or the auto-upgrade after a
// chest; a level chest, 0x82ea1c): it got the next index (home +0xb0 + 1)
// but no content. The server sends the content right away with
// LogicClaimRewardCommand 420 (TakePushes), which sets it (0x3358d4) and
// opens the chest (0x313ef4), as the live server did.
func (a *Account) pushChest(r *Reward, chest, source int32, c ChestContent) {
	a.addChest(r, chest, source, c, true)
}

func (a *Account) addChest(r *Reward, chest, source int32, c ChestContent, push bool) {
	if !ChestsInHome {
		if c.empty() {
			return
		}
		if r == nil {
			r = &Reward{}
		}
		a.openContent(r, c)
		return
	}
	// the client counts its own chests even when they give nothing
	a.ChestIndex++
	if c.empty() && !push {
		a.ChestIndex--
		return
	}
	// level chests and shop purchases always open with the chest (a
	// bought hero comes in a chest, not silently; chests.json has
	// shop_miniHero_* without the opening)
	always := source == sourcePlayerLevel || source == sourceShop
	show := chest == winChest || always
	if d, ok := Data.Chest(chest); ok {
		show = d.ShowChestOpening || always
	}
	a.Chests = append(a.Chests, PendingChest{Index: a.ChestIndex, Chest: chest, Content: c,
		Source: source, Show: show, Push: push})
	if r != nil {
		r.Chests = append(r.Chests, chest)
	}
}

// TakePushes returns the chests to send with 420 now and forgets them as
// pushes (they stay waiting until opened).
func (a *Account) TakePushes() []PendingChest {
	var out []PendingChest
	for i := range a.Chests {
		if a.Chests[i].Push {
			out = append(out, a.Chests[i])
			a.Chests[i].Push = false
		}
	}
	return out
}

// ClearPushes: a home is sent, which has every chest with its content.
func (a *Account) ClearPushes() {
	for i := range a.Chests {
		a.Chests[i].Push = false
	}
}

// OpenPending is LogicChestOpenFinishCommand 550: the chest with that index
// gives its content. ok = there was one.
func (a *Account) OpenPending(index int32) (r *Reward, chest int32, ok bool) {
	r = &Reward{}
	for i, c := range a.Chests {
		if c.Index != index {
			continue
		}
		a.Chests = append(a.Chests[:i:i], a.Chests[i+1:]...)
		a.openContent(r, c.Content)
		return r, c.Chest, true
	}
	return r, 0, false
}

// openContent gives a chest's content and what the client does then, in its
// order (0x823924): the resources, so a level reached makes its level chest;
// the fragments; then the auto-upgrade (0x823d1c), each level with its exp
// chest. Those chests are the client's own, so their content is pushed.
// Without chests in the home (chests=off) everything is given at once.
func (a *Account) openContent(r *Reward, c ChestContent) {
	levels := len(r.LevelUps)
	a.applyContent(r, c)
	for _, l := range r.LevelUps[levels:] {
		a.pushLevelChest(r, l)
	}
	for n := a.autoUpgrade(r); n > 0; n-- {
		a.pushChest(r, upgradeRewardChest, sourceUpgrade, upgradeExpContent())
	}
}

// pushLevelChest gives the level chest of a player level, once. The client
// makes it when it applies the exp, which for an exp chest it got with 420
// happens after the next 420 already ran (seen: both 420s run, the level
// chest's 550 never comes; the 550 goes through 0x6a2344 first); a 420 for
// a chest not there yet is dropped (0x6a5d34), so the level chest's
// content goes when the exp chest's 550 arrives here. The client sends that
// 550 at once (immediate, 0x8d9c74 -> 0x6a2344(..., 1)): ~10 ms.
func (a *Account) pushLevelChest(r *Reward, level int32) {
	for _, l := range a.LevelChests {
		if l == level {
			return
		}
	}
	a.LevelChests = append(a.LevelChests, level)
	a.pushChest(r, levelChest(level), sourcePlayerLevel, a.levelChestContent(level))
	if n := len(a.Chests); ChestsInHome && n > 0 && a.Chests[n-1].Index == a.ChestIndex {
		a.Chests[n-1].SourceValue = level
	}
}

func upgradeExpContent() ChestContent {
	return ChestContent{Resources: map[string]int32{"ExpPoints": upgradeExp()}}
}

func (a *Account) applyContent(r *Reward, c ChestContent) {
	for name, n := range c.Resources {
		switch name {
		case "Gold":
			a.give(r, n)
		case "Diamonds":
			a.Gems += n
			r.Gems += n
		case "ExpPoints":
			a.addExp(r, n)
		case "SeasonPoints":
			a.addSeasonPoints(r, n)
		case "PromotionToken":
			a.addCrystalCoins(r, n)
		}
	}
	ids := make([]int, 0, len(c.Fragments))
	for id := range c.Fragments {
		ids = append(ids, int(id))
	}
	sort.Ints(ids)
	for _, id := range ids {
		a.addFragments(r, int32(id), c.Fragments[int32(id)])
	}
}

// rollChest decides a chest's content by its chests.json definition (the
// rules of OpenChest), without giving it.
func rollChest(rng *rand.Rand, id int32) (ChestContent, error) {
	c, ok := Data.Chest(id)
	if !ok {
		return ChestContent{}, fmt.Errorf("chest %d unknown (game data not loaded?)", id)
	}
	var out ChestContent
	for _, res := range c.Resources {
		if _, known := resourceRows[res.Name]; !known {
			continue
		}
		n := res.Min + roll(rng, pickTier(rng, res.Tiers, res.TierWeights))
		if n > 0 {
			if out.Resources == nil {
				out.Resources = map[string]int32{}
			}
			out.Resources[res.Name] += n
		}
	}
	t := pickTier(rng, c.FragTier, c.FragTierWeight)
	if len(c.FragList) > 0 && t.Picks > 0 {
		total := c.FragMin + roll(rng, t)
		each := max(total/t.Picks, 1)
		weights := append([]int32(nil), c.FragWeight...)
		for len(weights) < len(c.FragList) {
			weights = append(weights, 1)
		}
		out.Fragments = map[int32]int32{}
		for p := int32(0); p < t.Picks; p++ {
			i := pickWeighted(rng, weights)
			if weights[i] <= 0 {
				break
			}
			if !c.DuplicatesAllowed {
				weights[i] = 0
			}
			out.Fragments[c.FragList[i]] += each
		}
	}
	return out, nil
}

// winChest: the chest the win fragments come in, the daily quest's mini
// chest (quest_Daily_Mini, showChestOpening).
const winChest int32 = 127_000_062
