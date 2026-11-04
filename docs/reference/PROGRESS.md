# Progress

Order of work: small and verifiable first, then outwards.

## Done
- [x] Asset access straight from the APK (or an extracted folder)
- [x] Supercell LZMA, CSV tables (two header rows, continuation rows as arrays)
- [x] KTX textures (ETC1, ASTC 4x4) → RGBA; `.pvr` references mapped to `.ktx`
- [x] SC3D (`.scw`) chunk container
- [x] Typed data: characters, skins, locations, animation clips, abilities
- [x] glTF repair (`cm_assets::gltf`): drops stripped primitives, adds POSITION bounds,
      maps `SC_shader` diffuse to a PBR fallback
- [x] Viewer: location board + units with idle animation on the 5×8 grid
      (`CM_SCREENSHOT=out.png cargo run -p cm-client` saves a frame and exits)
- [x] Battle simulation core (`cm_logic::battle`): targeting, grid movement, normal
      abilities, energy → specials, area damage, crits; deterministic event log
      (`cargo xtask battle "KnightB1@1,0" "BarbarianB1@1,7"`)
- [x] Client plays back simulated battles (move / attack / death / victory clips)
- [x] Match loop (`cm_logic::matchplay`): economy table, shop/pick list, buy-to-upgrade,
      sell, reroll, unit limit, HP and win conditions
      (`cargo xtask match`)
- [x] Buffs, damage pipeline (dodge, reduction, shields, reflection), damage/heal over
      time, stuns, projectiles in flight, area effects, leap abilities; fixed-point HP
- [x] Heroes on the board (`PlayerSetup::hero`)
- [x] Playable client match vs a bot: stand-in UI (shop, reroll, fight), click to
      deploy/move, round flow, HP bars, projectile models
      (`cargo run -p cm-client`; `CM_AUTOPLAY=1` lets the bot play your side)
- [x] Supercell `.sc` 2D format: textures, shapes, movie clips, matrices, colour
      transforms; software renderer; client overlay (shop panel art, unit icons)
- [x] Game fonts (Supercell-Magic) and localized names (`texts.csv`, `GameData::text`)
- [x] Upgrade modifiers (`character_modifiers.json`): path patches on characters,
      abilities, projectiles, area effects, buffs; per-unit copies in each battle
- [x] `cargo xtask check`: all 1194 asset files parse (tables, KTX, glTF, SC3D, `.sc`)
- [x] Board elements per location (energy pads, mirrors, jump pads, …): rolled per
      round into each player's zone, triggered on landing, rendered with idle clips
- [x] Original materials: port of the `uber`/`uber_scene*` shaders (`ScMaterial`)
- [x] Original bots: behaviour-tree interpreter for `bot_ais.json` (6 behaviours),
      pick strategies, formations (`formations.csv`), rerolls, board-element use
- [x] Fixed portrait screen in any window size (letterboxing, virtual 966×1280, the 3:4 aspect of 1.1143.2 footage)
- [x] Drag and drop as in the original: shop figurines → board, move/swap minis,
      sell on the elixir collector, reroll button
- [x] 3D shop: battle box with figurines on bases (`chess_A/B/C`), cost badges,
      elixir orb and reroll button, calibrated to Jan 2022 footage
- [x] Sudden death, death/startup/drag abilities, mirror clones, granted abilities
- [x] `.sc` text fields (named paths, TID texts) and timeline playback (`flatten_at`)
- [x] Prepare screen matched to 1.1143.2 footage: blue/red deploy areas (`deployArea_*`),
      opponent ghosts, original timer (round + seconds) and avatar HUD, round intro
      (`set_versus_round`), round-end flag, win/lose end screens
- [x] Original unit health bars (`unit_health_bar`: ally/enemy fill, energy strip)
- [x] HUD layout as in footage: opponent flag+name top left, player's above the elixir
      orb; stand-in text line and Fight! button removed (rounds start on the timer)
- [x] `.sc` text alignment, bounds-based placement, hiding parts, per-part rendering
- [x] Main screen: Clashmas 2021 stage with the deck on the original pedestals, original
      top bar (`main_page`), Duel / Game Modes (`battle_page_new`), tab bar
      (`set_main_buttons`); Duel starts the match (`CM_SKIP_HOME=1` skips it)
- [x] `.sc` meshes drawn in order (per-page runs), literal `\n` line breaks in texts
- [x] Main screen tabs: Shop / Leaderboard / Info pages (empty: server data), Collection
      page filled with every playable mini (`collection_item_mini` + card art);
      fixed Duel hit test and the elixir bottle's endless pumping
- [x] Collection: Heroes / Minis switch (page buttons), hero tiles
      (`collection_item_hero_mini` + hero card art), mouse-wheel scrolling
- [x] Battle box plays its entry clip, then its idle loop (fills the elixir orb);
      shop figurines face the camera
- [x] Audio: all 298 FMOD samples decode (Vorbis setups rebuilt from `libfmod.so`);
      menu and location music (prepare/fight layers), countdown, round/match jingles,
      unit sounds from animation frame events, hit/projectile/buff/area/move/death
      effects, shop and menu click sounds
- [x] Shop parts play their `player_panels.csv` clips on events: box reroll/fail, reroll
      button push/release, elixir collector open/close/sell/buy, elixir bar up/down
- [x] Particle effects (`particle_emitters.json`, 964 emitters): spawn components,
      forces, Bézier curves, `.sc` sprites as billboards; triggered by frame events,
      hits, projectiles (+ trails), buffs, areas, deaths, moves, deploys, level-ups,
      skin `PermanentEffect`s (`CM_EFFECT=<name>` previews one); `SWF3D` parts play their
      `.sc` animation on a ground quad or a billboard; effects follow units/bones
      (`FollowParent`/`FollowBone`/`FollowAngle`) and end at the frame event's end frame
- [x] Versus intro before round 1 (`set_versus`) with both players' hero banners
      (`ui_hero_banners`, swapped into the export's `hero_banner` parts)
- [x] No floating damage numbers: `IS_USING_FLOATING_DAMAGE` is FALSE in this version
- [x] Opponents as the game builds them: `bot_configs.csv` (deck groups, behaviours,
      `ElixirsKeepPerRound` reserve) → `bot_decks.csv` → `support_heroes.csv` (hero unit,
      start cell, versus banners); name from `npcs.csv` `OfflineMatchNPC`
      (`CM_BOT_CONFIG=<row>` picks the config)
- [x] Team colours (`TeamColorAlly/Enemy` of the skin, masked by the mask texture's
      blue channel as in `uber`'s `TEAM_COLOR`)
- [x] `uber_pbr` port (heroes, Windy Plains board): GGX, MRA, matcap reflection with the
      game's mip atlas (`scTextureLod`), SSS, team-colour mask, Fresnel
- [x] `uber2` port (lit characters such as the bowler, several skins)
- [x] Input works in any window size (Bevy's viewport conversions are in window
      coordinates; labels, health bars and hit tests use `Letterbox::window_to_virtual`)
- [x] Main screen matched to pre-Clashmas footage: deck on the stage's measured step
      tops, `menu_topLayer` resource bar, season road banner (`TID_SEASONROAD_SEASON1_NAME`)
- [x] Match end: `button_quit` on the end screen returns to the main screen; the match
      is cleared and the next one prepared (new opponent)
- [x] Default team colours for skins without their own (wizard robe etc.)
- [x] Prepare screen: deploy areas in `deployArea_base` colours (no longer shadowing the
      board), elixir badges on shop costs, bottle count and `x<n>` rerolls as in footage
- [x] `.sc` software renderer composites into straight alpha
- [x] Game Modes window (`game_mode_selection`): Duel / Rumble; the play button shows
      the mode; selected tab state in the tab bar (tab clips' frame 20)
- [x] Matches of any size: pairings per round (`RuleNewShuffle`), byes, knock-outs,
      ranking; Rumble = 8 players (7 bots), defeat HUD with the placing (`#4`)
- [x] Bot deck groups resolved through `bot_decks.csv` `Decks` lists
- [x] Prepare screen matched to the user's 1.1143.2 reference frame: 3:4 screen,
      yellow enemy / light green player halves, elixir bottle and reroll button lower
- [x] Projectiles without a model (wizard fireball) fly as their `TrailEffect`; free
      particles stay in the world (trails), `followEmitter` ones move with it
- [x] Dragging a shop card lifts its mini out of the box and over the board
- [x] `CM_TIME_SCALE` slow motion for checking effects
- [x] Abilities: kick jumps and rushes (miner digging across, P.E.K.K.A / Mega Knight
      mirror jumps, prince rush with knock-back and stun), `push_back`, jump/dash buffs,
      `area_effect_on_dash`, `cleave_targets`, `damage_to_isolated_target`,
      `override_ability` with `override_chance`, backstab, `check_target_*` bonus damage,
      `BuffOnLastAlive`, delayed death spawns (the prince's pony after 1.2 s), areas bound
      to a moving channel (Valkyrie axe storm)
- [x] Super choice at level 3 (`select_modifier_panel`: the two supers with their
      `ui_ability_icons` and texts; bots pick at random); `CM_OFFER_SUPER=<mini>` opens it
- [x] `.sc` multi-line text fields wrap; running text in SC CCBackBeat
- [x] Battle tasks (`battle_task_rewards.csv` via `BattleTaskData`): Kill / Hit /
      Critical / Death / Stun / NewRound triggers, roles, caps, instant and permanent
      buffs, areas, healing, energy, granted abilities; `BattleEventAbility` slot filter,
      `BuffMaxStack`, `HitHero`, `BaseShieldLost`, `CountEachHit`, `TargetRole`, permanent
      buffs kept by the units for later rounds
- [x] Battle view: IN camera, battle box slides out / back in, fight timer counts down
- [x] Clip poses for unanimated bones (the prince rides his pony)
- [x] Shadows: shadow map from `DirShadow*`, `ShadowDarkness` as `minShadowWeight` in
      the ported shaders (table and shop excluded as casters)
- [x] Original RNG (`LogicRandom`, found in `libg.so` code) and generated bot decks for
      all five `BOT_DECK_*` builders (NEWBIE from starting choices, NORMAL/EXPERT from the
      player's minis, RANDOM by cost patterns; REAL falls back to RANDOM)

## Next
1. Original UI screens: home screen (`main_page`, `set_main_buttons`, Duel button),
   round start ("CLASH"), round end (`round_end_flag_duel`), end screen
   (`battle_end_screen`), avatars/HP; enemy ghosts during prepare
2. Rendering fidelity: normal maps, PBR shadow mask (second UV set); remaining effect types (`ColorFilter`, `Fresnel`,
   `ModelEffect`), particle trails, animated particle clips
3. Remaining battle rules: `InterrupNotQueue` of battle tasks (ability slots, BuffMaxStack, permanent buffs across rounds),
   `HitSpeed`, speed units, jump-pad forced moves
4. (Legacy `.scw` clips are referenced by no skin in use — low priority.)
5. Which bot config a player gets (trophies/arena), bot mini levels; Rumble: the HP
   damage formula, `RuleSplitBrackets`, spectating, bot names
6. Audio: FMOD event parameters/volumes; sounds of the matchmaking and chest screens
7. Menus, collection/progression, chests, tutorial (`tutorials_*.csv`)
