package srv

import (
	"encoding/hex"
	"encoding/json"
	"path/filepath"
	"testing"

	"cmserver/home"
	"cmserver/proto"
)

// Batches captured from the phone (1.1143.2), decrypted by the server.
func TestParseCapturedTurns(t *testing.T) {
	cases := []struct {
		hex  string
		want string
	}{
		{"aa020000ffffffff01", ""},
		{"ab0200018609aa02aa020100027f00ffffffff01", "StartMatchmake mode=2 v=-1 ranked=false"},
		{"bb0300018709ba03ba0301008fc38566ffffffff01", "SelectSupportHero 107000015"},
		{"800500018709bf04bf0401008ec38566ffffffff01", "SelectSupportHero 107000014"},
		{"ab0700019009aa07aa07010080b2b620988cfb29ffffffff01", "SelectSkin character=34000000 skin=44000024"},
		// 411 echoed by the client after a rename (from a log)
		{"aa0200019b0600000006506c6179657200000000010024240100ffffffff01", "LogicChangeAvatarNameCommand [0 1 0]"},
		// 504 from a log: base, mode 2, ranked
		{"aa020001b807b106b10601000201ffffffff01", "LogicChooseGameModeCommand [2 1]"},
	}
	for _, c := range cases {
		b, _ := hex.DecodeString(c.hex)
		cmds, err := parseEndClientTurn(b)
		if err != nil {
			t.Fatalf("%s: %v", c.hex, err)
		}
		got := ""
		if len(cmds) > 0 {
			got = cmds[0].String()
		}
		if got != c.want {
			t.Errorf("%s: got %q want %q", c.hex, got, c.want)
		}
	}
}

func turn(build func(w *proto.Writer)) []byte {
	w := proto.NewWriter()
	w.VInt(100) // tick
	w.VInt(0)   // checksum
	build(w)
	w.Int(-1)
	w.Byte(1)
	return w.Bytes()
}

func baseW(w *proto.Writer) { w.VInt(100); w.VInt(100); w.VInt(1); w.VInt(0) }

func TestApplyAndPersist(t *testing.T) {
	path := filepath.Join(t.TempDir(), "accounts.json")
	acc, err := home.LoadAccount(path)
	if err != nil {
		t.Fatal(err)
	}
	b := turn(func(w *proto.Writer) {
		w.VInt(4)
		w.VInt(cmdChangeName)
		w.String("Bob")
		baseW(w)
		w.VInt(cmdSelectHero)
		baseW(w)
		w.VInt(107000015)
		w.VInt(cmdSwapChess)
		baseW(w)
		w.VInt(9) // mini row 9 into slot 2
		w.VInt(2)
		w.VInt(-1)
		w.VInt(cmdStartMatchmake)
		baseW(w)
		w.VInt(2)
		w.VInt(-1)
		w.Bool(false)
		w.Bool(false)
	})
	cmds, err := parseEndClientTurn(b)
	if err != nil || len(cmds) != 4 {
		t.Fatalf("parse: %v %v", cmds, err)
	}
	if mode, _ := apply(acc, cmds); mode != 2 {
		t.Fatalf("mode %d", mode)
	}
	again, err := home.LoadAccount(path)
	if err != nil {
		t.Fatal(err)
	}
	if again.Name != "Bob" || again.Heroes[0] != 15 || again.Decks[0] != [5]int32{0, 1, 9, 5, 6} {
		t.Fatalf("not persisted: %+v", again)
	}
	again.SelectSkin(44000095) // Archer_Sparkle
	if again.Skins[36000004] != 44000095 {
		t.Fatalf("skin: %v", again.Skins)
	}
	again.SelectSkin(44000000) // Archerlv1, the default
	if len(again.Skins) != 0 || !again.NameSet {
		t.Fatalf("skin reset / name_set: %v %v", again.Skins, again.NameSet)
	}
	if len(again.OwnHomeData()) < 400 {
		t.Fatal("home too short")
	}
}

func TestUnlockCollectableParse(t *testing.T) {
	w := proto.NewWriter()
	w.VInt(0) // tick
	w.VInt(0) // checksum
	w.VInt(1)
	w.VInt(cmdUnlockCollect)
	for i := 0; i < 4; i++ {
		w.VInt(0)
	}
	w.Int(107000012)
	w.VInt(1)
	cmds, err := parseEndClientTurn(w.Bytes())
	if err != nil || len(cmds) != 1 || cmds[0].args[0] != 107000012 || cmds[0].args[1] != 1 {
		t.Fatalf("%v %v", cmds, err)
	}
}

func TestBattleSignalLen(t *testing.T) {
	n := BattleSignalLen(3, false, 13, 10, 4, 3)
	if n%4 != 3 || n/4%2 != 0 || n/8%16 != 13 || n/128%11 != 10 || n/1408%5 != 4 || n/7040%8 != 3 {
		t.Fatalf("%d does not decode", n)
	}
	if BattleSignalLen(2, true, 0, 0, 0, 0) != 6 {
		t.Fatal("hold")
	}
}

func TestTutorialDone(t *testing.T) {
	acc := home.NewAccount()
	b := turn(func(w *proto.Writer) {
		w.VInt(1)
		w.VInt(cmdCompleteTutorial)
		w.VInt(0) // no tutorial reference
		baseW(w)
	})
	cmds, err := parseEndClientTurn(b)
	if err != nil || len(cmds) != 1 {
		t.Fatalf("%v %v", cmds, err)
	}
	if !acc.TutorialPending {
		t.Fatal("new account without tutorial")
	}
	apply(acc, cmds)
	if acc.TutorialPending {
		t.Fatal("tutorial still pending")
	}
}

// A win's chest waits in the home until the client finishes opening it (550).
func TestChestOpenFinish(t *testing.T) {
	acc, err := home.LoadAccount(filepath.Join(t.TempDir(), "accounts.json"))
	if err != nil {
		t.Fatal(err)
	}
	acc.SetProgression("fresh")
	home.WinFragments = true
	defer func() { home.WinFragments = false }()
	acc.BattleResult(home.ModeDuel, 1, false)
	if len(acc.Chests) != 1 {
		t.Fatalf("chests %+v", acc.Chests)
	}
	idx := acc.Chests[0].Index
	b := turn(func(w *proto.Writer) {
		w.VInt(1)
		w.VInt(550)
		baseW(w)
		w.VInt(idx)
	})
	cmds, err := parseEndClientTurn(b)
	if err != nil || len(cmds) != 1 {
		t.Fatalf("%v %v", cmds, err)
	}
	apply(acc, cmds)
	for _, c := range acc.Chests {
		if c.Index == idx {
			t.Fatalf("chest not opened: %+v", acc.Chests)
		}
	}
}

// 420 as the client decodes it (0x4dad24): content, index, server base,
// LogicCommand base.
func TestClaimRewardServerCommand(t *testing.T) {
	ch := home.PendingChest{Index: 7, Chest: 127000059,
		Content: home.ChestContent{Resources: map[string]int32{"ExpPoints": 1}}}
	b := claimRewardServerCommand(ch)
	r := proto.NewReader(b)
	if r.VInt() != 420 || r.VInt() != 1 { // type, one reward entry
		t.Fatalf("% x", b)
	}
	ids := []int32{}
	for n := r.VInt(); n > 0; n-- {
		ids = append(ids, r.VInt())
	}
	counts := []int32{}
	for n := r.VInt(); n > 0; n-- {
		counts = append(counts, r.VInt())
	}
	for i := 0; i < 3; i++ { // fragment ids, counts, +0x48
		for n := r.VInt(); n > 0; n-- {
			r.VInt()
		}
	}
	r.Bool()
	r.VInt()
	r.Bool() // content +0x18 (show)
	if len(ids) != 1 || ids[0] != 5000011 || counts[0] != 1 {
		t.Fatalf("resources %v %v", ids, counts)
	}
	if idx := r.VInt(); idx != 7 {
		t.Fatalf("index %d", idx)
	}
	for i := 0; i < 5; i++ {
		r.VInt()
	}
	if r.Err() != nil || r.Remaining() != 0 {
		t.Fatalf("err %v, %d left", r.Err(), r.Remaining())
	}
}

func TestClaimRewardEchoKeepsTheBatch(t *testing.T) {
	// from a log: our 420 echoed, then 550 for the same chest, which was lost
	rest, _ := hex.DecodeString("01018bade20401010000000000001600bb02bb020100a608bb02bb02010016ffffffff01")
	w := proto.NewWriter()
	w.VInt(100)
	w.VInt(0)
	w.VInt(2)
	w.VInt(420)
	w.RawAppend(rest)
	cmds, err := parseEndClientTurn(w.Bytes())
	if err != nil || len(cmds) != 2 || cmds[0].String() != "LogicClaimRewardCommand (ours, echoed) chest #22" ||
		cmds[1].typ != cmdChestOpenFinish || cmds[1].args[0] != 22 {
		t.Fatalf("%v %v", cmds, err)
	}
}

// Our 417 as the client reads it, and its echo read past.
func TestChronosEventsServerCommand(t *testing.T) {
	events := home.LiveEvents()
	if len(events) == 0 || !json.Valid([]byte(events[0].Parameters)) {
		t.Fatalf("events %+v", events)
	}
	b := chronosEventsServerCommand(events)
	w := proto.NewWriter()
	w.VInt(100)
	w.VInt(0)
	w.VInt(2)
	w.RawAppend(b) // type + body, as echoed
	w.VInt(550)
	baseW(w)
	w.VInt(9)
	cmds, err := parseEndClientTurn(w.Bytes())
	if err != nil || len(cmds) != 2 || cmds[1].typ != cmdChestOpenFinish || cmds[1].args[0] != 9 {
		t.Fatalf("%v %v", cmds, err)
	}
}

// 601 (base, bool, tier) claims the road's tiers reached; 505 (base,
// season) is read and keeps the tier seen.
func TestClaimSeason(t *testing.T) {
	acc, err := home.LoadAccount(filepath.Join(t.TempDir(), "accounts.json"))
	if err != nil {
		t.Fatal(err)
	}
	acc.SetProgression("fresh")
	key, _, _ := acc.SeasonSummary()
	acc.Season.Points = 7
	b := turn(func(w *proto.Writer) {
		w.VInt(2)
		w.VInt(601)
		baseW(w)
		w.Bool(false)
		w.VInt(1)
		w.VInt(505)
		baseW(w)
		w.VInt(key)
	})
	cmds, err := parseEndClientTurn(b)
	if err != nil || len(cmds) != 2 || cmds[0].typ != cmdClaimSeason || cmds[1].typ != cmdSeasonSeen || cmds[1].args[0] != key {
		t.Fatalf("%v %v", cmds, err)
	}
	apply(acc, cmds)
	if _, pts, claimed := acc.SeasonSummary(); pts != 7 || claimed != 7 || acc.Season.PaidClaimed != 7 || acc.Season.Seen != 7 {
		t.Fatalf("season %+v", acc.Season)
	}
}

// 702 (base, event, item) buys from today's shop.
func TestBuyChest(t *testing.T) {
	acc, err := home.LoadAccount(filepath.Join(t.TempDir(), "accounts.json"))
	if err != nil {
		t.Fatal(err)
	}
	acc.SetProgression("fresh")
	acc.Gold = 500
	ev := home.LiveEvents()[2]
	b := turn(func(w *proto.Writer) {
		w.VInt(1)
		w.VInt(702)
		baseW(w)
		w.VInt(ev.ID)
		w.VInt(0)
	})
	cmds, err := parseEndClientTurn(b)
	if err != nil || len(cmds) != 1 || cmds[0].typ != cmdBuyChest || cmds[0].args[0] != ev.ID {
		t.Fatalf("%v %v", cmds, err)
	}
	apply(acc, cmds)
	// without game data there are no mini offers: item 0 is a hero's 25
	// fragments for 100 gems
	if acc.Gold != 500 || acc.Gems != 0 || len(acc.Purchases) != 1 {
		t.Fatalf("gold %d gems %d purchases %+v", acc.Gold, acc.Gems, acc.Purchases)
	}
}
