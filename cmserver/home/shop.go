package home

import (
	"encoding/json"
	"math/rand"
	"time"
)

// The shop (docs/LIVE_CONTENT.md).
//
// A SHOP_LAYOUT chronos event (type 0, LogicLiveShopLayoutData 0x46e118):
// "Modules", each (0x8e6494) ModuleName (shop_modules.csv), ModuleTitle,
// Items, RotatingItems, …; an item (0x6bb398): ResourceType (a resource's
// name), ResourceCost, PurchaseLimit, ItemTID, ButtonWidgetType
// (SmallIcon/LargeIcon), Chests (by name), IAP (real money, left out).
// An item's number is its place counted over the modules, each module's
// Items before its RotatingItems (0x503e08).
//
// Buying with gold or gems: LogicBuyChestCommand 702 (base, VInt event,
// VInt item; execute 0x93286c): the client checks the price and the
// limit, takes the price, counts the purchase in the event manager's list
// (+0x28, 0x461c20) and makes one chest of the item's chests (source 4,
// value the event id, 0x932bfc); the server pushes the content (420).
// The purchases come back in the home (the chronos part's first list of
// (VInt event, VInt item), 0x8198f8), which is how the limits hold.
//
// The shop as on screenshots of 1.1143.2 (December 2021, docs/ORIGINAL.md):
//   - Minis: three offers a day, one fragment each, once each, for 250 /
//     500 / 750 gold by the mini's elixir cost (2 / 3 / 4);
//   - Heroes: three offers a day, 25 fragments each, five times each, for
//     100 gems.
// A new shop every day (UTC), its own event id per day so the limits start
// anew. The chests are the game's shop chests (shop_unit_*,
// shop_miniHero_*_25). Left out: gem packs and bundles (real money).

// sourceShop: a chest bought in the shop (0x932bf4).
const sourceShop int32 = 4

// shopEventBase: the shop's event id is this plus the day number.
const shopEventBase int32 = 3_000_000

type shopItem struct {
	Chest    int32
	Resource string // "Gold" or "Diamonds"
	Cost     int32
	Limit    int32
}

type shopModule struct {
	Name, Title string
	Items       []shopItem
}

// Purchase is one item bought.
type Purchase struct {
	Event int32 `json:"event"`
	Item  int32 `json:"item"`
}

// The game's shop chests (chests.json): shop_unit_* one fragment of a mini,
// shop_miniHero_*_25 25 of a hero.
var (
	shopMiniChests = func() []int32 {
		var out []int32
		for id := int32(127_000_017); id <= 127_000_040; id++ {
			out = append(out, id)
		}
		return out
	}()
	// shop_miniHero_{VampireCountess,BarbarianKing,ArcherQueen,Monk,
	// ShieldMaiden,RoyalChampion}_25: the six heroes of the client
	shopHeroChests = []int32{127_000_076, 127_000_071, 127_000_072, 127_000_075, 127_000_073, 127_000_169}
)

// Prices and limits from the screenshots.
const (
	shopMiniLimit    int32 = 1
	shopHeroGems     int32 = 100 // 25 hero fragments
	shopHeroLimit    int32 = 5
	shopMinisPerDay        = 3
	shopHeroesPerDay       = 3
)

// shopMiniGold: a mini fragment's price by its elixir cost: 2 → 250,
// 3 → 500, 4 → 750 (Skeleton Guard, Musketeer, Battle Healer on the
// screenshot).
func shopMiniGold(cost int32) int32 {
	return 250 * min(max(cost-1, 1), 3)
}

// shopMiniOffers: the shop_unit chests of minis the client has, with their
// price.
func shopMiniOffers() []shopItem {
	inClient := map[int32]bool{}
	for _, r := range miniSeasonalRows {
		inClient[miniID(r)] = true
	}
	var out []shopItem
	for _, id := range shopMiniChests {
		c, ok := Data.Chest(id)
		if !ok || len(c.FragList) != 1 || !inClient[c.FragList[0]] {
			continue
		}
		row := c.FragList[0] - miniID(0)
		cost := Data.MiniCost(row)
		if cost == 0 {
			cost = 3
		}
		out = append(out, shopItem{id, "Gold", shopMiniGold(cost), shopMiniLimit})
	}
	return out
}

func shopDay(t time.Time) int32 {
	return int32(t.UTC().Unix() / 86400)
}

// shopModules: the day's shop, the same all day.
func shopModules(day int32) []shopModule {
	rng := rand.New(rand.NewSource(int64(day) * 7919))
	offers := shopMiniOffers()
	var miniItems []shopItem
	if n := min(shopMinisPerDay, len(offers)); n > 0 {
		for _, i := range rng.Perm(len(offers))[:n] {
			miniItems = append(miniItems, offers[i])
		}
	}
	var heroItems []shopItem
	for _, i := range rng.Perm(len(shopHeroChests))[:shopHeroesPerDay] {
		heroItems = append(heroItems, shopItem{shopHeroChests[i], "Diamonds", shopHeroGems, shopHeroLimit})
	}
	return []shopModule{
		{"BasicModule", "TID_CHRONOS_SHOP_TITLE_MINIS", miniItems},
		{"BasicModule", "TID_CHRONOS_SHOP_TITLE_HEROES", heroItems},
	}
}

// shopItemAt: an item by its number.
func shopItemAt(day, n int32) (shopItem, bool) {
	for _, m := range shopModules(day) {
		if n < int32(len(m.Items)) {
			if n < 0 {
				break
			}
			return m.Items[n], true
		}
		n -= int32(len(m.Items))
	}
	return shopItem{}, false
}

func shopLiveEvent() LiveEvent {
	t := now().UTC()
	day := shopDay(t)
	start := time.Unix(int64(day)*86400, 0)
	end := start.AddDate(0, 0, 1)
	var modules []any
	for _, m := range shopModules(day) {
		var items []any
		for _, it := range m.Items {
			items = append(items, map[string]any{
				"ResourceType":     it.Resource,
				"ResourceCost":     it.Cost,
				"PurchaseLimit":    it.Limit,
				"ButtonWidgetType": "SmallIcon",
				"CountdownMode":    "EndTimer",
				"Chests":           chestNames([]int32{it.Chest}),
			})
		}
		modules = append(modules, map[string]any{
			"ModuleName":               m.Name,
			"ModuleTitle":              m.Title,
			"Items":                    items,
			"RegularItemsRefreshDaily": true,
			// the clock: "ForceNoShow" 1, "RotationTimer" 2, "EndTimer" 3
			// (0x8e6864, items 0x5b6060), none without; EndTimer counts
			// down to the event's end, the next shop
			"CountdownMode": "EndTimer",
			"Priority":      len(modules),
		})
	}
	b, _ := json.Marshal(map[string]any{
		"Modules":        modules,
		"LevelLimitLow":  0,
		"LevelLimitHigh": 999,
	})
	return LiveEvent{
		ID: shopEventBase + day, Name: "daily_shop", Type: EventShopLayout,
		Start: int32(start.Unix()), End: int32(end.Unix()),
		VisibleStart: int32(start.Unix()), VisibleEnd: int32(end.Unix()),
		Parameters: string(b),
	}
}

// todaysPurchases drops the purchases of other days' shops.
func (a *Account) todaysPurchases() []Purchase {
	ev := shopEventBase + shopDay(now())
	keep := a.Purchases[:0]
	for _, p := range a.Purchases {
		if p.Event == ev {
			keep = append(keep, p)
		}
	}
	a.Purchases = keep
	return keep
}

// BuyShopItem buys an item of today's shop (702) as the client does: the
// price, the limit, one chest. It returns the chest bought.
func (a *Account) BuyShopItem(r *Reward, event, item int32) (int32, string) {
	if !Live {
		return 0, "no shop"
	}
	day := shopDay(now())
	if event != shopEventBase+day {
		return 0, "not today's shop"
	}
	it, ok := shopItemAt(day, item)
	if !ok {
		return 0, "no such item"
	}
	n := int32(0)
	for _, p := range a.todaysPurchases() {
		if p.Item == item {
			n++
		}
	}
	if it.Limit > 0 && n >= it.Limit {
		return 0, "sold out"
	}
	switch it.Resource {
	case "Gold":
		if a.Gold < it.Cost {
			return 0, "not enough gold"
		}
		a.Gold -= it.Cost
	case "Diamonds":
		if a.Gems < it.Cost {
			return 0, "not enough gems"
		}
		a.Gems -= it.Cost
	}
	a.Purchases = append(a.Purchases, Purchase{event, item})
	c, err := rollChest(a.random(), it.Chest)
	if err != nil {
		c = ChestContent{}
	}
	a.pushChest(r, it.Chest, sourceShop, c)
	if n := len(a.Chests); ChestsInHome && n > 0 && a.Chests[n-1].Index == a.ChestIndex {
		a.Chests[n-1].SourceValue = event
	}
	return it.Chest, ""
}
