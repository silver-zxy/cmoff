package srv

import (
	"encoding/hex"
	"fmt"
	"log"

	"cmserver/home"
	"cmserver/proto"
)

// EndClientTurnMessage 17306: VInt tick, VInt checksum, VInt count, then
// count × (VInt type, command body). Every command body starts with the
// LogicCommand base (encoder 0x8f2a40: two VInts, then the account id as two
// VInts); the name command writes its string before the base. Layouts from
// the command encoders (PORTING.md §6b).
const (
	cmdSwapChess      = 500
	cmdChangeName     = 549
	cmdStartMatchmake = 582
	cmdSelectHero     = 583
	cmdAddTroopToDeck = 595
	cmdSelectSkin     = 592
	cmdSelectDeck     = 597
	cmdUnlockCollect  = 602 // LogicUnlockCollectableCommand: int id, vint current level

	cmdChestOpenFinish  = 550 // LogicChestOpenFinishCommand: base, vint chest index
	cmdClaimQuest       = 503 // LogicClaimQuestRewardCommand: base, vint quest id, bool
	cmdRerollQuest      = 502 // LogicRerollQuestCommand: base, vint quest id
	cmdClaimSeason      = 601 // LogicClaimAllSeasonRewardsCommand: base, bool, vint tier
	cmdSeasonSeen       = 505 // LogicUpdateSeasonTierSeenCommand: base, vint season
	cmdBuyChest         = 702 // LogicBuyChestCommand: base, vint event, vint item
	cmdSkipTutorial     = 607
	cmdCompleteTutorial = 988
)

// skipLayouts: commands the server only reads past (so the rest of the batch
// is not lost), from their encoders (vtable slot 5). "b" base, "v" VInt, "i"
// int, "o" bool, "s" string, "d" data reference (0x908850: VInt 0, or class
// and row).
var skipLayouts = map[int32]struct{ name, layout string }{
	504: {"LogicChooseGameModeCommand", "bvo"}, // mode, ranked
	506: {"LogicMarkSurveyLinkSeenCommand", "bv"},
	550: {"LogicChestOpenFinishCommand", "bv"},
	593: {"LogicMetaSelectModifierCommand", "bvvv"},
	594: {"LogicBoardSkinSelectionCommand", "bv"},
	596: {"LogicRandomizeSelectedDeckCommand", "bo"},
	598: {"LogicSetPlayerAgeCommand", "bi"},
	600: {"LogicClaimTrophyRoadRewardCommand", "bvvo"},
	604: {"LogicTutorialSetReqStateHomeCommand", "dvob"}, // tutorial, state, flag
	605: {"LogicTutorialTryRestartCommand", "b"},
	607: {"LogicSkipTutorialCommand", "b"},
	988: {"LogicCompleteTutorialHomeCommand", "db"}, // tutorial
	// the client echoes our name server command: name, changes, set, base
	411: {"LogicChangeAvatarNameCommand", "siovb"},
}

type command struct {
	typ  int32
	args []int32
	name string
	flag bool
}

func (c command) String() string {
	switch c.typ {
	case cmdSwapChess:
		return fmt.Sprintf("SwapChess mini=%d slot=%d slot2=%d", c.args[0], c.args[1], c.args[2])
	case cmdChangeName:
		return fmt.Sprintf("ChangeName %q", c.name)
	case cmdStartMatchmake:
		return fmt.Sprintf("StartMatchmake mode=%d v=%d ranked=%v", c.args[0], c.args[1], c.flag)
	case cmdSelectHero:
		return fmt.Sprintf("SelectSupportHero %d", c.args[0])
	case cmdAddTroopToDeck:
		return fmt.Sprintf("AddTroopToDeck %d add=%v", c.args[0], c.flag)
	case cmdSelectDeck:
		return fmt.Sprintf("SelectDeck %d", c.args[0])
	case cmdSelectSkin:
		return fmt.Sprintf("SelectSkin character=%d skin=%d", c.args[0], c.args[1])
	case cmdUnlockCollect:
		return fmt.Sprintf("UnlockCollectable %d from level %d", c.args[0], c.args[1])
	case cmdRerollQuest:
		return fmt.Sprintf("LogicRerollQuestCommand quest %d", c.args[0])
	case cmdClaimQuest:
		return fmt.Sprintf("LogicClaimQuestRewardCommand quest %d", c.args[0])
	case cmdClaimSeason:
		return fmt.Sprintf("LogicClaimAllSeasonRewardsCommand tier %d", c.args[0])
	case cmdSeasonSeen:
		return fmt.Sprintf("LogicUpdateSeasonTierSeenCommand season %d", c.args[0])
	case cmdBuyChest:
		return fmt.Sprintf("LogicBuyChestCommand event %d item %d", c.args[0], c.args[1])
	case cmdServerUpdateChronosEvents:
		return "LogicUpdateChronosEventsCommand (ours, echoed)"
	case cmdServerClaimReward:
		return fmt.Sprintf("LogicClaimRewardCommand (ours, echoed) chest #%d", c.args[0])
	}
	if l, ok := skipLayouts[c.typ]; ok {
		return fmt.Sprintf("%s %v", l.name, c.args)
	}
	return fmt.Sprintf("cmd %d %v", c.typ, c.args)
}

// skipContent reads past a chest content object (0x3d7a8c: VInt count of
// reward entries, each five VInt lists, a bool and a VInt (0x90c344); a bool).
func skipContent(r *proto.Reader) {
	for n := r.VInt(); n > 0 && r.Err() == nil; n-- {
		for l := 0; l < 5; l++ {
			for k := r.VInt(); k > 0 && r.Err() == nil; k-- {
				r.VInt()
			}
		}
		r.Bool()
		r.VInt()
	}
	r.Bool()
}

func base(r *proto.Reader) {
	r.VInt()
	r.VInt()
	r.VInt()
	r.VInt()
}

// parseEndClientTurn returns the commands it understood. It stops at the first
// unknown type (its length is unknown) and reports the rest.
func parseEndClientTurn(p []byte) (cmds []command, err error) {
	r := proto.NewReader(p)
	r.VInt() // tick
	r.VInt() // checksum
	n := r.VInt()
	for i := int32(0); i < n && r.Err() == nil; i++ {
		c := command{typ: r.VInt()}
		switch c.typ {
		case cmdSwapChess:
			base(r)
			c.args = []int32{r.VInt(), r.VInt(), r.VInt()}
		case cmdChangeName:
			c.name = r.String()
			base(r)
		case cmdStartMatchmake:
			base(r)
			c.args = []int32{r.VInt(), r.VInt()}
			c.flag = r.Bool()
			r.Bool()
		case cmdSelectHero:
			base(r)
			c.args = []int32{r.VInt()}
		case cmdAddTroopToDeck:
			base(r)
			c.args = []int32{r.VInt()}
			c.flag = r.Bool()
		case cmdSelectDeck:
			base(r)
			c.args = []int32{r.VInt()}
		case cmdSelectSkin:
			base(r)
			c.args = []int32{r.VInt(), r.VInt()}
		case cmdRerollQuest:
			// decoder 0x94cef8: base, VInt quest id
			base(r)
			c.args = []int32{r.VInt()}
		case cmdClaimQuest:
			// decoder 0x50429c: base, VInt quest id, bool
			base(r)
			c.args = []int32{r.VInt()}
			r.Bool()
		case cmdClaimSeason:
			// encoder 0x4c59a0: base, Bool (+0x1c), VInt tier (+0x20)
			base(r)
			c.flag = r.Bool()
			c.args = []int32{r.VInt()}
		case cmdSeasonSeen:
			// encoder 0x7ee1c8: base, VInt season key (+0x1c)
			base(r)
			c.args = []int32{r.VInt()}
		case cmdBuyChest:
			// ctor 0x74a208: base, VInt event (+0x1c), VInt item (+0x20)
			base(r)
			c.args = []int32{r.VInt(), r.VInt()}
		case cmdUnlockCollect:
			// encoder 0x6d5ca0: base, writeInt id, writeVInt level
			base(r)
			c.args = []int32{r.Int(), r.VInt()}
		case cmdServerUpdateChronosEvents:
			// our 417 echoed: the bases, a bool, the events
			r.VInt()
			base(r)
			if r.Bool() {
				r.VInt()
				for n := r.VInt(); n > 0 && r.Err() == nil; n-- {
					r.VInt()
					_ = r.String()
					for i := 0; i < 5; i++ {
						r.VInt()
					}
					for i := 0; i < 4; i++ {
						_ = r.String()
					}
					r.VInt()
					r.Bool()
				}
			}
		case cmdServerClaimReward:
			// the client echoes our 420 (encoder 0x37fee8): the content,
			// VInt chest index, the server command base, the base
			skipContent(r)
			c.args = []int32{r.VInt()}
			r.VInt()
			base(r)
		default:
			l, ok := skipLayouts[c.typ]
			if ok {
				for _, k := range l.layout {
					switch k {
					case 'b':
						base(r)
					case 'v':
						c.args = append(c.args, r.VInt())
					case 'i':
						c.args = append(c.args, r.Int())
					case 's':
						c.name = r.String()
					case 'o':
						if r.Bool() {
							c.args = append(c.args, 1)
						} else {
							c.args = append(c.args, 0)
						}
					case 'd':
						class, row := r.DataRef()
						c.args = append(c.args, class, row)
					}
				}
				break
			}
			return cmds, fmt.Errorf("unknown command %d (%d of %d), rest %s",
				c.typ, i+1, n, hex.EncodeToString(p[len(p)-r.Remaining():]))
		}
		if r.Err() != nil {
			break
		}
		cmds = append(cmds, c)
	}
	return cmds, r.Err()
}

// apply runs the account-changing commands and saves. It reports the
// matchmake mode (0 = no matchmake in this batch) and whether it is ranked.
func apply(acc *home.Account, cmds []command) (matchMode int32, ranked bool) {
	changed := false
	acc.Lock()
	for _, c := range cmds {
		log.Printf("   command: %s", c)
		switch c.typ {
		case cmdSwapChess:
			acc.Swap(c.args[0], c.args[1], c.args[2])
		case cmdChangeName:
			acc.SetName(c.name)
		case cmdSelectHero:
			acc.SelectHero(c.args[0])
		case cmdAddTroopToDeck:
			acc.AddTroop(c.args[0], c.flag)
		case cmdSelectDeck:
			acc.SelectDeck(c.args[0])
		case cmdSelectSkin:
			acc.SelectSkin(c.args[1])
		case cmdUnlockCollect:
			r, err := acc.Upgrade(c.args[0], c.args[1])
			if err != nil {
				log.Printf("   upgrade refused: %v", err)
				continue
			}
			lvl, pts := home.ExpLevel(acc.Exp)
			log.Printf("   upgraded: %s -> player level %d (%d exp)", r.String(), lvl, pts)
		case cmdStartMatchmake:
			matchMode, ranked = c.args[0], c.flag
			continue
		case cmdRerollQuest:
			from, to, ok := acc.RerollQuest(c.args[0])
			if !ok {
				log.Printf("   quest %d rerolled, but the server could not (no such quest, no reroll left or no quest left in the pool)", c.args[0])
				continue
			}
			log.Printf("   quest %d rerolled: %s -> %s", c.args[0], from, to)
		case cmdClaimQuest:
			rw := &home.Reward{}
			if !acc.ClaimQuest(rw, c.args[0]) {
				log.Printf("   quest %d claimed, but the server has no finished quest with that id", c.args[0])
				continue
			}
			log.Printf("   quest %d claimed: %s", c.args[0], rw.String())
		case cmdClaimSeason:
			rw := &home.Reward{}
			n := acc.ClaimSeasonRewards(rw)
			if n == 0 {
				log.Printf("   adventure road claimed, but the server has no tier to give")
				continue
			}
			k, pts, claimed := acc.SeasonSummary()
			log.Printf("   adventure road: %d tiers claimed (season %d, %d fuel, up to %d)", n, k, pts, claimed)
		case cmdSeasonSeen:
			acc.SeeSeasonTier()
		case cmdBuyChest:
			rw := &home.Reward{}
			chest, why := acc.BuyShopItem(rw, c.args[0], c.args[1])
			if chest == 0 {
				log.Printf("   shop: item %d bought, but the server refused: %s", c.args[1], why)
				continue
			}
			log.Printf("   shop: item %d bought, chest %d (%d gold, %d gems left)", c.args[1], chest, acc.Gold, acc.Gems)
		case cmdChestOpenFinish:
			r, chest, ok := acc.OpenPending(c.args[0])
			if !ok {
				log.Printf("   chest %d opened, but the server has no chest with that index", c.args[0])
				continue
			}
			log.Printf("   chest %d (index %d) opened: %s", chest, c.args[0], r.String())
		case cmdSkipTutorial, cmdCompleteTutorial:
			if !acc.TutorialPending {
				continue
			}
			acc.TutorialPending = false
			log.Printf("   tutorial done")
		default:
			continue // read past, nothing to save
		}
		changed = true
	}
	acc.Unlock()
	if changed {
		if err := acc.Save(); err != nil {
			log.Printf("save account: %v", err)
		} else {
			log.Printf("   account saved: deck %d %v hero %v name %q",
				acc.Selected, acc.Decks[acc.Selected], acc.Heroes[acc.Selected], acc.Name)
		}
	}
	return matchMode, ranked
}
