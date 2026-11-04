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
