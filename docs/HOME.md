# OwnHomeData and the account

`OwnHomeDataMessage` 25091 (encoder 0x94f060, decoder 0x48b264) is
`LogicClientHome ‖ LogicClientAvatar ‖ 4 vints`. `cmserver/home/ownhome.go`
mirrors the client's own encoders write by write (ported from .gnoks's
`own_home.rs`; `home/diff_test.go` checks it byte-identical against his
known-good 371-byte body). An empty home is 199 bytes; our account is ~760.

## Byte stream (Titan)

- **VInt**: first byte 6 data bits + sign bit 0x40 + continue bit 0x80, then 7
  bits per byte. **Int** big-endian. **String**: int length (-1 = null) + bytes.
- **Bool**: packed LSB-first, 8 per byte; a run continues across sub-encoders
  until the next non-bool write.
- **LogicLong**: two ints (low, high) or two vints (0x9052f4).
- **Data reference**: row reference = vint(row + 1), 0 = none (0x37cc70);
  or (class, instance) vints, null = single vint 0 (0x8e9860).
- Stream vtable slots: 0x20/0x28/0x38 string, 0x40 bool, 0x48 int, 0x80 vint,
  0x90/0x98 long; readers 0xd0 bool, 0xd8 int, 0xf8 vint, 0x110 long.

Global data id = class × 1 000 000 + row. Classes used: 5 resources,
34 characters, 36 seasonal_character_mapping (the minis), 44 skin_confs,
107 support_heroes.

## LogicClientHome (encoder 0x596f24, decoder 0x3d4c94)

What the server fills (everything else is written empty, see `ownhome.go`):

| Offset | Content | Notes |
|---|---|---|
| +0x18 | home id | long |
| +0x28 | owned minis: (characters row, value) | 22 minis; value 1 (`-mini-level`) |
| +0x38 | current deck: 5 slots, each (seasonal row, **value**) | **value must be 0** — what the client writes itself (0x7a0d1c). 1 made the battle hand empty after five deploys (`TID_BATTLE_DECK_EMPTY`) |
| +0x90 | hero per saved deck | hero global ids |
| +0x80 | 3 saved decks × 5 | seasonal global ids, -1 = empty |
| +0xa0 | selected deck | 0–2 |
| +0x20 | owned heroes (row refs) **+ selected hero** | the vint after the list (hero object +0x10) is the **worn hero's global id**; 0 made the client fall back to the first hero. Set by 583 (0x5a6888 → 0x7561b4 → 0x933fb8) |
| +0x1c0 | **skin store** | see below |
| +0x200, +0x228, +0x250 | three (key, value) maps | the third = collectible levels: key = collectible id, value = level (minis 4, heroes 10). Without an entry a mini shows locked |

### Skin store (+0x1c0, encoder 0x7295d8, entry 0x72c81c, decoder 0x970210/0x524264)

A list of entries, one per mini/hero that has skins:

    vint owner        mini or hero global id (36xxxxxx / 107xxxxxx), found via
                      character +0x250 or 0x4db698
    vint selected     INDEX into the list below (entry +0x18)
    vint count
    count × vint      skin global ids (44xxxxxx)

- Ownership check for equipping and for the lock icon: 0x91dcd8 (the
  character's default skin is always allowed, then 0x50ad0c searches the list).
- 592 stores the index of the chosen skin (0x8d90d8); choosing the default
  appends the default skin to the list first (0x8fcff8).
- So the server writes **default skin first, then the extra skins**, and the
  index of the saved choice (0 = default). Writing 0 with only extra skins
  forced the extra skin; writing a skin id (not an index) always showed default.

The 13 real skins (rows that exist in `skin_confs.csv`; the other skins of
`collectibles.json` were stripped from this build) and every default skin are
in `home/skins.go`. Skins are also written as collectibles (level 1).

## LogicClientAvatar (encoder 0x5cb380)

| Field | We send |
|---|---|
| 3 LogicLongs (vint pairs) | account id, home id (1), 0 |
| name | the saved name |
| name changes (+0x138) | always 0 (renaming stays free) |
| trophies (+0x70 "score"), +0x74 "lastScore", +0x34 "seasonBestScore" | `trophies`, `trophies`, `best_trophies` (RankPoints; profile 0x305dd4, league lookup 0x483e40) |
| 5 int pairs, written as VInt deltas (0x720dac): +0x100 battleCounts, +0x108 battleWins, +0x110 battleLosses, +0x118 ?, +0x120 battleRankTopCounts | `stats` — [0] duel, [1] Rumble; the profile's battle_status (0x49cd10) shows counts, wins and Rumble 1st places |
| +0x128 allTypeGameWinLoseStreak, +0x12c promoteGameWinLoseStreak | `stats.streak`, `stats.ranked_streak` (> 0 wins, < 0 losses in a row; the league screen shows +0x12c) |
| exp points (+0xc0), exp level (+0xbc) | from `exp` with exp_levels.csv: points are within the level (0x8363a0) |
| 8 data-slot lists | slot 2: resources (shown as 0; shop is not server-driven here) |
| tutorial flags (+0x130) | 0x100: skip the tutorial battle |
| name set by user (+0x135) | the account's `name_set`; false shows the first-start name dialog |

## Progression (`home/collection.go`)

With `progression` on, the account owns only what it earned:

| Where | What |
|---|---|
| home minis list (+0x28), heroes list (+0x20) | owned minis (characters row) and heroes only |
| collectible levels (+0x250 map) | owned minis 1–4, heroes 1–10; skins always 1 |
| fragments (+0x228 map) | fragments per collectible id |
| avatar data slot list 0 | `(resources, Gold)` = gold; the HUD reads it there (0x598d98). List 2, where we used to put gold, is not the resources list |
| avatar +0xc4 | gems (HUD, 0x4d24a0); not used |

The collection object at home+0x200 holds both maps (apply 0x7eadc4 looks
fragments up at its +0x28, levels at its +0x50). Upgrade costs per current
level, from collectibles.json (the same for every mini, every hero): minis
1, 1, 2, 3 fragments; heroes 10, 10, 25, 50, 75, 100, 150, 200, 250, 300
fragments and 0, 100, 200, 400, 800, 1200, 1600, 2000, 2500, 3000 gold. The
client checks and pays itself and sends command 602; the server repeats it.
A hero is unlocked by the client itself once it has the fragments (no
command); its first 602 then names level 1 while the server still has 0, so
the server pays and takes the missing levels first.

A brand-new account (no accounts.json, or `reset`) starts without a name, so
the name dialog appears, and it plays the game's tutorial (`TUTORIAL=off`
skips it): tutorial flags 0 instead of 0x100 (CLIENT_HOOK.md "Tutorial"). At
its end the player picks a starting hero, and the starter collection above
gets that hero instead of the Barbarian King (base deck at level 1, no
starting chests).

Chests are opened from chests.json (`home/chests.go`): resources (Gold,
Diamonds, ExpPoints) with their tier bonus and spread; fragment picks
(fragTier by weight), each pick max(1, total / picks) fragments to an entry
of fragList, every entry once when there are as many picks as entries;
collectiblePicks owned at level 1. Minis and heroes whose fragments cover the
unlock (and need no gold) are unlocked, as the client does it itself.

Start: Knight, Barbarian, Archer, Spear Goblin, Wizard and the Barbarian
King at level 1, 100 gold. The economy follows the game data
(`home/economy.go`), read at runtime from the player's own APK
(`home/gamedata.go`; in the game via /proc/self/maps, on a PC with `-apk`;
without it the same values for Bronze):

- every finished battle: the gold of the league's rank reward chests
  (leagues.json ChestRewardsForRanking → chests.json), which is also what
  the end screen shows (0x945098), e.g. Bronze duel 12/5, Master 30/10;
  at most 200 gold a day from battles (globals.csv
  DAILY_GOLD_GAIN_LIMIT_FROM_MATCH; the day changes at local midnight). The
  home carries what is left (+0xbc) and the timer to the next day (+0xc4,
  ticks at 20 a second), which the ranked screen shows (0x54a9a4); before,
  both were 0, so the client started a new day on every home (200 left,
  23:59 to go);
- ranked trophies from the same chests' "RewardsCompetitive" (Bronze to
  Gold: duel win +5, loss +1; Rumble +10/+6/+4/+1), plus the win streak
  bonus: from the third ranked win in a row on, every win adds the
  WINSTREAK_BONUS_CHEST "winStreak_Trophy_3" (+3; the end screen compares
  the streak + 1 with WINSTREAK_BONUS_REQUIREMENT 3, 0x9454c4);
- experience only from upgrades: each level after the unlock opens the
  collectible's upgradeRewardChestId, +1 exp (battles give none,
  BATTLE_END_EXP_REWARD 0);
- each player level: its exp_levels.csv ChestWhenReached chest, 100 gold +
  10 gems up to level 9, 250 + 15 to 19, 500 + 20 after. Gems are the
  avatar's +0xc4.

Chests: win fragments, the upgrade exp (+1, collectibleUpgradeReward_EXP_1)
and level chests come as chests in the home's chest manager (+0x1b0) with
their content. The client opens them one after the other in the home (with
a popup where chests.json has showChestOpening: the content's last bool,
0x9443a4; without it the client sends 550 at once and shows nothing, which
is what the first version did; level chests have source 6 and get the
level-up popup) and sends
LogicChestOpenFinishCommand 550 with the chest's index; the server gives the
content then (`home/chests_pending.go`, the format is there). So experience
from an upgrade arrives when its chest is opened, after the next home (a
battle or a restart). After applying a chest the client upgrades by itself,
without a command, whatever the fragments pay for without gold (unlocks and
mini levels; 0x823d1c -> 0x4abb74); the server does the same
(`autoUpgrade`), and at start it catches up an account whose fragments a
version before left unspent. The chests the client makes itself (an upgrade's exp chest: 602 or the
auto-upgrade, 0x7eb554; a level chest when a player level is reached,
0x82ea1c) take the next index (home +0xb0 + 1) and have no content; the
home+8 content source is a stub here (vtable+0x18 = ret). The server
counts those indexes the same way and sends the content right away with
the server command LogicClaimRewardCommand 420 (content, VInt index; execute
0x6a5c38 sets it, 0x3358d4, and opens the chest, 0x313ef4), so the exp,
the level-up popup and its gold and gems come at once. `CHESTS=off` (cmserver `-chests=false`) gives
everything straight away instead.

As in the original, a battle brings gold (and rank points) only:
leagues.json's ChestRewardsForRanking and sources.json's "Battle rewards"
(gameMode_Ranked_1W..5W) hold no fragments. Fragments come from the daily
quests' chests (quest_Daily_Mini: one fragment, every mini weighted 1), and
in the original also from the shop, the season road and the starter chests,
which this server does not have yet. Hero fragments therefore have no
source until the shop exists. `WIN_FRAGMENTS=on` (cmserver
`-win-fragments`) brings back the earlier stand-in: a win adds 2 mini
fragments (Rumble 1st: 4) and in 1 of 4 wins 5 hero fragments, all
collectibles that still need some equally likely.

Modes (`cmserver -progression`, standalone `PROGRESSION=` at build): `fresh`
starts once and then resumes, `unlocked` shows everything (progress stays in
the file), `reset` starts over (once per build). The first fresh start keeps
the old file as `accounts.json.unlocked-backup`.

## Account (`home/account.go`, `accounts.json`)

    { "name", "name_set", "selected_deck", "decks": [[5 seasonal rows]×3],
      "deck_heroes": [hero rows×3], "skins": {owner: skin global id},
      "trophies", "best_trophies", "exp",
      "stats": {"played", "wins", "losses", "firsts": [duel, rumble],
                "streak", "ranked_streak"},
      "progression", "gold", "rewarded_league", "reset_token",
      "collection": {collectible id: {"level", "fragments"}} }

Field names of the avatar in the client's own JSON (0x392f00) name the
fields: score, lastScore, expLevel, expPoints, battleWins, battleLosses,
battleCounts, battleRankTopCounts, the two streaks, seasonBestScore.

The collection itself (all minis, heroes, skins at max) is not stored; every
account has all of it.
