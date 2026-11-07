# Roadmap

The best version of Clash Mini 1.1143.2, close to the original: where the
original had several variants, the one best for the player that really
existed; nothing made up, nothing of 2.0. What cost real money is free
(the Mini Pass, real-money skins); no gem packs or bundles of unknown
content (variant B). docs/ORIGINAL.md has what is known of the original.

**A snapshot of Clashmas 2021, forever.** The game stays as it was in the
days after 21 December 2021: Season 1 "The Adventure Begins", the
Clashmas Special in the shop, the Clashmas quests, the Christmas look.
Days still pass (daily quests and shop offers are new every day), but no
later content ever comes. Season 1 starts again every two months, so the
road stays worth playing.

Order: part A first (everything known), then part B (what is not known
yet; until then our values, marked as ours in the code and docs).
Each point: analysis (addresses in docs/), a patch with tests, a test on
the phone before the next.

## Done

- Offline server and hook; accounts saved locally.
- Duels and Rumble against bots, trophies, leagues, gold and XP.
- Collection, fragments, mini and hero levels, upgrades, decks.
- Chests, also a chest with a list (the starting chest).
- Starting values as the original: 500 gold, 100 gems, starting chest;
  existing accounts got them once (0055, 0056).
- Daily quests with reroll and claim (tested).
- Adventure Road (Season 1) with tiers, claim, info page (0051–0053).
- Shop with daily minis and heroes, timers, purchases as chests.
- Tested on the phone: the starting chest as one chest, the road's info
  page, the shop's chests.
- Research: docs/ORIGINAL.md (sources, wiki, screenshots).
- Tested on the phone (7 November 2026): the Clashmas quests under their
  header with the Candy Elf, skins locked and unlocked through the road,
  the quests and the shop (0087-0092), hero quests and new minis first
  (0090), dev mode (0093).
- Tested on the phone (November 2026): boards by league (A0), bots with
  their decks' own levels (A1), the shop with the original prices (A2),
  daily quests six at once, the battle kinds counted by the client
  (0079), mini quests with the next step right after a claim (0083),
  Crystal Coins (A4), the Adventure Road with 90 tiers and the free Mini
  Pass (A6, A7), the help page.

## Decisions (7 November 2026): close to the original

- Season end: the star exchange as in December 2021 (minis back to level
  1, rewards by stars); the choice of March 2022 is not in our client.
- Mini quests only for newly unlocked minis, as in the original.
- The Clashmas quests end after their list (0098): the known quests once,
  the fragment ones first; the first ones again up to the Candy Elf's 12
  (ours: only 10 are known); then the Crystal Coin ones once more, as the
  screenshots show quests open at 12/12; then no more.
- Mini Pass and the Clashmas Skins bundle stay free; no gem packs or
  other real-money offers.
- The four older skins are everyone's (how they were sold is not known).

## Next (from 7 November 2026, in this order)

1. ~~Season end: the road's rewards with 418~~ (0111 dropped, not a goal).
2. Mini quests at once on unlock without 417 (it crashed, 0099): find
   how the original handed them out mid-session.
3. The two missing Clashmas quests: "Prince charges 3 enemies"
   (AbilityHitInOneGame, its state) and "Win 10 trophies"
   (MetaResourceGained crashed the quest screen, 0075).
4. Quests found in the client: the tutorial chain, the competitive chain
   (quest_Competitive_5W), "Use N Crystal Coins".
5. Part B research (B1 road per tier, B2 Clashmas quests, B3 mini quest
   rewards, B5 daily quests, B7 the skins bundle).
6. A10 Mini Room with real players: way A (local network) and way B
   (relay), see A10.

## Part A: what we know

### A0. Boards by league (0071, 0072; tested)

- Season 1's game modes by league (game_modes.csv): Silver plays on
  Electro Valley and Healing Temple, Gold and up also on Banquet Hall;
  Bronze stays on Windy Plains. The mapping goes out as GAME_MODE_MAPPING;
  for bot battles the server picks the board and the hook sets it on the
  bot (the client took the mapping, but bot battles keep the bot's board).

### A1. Bots as in the original (patch 0064; tested)

- The bots keep their decks' own levels from bot_decks.csv (93 decks by
  league: Bronze heroes 1–4, minis 1–2 … Grandmaster 10 and 4); the
  config by league from bot_configs.csv (22, already).
- Only the unlocked collection (no progression) still sets every deck to
  10 and 4. The generated decks (BOT_DECK_NORMAL / NEWBIE) are built
  from the player's collection by the client and not touched.

### A2. Shop as on the screenshots (0065: minis, heroes; tested)

| Module | Original | Ours |
|---|---|---|
| Minis | 3 a day, 1 each, +1 fragment, 250 / 500 / 750 gold (by elixir) | ✅ 0065 |
| Heroes | 3 a day, 5 each, +25 fragments for 100 gems | ✅ 0065 |
| Clashmas Special | Jolly 159 gems, Helpful 99 gems, "Clashmas Skins" (Gift, Sparkle, Snowler) | 0091, 0092 (tested) |
| Crystal Coin skins | Crystal P.E.K.K.A 100, Fabulous Prince 600 | 0091, 0092 (tested) |

- The Clashmas Skins bundle cost real money (7.99 €): free (claim for 0).
- Left out: gem packs, Starter Bundle (real money), our old 240 gold for
  30 gems.

### A3. Quests as on the screenshots

Done in 0065 (tested):
- Daily: **6 a day, all at once** (0075), 1 reroll; rewards the game's chests:
  quest_Daily_Small (1 Pony Fuel, 10–30 gold) or 100 gold and 1 fuel.
- Kinds the server can count: play duels / rumbles / matches, win matches,
  finish a Rumble first, advance on the road N times ("win N trophies"
  crashed the quest screen and is out, 0075).

Also done:
- Battle kinds (0079, tested): a battle copies the home's quests that
  have not ended (+0x1c end time > now); we sent 0, so the battle had none
  and counted nothing. Now the quests carry the daily reset as their end;
  the client counts knock outs, rounds, no reroll, rounds without a loss,
  3:0, Clash and upgrades on the battle's copies (battle +0x238 per
  player) and the hook reports them after the battle. Rounds and 3:0 the
  server also counts from the hook's rounds (0078).
- Mini quests (0077, 0080, 0081, 0083; tested): win 1, 3, 5 matches
  using each owned mini, three at a time, quest_Daily_Mini; category 3 (MINI
  QUESTS). They were missing on the quest screen because it lists only
  quests not ended (0x753e88, +0x1c), and we sent 0 (fixed in 0079),
  and a chain's quest belongs in the chains' state, one chain event per
  mini (0081).

Also (0090, tested):
- Hero quests in the daily pool ("a larger pool of hero quests", patch
  notes): win 6 rounds / play 3 matches using the day's hero (one of the
  five starting heroes, by day), quest_daily_mystery_Hero (30-50 gold, a
  Pony Fuel, 3 hero fragments). The client counts them in battle.
- Mini quests come first for a newly unlocked mini (Deconstructor of
  Fun: "triggered when a new unit is unlocked"); from 0097 only for
  newly unlocked minis (decided), quests open from before stay.
- Seasonal quests: the client has the section (SEASONAL QUEST, category
  1, a chain with Category "Season") but no quest of its own for it; no
  source shows seasonal quests in December 2021, so none.
- Found in the client, built in 0114: a tutorial chain for new players
  (TID_CHRONOS_QUESTS_TUTORIAL_CHAIN_1-4: put Magic Archer / Shield
  Skeleton in the deck, play a match using Wizard, upgrade a hero, play a
  ranked match), a competitive chain ("Win N rounds in Duel or Rumble",
  quest_Competitive_5W: 1 Crystal Coin), "Use N Crystal Coins".

Still to do:
- Clashmas quests (0087; category 6, under their header, 0089; tested): Clashmas was a time, not a single
  event: in it the Clashmas quest group was on (QUEST_GROUP, event 3001),
  three of its quests at a time; 12 done unlock the Candy Elf (milestone,
  508), after that the group goes on with Crystal Coins. Our snapshot
  keeps Clashmas forever: its clock always shows about 14 days; its list
  ends (0098: the known ones, the first ones again up to 12, then the
  Crystal Coin ones once more; the original list was longer than 12).

  | Quest (screenshots) | Reward |
  |---|---|
  | Play 6 duels | random fragments (ours; "1 Musketeer fragment") |
  | Play 3 rumbles | random fragments (ours; "1 fragment") |
  | Win 3:0 in a duel (3×) | 1–3 random fragments |
  | 6 matches using Royal Champion | 15 Crystal Coins (3 × 5) |
  | Advance on the Adventure Road 5 times | 5 Crystal Coins, 25 Monk fragments |
  | Play 20 duels | 15 Crystal Coins |
  | Win 3 matches using Musketeer | 10 Crystal Coins |
  | Play 10 rumbles | 15 Crystal Coins |

  | Win 10 trophies (0113) | 100 gold |
  | Use Prince to Charge 3 enemies in a single match (0113) | 10 Crystal Coins, 10 gems |

### A4. Crystal Coins (0065, 0074; tested)

- The server counts PromotionToken ("Crystal Coin", at most 9999) from
  chests and shows it in the home.
- Fragments beyond a unit's max level become 1 Crystal Coin each, as the
  client does (0x7d7550; collectibles.json fragmentToRes); fragments saved
  past max level before 0074 turn into coins once at the start.
- Spending them: the Crystal Coins Skin module (0091).

### A5. Skins locked, unlocked as in the original (0088; tested)

- Every account, the existing ones too, starts with only the default
  skins (locked once at the start; a locked skin worn goes back to the
  default). A skin is owned once a chest gives it, and never lost.
- Unlocked by (0088): the road (Plush P.E.K.K.A tier 26, Luchador Monk
  tier 87), the Clashmas quests (Candy Elf, 12 quests).
- The shop's skins (0091; small buttons 0092; tested): Clashmas Special Jolly
  159 gems, Helpful 99 gems; Clashmas Skins (BundleModule) Gift, Sparkle,
  Snowler free; Crystal Coins Skin: Crystal P.E.K.K.A 100,
  Fabulous Prince 600 Crystal Coins. A skin owned shows as bought in
  every day's shop; the server refuses to sell it again.
- The older skins (Celtic Magic Archer, Scientist Electro Wizard,
  Gravedigger Miner, Victory Shield Maiden) were not on sale in December
  2021 (shop chests, so sold before, how is not known): decided, everyone
  owns them (0095).
- The Crystal P.E.K.K.A was missing from our skin list (it was locked
  even in the unlocked collection); added (0088).

### A6. Season 1 road: 90 tiers (0065; tested)

- 90 tiers, 1 / 2 / 3 fuel on tiers 1–30 / 31–60 / 61–90 (180 in all);
  a new Season 1 every two months; Pony Fuel from the daily quests.
- The free lane: the game's season chests; the season's mini on 10, 30, 60.

### A7. Mini Pass free (0065; tested)

- The pass lane is everyone's (+0x4c true), 601 claims both lanes.
- The pass's text: 300+ hero fragments (ours: about 334), 2 seasonal skins
  (P.L.U.S.H. P.E.K.K.A at tier 26, Luchador Monk at 87, wiki), 200 gems,
  2000 gold.

### A8. Season end (every two months) (0096-0109; tested)

- When the season key changes (dev mode: at every start):
  - the road's tiers reached and not claimed come in the star chest (both
    lanes; 418 in the original, see below);
  - the star exchange as in December 2021 (decided: the choice of March
    2022 has no screen in our client): every mini back to level 1, for 6
    or more stars the tier's chests of season_rewards.csv (the three
    EndStars_Min_*_Cost with mini fragments and EndStars_1..9: 500-4000
    gold, 10-100 gems, up to 5 Crystal Coins, fragments);
  - trophies from Diamond 3 up back as in leagues.json (Diamond 3 / 2 to
    700, Diamond 1 800, Master 900, Grandmaster 1000, Legendary 1100);
  - Season 1 starts again.
- Mini quests at once on unlock (0099) crashed the client: the 417 it
  sends replaces the events while the season manager keeps a pointer to
  the old ones (0x932d98, linked again only with the flag 0x118abd8).
  Off again (0100); the quest comes with the next home. To find: how the
  original did it without a crash.
- Tested (7 November 2026): minis back to level 1, the road and star
  chests. Dev mode sets the trophies to 950 before its season end, to see
  the reset (0099).
- The popup "Season has ended!" / "Total Mini Stars" (0101): as in the
  original the client does the exchange with LogicSeasonEndCommand 451
  (VInt, Int new season, Int its event, Long, Long, the server command
  base; execute 0x95918c -> 0x48a4dc). It runs only while the avatar's
  season key (+0x44, was always 0) is lower than the new one; then the
  client counts the stars (characters.csv ModifierLevel 2/3/4 reached),
  makes one chest of the season_rewards.csv tier (source 9), sets the
  minis to level 1, resets the trophies and shows the popup. The server
  claims the road at the change (the chest is in the home), sends the
  home with the old key, levels and trophies, then 451, does the same as
  the client and pushes the star chest's content (420). Unknown: the two
  Longs (0 sent).
- 0101 on the phone: 451 ran (the client echoes it), but right after a
  home with 13 chests: the trophy reset's league screen and the chests'
  popups ran into each other, the home got stuck and the minis were not
  drawn until the screen was left. The dev account had piled up chests
  (a 90-tier road chest with every dev start). 0102: 451 waits until no
  chest is left in the home (the road's chest first), and dev mode ends
  the season only with no chest waiting.
- 0102: the road's chest came before the popup. 0103 tried the
  original's LogicClaimSeasonRoadRewardsOnSeasonEndCommand 418 (VInt list,
  VInt season, VInt source 5, VInt season, Bool + content; execute
  0x53338c, the ended season as a second road manager entry): popup, then
  nothing. 0104: the road's tiers into the star chest instead (under 6
  stars: a chest with the next home): the chest stayed empty, as the 420
  sent with 451 right after the home was never executed (in 0103 the
  same, so 418 may not have been at fault). 0105: the star chest's 420
  waits for 451's echo: works (tested). 0106: silent exp chests (whose
  opening the client reports only with its next command) no longer hold
  the season end back (tested).
- 0107: after the star chest, 21 exp chests were pushed at once; the
  client opened 20 in one go, levelled up four times and its level-up
  popup (which opens only the current level's chest, 0x45fd8c) got stuck.
  Exp and level chests went one at a time (the next after the last
  one's 550, a level chest first); a past level's chest comes in the home
  as an ordinary chest. On the phone stuck again: the client opens its
  chests one after the other and waited for the next exp chest's content
  while the level-up popup was up.
- 0108: all contents at once again, but only one level-up comes of an
  upgrade batch: the exp chests after the one that reaches the next level
  give nothing, their exp is held back (WithheldExp) and goes into the
  level chest, as much as the next level takes, so every popup is one
  level.
- 0108 on the phone: hung again, and the log shows why (also for 0106 and
  0107): the server made one exp chest more than the client (19 to 18),
  so the client's level chest had the index of the server's last exp
  chest and got its content. The client's season end takes the minis'
  fragments too (0x48aaa0), the server kept them and upgraded once more.
  0109: the fragments go on the server as well (tested 7 November 2026:
  star chest, several level-ups, no hang).
- 0111 (418 for the road's rewards, apart from the star chest) did not
  work on the phone and was dropped (decided: not a goal); the road's
  rewards stay in the star chest.
- 0112: mini quests at once on unlock again: 417, and the hook links the
  season manager to the new events after it (docs/LIVE_CONTENT.md). To
  test.
- 0113: the two Clashmas quests left out are in: "Win 10 trophies"
  (MetaResourceGained; the live data's key is ResourceData, 0x546b0c, and
  0075's "Resource" left the hint without a resource) and "Use Prince to
  Charge 3 enemies in a single match" (AbilityHitInOneGame: Unit,
  AbilityMask with CountStartUp 4, HitsReqInOneGame; ours: a match of 3
  rounds with the Prince in the deck counts, as a battle copies no group
  quests). "Win N Trophies" is back in the daily pool. To test.
- 0114: the client's other quests: the tutorial chain for new players
  (player level 3 or lower, once: deck with Magic Archer or Shield
  Skeleton, a match using Wizard, a hero upgrade, a ranked match; under
  MINI QUESTS), the competitive chain ("Win 5 rounds in Duel or Rumble",
  1 Crystal Coin, again and again; its own section) and "Use 100 Crystal
  Coins" in the daily pool (home/chains.go). Ours: the tutorial chain's
  rewards and who gets it, the amounts. Tested: the competitive chain and
  the Clashmas quests work.
- 0116: a chain's next step keeps the quest's id, as the client does (the
  hero upgrade step could not be claimed: the client claimed id 18, the
  server had given it 19; it came back with the next home). Mini quests
  the same. The deck step counts any change of the deck, as the client
  does (MetaChangeDeck has no unit); its text now names a mini the player
  has. To test.
- 0117: mini quests at once dropped again: even with the hook's relink
  the client crashed 4 s after the 417. 0118: on again; the hook keeps the
  old and the command's events from being freed (docs/LIVE_CONTENT.md).
  On the phone: no crash; but quests with ids the server did not know
  (claiming one left a chest without content and the home stuck), mini
  quests done for minis not played, the deck step done with any mini.
- 0119: the quest manager's +0xb0 is the id the client gives its next
  quest (0x605da0, 0x84fba0: the quest gets +0xb0, then +0xb0 + 1); we
  sent the last id, so every quest the client handed out itself had the
  id of the server's last one (also 0116's "no finished quest", wrongly
  read as "the client keeps the id"; 0116 undone). Now the last id + 1.
  Unit quests (mini quests, the tutorial's units) only the client counts
  (the hook reports them); the server counted by the deck. The tutorial's
  step 1 is a match using Magic Archer or Shield Skeleton (PlayWithUnit,
  with the same text): MetaChangeDeck counts any change. Tested: works,
  but the deck step needed a match.
- 0120: the deck step is done at once when the deck gets its mini: the
  server marks it and tells the client with LogicUpdateQuestProgressCommand
  351 (base, Bool, VInt count, the quests as in the home; execute
  0x7249e4 -> 0x750b9c updateQuest by id). Offline mode done after this
  (7 November 2026). Next: the shop's gold and gems at its end, then the
  Mini Room. The deck step counts any deck change also
  in the original (its tooltip: "Modify your deck to complete this
  Quest"; the tutorial's deck builder guides the player to the new mini).
- 0121: the currency shop at the end, as the original's "Gold Rush" and
  "Gems Galore" (PermCurrencyModule, the game's shop_gold_* and
  shop_gems_* chests): 1000 / 2500 / 8000 gold for 60 / 150 / 450 gems
  (10 a day), 50 / 300 / 1000 gems free (real money in the original),
  once a day. The packs and prices are ours. 0125: the gem packs cost
  5000 / 25000 / 75000 gold (decided: free they made gems worthless; a gem
  costs 75-100 gold and gives 16-18 in the gold packs, so going round
  does not pay).
- Tested 7 November 2026: everything from 0112 to 0121 works on the
  phone. The offline mode is done; next: the Mini Room (A10).
- 418 (the road's rewards apart from the star chest): not a goal (0111
  dropped).

### A9. News tab (dropped)

Not built: the tab stays as the client shows it without messages.

### A10. Mini Room with real players (next)

Decided (7 November 2026): way A (local network) and way B (a relay), so
that we can play in the same Wi-Fi and with people anywhere. Mini Room
battles count for neither quests nor ranked (as in the original).

How it fits together: the client only ever talks to its own server (the
one in the APK, 127.0.0.1); it cannot reach another phone. Every phone
keeps its own server and account; a meeting point passes the room and
the battle between the two servers. Battles are lockstep (both clients
simulate, checksums 0x994008): the meeting point hands out the start
(SectorState) and passes the players' commands (SectorCommand 16634); it
does not simulate. The client takes a separate battle address and keys
from UdpConnectionInfoMessage 23036 (decoder 0x7f93f0: a VInt, a string,
an int, bytes), so the battle may go to another host than the home.

1. Understand the client:
   - the room messages: Create / Join / Leave / StartGameRoomMatch
     (10181, 10186, 14761, 10190), their responses (24566, 24568, 24571,
     24573), GameRoomMembers 20188, GameRoomShutdown 24572;
   - SectorState 22681 for two real players (an earlier server-built
     battle kept the tutorial's rules, reference/SERVER.md), SectorStateOk
     17239, RequestSectorState 17918;
   - UdpConnectionInfo 23036 and the UDP channel (PepperPerMessageEncrypter,
     reference/SERVER.md), or whether battles also run over TCP;
   - whether the client needs the server's checksums or plain passing on
     is enough.
2. Way A, local network: one phone is the host; its server also listens
   in the Wi-Fi and is the meeting point. The other phone's server
   connects to it and passes its client's room and battle messages on.
3. Way B, relay: the same meeting point as a small program of its own
   (Go, cmserver/relay) for a VPS or PC reachable from the internet. Both
   phones connect out to it (phones behind NAT cannot be reached from
   outside). It keeps no accounts and no game data, only rooms in memory,
   and passes messages. An access code for the group. One small VPS (4-5 EUR
   a month) is plenty for a group of friends; an alternative without a
   relay: a VPN such as Tailscale, then way A over it.
4. In the app: be the host / join a host (address) / relay address and
   code; a guide for setting up a VPS.
5. Later, only if needed: several relays and a lobby (room code -> relay).
   A relay does not simulate and only passes small messages, so one small
   server carries thousands of players; beyond that, more relays, each
   room on one of them.

Much later: real PvP (matchmaking, ranked) on top of the same relay.

## Part B: TODO, not known yet (ours until found)

Where to look: videos and screenshots from December 2021 to February
2022 (YouTube "Clash Mini Mini Pass all rewards", the Clashmas update's
videos, r/ClashMini), the wiki's Mini Pass page as text.

- **B1. The road per tier**: which reward on each of the 90 tiers, the
  fuel per tier, the free / pass lane split (ours in 0065).
- **B2. All Clashmas quests** and their rewards (10 known, 8 in the
  game, more than 12 in all). Where: videos of the quest screen from 21 December 2021 into
  January 2022 (the group shows a few quests at a time, so
  each video adds some), screenshots on r/ClashMini and Discord.
- **B3. Mini quest rewards**: which fragment and how many.
- **B4. Star exchange in detail**: the screen and how the
  EndStars_Min_2/3/4_Cost refunds count (if the client does not show it).
- **B5. Daily quests**: all kinds and which give 100 instead of 10–30
  gold.
- ~~B6. Crystal Prince skin~~: its chest (shop_skin_PrinceCrystal) gives
  the Fabulous skin (44000089); there is no Crystal Prince skin.
- **B7. Clashmas Skins bundle**: "+160% extra" hints at more than the
  three skins (gems, gold?).

## Later

- ~~Help & Support~~ (0067, tested): opens our own help page
  (cmoffline/help/help.html, built into the APK's locales.csv); keep its
  "New" list up to date with each patch, but short: as a URL the page must
  stay under 8192 characters, at 8256 the client crashed at its start
  (0115; patch_apk.py stops the build above 8000).

Then: decide together whether and how to publish.

## Sources (roadmap)

- Screenshots of our version (December 2021): docs/ORIGINAL.md.
- gamea: http://clashmini.gamea.co/c/4nvp78h6 (27 April 2022)
- gamingonphone: https://gamingonphone.com/guides/clash-mini-beginners-guide-and-tips/ (May 2022)
- digistatement: https://digistatement.com/clash-mini-daily-quests-mini-quests-details/
- noff: https://noff.gg/clash-mini/faqs/seasons
- deconstructor: https://www.deconstructoroffun.com/blog/2022/1/14/clash-mini (January 2022)
- taptap: https://www.taptap.io/post/3932898 (December 2022)
