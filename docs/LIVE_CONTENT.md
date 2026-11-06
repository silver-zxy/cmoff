# Live content: quests, Adventure Road, shop (analysis)

Findings from libg 1.1143.2 and the APK's data, before building anything.
Addresses are Ghidra addresses (libg offset + 0x100000).

## Where it came from

Quests, the Adventure Road (season), the shop layout and the game mode
mapping are not in the APK. The live server sent them as **Chronos events**:

- LoginOk (20375) carries a "Chronos content URL list" (decoder 0x3c0a6c,
  message +0x130/+0x138); downloaded content.
- `LogicUpdateChronosEventsCommand` 417 (server command; decoder 0x461524:
  server base, bool, event collection 0x82d99c decoded by 0x295f9c, one event
  by 0x4915fc; execute 0x682b80) replaces the events at any time.
- In offline mode (flag 0x118abd8) the client reads them from JSON instead
  (0x997b8c → 0x4ca1c0): `../offline_events.json`, next to
  `../offline_home.json`, `../offline_avatar.json`, `../offline_configs.json`.

One event (JSON of 0x4ca1c0 / binary of 0x4915fc): `id`, `name`, `type`,
`startDateTime`, `endDateTime`, `visibleDateTime`, `visibleEndDateTime`,
`parameters` (the type's own JSON). Binary: VInt id, string, VInts +0x18..+0x28,
three strings, nullable string +0x60 (parameters), VInt +0x70, bool +0x74.

Types (0x4ccac0): `SHOP_LAYOUT`, `QUEST_POOL`, `QUEST_CHAIN`,
`GAME_MODE_MAPPING`, `SEASON_DATA`, `LINK_POPUP`, `QUEST_GROUP`.

## The types' JSON keys (from their parsers)

| Type | Class | Keys |
|---|---|---|
| QUEST_POOL | LogicLiveQuestPoolData (0x6b2b6c) | Pool, CycleCategory (Daily/Weekly), DailyLimit, CycleLimit, PoolPriority, DuplicateAllowed, LevelLimitLow/High |
| one quest | LogicLiveQuestData (0x5887fc) | QuestTextTID, MinProgress, MaxProgress, Rewards, ClaimRestriction, PromotionToken, InstanceTag; kinds: WinRumble, WinWithUnit, PlayGameMode, UpgradeUnit, MetaOpenChest, MetaPlayerLevel, MetaSeasonRoadLevel, … (LogicLiveQuest* classes) |
| QUEST_CHAIN | (0x71d454) | Chain, Category (Competitive/Season/Tutorial), LoopWhenDone, LevelLimitLow/High |
| QUEST_GROUP | LogicLiveQuestGroupData (0x608d94) | TitleTID, Quests, PickLimit, PickRandomly, Milestones, QuestsDoneReq, Chest, LevelLimitLow/High |
| quest settings | LogicLiveGlobalQuestSettings (0x919330) | Enabled, MaxConcurrentQuests, MaxConcurrentDailyQuests, MaxTotalDailyQuests, DailyQuestRerollLimit, …Weekly… |
| SEASON_DATA | LogicLiveSeasonData (0x49c3ac) | SeasonName, SeasonIndex, BasePass, PremiumPass, PassValue, PassInfoTID, ShowEndTime, HasInfoPage, InfoPageConfig, LevelLimitLow/High |
| SHOP_LAYOUT | LogicLiveShopLayoutData (0x46e118) | Modules (shop_modules.csv), LevelLimitLow/High |
| rotating shop | LogicLiveGlobalRotatingShop (0x4e5068) | Rotations, RotationName, PossibleItems |
| GAME_MODE_MAPPING | LogicLiveGameModeMappingData (0x34cd28) | SoloMode, TournamentMode, LevelLimitLow/High |

## Commands around them

| Id | Name | Side |
|---|---|---|
| 351 | LogicUpdateQuestProgressCommand | server → client: the server counts quest progress |
| 502 | LogicRerollQuestCommand | client |
| 503 | LogicClaimQuestRewardCommand | client (reward = a chest, filled with 420) |
| 508 | LogicClaimQuestMilestoneCommand | client |
| 505 | LogicUpdateSeasonTierSeenCommand | client |
| 601 | LogicClaimAllSeasonRewardsCommand | client |
| 418 | LogicClaimSeasonRoadRewardsOnSeasonEndCommand | server |
| 451 | LogicSeasonEndCommand | server |
| 701 | LogicStartChronosPurchaseFlowCommand | client (shop purchase) |
| 421 | LogicContinueChronosPurchaseCommand | server |
| 309 | LogicShopItemPurchaseCommand | server |

## Data that is in the APK

- `chests/sources.json`: every reward source and its chests — battle rewards
  (1–5 wins: gameMode_Ranked_1W..5W), Challenge Match (promotion/demotion),
  quests (quest_Competitive_5W, quest_Daily_Small, quest_Daily_Mini),
  upgrades, shop items per mini and hero, starter chests.
- `season_rewards.csv`: Adventure Road tiers 1–9, star count and chests.
- `csv_client/season_road_segments.csv`: the road's segments (art).
- Texts: the Adventure Road is the season road ("Each QUEST you complete
  provides some PONY FUEL"): quests give SeasonPoints, which move the road.

## Mini Room / PvP

Battles are lockstep (client and server compare checksums, 0x994008,
"Check sum mismatch. Request full update."), so both clients simulate the
battle themselves; a host that hands out the same start (SectorState) and
relays the players' commands (SectorCommand 16634) would be enough in
principle. Rooms: Create/Join/Leave/StartGameRoomMatch (10181, 10186, 14761,
10190) with their responses, GameRoomMembers 20188. Open: the SectorState
for two real players (an earlier server-built battle ran but kept the
tutorial's rules, SERVER.md) and the UDP channel (UdpConnectionInfo 23036).

## First step (server: `home/live.go`)

- The home's +0x148 carries `{"ID":"QUEST_SETTINGS","Params":{"Enabled":true,
  "MaxConcurrentDailyQuests":3,…}}` (decoder 0x4fa778: three times a bool and
  a nullable string; 0x4bfe7c parses each as {ID, Params}).
- After the login home the server sends 417 with one QUEST_POOL event
  (type 1; times in unix seconds, 0x6170cc counts days from 1970) holding
  three daily quests of the game's kinds (PlayGameMode 2_PLAYER, WinRumble,
  MetaUpgrade) with quest_* chests as rewards. Its echo is read past.
- `LIVE=off` (cmserver `-live=false`) leaves all of it out.

First test: the home loaded, the 417 was taken silently, the quest screen
stayed empty: the client does not pick quests from a pool. The server hands
them out.

## Second step: quests in the home (`home/quests.go`)

- The chronos part of the home (+0x140, encoder 0x8197fc) holds the event
  collection at +0x10 (0x7ea4f4, the same format as 417); the events go
  there now, the 417 is no longer sent.
- The quest manager (+0x180, 0x31b774 / 0x4bccd8) holds the active quests;
  layout in quests.go. Quest kind ids (0x3807a8): WinWithUnit 1 …
  MetaUpgrade 9 … WinRumble 19 … PlayGameMode 26. Each quest carries its
  kind's state (MetaUpgrade two bools, PlayGameMode the mode 2/3).
- Claim: 503 (base, VInt quest id, bool); the client makes one chest of
  the rewards (source 2, value the quest id), the server pushes its
  content (420).
- The server counts progress: battles (PlayGameMode, WinRumble), upgrades
  incl. the client's auto-upgrades (MetaUpgrade); it shows with the next
  home. LogicUpdateQuestProgressCommand 351 is not sent yet.

Third test: the quests counted, but neither claim (503) nor reroll (502)
reached the server. The quest manager's bool +0xc8 (0x5c53dc) is the quest
system's switch: 502 (0x328d88), 503 (0x68d9e4), 508 (0x7d7428) and 351
(0x7249e4) return 3 while it is false, and the command manager (0x6a2344)
drops a command whose local execute fails, without sending it. The UI does
not look at it, so the quests showed. It is now true with live content.

The saved quests point into the pool by index; when the pool changes
(0044 replaced the third quest) the account's quests are made anew
(`quest_pool` holds the pool's signature), else an old "upgrade 2/2"
showed as a finished "play 2 Rumbles".

Fourth test: claim and reroll reached the server, but the client's reroll
picked a different quest than the server's. The quest manager's pool state
(VInt count; per pool Int event, Int handed out, VInt list of the taken
pool indices; encoder 0x31b864, decoder 0x4bcf18) was empty, so for the
client every index was free. A reroll (0x785fac) takes one back from the
count, picks among the free indices and frees the old one afterwards; with
all of today's indices listed exactly one is free, the server's pick.

Fifth test: still the old quest again, without rewards. Two causes:

- The client's time. A reroll first looks for an active pool of the
  quest's category (0x5e0478: event active by 0x5dbb2c, now between
  start +0x1c and end +0x20). "Now" is ticks/20 + the logic's time base
  +0x1d4 (0x4df510), set (0x327c54) from the second VInt of OwnHomeData's
  tail (msg +0x9c, decoder 0x48b2e4, handler 0x4554fc → 0x85f100). We sent
  0, so for the client it was 1970 and no pool was active; the fallback
  (0x8c8c8c) rebuilds the quest at the old pool index. The tail now carries
  the unix time (live content only).
- The rewards of a quest the client makes come from the pool definition
  (0x72049c copies def +0x28), whose "Rewards" are chest names
  (0x5887fc → 0x865888, a lookup by name). Numbers found nothing. The pool
  JSON now names them (from the APK's chests.json).

## Adventure Road (`home/season.go`)

- The road is a SEASON_DATA event (type 4, 0x49c3ac): SeasonName,
  SeasonIndex, `SeasonPassTiers` (0x5fd020; a tier 0x3291ec: `PointsReq`,
  `FreeChests`, `PaidChests` by chest name, optional `FreeVisualStyle`,
  `PaidVisualStyle`, `RoadVisualStyle` (unknown → default, 0x5e4ed0),
  `IsKeyTier`), the passes as optional shop items (`BasePass`,
  `PremiumPass`, parsed as shop items by 0x6bb398).
- `VisualConfig` (0x786afc: `TierFileName`, `TierExportName`,
  `ProgressMarkerFileName`, `ProgressMarkerExportName`, `Decorations`) is
  required: the road screen reads it without a check (0x6f24c8 → pass
  +0x18 → 0x689b84, a null pointer crash when missing). Ours names the
  game's `sc/season_road_01.sc` and its exports `road_segment` and
  `progress_marker` (season_road_segments.csv names the same file).
- The "i" button: `HasInfoPage` and `InfoPageConfig` (0x6e95a8:
  `InfoPageSWF`, `LayoutExportName`, `PagesExportName` a list). The road
  file has one info page, `season_info_1_layout` / `season_info_1_page_1`
  (texts TID_SEASON_INFO_SEASON_1_*). `SeasonName`
  TID_SEASONROAD_SEASON1_NAME is "The Adventure Begins".
- `season_rewards.csv` is not the road: it holds the league's end of season
  rewards by stars (season*_EndStars_*).
- Home +0x1c8, the season manager (0x31e77c / 0x4bb660): VInt current
  season key (+0x18), VInt +0x20, VInt +0x1c, entries: VInt event id (the
  client finds its SEASON_DATA event by it, 0x6116b8, else a default
  season), VInt season key, optional progress (0x4aa038): VInt, VInt points
  (+0x40), Bool pass (+0x4c), VInt highest free PointsReq claimed (+0x44;
  claimed when ≥ PointsReq, 0x4ebaf8), the same for the paid lane (+0x48),
  per-tier lists the client rebuilds from the event (0x6eda50).
- 601 LogicClaimAllSeasonRewardsCommand (0x4c59a0: base, Bool, VInt tier;
  execute 0x6bacac → 0x3ac468): every tier reached and not claimed
  (0x8ee9b0) gives its chests as one chest (source 5, value the season
  key); the server pushes the content (420).
- 505 LogicUpdateSeasonTierSeenCommand (base, VInt season key): the client
  puts the index of the highest tier reached (0x5b3bf0, −1 none) into home
  +0x1e0.
- Pony Fuel is the resource SeasonPoints (resources.csv row 14); every
  daily quest chest holds one.
- Ours (patch 0065): Season 1 as the game knew it, again every two
  calendar months (UTC): 90 tiers, 1 fuel each on tiers 1–30, 2 on 31–60,
  3 on 61–90 (180 in all; the wiki says 1–3, the split is ours). The free
  lane: the game's season chests (gold, gems, one fragment of any mini,
  hero fragments; the season's mini on tiers 10, 30, 60). The Mini Pass
  lane is everyone's (progress +0x4c true, 601 claims both lanes): the
  P.L.U.S.H. P.E.K.K.A at tier 26 and the Luchador Monk at tier 87 (wiki),
  and the pass's "300+ Hero Fragments, 200 Gems, 2000 Gold" (its text):
  Monk 50, Countess 10, hero chests (about 334 fragments in all), gems
  40 + 100 + 40 + 20, gold 8 × 250. A road saved when a season was one
  month keeps its fuel.
- Quests count the road: "Advance on the Adventure Road N times"
  (MetaSeasonRoadLevel 24, no state) counts the tiers the fuel reaches.

## Shop (`home/shop.go`)

- A SHOP_LAYOUT event (type 0, 0x46e118): `Modules`, each (0x8e6494)
  `ModuleName` (shop_modules.csv: BasicModule, PermCurrencyModule, …),
  `ModuleTitle`, `Items`, `RotatingItems`, `CountdownMode`, `RotationTimer`,
  `EndTimer`, `Priority`, `HideSoldOutItems`, `RegularItemsRefreshDaily`;
  an item (0x6bb398): `ResourceType` (a resource's name, 0x6e88c8),
  `ResourceCost`, `IAP`, `ExtraValuePercent`, `PurchaseLimit`, `ItemTID`,
  `ButtonWidgetType` (SmallIcon/LargeIcon), `ButtonWidgetImage`, `Chests`
  (by name), `ShowConfirmPopup`.
- The clock of a module or item: `CountdownMode` "ForceNoShow" (1),
  "RotationTimer" (2), "EndTimer" (3, to the event's end), none when
  missing (0x8e6864, items 0x5b6060). Ours: EndTimer, the next day's shop.
- An item's number counts over the modules, each module's Items before its
  RotatingItems (0x503e08).
- The buy button (0x3405e4, by the item's state 0x39e714: 0 buyable,
  1 too expensive, 2 limit reached, 3 free) goes to 0x4bddec: an item with
  an IAP product starts the store purchase (701 LogicStartChronosPurchase-
  FlowCommand, 421 and 309 from the server), any other sends
  **702 LogicBuyChestCommand** (ctor 0x74a208: base, VInt event, VInt item;
  execute 0x93286c): the client checks price and limit, takes the price,
  counts the purchase in the event manager's list (+0x28, 0x461c20) and
  makes one chest of the item's chests (source 4, value the event id); the
  server pushes the content (420).
- The purchases come with the home: the chronos part's first (VInt, VInt)
  list (0x8198f8) holds (event, item), the limits count them (0x6e5130).
- Ours (patch 0065), as on screenshots of 1.1143.2 (docs/ORIGINAL.md):
  one shop a day (UTC) with its own event id (the limits start anew).
  Minis: three offers, one fragment each, once each, for 250 / 500 / 750
  gold by the mini's elixir cost (characters.csv ManaCost, read from the
  APK at runtime: home/sclzma.go decodes the LZMA of csv_logic). Heroes:
  three offers, 25 fragments (shop_miniHero_*_25) for 100 gems, five
  times each. No gem packs or bundles (real money); the Clashmas skins and
  the Crystal Coin skins come with the skins (ROADMAP A5).

## Daily quest pool

- QUEST_SETTINGS: six daily quests a day, all shown at once, one reroll
  ("Quests completed: 0/6"). A claimed quest leaves the list.
- The pool: seven quests a day out of the catalog (six and one for the
  reroll). The home's pool state lists every index but the reroll's as
  taken, the ones still to come too, so the client's reroll (0x785fac,
  random among the free indices) takes the pool's last quest; an index
  rerolled away is not handed out again.
- Kinds the server counts: PlayGameMode (duels, rumbles), PlayWithUnit (2,
  state: VInt RequirementMini, 0 none), WinWithUnit (1, state: Bool
  UseRoundWins, VInt RequirementMini; a duel won or a Rumble in the top 4), WinRumble, MetaResourceGained (25, state: VInt resource row + 1;
  RankPoints, "Win N Trophies", ranked trophies won; out of the pool since
  0075: a quest of that kind crashed the quest screen's hint, 0x834d64,
  which found no resource), MetaSeasonRoadLevel.
  The data references are written by 0x37cc70: VInt row + 1, 0 for none.
- The home reads its quests with flag 0 (0x4bce10 → 0x3b3af4): with the
  quest's tail (+0x30 …) and the states' short form. WinWithUnit's Bool
  +0x11 (also WinDuelPerfect's and WinGameNoReroll's Bool) belongs to the
  other form only; 0065 wrote it and the home's next component read past
  its end (a crash at login, fixed in 0066).
- Rewards: the game's chests, as on the screenshots: quest_Daily_Small
  (1 Pony Fuel, 10–30 gold) or quest_gold_100 with SeasonPoint_1 ("100
  gold" and a fuel). The amounts to reach are ours (ROADMAP B5).
- Quest kind ids (0x3807a8): WinWithUnit 1, PlayWithUnit 2, UpgradeUnit 3,
  TotalKOs 4, UseKeywordClash 5, UseKeywordEvolve 6, EndRoundWithUnitAlive
  7, MetaOpenChests 8, MetaUpgrade 9, MetaPlayerLevel 10,
  MetaUsePromotionToken 11, MetaChangeDeck 17, WinDuelPerfect 18,
  WinRumble 19, WinGameNoReroll 20, WinRoundNoDeath 21, WinWithUnitList 22,
  AbilityHitInOneGame 23, MetaSeasonRoadLevel 24, MetaResourceGained 25,
  PlayGameMode 26 (state classes by 0x506b4c, their writers at vtable
  +0x18). The battle kinds (rounds, knock outs, rerolls, 3:0) need what
  happened in the battle, which the hook does not report yet.

## Boards (`home/boards.go`)

- A battle's board (locations.csv) comes from its game mode's LocationData
  (game_modes.csv). Without a server the client uses Autochess_2_Player /
  Autochess_8_Player (0x787b34: mode 2 / 3).
- GAME_MODE_MAPPING (LogicLiveGameModeMappingData 0x34cd28): `SoloMode`
  (mode 2) and `TournamentMode` (mode 3), each (0x862cb4) `Status`
  (Default, Disabled, OverrideGameMode), `OverrideGameMode` (a
  game_modes.csv name) and optional `LocationsByRank` (0x817b4c: entries of
  `Location`, `RankLimitLow`, `RankLimitHigh`, 0x4ae248);
  `LevelLimitLow/High`. The client finds the active mapping (0x7e9d58) when
  it builds a battle (0x787b34), the offline bot battle included (home
  update 0x467e78 → 0x940664, the selection as its fifth argument).
- Season 1's modes by league: Silver `Autochess_{2,8}_Season_1_Silver`
  (Electro Valley, Healing Temple), Gold and up `…_Season_1_Gold+`
  (Banquet Hall = VampireCastle, Healing Temple, Electro Valley). Fish
  Town is Season 2's. Bronze keeps the plain modes (the Basic modes list
  no boards).
- Ours: the home carries the mapping for the account's league (0071).
  The client takes it (StartMatchmake then carries its event id, 4001),
  but a bot battle is PvE and plays on its npc's board (npcs.csv
  OfflineMatchNPC Location = WindyPlains; npc +0x38, resolved at load by
  0x47d8dc). So the server picks one of the league's Season 1 boards for
  every bot battle (`home.BoardFor`) and sends it with the battle signal;
  the hook points the npc at it (`choose_board`: item n of the locations table
  `*(libg+0x10807c0)`; not the CSV row, which is line 2n) and back to
  its own board in Bronze (0072).
