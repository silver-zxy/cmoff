# Protocol (1.1143.2)

All addresses are Ghidra addresses of `libg.so` (image base 0x100000).
Message ids are shuffled per build; the full table is [ids.tsv](ids.tsv).

## Frames

TCP, port 9339. Every message: 7-byte header in the clear, then the body.

    id u16 BE | length u24 BE | version u16 BE | body

After the login, bodies are encrypted but headers stay readable; the hook
relies on that ([CLIENT_HOOK.md](CLIENT_HOOK.md)).

## Server key

libg does not store the server public key; it rebuilds it at load time from
two halves (16 little-endian u16 limbs, Ghidra 0x83ec4c):

    pub_i = (rotr16(k0_i, 3 + i%8) - k1_i) mod 2^16

`k0` is in the clear in `.rodata` (file offset 0xBC3DE0), `k1` is in the
encrypted `.data` strings (file offset 0xF7242C, decrypted:
`63137ada…a4e41d`). libg checks its own code and strings, so only `k0` may
change. `patch_apk.py` writes

    k0_i = rotl16((pub_i + k1_i) mod 2^16, 3 + i%8)

so the rebuilt key is ours. (Writing our key raw into `k0` — an early bug —
makes the client encrypt to a key nobody holds.)

## Handshake (Pepper)

    ClientHello 10100  clear: int protocol(2), int keyVersion(72), int major(1),
                       int minor(0), int build(1143), string contentHash,
                       int deviceType, int appStore
    ServerHello 20100  clear: int length(24) + 24 random bytes (snonce)
    Login 10101        client_pk(32) | crypto_box(snonce | cnonce(24) | payload)
                       box nonce = Blake2b-24(client_pk | server_pk)
    LoginOk 20375      crypto_box(rnonce(24) | key(32) | payload)
                       box nonce = Blake2b-24(cnonce | client_pk | server_pk)
    then               XSalsa20-Poly1305 secretbox with `key`; the server seals
                       with rnonce and opens with cnonce, each side adds 2 to
                       its 24-byte little-endian nonce before every message

`crypto_box` is the NaCl one (X25519 + HSalsa20), i.e. Go's `nacl/box`. Every
client frame after LoginOk must be opened in order, even ignored ones, or the
nonces drift (`srv/server.go`).

Login payload (0x31d45c): long accountId, string passToken, vint major, minor,
build, string resourceSha, then device data (model, language, ...).

## LoginOk (decoder 0x78fdf4)

| Field | We send | Notes |
|---|---|---|
| 2 longs | account id, home id (1) | LogicLong = int low, int high |
| 3 strings | pass token, null, null | the client sends the token back next login |
| 4 vints | major, minor, build echoed from Login, content version 0 | a different version sends the client into an update flow (stalls ~50 %) |
| string | `clashmini` | environment |
| 3 vints | 1, 0, 0 | sessions, play time, days since created |
| 3 strings | null, `US`, null | |
| vint | 0 | |
| 6 strings | null | |
| 3 vints | 0 | |
| 2 string lists | empty | |
| bytes | empty | |
| 3 bools | false | |
| **vint** | **0** | object +0x14c = **days until account deletion**. Non-zero opens `TID_ACCOUNT_DELETION_WARNING` every login (see JOURNAL) |
| string | null | |

Right after LoginOk the server sends OwnHomeData ([HOME.md](HOME.md)); the
client stays on the loading screen until it arrives.

## Messages handled

| Id | Class | Server does |
|---|---|---|
| 10108 | KeepAliveMessage | answers 20108 KeepAliveServerMessage (empty) |
| 17306 | EndClientTurnMessage | applies the commands in it, saves the account; on StartMatchmake sends the battle signal |
| 15967 | GoHomeMessage | Exit after a battle: sends OwnHomeData |
| 17141 | AvatarNameCheckRequestMessage | name dialog OK: answers 28946, saves the name, sends server command 411 |
| 11524 | ChangeAvatarNameMessage | saves the name, sends 411 (this build never sends it in practice) |

Ignored (no answer needed): 17760 (1 byte), 30000 (device info: user agent,
language, version, model), 12768 (latency ping every 5 s: vint ms, "lo"),
10483 (empty, after a matchmake the server never answered).

### AvatarNameCheckResponseMessage 28946 (decoder 0x4515c4)

`bool invalid, int errorCode, string name`. `invalid = false` closes the
dialog (receive handler 0x4549a8, case at 0x45612c → 0x45719c). The client
then sends nothing else, so the server applies the name right away.

### AvailableServerCommandMessage 24449 (decoder 0x5929a0 → 0x486c50)

`vint commandType`, then the command. Used with
**LogicChangeAvatarNameCommand 411** (decoder 0x746b48, execute 0x95088c):

    string name, int nameChanges (avatar +0x138), bool setByUser (avatar +0x135),
    vint (server command base, 0x54794c),
    vint executeTick (-1 = now), vint, vint accountHigh, vint accountLow  (LogicCommand base 0x90ab7c)

## EndClientTurnMessage 17306 and commands

    vint tick, vint checksum, vint count, count × (vint type, command), int -1, byte

Every command starts with the LogicCommand base (encoder 0x8f2a40): two vints
(ticks), then the account id as two vints. Commands are found like messages:
`getCommandType()` is vtable slot 4 (`mov w0,#id; ret`), execute is slot 3,
encode slot 5.

| Type | Class | Body after the base | Server |
|---|---|---|---|
| 500 | LogicSwapChessCommand | vint mini (seasonal row, -1 none), vint slot 0–4, vint slot2 (-1 none) | put in / swap / clear a deck slot |
| 549 | LogicChangeAvatarNameByUserCommand | string name (**before** the base) | set name |
| 582 | LogicStartMatchmakeCommand | vint mode (2 = duel, 3 = Rumble), vint (-1), bool ranked, bool | battle signal; ranked battles count for trophies |
| 583 | LogicSelectSupportHeroCommand | vint hero global id (107xxxxxx) | hero of the selected deck |
| 592 | LogicTroopSkinSelectionCommand | vint character global id (34xxxxxx), vint skin global id (44xxxxxx) | skin per mini/hero |
| 595 | LogicAddTroopToDeckCommand | vint mini global id, bool add | add/remove in the deck |
| 597 | LogicSelectDeckCommand | vint index 0–2 | selected deck |
| 602 | LogicUnlockCollectableCommand | int collectible id, vint current level (encoder 0x6d5ca0) | progression: one level up for fragments (+ gold for heroes) |
| 607, 988 | LogicSkipTutorialCommand, LogicCompleteTutorialHomeCommand | base; 988: tutorial data reference first | tutorial done |
| (server) 420 | LogicClaimRewardCommand in AvailableServerCommand: content (as in the home's chests), VInt chest index, server base, LogicCommand base | | sent for every chest the client makes itself (HOME.md "Chests"); the client echoes it in its next EndClientTurn, which the server reads past |
| 550 | LogicChestOpenFinishCommand: base, VInt chest index | | the server gives that chest's content (HOME.md "Chests") |
| 504 505 506 593 594 596 598 600 604 605 702 | read past (layouts in `srv/commands.go` skipLayouts) | | nothing; before, an unknown command dropped the rest of its batch |

Battle commands (ids 24–53: deploy, reroll, sell, level up, ...) also arrive
in 17306 while the client is online during a battle; the server ignores them
(the battle runs in the client).

## Battle signal

The client's offline duel is started by the hook, not the server
([CLIENT_HOOK.md](CLIENT_HOOK.md)). On StartMatchmake the server sends a
KeepAliveServer (20108) the client never asked for, padded with

    mode + 4 × hold + 8 × bot config + 128 × bot hero level + 1408 × bot mini level
         + 7040 × board

bytes (`srv.BattleSignalLen`; mode 2/3, hold = keep the battle offline
throughout, an experiment; bot config 0–15 by the account's trophies
(`home.BotConfig`); bot levels 0 = the decks' own (bot_decks.csv, by league,
with progression), else every deck at them (10 and 4 for the unlocked
collection); board = the board's locations item + 1 of the bot battle's board, 0 = the
npc's own (`home.BoardFor`)). The decoder of 20108 reads nothing, so the
padding is harmless; the hook reads it from the plaintext frame length
(ciphertext = body + 16).

## Battle result (hook → server)

The client never tells the server how a bot battle ended (GoHome is the same
after a win and a loss). The hook reads it from memory and reports it on a
connection of its own, one plain frame:

    id 19999 (not a game message), length 2, version 0, body: u8 mode, u8 place

`place`: duel 1 = won, 2 = lost; Rumble 1, 2, 3 (3rd–4th), 5 (5th–8th);
0 = left before the end (worst place). After it the hook sends a second
frame, mode 11: count, then (int32 quest id, int32 progress) pairs, the
client's own quest progress (big-endian). The server applies it to the battle
it started last. Ranked battles (StartMatchmake's ranked bool) get the rank
points (trophies) of the league's rank reward chests (leagues.json
"RewardsCompetitive", chests.json RankPoints), e.g.

| | duel won / lost | Rumble 1st / 2nd / 3rd–4th / 5th–8th |
|---|---|---|
| Bronze | +5 / +1 | +10 / +6 / +4 / +1 |
| Diamond 3 | +5 / −1 | +10 / +6 / +3 / −2 |
| Master | +5 / −2 | +10 / +6 / +3 / −4 |
| Legendary | +5 / −5 | +10 / +6 / +3 / −10 |

Every finished battle, ranked or casual, counts for the profile statistics
(a Rumble in the top 4 counts as a win) and gives gold (HOME.md,
Progression). Leagues as in
`csv_logic/leagues.json` (Bronze 3 at 5, Silver 3 at 100, … Master at 1000,
Legendary at 1200).
