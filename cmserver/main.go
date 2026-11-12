// Command cmserver runs the local Clash Mini 1.1143.2 server on a PC or in
// Termux. The same server also runs inside the game (cmd/libcmserver).
package main

import (
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"time"

	"cmserver/home"
	"cmserver/srv"
)

func main() {
	addr := flag.String("listen", "0.0.0.0:9339", "listen address")
	keyPath := flag.String("key", "server.key", "server private key (hex); created if missing")
	accPath := flag.String("accounts", "accounts.json", "saved account (name, decks, heroes, skins)")
	trace := flag.Bool("trace", true, "log every frame")
	printPub := flag.Bool("print-pubkey", false, "print the server public key (create the key if needed) and exit")
	battleOnline := flag.Bool("battle-online", true, "hook goes back online 1 s into a bot battle; false keeps it offline and reconnects after it")
	signalEmpty := flag.Bool("signal-empty", false, "send the battle signal without the mode padding (always duel)")
	slotValue := flag.Int("slot-value", 0, "value after each deck slot (0 = what the client writes; 1 empties the battle hand)")
	miniLevel := flag.Int("mini-level", 1, "level written for every owned mini")
	apk := flag.String("apk", "", "your clashmini.apk: battle gold as the end screen shows it (else Bronze values)")
	live := flag.Bool("live", true, "live content: quest settings in the home, quest events (-live=false: none)")
	winFragments := flag.Bool("win-fragments", false, "a won battle also brings fragments (not the original: there battles gave gold only)")
	chests := flag.Bool("chests", true, "rewards come as chests to open in the home (-chests=false: straight into the account)")
	tutorial := flag.Bool("tutorial", true, "new accounts play the game's tutorial (-tutorial=false: skip it)")
	dev := flag.Bool("dev", false, "dev mode: every currency at its top, the road's fuel, every quest done (refilled with every home); saved in accounts-dev.json unless -accounts says otherwise")
	sectorTest := flag.String("sector-test", "", "Mini Room test (docs/MINIROOM.md): a log with the hook's sector dump (or its raw .bin); a duel then gets that SectorState and heartbeats instead of the bot battle")
	sectorInterval := flag.Duration("sector-interval", 100*time.Millisecond, "sector test: time between heartbeats")
	sectorStep := flag.Int("sector-step", 1, "sector test: how much each heartbeat raises the end (the client simulates up to end*10)")
	progression := flag.String("progression", "", "fresh: earn the collection (starts once, then resumes); reset: start over; unlocked: everything unlocked; empty: as saved")
	flag.Parse()

	key, err := srv.LoadOrMakeKey(*keyPath)
	if err != nil {
		log.Fatal(err)
	}
	if *printPub {
		fmt.Println(hex.EncodeToString(key.Pub[:]))
		return
	}
	log.Printf("server public key: %s (patch_apk.py needs it)", hex.EncodeToString(key.Pub[:]))

	home.DeckSlotValue = int32(*slotValue)
	home.MiniLevel = int32(*miniLevel)
	home.Dev = *dev
	if *dev && *accPath == "accounts.json" {
		*accPath = home.AccountFile(true)
	}
	acc, err := home.LoadAccount(*accPath)
	if err != nil {
		log.Fatal(err)
	}
	home.Tutorial = *tutorial
	home.ChestsInHome = *chests
	home.Live = *live
	home.WinFragments = *winFragments
	if *apk != "" {
		d, err := home.LoadGameData(*apk)
		if err != nil {
			log.Fatal(err)
		}
		home.Data = d
	}
	if msg, err := acc.ApplyProgression(*progression); err != nil {
		log.Fatal(err)
	} else if msg != "" {
		log.Printf("%s", msg)
	}
	if msg := acc.CatchUp(); msg != "" {
		log.Printf("%s", msg)
	}
	if home.Dev {
		log.Printf("dev: %s", acc.DevStart())
	}
	log.Printf("account %s: %q, deck %d %v, progression %v", *accPath, acc.Name, acc.Selected,
		acc.Decks[acc.Selected], acc.Progression)

	s := srv.New(key, acc, srv.Options{Trace: *trace, BattleOnline: *battleOnline, SignalEmpty: *signalEmpty,
		SectorFile: *sectorTest, SectorInterval: *sectorInterval, SectorStep: *sectorStep})
	log.Fatal(s.ListenAndServe(*addr))
}
