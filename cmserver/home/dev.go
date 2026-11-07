package home

import "fmt"

// Dev mode (build option DEV=on, server option dev=on / -dev): an account
// of its own (accounts-dev.json, so the real one stays as it is) with
// everything at hand to test: every currency at its top, the Adventure
// Road's fuel for all its tiers (to claim one by one), every open quest
// done (to claim: daily, mini, Clashmas). Refilled with every home. The
// season ends at every start (seasonend.go), with the trophies set to
// Diamond 1 (devTrophies) just before, so the reset can be seen.
// Skins, units and trophies are earned as usual, so unlocking and upgrading
// stay testable (the gold, gems and coins buy and upgrade everything).
var Dev bool

const (
	devGold  int32 = 999_999
	devGems  int32 = 99_999
	devCoins       = maxCrystalCoins
)

// devTopUp fills the account as dev mode wants it. The caller holds the
// lock.
func (a *Account) devTopUp() {
	if !Dev || !a.Progression {
		return
	}
	a.Gold, a.Gems, a.CrystalCoins = devGold, devGems, devCoins
	if n := len(seasonTiers); n > 0 {
		s := a.season()
		s.Points = max(s.Points, seasonTiers[n-1].Points)
	}
	a.refreshQuests()
	pool := todaysQuests()
	for i := range a.Quests {
		if q := &a.Quests[i]; int(q.Index) < len(pool) {
			q.Progress = pool[q.Index].Target
		}
	}
	a.refreshMiniQuests()
	chain := miniChain()
	for i := range a.MiniQuests {
		if q := &a.MiniQuests[i]; int(q.Index) < len(chain) {
			q.Progress = chain[q.Index].Target
		}
	}
	a.refreshClashmas()
	list := clashmasQuests()
	for i := range a.Clashmas.Quests {
		if q := &a.Clashmas.Quests[i]; int(q.Index) < len(list) {
			q.Progress = list[q.Index].def.Target
		}
	}
}

// AccountFile: where the account is saved; dev mode has its own.
func AccountFile(dev bool) string {
	if dev {
		return "accounts-dev.json"
	}
	return "accounts.json"
}

// devTrophies: Diamond 1 (900-999); the season's end takes it to 800.
const devTrophies int32 = 950

// DevStart: dev mode at the server's start: the trophies to Diamond 1,
// then the season ends; not while chests with a popup wait in the home or
// a season's end is still to come (they piled up, 0101).
func (a *Account) DevStart() string {
	a.Lock()
	if n := a.popupChests(); n > 0 || a.SeasonEndDue {
		a.Unlock()
		return fmt.Sprintf("no season end: %d chests to open first, 451 due %v", n, a.SeasonEndDue)
	}
	if a.Progression {
		a.Trophies = max(a.Trophies, devTrophies)
		if a.BestTrophies < a.Trophies {
			a.BestTrophies = a.Trophies
		}
	}
	a.Unlock()
	return a.EndSeasonNow()
}
