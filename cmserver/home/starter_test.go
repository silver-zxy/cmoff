package home

import "testing"

// starterData: chests for the starting choices, one fragment
// of the Spear Goblin each, 500 gold in shop_gold_500.
func starterData() *GameData {
	d := &GameData{chests: make([]chestDef, 80)}
	for _, choice := range startingChoices {
		for _, id := range choice.chests {
			c := &d.chests[id-127_000_000]
			c.FragMin, c.FragTier, c.FragTierWeight = 1, []tier{{Picks: 1}}, []int32{1}
			c.FragList, c.FragWeight = []int32{miniID(5)}, []int32{1}
		}
	}
	d.chests[77] = chestDef{Name: "shop_gold_500", Resources: []chestResource{{Name: "Gold", Min: 500}}}
	return d
}

// A new player gets the starting values and the chosen hero's twelve
// chests, each opened with the chest.
func TestStarterChests(t *testing.T) {
	Data = starterData()
	defer func() { Data = nil }()
	a := NewAccount()
	a.StartProgression()
	if a.Gold != starterGold || a.Gems != starterGems {
		t.Fatalf("start gold %d gems %d", a.Gold, a.Gems)
	}
	if _, err := a.StartWithHero(13); err != nil {
		t.Fatal(err)
	}
	// one chest of all twelve, as the client makes it (0x687ccc)
	if !a.StarterChests || len(a.Chests) != 1 {
		t.Fatalf("starter chests %v %d", a.StarterChests, len(a.Chests))
	}
	c := a.Chests[0]
	if !c.Show || c.Push || c.Source != sourceStarter || len(c.List) != 12 || c.Content.Fragments[miniID(5)] != 11 || c.Content.Resources["Gold"] != 500 {
		t.Fatalf("chest %+v", c)
	}
	// in the home: the chest list, chest id -1, source 8
	h := a.ownHome()
	h.home.skinStore = false
	m, err := decode(h.encode())
	if err != nil || len(m.home.chests) != 1 || len(m.home.chests[0].List) != 12 || m.home.chests[0].Chest != -1 || m.home.chests[0].Source != sourceStarter {
		t.Fatalf("%v home chests %+v", err, m.home.chests)
	}
	// a new pick replaces it
	if _, err := a.StartWithHero(12); err != nil || len(a.Chests) != 1 {
		t.Fatalf("again: %v %d", err, len(a.Chests))
	}
	gold := a.Gold
	for len(a.Chests) > 0 {
		a.OpenPending(a.Chests[0].Index)
	}
	if a.Gold < gold+500 {
		t.Fatalf("gold %d", a.Gold)
	}
	// nothing to catch up
	if a.catchUpStarter(&Reward{}) {
		t.Fatal("caught up again")
	}
}

// An account from before gets them once, with the gold and gems the start
// lacked.
func TestStarterCatchUp(t *testing.T) {
	Data = starterData()
	defer func() { Data = nil }()
	a := NewAccount()
	a.StartProgression()
	a.TutorialPending = false
	a.Heroes[0] = 13
	a.entry(heroID(13)).Level = 1
	a.StarterChests = false
	a.Chests = nil
	a.Gold, a.Gems = 50, 20
	if !a.catchUpStarter(&Reward{}) || len(a.Chests) != 1 || a.Chests[0].List[0] != 127_000_067 {
		t.Fatalf("catch up: %d chests", len(a.Chests))
	}
	if a.Gold != 50+starterGold-oldStarterGold || a.Gems != 20+starterGems {
		t.Fatalf("gold %d gems %d", a.Gold, a.Gems)
	}
	if a.catchUpStarter(&Reward{}) {
		t.Fatal("twice")
	}
	// still in the tutorial: not yet
	b := NewAccount()
	b.StartProgression()
	if b.catchUpStarter(&Reward{}) {
		t.Fatal("during the tutorial")
	}
}
