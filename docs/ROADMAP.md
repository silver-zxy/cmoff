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
- Tested on the phone (November 2026): boards by league (A0), bots with
  their decks' own levels (A1), the shop with the original prices (A2),
  daily quests six at once, the battle kinds counted by the client
  (0079), mini quests with the next step right after a claim (0083),
  Crystal Coins (A4), the Adventure Road with 90 tiers and the free Mini
  Pass (A6, A7), the help page.

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
| Clashmas Special | Jolly 159 gems, Helpful 99 gems, "Clashmas Skins" (Gift, Sparkle, Snowler) | with A5 |
| Crystal Coin skins | Crystal P.E.K.K.A 100, Fabulous Prince 600 | with A4/A5 |

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

Still to do:
- Clashmas quests: the quest group (event type 6) with the Candy Elf
  (shop_skin_SpearGoblinElf, icon_xmas_quest); the 6 known:

  | Quest | Reward |
  |---|---|
  | Win 10 trophies | 100 gold |
  | Play 6 duels | 1 Musketeer fragment |
  | Play 3 rumbles | 1 fragment |
  | Win 3:0 in a duel (3×) | 1–3 random fragments (quest_daily_Mystery) |
  | Prince charges 3 enemies in a match | 10 Crystal Coins, 10 gems |
  | 6 matches with Royal Champion | 15 Crystal Coins (3 × 5) |
  | Advance on the Adventure Road 5 times | 5 Crystal Coins, 25 hero fragments |
  | Play 20 duels | 15 Crystal Coins (3 × 5) |

  The group's header: "Finish 12 Clashmas Quests to unlock Candy Elf
  Skin!" (TID_QUEST_GROUP_PROGRESS_TOOLTIPS); the screenshot shows
  12 days 11 hours left (its date unknown, so the end too). There were more
  than 12 quests: after 12 done the group went on (a screenshot at 12/12
  still shows open quests). The quests themselves were server data, not
  in the APK (only the group's texts are); see B2.

### A4. Crystal Coins (0065, 0074; tested)

- The server counts PromotionToken ("Crystal Coin", at most 9999) from
  chests and shows it in the home.
- Fragments beyond a unit's max level become 1 Crystal Coin each, as the
  client does (0x7d7550; collectibles.json fragmentToRes); fragments saved
  past max level before 0074 turn into coins once at the start.
- Spending them: with A5.

### A5. Skins locked, unlocked as in the original

- Locked at the start (now all open). Decided: every account, the
  existing ones too, starts with the skins locked as in the original;
  they are unlocked again through the road, the quests and the shop.
- Unlocked by: the road (Plush P.E.K.K.A, Luchador Monk), the Clashmas
  quests (Candy Elf), the shop (Jolly, Helpful, Crystal P.E.K.K.A,
  Fabulous Prince, the Clashmas Skins bundle).
- A skin owned is never lost; what the client does with one it owns
  ("You already own this Skin!") and how the home holds owned skins: to
  check.

### A6. Season 1 road: 90 tiers (0065; tested)

- 90 tiers, 1 / 2 / 3 fuel on tiers 1–30 / 31–60 / 61–90 (180 in all);
  a new Season 1 every two months; Pony Fuel from the daily quests.
- The free lane: the game's season chests; the season's mini on 10, 30, 60.

### A7. Mini Pass free (0065; tested)

- The pass lane is everyone's (+0x4c true), 601 claims both lanes.
- The pass's text: 300+ hero fragments (ours: about 334), 2 seasonal skins
  (P.L.U.S.H. P.E.K.K.A at tier 26, Luchador Monk at 87, wiki), 200 gems,
  2000 gold.

### A8. Season end (every two months)

- Optional star exchange (the best variant, from March 2022; in our
  version it was forced): season_rewards.csv by total stars (6..50):
  gold 500–4000, gems 10–100, Crystal Coins 0–5, fragments. How the
  client counts and shows it: read from the code (B4 if not there).
- Road rewards not claimed come automatically (418).
- Trophies reset as in leagues.json: from Diamond 3 up back to
  700 .. 1100; below Diamond nothing changes. No rank rewards (none in
  1.1143.2).
- The new season's popup (451); Season 1 starts again.

### A9. News tab (dropped)

Not built: the tab stays as the client shows it without messages.

### A10. Mini Room / PvP

Rooms (10181/10186/14761/10190, 20188), a battle for two real players
(SectorState, relayed SectorCommand 16634, UDP 23036); Mini Room battles
count for neither quests nor ranked (as in the original).

## Part B: TODO, not known yet (ours until found)

Where to look: videos and screenshots from December 2021 to February
2022 (YouTube "Clash Mini Mini Pass all rewards", the Clashmas update's
videos, r/ClashMini), the wiki's Mini Pass page as text.

- **B1. The road per tier**: which reward on each of the 90 tiers, the
  fuel per tier, the free / pass lane split (ours in 0065).
- **B2. All Clashmas quests** and their rewards (9 known, more than 12
  in all). Where: videos of the quest screen from 21 December 2021 into
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
  "Neu" list up to date with each patch.

Then: decide together whether and how to publish.

## Sources (roadmap)

- Screenshots of our version (December 2021): docs/ORIGINAL.md.
- gamea: http://clashmini.gamea.co/c/4nvp78h6 (27 April 2022)
- gamingonphone: https://gamingonphone.com/guides/clash-mini-beginners-guide-and-tips/ (May 2022)
- digistatement: https://digistatement.com/clash-mini-daily-quests-mini-quests-details/
- noff: https://noff.gg/clash-mini/faqs/seasons
- deconstructor: https://www.deconstructoroffun.com/blog/2022/1/14/clash-mini (January 2022)
- taptap: https://www.taptap.io/post/3932898 (December 2022)
