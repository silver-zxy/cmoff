package home

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"
)

// shopData: the shop chests with one fragment each (a disabled mini among
// them) and the minis' elixir costs.
func shopData() *GameData {
	d := &GameData{chests: make([]chestDef, 280), miniCost: map[int32]int32{}}
	frag := func(id, of, n int32) {
		d.chests[id-chestBase] = chestDef{Name: fmt.Sprint("chest", id), FragMin: n,
			FragTier: []tier{{Picks: 1}}, FragTierWeight: []int32{1}, FragList: []int32{of}, FragWeight: []int32{1}}
	}
	for i, id := range shopMiniChests {
		row := miniSeasonalRows[i%len(miniSeasonalRows)]
		if i == 0 {
			row = 2 // a row the client does not have
		}
		frag(id, miniID(row), 1)
		d.miniCost[row] = int32(2 + i%3)
	}
	for _, id := range shopHeroChests {
		frag(id, heroID(12), 25)
	}
	for id, row := range map[int32]int32{chestSkinCrystalPekka: 35, chestSkinFabulous: 89, chestSkinJolly: 90,
		chestSkinHelpful: 91, chestSkinGift: 92, chestSkinSnowler: 94, chestSkinSparkle: 95} {
		frag(id, globalID(classSkin, row), 1)
	}
	return d
}

// Buying: the price, the limit, one chest (source 4, the event); the
// purchases come back in the home; the next day's shop starts anew.
func TestShop(t *testing.T) {
	Data = shopData()
	defer func() { Data = nil }()
	day := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)
	now = func() time.Time { return day }
	defer func() { now = time.Now }()
	a := NewAccount()
	a.StartProgression()
	a.Gold, a.Gems = 5000, 1000
	ev := shopLiveEvent()
	var p struct {
		Modules []struct {
			ModuleName    string
			CountdownMode string
			Items         []struct {
				ResourceType  string
				ResourceCost  int32
				PurchaseLimit int32
				Chests        []any
			}
		}
	}
	if err := json.Unmarshal([]byte(ev.Parameters), &p); err != nil || len(p.Modules) != 5 ||
		len(p.Modules[0].Items) != shopMinisPerDay || len(p.Modules[1].Items) != shopHeroesPerDay ||
		p.Modules[0].CountdownMode != "EndTimer" {
		t.Fatalf("%v %s", err, ev.Parameters)
	}
	for _, it := range p.Modules[0].Items {
		if it.ResourceType != "Gold" || it.PurchaseLimit != 1 || (it.ResourceCost != 250 && it.ResourceCost != 500 && it.ResourceCost != 750) {
			t.Fatalf("mini offer %+v", it)
		}
		if it.Chests[0] == "chest127000017" {
			t.Fatal("a mini the client does not have")
		}
	}
	for _, it := range p.Modules[1].Items {
		if it.ResourceType != "Diamonds" || it.PurchaseLimit != 5 || it.ResourceCost != 100 {
			t.Fatalf("hero offer %+v", it)
		}
	}
	// item 0: a mini fragment for gold, once a day
	price := p.Modules[0].Items[0].ResourceCost
	if c, why := a.BuyShopItem(&Reward{}, ev.ID, 0); c == 0 {
		t.Fatalf("buy: %s", why)
	}
	if c, why := a.BuyShopItem(&Reward{}, ev.ID, 0); c != 0 || why != "sold out" || a.Gold != 5000-price {
		t.Fatalf("sold out not kept: %s gold %d", why, a.Gold)
	}
	last := a.Chests[len(a.Chests)-1]
	if last.Source != sourceShop || last.SourceValue != ev.ID {
		t.Fatalf("chest %+v", last)
	}
	// item 3: a hero, 25 fragments for 100 gems, five times
	for i := 0; i < 5; i++ {
		if c, why := a.BuyShopItem(&Reward{}, ev.ID, 3); c == 0 {
			t.Fatalf("hero %d: %s", i, why)
		}
	}
	if c, _ := a.BuyShopItem(&Reward{}, ev.ID, 3); c != 0 || a.Gems != 1000-5*shopHeroGems {
		t.Fatalf("hero limit: gems %d", a.Gems)
	}
	if got := a.Chests[len(a.Chests)-1].Content.Fragments[heroID(12)]; got != 25 {
		t.Fatalf("hero chest gave %d fragments", got)
	}
	// not enough gems
	a.Gems = 99
	if c, why := a.BuyShopItem(&Reward{}, ev.ID, 4); c != 0 || why != "not enough gems" {
		t.Fatalf("bought without gems: %s", why)
	}
	if c, _ := a.BuyShopItem(&Reward{}, ev.ID, 6); c != 0 {
		t.Fatal("no item 6")
	}
	if c, _ := a.BuyShopItem(&Reward{}, ev.ID-1, 0); c != 0 {
		t.Fatal("yesterday's shop")
	}
	h := a.ownHome()
	h.home.skinStore = false
	m, err := decode(h.encode())
	if err != nil || len(m.home.purchases) != 6 || m.home.purchases[0] != (Purchase{ev.ID, 0}) || m.home.season == nil {
		t.Fatalf("%v purchases %+v", err, m.home.purchases)
	}
	// the skins: 6 Jolly (159 gems), 7 Helpful, 8 the Clashmas bundle
	// (free, three skins), 9 Crystal P.E.K.K.A (100 Crystal Coins), 10
	// Fabulous Prince (600)
	a.Gems, a.CrystalCoins = 200, 150
	// a skin is owned once its chest is opened
	buy := func(item int32) (int32, string) {
		c, why := a.BuyShopItem(&Reward{}, ev.ID, item)
		if c != 0 {
			a.OpenPending(a.Chests[len(a.Chests)-1].Index)
		}
		return c, why
	}
	if c, why := buy(8); c == 0 || a.Gems != 200 || len(a.OwnedSkins) != 3 {
		t.Fatalf("bundle: %s gems %d skins %v", why, a.Gems, a.OwnedSkins)
	}
	if c, why := buy(6); c == 0 || a.Gems != 41 {
		t.Fatalf("jolly: %s gems %d", why, a.Gems)
	}
	if c, why := buy(9); c == 0 || a.CrystalCoins != 50 || !a.ownsSkin(globalID(classSkin, 35)) {
		t.Fatalf("crystal pekka: %s coins %d", why, a.CrystalCoins)
	}
	if c, why := a.BuyShopItem(&Reward{}, ev.ID, 10); c != 0 || why != "not enough crystal coins" {
		t.Fatalf("fabulous: %s", why)
	}
	// the next day the owned skins are still "bought"
	day = day.AddDate(0, 0, 1)
	ev2 := shopLiveEvent()
	got := map[int32]bool{}
	for _, p := range a.shopPurchases() {
		if p.Event == ev2.ID {
			got[p.Item] = true
		}
	}
	if !got[6] || !got[8] || !got[9] || got[7] || got[10] {
		t.Fatalf("owned skins in the next shop %v", got)
	}
	if c, why := a.BuyShopItem(&Reward{}, ev2.ID, 9); c != 0 || why != "skin already owned" {
		t.Fatalf("bought an owned skin: %s", why)
	}
	day = day.AddDate(0, 0, -1)
	// the next day
	day = day.AddDate(0, 0, 1)
	if len(a.todaysPurchases()) != 0 || shopLiveEvent().ID != ev.ID+1 {
		t.Fatal("next day")
	}
}

func TestShopMiniGold(t *testing.T) {
	for cost, want := range map[int32]int32{1: 250, 2: 250, 3: 500, 4: 750, 5: 750} {
		if g := shopMiniGold(cost); g != want {
			t.Fatalf("elixir %d: %d gold, want %d", cost, g, want)
		}
	}
}

// The quest pool changes with the day: four different quests of the
// catalog, the same all day.
func TestQuestPoolDaily(t *testing.T) {
	day := time.Date(2026, 11, 5, 1, 0, 0, 0, time.UTC)
	now = func() time.Time { return day }
	defer func() { now = time.Now }()
	a := questPoolSignature()
	day = day.Add(20 * time.Hour)
	if questPoolSignature() != a {
		t.Fatal("pool changed within the day")
	}
	seen := map[string]bool{a: true}
	for i := 0; i < 6; i++ {
		day = day.AddDate(0, 0, 1)
		p := todaysQuests()
		if len(p) != questPoolSize {
			t.Fatalf("pool %d", len(p))
		}
		seen[questPoolSignature()] = true
	}
	if len(seen) < 4 {
		t.Fatalf("pools barely change: %d different in 7 days", len(seen))
	}
}

// The currency shop at the end: gold for gems, gem packs for gold once a day.
func TestShopCurrency(t *testing.T) {
	apk := os.Getenv("CMSERVER_TEST_APK")
	if apk == "" {
		t.Skip("CMSERVER_TEST_APK not set")
	}
	d, err := LoadGameData(apk)
	if err != nil {
		t.Skip(err)
	}
	Data = d
	defer func() { Data = nil }()
	a := NewAccount()
	a.StartProgression()
	a.Gold, a.Gems = 0, 100
	mods := shopModules(shopDay(now().UTC()))
	gold, gems := mods[len(mods)-2], mods[len(mods)-1]
	if gold.Name != "PermCurrencyModule" || len(gold.Items) != 3 || len(gems.Items) != 3 {
		t.Fatalf("%+v %+v", gold, gems)
	}
	idx := int32(0)
	for _, m := range mods[:len(mods)-2] {
		idx += int32(len(m.Items))
	}
	ev := shopLiveEvent()
	if c, why := a.BuyShopItem(&Reward{}, ev.ID, idx); c == 0 || a.Gems != 40 {
		t.Fatalf("gold pack: %s, gems %d", why, a.Gems)
	}
	// gem packs for gold, once a day
	gem := idx + 3
	if c, _ := a.BuyShopItem(&Reward{}, ev.ID, gem); c != 0 {
		t.Fatal("gem pack without the gold")
	}
	a.Gold = 6000
	if c, why := a.BuyShopItem(&Reward{}, ev.ID, gem); c == 0 || a.Gold != 1000 || a.Gems != 40 {
		t.Fatalf("gem pack: %s, gold %d gems %d", why, a.Gold, a.Gems)
	}
	a.Gold = 6000
	if c, _ := a.BuyShopItem(&Reward{}, ev.ID, gem); c != 0 {
		t.Fatal("gem pack twice a day")
	}
}
