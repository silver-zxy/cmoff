# Reverse-engineering notes

How the rewrite learns the game's rules, what is confirmed, and what is still
assumed.

## Sources and limits

- **Data files** (primary source). `csv_logic/*.csv`, `csv_logic/*.json` and
  `csv_client/*` describe units, abilities, buffs, projectiles, area effects,
  economy, modes, bots, animations, lighting and camera. These are read as-is.
- **Assets.** `sc3d/*.glb` (glTF + `SC_shader` extension), `shader/*.vsh/.fsh`
  (GLSL source), `sc3d/*.ktx`, `sc/*.sc`, FMOD banks.
- **`libg.so`** (ARM64, stripped).
  - Its game strings are encrypted (decrypted by
    `scripts/decrypt_libg.py`, see [String encryption](#string-encryption);
    contents in [STRINGS.md](STRINGS.md)) and it carries an anti-tamper layer.
  - Rules the data does not settle are reconstructed from observed gameplay
    and marked `TODO(RE)` in the code.
- Ghidra 12.1.4 is set up for this (project outside the repo, in
  `~/tools/re`; scripts in `scripts/ghidra/`).
  - Addresses in these notes are Ghidra's (image base 0x100000): file vaddr =
    Ghidra address − 0x100000.
  - In the clear: `.text` (normal ARM64 code, entropy ~6.8) and most of
    `.rodata`. The game strings in `.data` (515 KB of its 2.5 MB) are
    encrypted and decrypted at load time. Load `libg_decrypted.so` from the
    script into Ghidra to see them. Function names
    are stripped, so getters such as `getHitSpeed` 
  - Code is therefore found by its shape: `PatternScan.java` (shift/XOR
    patterns, PRNG constants), `Callers.java` (call sites with their constant
    argument), `Decompile.java`.

## String encryption

Found by emulating the constructors in `.init_array` with Unicorn (the code is
OLLVM-obfuscated: indirect branches through `adr`/`br`, opaque predicates,
mixed boolean-arithmetic). Ghidra addresses (file vaddr + 0x100000).

- The first constructor, `FUN_00313ffc`, decrypts everything in one pass. In
  the emulation no other constructor writes to the encrypted bytes (five later
  ones stop on unmapped reads, unrelated to strings).
- **Key:** 128 bytes at 0x10bd037 (in `.data`, stored in the clear).
  Item byte `i` is XORed with `key[i & 0x7f]`; the key index restarts at 0
  for every item. The NUL terminator is encrypted too.
- **Item table:** entries of three `u32` at 0x10746e8 (in `.data`, in the
  clear). Entry 0 is a seed. Each entry is decoded with the one before it
  (`p` = previous, `c` = current, all arithmetic mod 2³²):
  - pointer = 0x10746e8 + (i32)(`c.w0 − p.w0 − p.w1 − p.w2`)
  - length = `c.w0 + p.w1 − c.w1`
  - `w2` only enters the next entry's pointer.
  The table ends with the first entry whose pointer leaves `.data` (or whose
  length is 0); 15,232 items, 515,027 bytes.
- The constructor also writes junk to 0x1072530 after every byte (a store the
  optimiser cannot remove); it has no meaning.
- `scripts/decrypt_libg.py` reproduces this without emulation. Its output
  matches the emulated `.data` byte for byte, apart from relocation slots and
  that junk word.
- A 1.3 MB block at 0xf05200–0x104e6e0 stays high-entropy after decryption. No
  `adrp` in `.text` and no relocation points into it (only at strings on its
  edges); what it is is unknown.

## Findings from the decrypted strings

Column and key names are looked up by string, so a name that is not among the
decrypted strings is never read by 1.1143.2. Checked against every shipped CSV
header and JSON key:

- **Bot formations are never mirrored by the data.** The code looks up
  `mirrorPossibility`; all 20 `BattleTaskApplyFormation` nodes in
  `bot_ais.json` spell it `mirrorPossiblity`. The lookup therefore misses and
  the code default applies (value not yet read from the code, most likely 0).
  `cm_logic::bot` currently reads the misspelled key and mirrors 50% of the
  time. `BattleTaskUpdateInfo.applyBotConfig` is not read either.
- **Columns the game never reads.** `characters.csv` / `characters_heroes.csv`:
  `AttackAbility2`, `AttackAbilityRate`, `BaseLevelChess`, `ChessLevel`,
  `Tribe`, `SpecialModeOnly`, `StartingBuffStackTagName`, `StartingBuffTagMatch`,
  `ExtraStartingBuffs`, `ExtraStartingBuffsTime`, `StartingAreaEffect`,
  `ShowHealthNumber`, `PercentageDamage`, `HasRotationOnTimeline`,
  `ShieldDiePushback`, `ShieldLostEffect`. `character_buffs.csv`:
  `NormalHitDisable`, `FilterAffectsTransformation`, `FilterInheritLifeDuration`,
  `MarkEffect`, `KickDirection`. `battle_task_rewards.csv`: `HasInstantEffect`,
  `InstantBuffTargetingEnemy`, `InstantAreaEffectObject`. `projectiles.csv`:
  `TargetType`, `BuffTimeIncreasePerLevel`, `ProjectileRadiusY`.
  `area_effect_objects.csv`: `BuffNumber`. `game_modes.csv`: `LocationData`.
  `npcs.csv`: `CardRerollName`. `abilities.json`: `is_special_ability`.
  `character_modifiers.json`: `character`.
  The rewrite reads some of these: `BaseLevelChess` (bot pools and the home
  deck filter in `cm-client/src/home.rs`), `AttackAbility2`/`AttackAbilityRate`
  and `ChessLevel` (parsed in `cm-data`), and
  `InstantBuffTargetingEnemy`/`InstantAreaEffectObject` (`battle/tasks.rs`).
  Whatever they do there is not what the original does.
- Numbered columns are built at run time: `Prize%d` (`tournament_tiers.csv`)
  and `Season` + index (`seasonal_character_mapping.csv`). `Card1`–`Card4` in
  `card_rolls.csv`/`card_rerolls.csv` have no name in the pool at all; they are
  read by column index or not at all (not checked).
- Everything else in the battle tables is read, including all of
  `autochess_economy.csv`, `autochess_buff.csv`, `board_elements.csv`,
  `board_element_rules.csv`, `board_waves.csv`, `bot_configs.csv`,
  `formations.csv`, and every enum value used in `abilities.json`
  (`type`: normal/drag/dash/channel/jump/blink, `target_type`,
  `jump_type`, `kick_dir`, `projectile_dir`, ...).
- Board sub-state names behind `BOARD_SUB_STATE_LEN_*` (`STATE_PREP_IDLE`,
  `STATE_BATTLING_FIGHT`, ...) and the full command list are in
  [STRINGS.md](STRINGS.md). Battles are lockstep: client and server compare
  checksums and ticks (`OutOfSyncMessage`, `Check sum mismatch. Request full
  update.`, `LogicDebugChecksumEncoder`).

## Confirmed from code

- **Network protocol** (framing, Pepper and legacy RC4 crypto, UDP
  per-message cipher, shuffled message ids, handshake layouts): see
  [SERVER.md](SERVER.md).

- **RNG = `LogicRandom`** (`FUN_0081045c` = `rand(max)`, `FUN_007262f0` = the
  bare seed step; 130 call sites):
  `if seed == 0 { seed = -1 }; x ^= x << 13; x ^= x >> 17 (arithmetic); x ^= x << 5;`
  `seed = x; return |x % max|` (signed remainder, then absolute value; 0 for
  `max <= 0`). Ported in `cm_logic::rng`.
- Shuffles are Fisher–Yates from the back: for `n` down to 2,
  `swap(a[n-1], a[rand(n)])` (`FUN_008f573c`).
- One seed is built as `a % 10000 + b % 10000` from two ids of an object
  (`FUN_006c48dc`, before a shuffle in `FUN_008edde8`); which ids is not known.
- **Generated bot decks** (`BOT_DECK_*`). The switch `FUN_006c5b44` names
  them by its string arguments (decrypted): case 0 `BOT_DECK_REAL` (builds
  nothing), 1 `BOT_DECK_NEWBIE` = `FUN_0072a358`, 2 `BOT_DECK_NORMAL` =
  `FUN_003c1fe4`, 3 `BOT_DECK_EXPERT` = `FUN_004a9a94`, 4 `BOT_DECK_RANDOM` =
  `FUN_008f573c`; `NONE` otherwise. `FUN_0077f96c` falls back to RANDOM when a
  deck id has no table row. Ported as `cm_logic::bot::generated_deck`
  (`GeneratedDeck`); `pick_bot` uses it for `BOT_DECK_*` groups.
  - Every builder works on the player the bot is built for (`*param_1`: rng at
    `+0x38`, deck slots at `+0x40`, count `+0x4c`; a slot has the hero at
    `+0x8`, the hero level at `+0x10`, its minis at `+0x18`/`+0x24`, a mini
    entry the level at `+0x8`). In `cm_logic` that is `DeckSlot`.
  - Deck size: the builders stop at a global at `+0x410` and size the list by
    `+0x414`; read as `CUSTOM_CHESS_DECK_SIZE_MIN` / `_MAX` (both 5). Too few
    picks log `Something wrong with deck builder: [%d/%d]`.
  - Minis are resolved through `seasonal_character_mapping.csv` by name and
    season (`FUN_005b29c0` clamps the season; all seasons are equal in
    1.1143.2).
  - **NEWBIE:** a random `starting_choices.csv` row that is enabled
    (`rand(count)`; error `[DECK] createDeckNewBie failed! ...` if none).
    Hero = its `SupportHero`; hero level = that of a random player slot
    (`rand(slots)`). For each chest in its `FirstMatchChestId` list
    (`chests.json` index = id − 127000000), every id of the chest's `fragList`
    is counted (`FUN_0037cdfc`; the counts ignore picks and weights). Then
    `collectibles.json` (table 0x81) is walked in order; each `Mini` entry
    whose `collectibleId` has a count joins with that count as its level,
    until the deck is full.
  - **NORMAL:** all minis of all player slots grouped by name (a hash map);
    the groups are
    shuffled; from each group one copy `rand(copies)` joins with its level
    until the deck is full. Hero and hero level = those of a random slot.
  - **EXPERT:** as NORMAL, but a hero level below 7 becomes `7 + rand(3)`.
  - **RANDOM:**
    1. `rand(4)` picks a pattern of counts for pools keyed 4, 3, 2 (read as
       mana cost): (1, 2, 3), (1, 3, 2), (2, 2, 2) or (1, 1, 4) — 6 minis.
       The patterns are in the clear (`.rodata` 0xcc4cc0 / 0xcc5040 plus
       immediates).
    2. Each pool is shuffled; minis are taken in order, skipping ones already
       picked, until the count is reached.
    3. The picked list is shuffled again; the first `+0x414` (5) are kept,
       each with a level from `r = rand(10·L + 1)` (`L` = the builder's level
       argument): `r < 2` → 1, `r < 5` → 2, `r < 21` → 3, otherwise 4.
    4. Hero level = that of a random slot; then `FUN_008d06b0`: one more
       `rand(slots)` (that slot's hero is the fallback), then a random support
       hero in use (table 0x6b, `rand(count)`).
  - Not known yet: `L` and the cost pools of RANDOM (built elsewhere), and
    the exact order of NORMAL's groups (hash-map order).
- Seen in the code but not yet labelled: bot behaviour-tree nodes with
  `rand(100) < chance` (`FUN_008c660c` places units from two lists into a
  formation's slots and returns 1/2 = fail/success; `FUN_00921cfc` acts with a
  50% chance when more than 5 units are on the board); `rand(max − min) + min`
  in `FUN_0099b4d0` (bot deck from table data).

## Reference footage

Version 1.1143.2 was released around 2022-01-01 (Clashmas update: Royal
Champion, Musketeer, Prince, deck slots, Mini Rooms). Only material from about
Dec 2021 – Feb 2022 counts as reference; check upload dates first.

- Usable: YouTube `97c34Z1fCrY` (2022-01-02), `BDdC7H78fxY` (2022-01-22),
  `CFKsMKLkiZ8` (2022-01-27).
- Not usable (wrong version): the Nov 2021 beta videos, late 2022 videos, the
  2023 draft mode.

From these frames:
- Prepare camera = the `OUT` preset: the board fills ~18-64% of the screen height.
- Screen aspect in footage (OJ Gaming, `moBt1HY_Pic`, and the user's frame): about
  3:4 (966 x 1280 virtual). Enemy deploy half yellow-olive, player half light green.
- Shop box at the bottom left (~73-85% of the height), elixir orb and reroll
  button to its right. The 2D `set_pick_hud` plank is not visible.
- Enemy minis show as ghosts on their half during prepare (not done yet).
- A "CLASH" banner opens each fight. It is in none of the APK's `.sc` or 3D
  files; probably downloadable content (several shipped exports end in `_dl`).
- Main screen = the Clashmas 2021 stage (`env_mainscreen_01`, textures
  `*_clashmas_*`), matching the version. Its backdrop meshes (snowflakes,
  `bg_color`) are thin strips far up/in front, invisible from our home camera;
  the original main-screen camera is unknown, so the sky is not reproduced yet.
- Shop figurines face the camera; reproduced by tilting them (orientation of
  the box's `chess_*` slots not understood yet).

## Confirmed from data

- Board: 5 × 8 cells (`GridX` 0–4, `GridY` 0–7), 4 deploy rows per player
  (`CHESS_DEPLOY_ZONE_ROWS`).
- Ranges (`ranges.csv`): `chessboard` (Chebyshev), `manhattan`, `cross`, `line`,
  with min/max distance in cells.
- Stats are small integers (HP 4–15, damage 1–3). Projectile `Damage` values of
  100/120/200/… are percentages of the shooter's damage.
- Battle phase lengths: `BOARD_SUB_STATE_LEN_*` in `globals.csv` (fight 28.5 s).
- Sudden death:
  - After the fight phase every unit gets `SUDDEN_DEATH_BUFF` = `SuddenDeathDamage`: 1 HP per 250 ms that ignores mitigation, healing blocked, no attacks or moves.
  - It lasts up to `..._SUDDEN_DEATH` (100 s).
- Buff multipliers follow the Clash Royale convention:
  - Positive values multiply (150 = 150%); negative values reduce (−50 = −50%, −100 = stopped).
  - The data shows both kinds side by side (e.g. `HitSpeedMultiplier` 120 and −100).
- Duel economy (`Autochess_2_Player`):
  - start 6 elixir, +6 per round, prepare 16/18/20 s
  - 3-card pick list, 6-unit limit
  - reroll tokens (start 1, +1 per round, max 5), 50% sell refund
  - MaxHp 2 (best of three)
- Camera: `CAM_*` in `client_globals.csv`.
- Lighting per location in `locations.csv`.
- Animation: one clip per glb; the clip files carry the skeleton only; frame
  events are in `*_animations.csv`.
- Tables contain obfuscation rows (random name, all other cells empty). They
  keep their row index and are skipped by the typed data.

## Assumptions (`TODO(RE)`)

Search the code for `TODO(RE)`; the main open questions:

| Area | Assumption |
|---|---|
| Tick | 50 ms logic tick |
| Timing | `HitSpeed` = pause in ms after an attack with `use_hit_speed` (minus `PerStarHitSpeed` per star); one footage measurement (~2.9 s Archer Queen cycle) |
| Stars | per-star stats add linearly to the 1-star values; max 3 stars, +1 per duplicate |
| Targeting | ties broken by lower unit id; move prefers closer, straight, forward |
| Projectiles | speed: 1 cell per 100000/Speed ms; `Damage` < 10 is absolute HP |
| Leaps | jump/dash: next free cell beside the target |
| Buff stacking | multiplier deltas add (`120%` + `130%` = `150%`) |
| Crits | `CriticalHitMulti` % then `+CriticalHitInc` |
| Shop | card tier = mana cost − 1 (2→T1 … 5→T4) |
| Round result | duel: loser −1 HP; 8-player formula unknown |
| Sudden death result | draw only if both sides fall in the same tick |
| RNG | algorithm confirmed (above); the game's seeds and order of draws are not |
| Generated bot decks | the player is one deck slot with card and hero level 1 (no progression yet); NEWBIE's fragment count is the card level; RANDOM: pools = the mapped minis by mana cost, `L` = 0; NORMAL's groups in first-seen order; `BOT_DECK_REAL` and unknown ids use RANDOM |
| Visual | 1 cell = 6 world units; `GlobalOffset` / 100; skeleton root renamed `Root` for clip binding |
| Blend modes | SC `blendMode` 4 opaque, 1 and 5 additive, 2 and 6 alpha blend (inferred) |
| Main screen | camera and deck slot positions estimated |
| Shop panel | `ShopPosition*`/`Scale` mapping unknown; box, elixir orb and reroll placed by calibration against the footage above |
| Clip root motion | translation keyframes of scaled roots are in the root's scaled units (confirmed: `battlebox_in` lands exactly on the rig pose) |
| Clip poses | clip glbs keep channels only for moving bones; every other bone's pose for that clip is its node transform in the clip file (confirmed: the prince's pony sits under him only with these poses). `cm_assets::gltf` adds them as constant channels |
| Effects | `FollowParent`/`FollowBone`/`FollowAngle` attach effects to the unit or bone; frame events' `FrameEventEffectEndFrame` ends them; offsets on `Ground` parts ignored; unattached looping parts end after 2 s |
| Shadows | `ShadowType` 2 = shadow map in `uber*` shaders, `color *= mix(shadow, 1, minShadowWeight)`; `DirShadow` read as z-up travel direction; `minShadowWeight` = 1 - `ShadowDarkness`/100; the closed table mesh and the shop cast no shadow |
