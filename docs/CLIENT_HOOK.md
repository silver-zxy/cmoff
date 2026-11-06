# The client hook (`cmoffline/src/cmoffline.c`)

A shared library added to the APK and loaded through a `DT_NEEDED` entry on
`libsentry-android.so`. It never patches code: libg checks its own bytes and
crashes if they change, so everything is done by writing data (GOT slots,
flags, object fields) from a background thread.

Addresses: libg offset = Ghidra − 0x100000 (written `libg+0x...` below).

## Start-up

1. Wait for `libg.so`, check its Build ID `128b8fb1…98f8df`.
2. Standalone builds (`-DCMOFFLINE_LOCAL_SERVER`): `dlopen("libcmserver.so")`
   and call `CmServerStart(filesDir, SERVER_KEY_HEX)`; the server listens on
   127.0.0.1:9339 and keeps `accounts.json` in
   `/data/user/<user>/<package>/files`.
3. Replace libg's GOT slot of `getaddrinfo` (libg+0xdf4fc8) so
   `game.prod.clashminigame.com` resolves to 127.0.0.1.
4. Hook libg's GOT slots of `read` 0xdf4c08, `recvfrom` 0xdf43e8, `write`
   0xdf6d18, `sendto` 0xdf4bf0, `connect` 0xdf55a8, `close` 0xdf49a8. The
   socket that connects to port 9339 is the game socket; the hooks parse the
   plaintext frame headers going both ways.

## Bot battles

The game has its own offline duel against `OfflineMatchNPC` (npcs.csv, shown
as "Jeff") with real bot AI. The hook turns it on at the right moment:

1. Duel/Rumble runs 0x973448, which sets the **offline flag** (libg+0x108abd8,
   read by 0x3b4300) to 0 and executes LogicStartMatchmakeCommand 582, which
   goes to the server.
2. The server answers with an unrequested KeepAliveServer 20108 whose body
   is padded to carry mode, bot config and bot levels
   ([PROTOCOL.md](PROTOCOL.md#battle-signal)).
3. The hook sees it (frame length − 16 = mode), sets the offline flag to 1 and
   writes the match into the game mode object, as listener 0x59e8a0 would:
   `mode = *(GameMain + 0x20)`, `mode+0x15c = mode`, `mode+0x160 = 0`.
4. Home update 0x467e78 sees +0x15c and builds the offline battle (0x940664).
5. One second after the game state becomes 5 (battle) the hook clears the
   flag. Exit then sends GoHome and the server answers with OwnHomeData; with
   the flag still set the client would load its local offline save (empty deck).

`GameMain = *(libg+0x1078728)`; game state = `*(int*)(GameMain+0x28)`
(0x4e68f4: 0 start, 2 loading, 4 home, 5 battle); game mode getter 0x5952bc.
The hook logs every state change.

Keeping the whole battle offline (`-battle-online=false` on the server) was
tried: it changed nothing in the battle and showed "connection lost" on Exit,
because the hook has to drop the connection to get the home back.

### Bot strength

`OfflineMatchNPC` (npcs.csv) uses `BotConfigIndex 0`: the tutorial setup
(tutorial decks, `BotDynamicDifficulty`, keeps elixir back every round), so
every match felt the same. Right before each bot battle the hook writes a
real config into that cell of the loaded CSV table; the battle builder reads
it at 0x940c40. The server picks the config by the account's trophies and
sends it with the battle signal (PROTOCOL.md): config n plays with the decks
of league n−1 (2 = Bronze, 5 = Silver 3, 8 = Gold 3, 10 = Diamond, 13 =
Master and above); the low ones also use the beginner behaviours. Without a
config in the signal the hook picks one at random from `BOT_CONFIGS`
(default 5–10).

- npc table `*(libg+0x10807d8)` (by name 0x92198c; vfunc +0x30 count, +0x38 item)
- item 3 = OfflineMatchNPC; npc +0x10 = CSV row (table at +0, row index at +8)
- `table+0x10` = columns, `column+0x18` = int cells (0x535e08 → 0x72c69c → 0x817a60)
- column index of BotConfigIndex: global libg+0xf57740 (loader 0x8c2b44)

**Boards.** A bot battle plays on OfflineMatchNPC's board (npc +0x38, a
LogicLocationData resolved at load by 0x47d8dc). The signal's board
(the board's item + 1 in the locations table `*(libg+0x10807c0)`: Windy
Plains 0, Banquet Hall 1, Electro Valley 2, Healing Temple 3, Fish Town 4)
makes the hook point it at that item; 0 puts the npc's own board back
(saved the first time). Not by CSV row: each board spans two CSV lines, so
item n is on line 2n (0072 looked them up by row and put Banquet Hall in
Silver).

**Bot levels as in the original.** Bot decks carry their own levels by
league (Bronze: heroes 1–4, minis 1–2; Diamond 6–7; Grandmaster and the FF
decks 10 and 4). With progression the server sends 0 and the decks keep them.
With the unlocked collection (the player maxed) it sends 10 and 4 and the
hook sets every deck to them; it saves the decks' own levels first and puts
them back when a later signal says 0. The bot_decks table (class 133) is
`*(libg+0x1080b70)`; its deck objects cache the levels at load (constructor
0x5b49ac): hero level +0x40, mini levels int list +0x58 with count +0x64.
The generated decks (`BOT_DECK_NORMAL` / `NEWBIE` / `RANDOM`, built from the
player's collection) are not in the table and not touched.

The npc's `Seed` is never read (getSeed 0x8344dc has no callers); the battle
seeds `srand` from `clock_gettime` itself. Build with e.g.
`-DBOT_CONFIGS=10` for always Diamond 3. Indexes above 10 are ambiguous
(by position 13 is "Fan", by name "Master") and untested.

## Battle result

The server cannot see who won, so the hook reads it the moment the end
screen exists (later the battle is torn down and reads as zeros) and reports
it to the server (PROTOCOL.md, "Battle result"):

- battle screen `*(libg+0x1085428)` (0x5b7f84); end screen at +0x1a0
  (created by 0x8ebf68)
- logic battle `*(*(*(screen+0x100)+0x68)+0x98)` (0x8dcb90): players +0x18
  (count +0x58, id at +0; bots have negative ids), player states +0xb0
- duel, as the end screen 0x945098 decides: won when rounds won
  (state+0x178) reach rounds needed (`*(battle+0x108)+0xe0`)
- Rumble (found by diffing all states over four Rumbles): state+0xb4 last
  stage reached (3 = last 8, 4 = last 4, 5 = final), state+0xc0 stage the
  player was knocked out in, −1 = never (champion)

The battle's own winner index (+0x98, `getWinnerAvatar` 0x94f4d0) stays 0
offline. All reads go through `process_vm_readv`, so a stale pointer cannot
crash the game. Leaving a battle before the end screen reports the worst
place.

## Tutorial

With tutorial flags 0 the client goes straight into the tutorial's NPC
battle at init (LogicTutorialManager::onInitStateCheckIfGoToBattle 0x878a38
→ 0x588b88) by sending StartPvEMission 15409. Online that waits for a
server-run battle (SectorState) and loading stops at 50 %. Offline, the
message layer (0x87641c) handles 15409 itself and builds the battle with the
same builder as the bot battles (0x940664).

So for a new player the server sends a signal with mode 1 right after
LoginOk, before the home. The hook keeps the offline flag on through the
first battle: turning it off 1 s in, as for bot battles, crashed the client
in battle code (0x6de600 reading a player state's +0xf8). The tutorial battle has
no end screen (state 5 goes straight to 4), so the hook reconnects after it:
the client logs in again and gets the server's home. (Staying offline left
the client in a home it made itself, without a hero; pushing a home unasked
crashed it with a double free.)

Closing the socket makes the client's network update (0x4f919c) show the
"Connection lost" popup: showError 0x8ff948(gui, 4, ...) keeps the type at
gui+0x124 (gui = `*(Ghidra 0x1198a28)`, getter 0x791c98, vtable Ghidra
0xec6f00). Its Reload button runs the GUI's vtable+0x18 (0x6dd634; the popup
handler 0x6dee70 sends type 4 to its default case), which sets gui+0x193;
the GUI's update (0x6246b0) then reloads the game (0x9b20f0), which also
closes the popup (0x7dbe00). The hook presses that button itself as soon as
gui+0x124 is 4 (`watch_reload`, log "reload: ... pressed its button"), so
the game reloads on its own; the popup shows for a frame at most.

The starting hero is picked at the start of that battle (ChooseStartingHeroHUD,
battle command LogicSelectStartingDeckCommand 52, execute 0x611fb8). The
execute finds the player index with 0x7553fc and the player's battle avatar
with 0x603108 = 0x4c98f0: `*(*battle + 0xc8 + index*8)`, and stores the
StartingChoiceIndex at avatar+0xb8 (0x545b0c; 1 Barbarian King, 2 Archer
Queen, 3 Shield Maiden). (A first version read +0xb8 of the player *states*
at battle+0xb0, where both players show 3 all along.) The hook logs every
battle avatar's id (+0x24) and value when they change and reports the local
one's (non-negative id) when the battle is over (result frame mode 9, place =
support_heroes row); the server gives the base collection with that hero
(the starter deck's five minis at level 1, the chosen hero at level 1, the
starter gold; `home/chests.go` StartWithHero; the hero's starting chests of
starting_choices.csv are not opened) and sends tutorial flags
0x200 (the home part) from then on, until 607, 988 or the name end the
tutorial. Tutorial flags seen offline: 0x1 first battle, 0, 0x200; bit 0x100
= done (0x5b073c). Bot-battle results are only reported for battles the
hook started.

## Renaming

The name button depends on the global `ENABLE_NAME_CHANGE` (LogicGlobals
+0x111, loader 0x4b0790). `globals.csv` has no such row (the live server
switched it on), so it is false. The hook keeps it set: `LogicGlobals* =
*(libg+0x1080760)` (getter 0x933020). The cost table `NAME_CHANGE_GEM_COST`
is `[0]`, and the server always sends 0 name changes, so renaming is free.

## Help and Support

The settings button (settings popup +0x140, click 0x7338a0) opens the
Helpshift chat only with a Helpshift manager (`*(libg+0x1097628)`, global
`HELPSHIFT_ENABLED` LogicGlobals +0x268, which only the live server set).
Without it (0x733a1c) it opens the in-game web view (0x4bc340, "Loading url
%s", the URL plus `?time=<seconds>`) at the locale's help URL (0x48be78 →
`csv_client/locales.csv`, columns HelpshiftURL / SelfhelpURL). The build
(`patch_apk.py --help-page`) puts our page there, `cmoffline/help/help.html`
as a `data:` URL: no network, no cleartext HTTP (blocked for targetSdk 30);
the page ends in an open comment, so the appended `?time=…` stays hidden.
The table's fingerprint.json entry gets the new SHA-1. "Close" links to
`selfhelp://close`.

## Crash logger

Unless built with `-DCMOFFLINE_NO_CRASH_LOGGER`, SIGSEGV/SIGBUS/SIGABRT are
logged with `pc`, `lr` and the frame chain as Ghidra addresses, then passed on.

## Build switches

| Define | Effect |
|---|---|
| (none) | redirect + battles + renaming |
| `CMOFFLINE_LOCAL_SERVER` + `SERVER_KEY_HEX="…"` | also start the in-game server |
| `CMOFFLINE_NO_BATTLES` | redirect only |
| `CMOFFLINE_FORCE_OFFLINE` | old client-only mode: offline flag always on, no server (empty deck) |
| `CMOFFLINE_NO_CRASH_LOGGER` | no signal handlers |
| `REDIRECT_TO="…"` | another server address |
| `BOT_CONFIGS=5,6,…` | bot configs picked at random when the server sends none (default 5–10) |
| `CMSERVER_OPTIONS="progression=fresh"` | options for the in-game server (build.sh `PROGRESSION=`) |

## Offsets (1.1143.2)

| What | Where |
|---|---|
| GOT getaddrinfo / read / recvfrom / write / sendto / connect / close | libg+0xdf4fc8 / 0xdf4c08 / 0xdf43e8 / 0xdf6d18 / 0xdf4bf0 / 0xdf55a8 / 0xdf49a8 |
| offline flag | libg+0x108abd8 (Ghidra 0x118abd8) |
| GameMain* | libg+0x1078728 |
| LogicGlobals* | libg+0x1080760 |
| game state / game mode | GameMain+0x28 / GameMain+0x20 |
| queued match | mode+0x15c (int mode), mode+0x160 (byte) |
| ENABLE_NAME_CHANGE | LogicGlobals+0x111 |
| battle screen / end screen | libg+0x1085428 / screen+0x1a0 |
| logic battle | `*(*(*(screen+0x100)+0x68)+0x98)` |
| server key half k0 | file 0xBC3DE0 (patch_apk.py) |
