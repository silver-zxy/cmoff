package home

import "cmserver/proto"

// Skins. Owning a skin is NOT the collectible level: the select-skin command
// 592 (execute 0x8effbc) asks the home's skin store (+0x1c0, encoder
// 0x7295d8, check 0x91dcd8) whether the mini or hero has that skin. The store
// is a list of entries (encoder 0x72c81c): VInt owner (the mini's
// seasonal_character_mapping or the hero's support_heroes global id, the
// character's +0x250 data), VInt selected skin (+0x18, set by 592), VInt
// count, count × VInt skin global id (class 44, skin_confs). A character's
// default skin is always allowed.

type skinInfo struct {
	row   int32 // skin_confs row
	owner int32 // collectible id of the mini (36) or hero (107)
	extra bool  // a real extra skin (else the default "lv1" skin)
}

// From skin_confs.csv: the real extra skins and the default skins, with the
// owner found through the Character column (seasonal Season0 / hero unit).
var skinTable = []skinInfo{
	{0, 36000004, false},   // Archerlv1 (ArcherB1)
	{2, 36000001, false},   // Barbarianlv1 (BarbarianB1)
	{4, 36000018, false},   // Valkyrielv1 (ValkyrieB1)
	{5, 36000007, false},   // Musketeerlv1 (MusketeerB1)
	{7, 36000005, false},   // Speargoblinlv1 (SpearGoblinB1)
	{9, 36000013, false},   // Dartgoblinlv1 (BlowdartGoblinB1)
	{18, 36000003, false},  // Minerlv1 (MinerB1)
	{19, 36000019, false},  // EliteArcherlv1 (EliteArcherB1)
	{24, 36000000, false},  // Knightlv1 (KnightB1)
	{25, 36000014, false},  // Bowlerlv1 (BowlerB1)
	{27, 36000010, false},  // Lumberjacklv1 (LumberjackB1)
	{28, 36000023, false},  // Skeletonguardlv1 (ShieldSkeletonB1)
	{29, 36000009, false},  // GiantSkeletonlv1 (GiantSkeletonB1)
	{30, 36000022, false},  // ElectroWizardlv1 (ElectroWizardB1)
	{31, 36000008, false},  // IceWizardlv1 (IceWizardB1)
	{32, 36000017, false},  // Pekkalv1 (PekkaB1)
	{35, 36000017, true},   // PekkaSkin_Crystal (PekkaB1)
	{36, 36000012, false},  // MiniPekkalv1 (MiniPekkaB1)
	{39, 36000006, false},  // Wizardlv1 (WizardB1)
	{40, 36000015, false},  // Princelv1 (PrinceB1)
	{42, 36000021, false},  // MegaKnightlv1 (MegaKnightB1)
	{46, 107000014, false}, // Monklvl1 (HeroMonk)
	{47, 107000015, false}, // ShieldMaidenlv1 (HeroShieldMaiden)
	{51, 107000011, false}, // VamplireGirllv1 (HeroVampireGirl)
	{52, 107000012, false}, // BarbarianKinglv1 (HeroBarbarian)
	{53, 107000013, false}, // ArcherQueenlv1 (HeroArcher)
	{54, 107000017, false}, // heroroyalchampionlv1 (HeroRoyalChampion)
	{59, 36000019, true},   // MagicArcher_Celtic (EliteArcherB1)
	{65, 36000022, true},   // ElectroWizard_Scientist (ElectroWizardB1)
	{70, 36000003, true},   // Miner_Gravedigger (MinerB1)
	{73, 36000017, true},   // Pekka_Plushy (PekkaB1)
	{74, 36000020, false},  // Battlehealerlv1 (BattleHealerB1)
	{75, 36000016, false},  // HealingRangerlv1 (AngelB1)
	{80, 107000015, true},  // shieldmaiden_Victory (HeroShieldMaiden)
	{81, 107000014, true},  // monk_Luchador (HeroMonk)
	{89, 36000015, true},   // Prince_Fabulous (PrinceB1)
	{90, 107000012, true},  // BarbarianKing_Jolly (HeroBarbarian)
	{91, 36000012, true},   // MiniPekka_Helpful (MiniPekkaB1)
	{92, 36000009, true},   // GiantSkeleton_Gift (GiantSkeletonB1)
	{93, 36000005, true},   // SpearGoblin_CandyElf (SpearGoblinB1)
	{94, 36000014, true},   // Bowler_Snowler (BowlerB1)
	{95, 36000004, true},   // Archer_Sparkle (ArcherB1)
}

// SkinOwner returns the owner of a skin (global id) and whether it is one of
// the extra skins. ok is false for unknown skins.
func SkinOwner(skinGlobal int32) (owner int32, extra, ok bool) {
	if skinGlobal/1_000_000 != classSkin {
		return 0, false, false
	}
	row := skinGlobal % 1_000_000
	for _, s := range skinTable {
		if s.row == row {
			return s.owner, s.extra, true
		}
	}
	return 0, false, false
}

// encodeSkinStore writes home +0x1c0. Per owner the skin list holds the
// default skin first, then the extra skins; the entry's +0x18 is the INDEX of
// the worn skin in that list (0x8d90d8 stores the index; choosing the default
// skin appends it to the list first, 0x8fcff8). sel maps owner -> chosen skin
// global id; missing or unknown = default (index 0). owned: the extra
// skins the player has (global ids); nil = all of them (the unlocked
// collection without progression).
func encodeSkinStore(w *proto.Writer, sel map[int32]int32, owned map[int32]bool) {
	var owners []int32
	skins := map[int32][]int32{}
	for _, s := range skinTable {
		if !s.extra || (owned != nil && !owned[globalID(classSkin, s.row)]) {
			continue
		}
		if _, seen := skins[s.owner]; !seen {
			owners = append(owners, s.owner)
			skins[s.owner] = []int32{defaultSkin(s.owner)}
		}
		skins[s.owner] = append(skins[s.owner], globalID(classSkin, s.row))
	}
	w.VInt(int32(len(owners)))
	for _, o := range owners {
		list := skins[o]
		idx := int32(0)
		for i, g := range list {
			if g == sel[o] {
				idx = int32(i)
			}
		}
		w.VInt(o)
		w.VInt(idx)
		w.VInt(int32(len(list)))
		for _, g := range list {
			w.VInt(g)
		}
	}
}

// defaultSkin is the owner's default (skin_confs "lv1") skin global id.
func defaultSkin(owner int32) int32 {
	for _, s := range skinTable {
		if s.owner == owner && !s.extra {
			return globalID(classSkin, s.row)
		}
	}
	return 0
}

// Skins as in the original (variant B, 0088): every account, the existing
// ones too, starts with only the default skins; an extra skin is owned once
// a chest gives it (the road's Plush P.E.K.K.A and Luchador Monk, the
// Clashmas quests' Candy Elf, the shop's skins). A skin owned is never lost.

// betaSkins: the older skins (skin_confs rows: Celtic Magic Archer 59,
// Scientist Electro Wizard 65, Gravedigger Miner 70, Victory Shield
// Maiden 80). They have shop chests (shop_skin_*), so they were sold
// before December 2021, but how and for what is not known: everyone owns
// them (decided, 0095).
var betaSkins = []int32{59, 65, 70, 80}

func isBetaSkin(skin int32) bool {
	for _, r := range betaSkins {
		if skin == globalID(classSkin, r) {
			return true
		}
	}
	return false
}

// ownSkin: a chest gave a skin (global id, class 44).
func (a *Account) ownSkin(skin int32) {
	for _, s := range a.OwnedSkins {
		if s == skin {
			return
		}
	}
	a.OwnedSkins = append(a.OwnedSkins, skin)
}

// ownsSkin: the player has the extra skin (all of them without progression).
func (a *Account) ownsSkin(skin int32) bool {
	if !a.Progression || isBetaSkin(skin) {
		return true
	}
	for _, s := range a.OwnedSkins {
		if s == skin {
			return true
		}
	}
	return false
}

// ownedSkinSet: the owned extra skins for the home; nil = all.
func (a *Account) ownedSkinSet() map[int32]bool {
	if !a.Progression {
		return nil
	}
	m := map[int32]bool{}
	for _, s := range a.OwnedSkins {
		m[s] = true
	}
	for _, r := range betaSkins {
		m[globalID(classSkin, r)] = true
	}
	return m
}

// lockSkins: once per account, the skins as in the original: a locked
// skin worn goes back to the default. Returns whether it changed the
// account.
func (a *Account) lockSkins() bool {
	if a.SkinsLocked || !a.Progression {
		return false
	}
	a.SkinsLocked = true
	for owner, skin := range a.Skins {
		if !a.ownsSkin(skin) {
			delete(a.Skins, owner)
		}
	}
	return true
}
