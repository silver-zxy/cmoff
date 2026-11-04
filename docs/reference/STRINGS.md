# `libg.so` strings (1.1143.2)

What the decrypted string pool of `libg.so` contains. How it is decrypted is
in [RE.md](RE.md#string-encryption); the findings that change the rewrite are
in [RE.md](RE.md#findings-from-the-decrypted-strings).

Get the strings with:

```
python scripts/decrypt_libg.py apk/lib/arm64-v8a/libg.so apk/decrypted
```

This writes `apk/decrypted/strings.tsv` (Ghidra address, length, text; one
row per encrypted item) and `apk/decrypted/libg_decrypted.so` (the library
with `.data` decrypted; import it into Ghidra so string references resolve).
Both stay under the git-ignored `/apk`.

## Overview

- 15,232 encrypted items, 515,027 bytes, all in `.data`.
- About 11,100 distinct text strings. The rest are binary tables that were
  encrypted the same way: CRC/hash tables, Unicode tables, codec tables, GLSL
  shader sources and `#define` headers (`#define SSS`, `#define CUTOUT`, ...).
  The largest item is 28,820 bytes (at 0xefd926).
- Items are shuffled: neighbouring items are unrelated, so the position of a
  string says nothing about the code that uses it.

| Kind | Count (distinct) | Examples |
|---|---|---|
| RTTI type names | ~1,060 | `19LogicMessageFactory`, `16ReplayStreamItem` |
| Format/debug strings | ~950 | `LogicBattle::loadReplay deck %d missing` |
| CSV/JSON column and key names | ~1,000 | `HitSpeed`, `mirrorPossibility` |
| `UPPER_CASE` names (globals, enums) | ~910 | `CHESS_DEPLOY_ZONE_ROWS`, `STATE_PREP_IDLE` |
| `TID_*` text ids | 494 | `TID_RANK_PLACE_%d` |
| Asset paths | ~200 | `csv_logic/bot_ais.json`, `sc/ui_battle_end.sc` |
| JNI class names and signatures | ~130 | `com/supercell/titan/GameApp` |

## Network and services

- Game servers: `game.prod.clashminigame.com` (port `9339`) and the staging
  host `game.stage.boardtest.supercelltest.com`.
- Eight hard-coded IPv4 addresses (AWS ranges): `18.232.86.106`,
  `34.217.2.47`, `35.162.98.64`, `52.81.84.194`, `54.152.132.4`,
  `54.185.157.94`, `54.222.197.69`, `68.79.41.138`. What they are used for is not
  traced; probably fallbacks.
- Links: `https://link.clashmini.clash.com`, `https://link.clashmini.clash.cn`,
  `https://link-staging.clashmini.clash.com`, event assets at
  `https://event-assets.clashmini.com`.
- Supercell ID (`id.supercell.com/ingame/...`, staging
  `staging.nonprod.id.supercell.com`), Helpshift (`supercell.helpshift.com`),
  Facebook Graph, Kakao, Unity Ads, Google Play Games, install referrer.
- China builds: Tencent MSDK (`TencentManager`, `LAST_TENCENT_OPEN_ID`),
  Kunlun and Yoozoo channel ids (`kunlun-xiaomi`, `yoozoo-taptap`, ...),
  real-name/anti-addiction checks (`kunlunGetIndulgeTime`,
  `CHINA_REGULATIONS_ENABLED`). Server environments `clashmini`,
  `clashmini-cn`, `clashmini-ed`.

## Protocol

The usual Supercell "Titan" client stack.

- Encryption: `PepperEncrypter`/`PepperPerMessageEncrypter` (NaCl box, as in
  other Supercell games) and `RC4Encrypter`; `ClientHelloMessage`,
  `ServerHelloMessage`, `LoginMessage`, `LoginOkMessage`, `LoginFailedMessage`,
  `KeepAliveMessage`, `SetEncryptionMessage`, `ClientCryptoErrorMessage`.
- UDP for battles: `UdpConnectionInfoMessage`, `UdpCheckConnectionMessage`,
  `UdpBigMessageFragmentMessage`, `UDP_HEARTBEAT_MS`.
- 256 message classes (`*Message`). Battle-related: `SectorCommandMessage`,
  `SectorHeartbeatMessage`, `SectorStateMessage`, `EndClientTurnMessage`,
  `BattleResultMessage`, `OutOfSyncMessage`, `ChessModeMatchmakingStatusMessage`,
  `FillMatchmakeWithBotMessage`, `CreateGameRoomMessage`, `StartGameRoomMatchMessage`,
  `RematchMessage`, `HomeBattleReplayMessage`.
- Lockstep checks: `Check sum mismatch. Request full update. (client tick %d)`,
  `Tick mismatch. Request full update. (client tick %d), server tick %d`,
  `OutOfSyncMessage received, clientChecksum:%d serverChecksum:%d`,
  `LogicDebugChecksumEncoder` and `LogicDebugByteStream`. Battles are
  simulated on both sides and compared by checksum, so the logic is
  deterministic by design (matches the rewrite's goal).

## Commands (68 `Logic*Command` classes)

Battle (auto chess): `LogicAutoChessRerollCommand`, `LogicAutoChessSellCommand`,
`LogicAutoChessLevelUpCommand`, `LogicAutoChessLockPickListCommand`,
`LogicSwapChessCommand`, `LogicReDeployCommand`, `LogicReDeployDropCommand`,
`LogicReDeployStateCommand`, `LogicMetaSelectModifierCommand`,
`LogicSelectSupportHeroCommand`. Placing a mini from the shop is
`LogicDoSpellCommand` (fields `actionIdx`, `m_globalId`, `m_logicState`,
`m_logicX`, `m_logicY`; it has no RTTI string of its own).

Home: `LogicSelectDeckCommand`, `LogicSelectStartingDeckCommand`,
`LogicAddTroopToDeckCommand`, `LogicRandomizeSelectedDeckCommand`,
`LogicChooseGameModeCommand`, `LogicStartMatchmakeCommand`, chests
(`LogicBuyChestCommand`, `LogicChestOpenFinishCommand`, `LogicAddChestCommand`),
quests (`LogicClaimQuestRewardCommand`, `LogicRerollQuestCommand`, ...), season
road, shop (`LogicShopItemPurchaseCommand`, Chronos purchase flow), skins
(`LogicTroopSkinSelectionCommand`, `LogicBoardSkinSelectionCommand`),
collectibles, tutorial and alliance commands. `LogicDebugCommand` exists but is
refused when debug is off.

## Logic classes

- Battle: `LogicBattleAutoChessListener`, `LogicAutoChessListener`,
  `LogicAutoChessShufflePool`, `LogicAutoChessChooseSpell`, `LogicCharacter`,
  `LogicCombatComponent`, `LogicHitpointComponent`, `LogicMovementComponent`,
  `LogicCharacterBuffComponent`, `LogicProjectile`, `LogicAreaEffectObject`,
  `LogicBoardElement`, `LogicSupportHero`, `LogicSpell`, `LogicRandom`.
- Data tables: one `Logic*Data` class per CSV (`LogicCharacterData`,
  `LogicAbilityData`, `LogicCharacterBuffData`, `LogicBattleTaskData`,
  `LogicBattleTaskRewardData`, `LogicBoardCharacterModifierData`,
  `LogicAutoChessBuffData`, `LogicAutoChessEconomyData`, `LogicBotConfigData`,
  `LogicBotDeckData`, `LogicBoardWaveData`, `LogicFormationData`,
  `LogicCardsRollData`, `LogicCardsRerollData`, ...).
- Bot behaviour trees (`bot_ais.json`): generic nodes `Selector`, `Sequence`,
  `RandomSelector`, `IfElse`, `LoopUntil`, `Repeat`, `Negate`, `Weight`,
  `AlwaysSuccess`, `AlwaysFailure`, `TaskSuccess`, `TaskFailure`, `TaskIdle`,
  `TaskDebug`, `TaskSetIntValue`, `Log`; battle nodes `BattleTaskDoPick`,
  `BattleTaskRandomPick`, `BattleTaskMakePickDecision`, `BattleTaskReroll`,
  `BattleTaskSell`, `BattleTaskDoReDeploy`, `BattleTaskApplyFormation`,
  `BattleTaskApplyDynamicFormation`, `BattleTaskApplyBotConfig`,
  `BattleTaskUseBoardElement`, `BattleTaskUpdateInfo`,
  `BattleTaskWaitUntilNextRound`, `BattleTaskWaitUntilNoPendingCommands`;
  conditions `BattleConCheckElixir`, `BattleConCheckMiniCount`,
  `BattleConCheckBlackboardInt`, `BattleConHaveCommands`,
  `BattleConIsOpponentPreviousRoundWon`. `BattleTaskRandomPick`,
  `BattleTaskSell`, `BattleTaskDoReDeploy` and `BattleConHaveCommands` are not
  used by the shipped trees. Bot behaviours: `LogicBattleBotBehaviorNormal`,
  `LogicBattleBotBehaviorLegacy`, `LogicBattleBotBTBehavior`.
- Live ops: `LogicLiveQuest*` (21 quest types, e.g. `WinGameNoReroll`,
  `WinRoundNoDeath`, `WinDuelPerfect`, `KeywordClash`, `KeywordEvolve`),
  `LogicLiveSeasonPassData`, `LogicLiveGlobalRotatingShop`,
  `LogicLiveGlobalKillSwitch`.

## Client classes

- Battle HUD: `AutoChessPickHUD`, `AutoChessEndHUD`, `AutoChessModifiersHUD`,
  `BattleStartHUD`, `BattleRoundVSHUD`, `BattleRoundRankHUD`,
  `BattleRewardBoardHUD`, `BattleGameOverHUD`, `BattleUnitListHUD`,
  `BattleStatsHUD`, `DamageCalculatorHUD`, `ChooseStartingHeroHUD`,
  `ChooseHeroFinishHUD`, `RoundEndFlagHUD`, `WaitingForOthersHUD`.
- Board scene: `BoardDecoShop`, `BoardDecoElixirBar`, `BoardDecoElixirBottle`,
  `BoardDecoRerollButton`, `BoardDecoSellIndicator`, `BoardASM*` (animation
  state machine: `SingleAnim`, `SequenceAnim`, `RandomAnim`, `Hierarchical`,
  `ControllerSkill`, `ControllerJumpMoveSeq`), `BoardPhysics` (rigid bodies for
  dropping minis), `IllegalPlaceRenderer`, `HybridMergeState`,
  `UnitMergeState`, `HybridSellState`.
- Engine: `sc3d*` (render graph, world, cameras), `wendelstein::*` (particle
  system with spawn/velocity/drag/vortex/curl-noise components),
  `scAsset*` loaders (SCW, SWF, KTX, PNG, PVR), `scZstdFilter`,
  `scLZMADecompressFilter`, `scZlibFilter`, `scCryptoBoxFilter`, FMOD Studio.

## Data files the code loads

Every file in `csv_logic` and `csv_client` except
`csv_logic/board_waves_testing.csv` (and the `sorted_regions` folder), plus
`chests/*.json`,
`supercell_id_config.json`, `fingerprint.json`, and seven sound banks
(`bat_buff`, `bat_common`, `bat_hit`, `bat_skill`, `bgm_jingle`, `ui`,
`ui_bat`). Debug/offline builds also read `../offline_avatar.json`,
`../offline_configs.json`, `../offline_events.json`, `../offline_home.json`,
`simulation.csv`, `simulationCombo.csv`, `capture.json`.

Referenced but not in the APK (downloaded or left over from Clash Royale):
`level/starting_home.json`, `sc3d/diffuse_lightmap.pvr`,
`sc3d/specular_lightmap.pvr`, `sc3d/war_map_*.png`, `sc3d/fortune_*`,
`sc3d/diff_scroll_gold02.png`, `shader/foam.pvr`.

## Globals

- 702 of the 736 names in `globals.csv` + `client_globals.csv` appear in the
  code. Most of the rest are built at run time (`DROP_RATE_CHEST_*_%s`, `_ANDROID`
  / `_TENCENT` / `_KUNLUN` platform suffixes) or are unused leftovers
  (`CLONE_LEVEL_OFFSET`, `PRESTIGE_*`, `SHOP_NEW_CARD_CHANCE`,
  `MAX_SPELL_PENDING_TIME`, `CHEST_AUTO_OPENING`).
- Many globals are inherited from Clash Royale (`KING_ACTIVATE_TIME_MS`,
  `TOURNAMENT_*`, `TRADE_*`, `CLAN_WAR_*`, `LOGIC_INFERNO_RESET_ON_SHIELD_LOST`,
  `LOGIC_ELIXIR_COLLECTOR_NEVER_ON_OPENING_HAND`, ...).
- Board sub-states (names used with `BOARD_SUB_STATE_LEN_*`):
  `STATE_PREP_CARRIER_SHOP_IN`, `STATE_PREP_ENEMY_BOARD_IN`, `STATE_PREP_IDLE`,
  `STATE_PREP_TIMER_APPEAR`, `STATE_READY_TO_BATTLE_CARRIER_SHOP_OUT`,
  `STATE_READY_TO_BATTLE_ENEMY_DROP`, `STATE_BATTLING_START`,
  `STATE_BATTLING_PRE_FIGHT`, `STATE_BATTLING_FIGHT`, `STATE_RESULT_BRIDGE_OUT`,
  `STATE_RESULT_ENEMY_BOARD_OUT` (plus `..._SUDDEN_DEATH`, `..._RESULT_*`
  lengths without a state name).
- Bot deck kinds: `BOT_DECK_NEWBIE`, `BOT_DECK_NORMAL`, `BOT_DECK_EXPERT`,
  `BOT_DECK_RANDOM`, `BOT_DECK_REAL`.

## Debug menu

The release build still contains the debug menu (disabled by the server).
Entries include `Win battle`, `Lose battle`, `Draw battle`, `Restart battle`,
`Next battle state`, `Full mana`, `Max Elixir`, `Add Reroll Token`,
`Add enemy troops`, `Damage all troops`, `Kill characters`, `Summon Mini`,
`Summon Hero`, `Show Opp Deck`, `Toggle Bot For Me`, `Toggle DamageCalcHud`,
`Combat balance`, `Skip Tutorial`, `Unlock All`, `Add Gold`, `Skip 1 day`,
`Use Staging Environment`, and render toggles (`Toggle Bloom`,
`Toggle Shadow`, `Toggle Orthographics`, `Toggle Overdraw`, ...).
`TestCase`/`TestRunner` and `SCRemoteConsole` (remote CSV editing:
`Failed writing editable CSV value`) are also present.

## Anti-tamper (clear `.rodata`, not encrypted)

Frida detection over D-Bus (`/re/frida/HostSession`,
`re.frida.HostSession14`, `EnumerateProcesses`), a `/proc/net/tcp` port scan,
memory-map and thread-name checks (`%p-%p r`, `(gum-`), root and emulator
checks (`test-keys`, `/system/app/Supe…`, `/system/xbin/noxsu`, `/system/bin/whic…`),
plus `isEmulator`, `testDeviceIntegrity`,
`isSignatureValid`, `Application APK signature %s` and
`GameApp.nSetAntiCheatStatus`. The hex string
`54255d720514ed5279a614dae203e3ea` (0x1068fe1) is probably the expected
signing-certificate hash.
