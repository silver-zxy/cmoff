# Journal: how it got here

Short records of each problem, how it was found and what fixed it. Dead ends
are kept on purpose: they save the next person the time. Addresses are Ghidra
addresses of libg.so 1.1143.2.

## 1. Login never decrypted — the server key (k0/k1)

About 400 combinations of nonce/key derivations failed to open Login. The
crypto was right; the patched key was not. libg rebuilds the server key from
two halves (`pub_i = rotr16(k0_i, 3+i%8) − k1_i`), and the old patcher wrote
our key raw into k0. The client then encrypted to a key nobody had. Fix:
solve for k0 (`patch_apk.py solve_k0`). k1 came out of the decrypted strings.
The handshake is plain NaCl `crypto_box`, see PROTOCOL.md.

## 2. OwnHomeData: from emulation to a port

First homes were produced by running libg's own encoders under Unicorn and
copying the bytes. That decoded, but hand-filled sub-objects crashed the
client at ~87 % (`0x60add4` reading `[[home+0x140]+0x10]`). With .gnoks's
`own_home.rs` the encoder was ported to Go field by field; `diff_test.go`
proves it byte-identical with his 371-byte body.

Loading progress by account content (empirical): minimal body 50 % (and the
client sends 15409 and waits), avatar fields 81 %, a hero collectible 87 %,
plus the LoginOk version echo → lobby. **The fuller the account, the further
it loads**; the final account owns everything.

## 3. LoginOk version

A LoginOk with a different version (we sent 1.0.0) sends the client into an
update path that stalls the loader. The server now echoes major/minor/build
from the Login payload. Environment string `clashmini` (`prod` is Clash
Royale's).

## 4. "Account will be deleted" on every login

`TID_ACCOUNT_DELETION_WARNING_TITLE/TEXT` over the lobby, OK → reconnect →
again. Black-box tests (LoginOk booleans, environment, the three activity
vints, the home tail, full collection, echoing 15409) changed nothing. Static
chain:

1. Dialog builder 0x8ff948(obj, type, subtype): jump table at 0xcbadd8, type
   19/20 = deletion warning (18 = deleted due to inactivity).
2. Caller with type 19 at 0x457f94 reads `0x5ac470(x19)` = `*(int*)(obj+0x14c)`;
   0 → no dialog.
3. x19 comes from the central receive handler 0x4549a8; walking the CFG back
   from 0x457f50 the last message-id compare is `cmp w21, #20375` → LoginOk.
4. LoginOk object (ctor 0x5e1440, 0x160 bytes): the vint after the three
   booleans lands at +0x14c.

We sent the server time there. It is **days until deletion**. Sending 0 fixed it.

## 5. Saving

Commands come in EndClientTurn 17306 (layouts in PROTOCOL.md). The server
applies swap/hero/deck/name/skin commands to `accounts.json` and builds every
OwnHomeData from it.

## 6. Bot battles

Pressing Duel sent StartMatchmake (582), then after 2 s a 10483 and a
disconnect: nobody answered. The recipe from .gnoks's PORTING.md §6 (the
game's own offline duel, started by the hook on an unrequested
KeepAliveServer) was implemented in the hook with GOT hooks on the socket
calls, so the hook can read frame headers. The mode is passed as padding in
the signal's body, which is how Rumble works too. Game state is at
GameMain+0x28 (the notes said +8).

## 7. "You have deployed all of your Minis!" (empty battle hand)

`TID_BATTLE_DECK_EMPTY` after five deploys, so no upgrades. Keeping the whole
battle offline did not help (and broke Exit). Two server-side experiment
switches were added; `-slot-value 0` fixed it: the int after each
current-deck slot (slot +8, encoder 0x6ab6c0) must be 0, which is what the
client writes when it creates a slot (0x7a0d1c). Its meaning is still unknown.

## 8. Hero not saved

583 was saved, but after a reload the client showed Vampire Girl (the first
hero). The worn hero is not the per-deck hero list: 583's execute
(0x5a6888 → 0x7561b4 → 0x933fb8) stores the hero's global id in the hero
collection object +0x10, the vint after the owned-hero list. We sent 0.

## 9. Renaming

The rename button was disabled: `ENABLE_NAME_CHANGE` (globals +0x111) is not
in globals.csv. The hook sets it. With `name_set = false` the first-start
dialog appears; its OK sends AvatarNameCheckRequest 17141 and waits. Found
by building the full id → class table from vtables + RTTI names
(`tools/re/msgnames.py`): the reply is AvatarNameCheckResponse 28946 (`bool
invalid, int, string`). After a positive answer the client closes the dialog
but sends neither ChangeAvatarName 11524 nor command 549, so the server
applies the name on the check and pushes LogicChangeAvatarNameCommand 411 in
AvailableServerCommand 24449.

## 10. Skins

Three wrong turns before it worked, all visible in logs:

1. Skins as collectibles only → still locked. The lock icon and the equip
   command (592, execute 0x8effbc) ask the home's **skin store** (+0x1c0,
   check 0x91dcd8). Found from the text `TID_DECK_SKIN_NOT_UNLOCKED` → xref
   0x84afe0 → callers of 0x91dcd8.
2. The store change silently did nothing: a string replace in the encoder did
   not match after gofmt. Now covered by a test.
3. Selection: we sent 0 (forced the extra skin), then a skin id (always
   default). The entry's +0x18 is an **index** into its skin list (0x8d90d8),
   and choosing default appends the default skin to the list (0x8fcff8). The
   store now lists default first.

Only 13 skins have real rows in skin_confs.csv; the others in
collectibles.json were stripped from the build (empty rows with random names).

## 11. A less predictable bot

The offline NPC uses bot config 0, the tutorial one. The hook now writes a
random Silver–Diamond config into the loaded npcs.csv table before each
battle (CLIENT_HOOK.md). The NPC's Seed column turned out to be dead data;
the battle is already seeded from the clock. The stronger configs brought
bots with hero level 3–4 against our level 10, so the hook also raises every
bot deck to our levels (hero 10, minis 4); the deck objects cache them at
load, so the cached fields are patched, not the CSV.

## 12. Who won? Trophies

GoHome is byte-identical after a win and a loss, so the result has to come
from the client's memory. First try: the battle's winner index (+0x98, read
by `getWinnerAvatar`, found from its "too soon" string): 0 in every test,
offline it is never set. Second: the end screen's own decision (0x945098,
found from the animation names `victory_start`/`winner_start`): rounds won
vs rounds needed for a duel, which was right at once. Read too late (when
the game state leaves battle) everything was zeros, so the hook now reads
when the end screen object appears. Rumble placement came from dumping all
eight players' states over four Rumbles with known places (top 4, 2nd,
champion, out early): +0xc0 is the stage a player was knocked out in.

The hook reports the place on its own connection (id 19999); the server
counts it only if the StartMatchmake was ranked. Trophies are the avatar's
RankPoints (+0x70, found from the profile's `current_trophy` text); leagues
come from leagues.json, and the bot config follows the league.

## 13. Profile, streak and level

The profile showed no battles, the league screen no streak, and the level
stayed 1 (the Mini Room needs level 5). The profile's "battle_status" clip
(found from `txt_duel_win`) reads avatar +0x100/+0x108/+0x120 pairs; the
client's avatar-to-JSON function (0x392f00, found from
`allTypeGameWinLoseStreak`) names every field and its offset, which mapped
the rest. The int pairs are written as deltas (0x720dac), not as two plain
values. The live game gave no exp for battles (BATTLE_END_EXP_REWARD 0); here
battles are the only source, so they give a little.

## 14. Progression

This build has no "upgrade card" command. Minis and heroes are collectibles
(chests/collectibles.json) with levels that rise for fragments, plus gold for
heroes, through LogicUnlockCollectableCommand 602. Its execute (0x6c39d4)
and apply (0x7eadc4) showed where the home keeps fragments and levels, and the
HUD's gold getter (0x598d98) that gold lives in the avatar's first data slot
list, not the third one we had filled. The server now keeps the collection,
hands out gold and fragments directly (no chest animations), and repeats
602. The bot levels follow the player's, sent with the battle signal.

## 15. First test of the progression

Unlocking and upgrading minis worked. The Old Monk did not: the client had
unlocked him by itself from his fragments and asked for level 1 → 2, the
server still had 0 and refused, so the next home took the level back. The
server now catches up. The end screen showed other gold than the server
gave: it shows the league's rank reward chests. Following that, the whole
economy was moved to the game's own data (read from the player's APK at
runtime): gold and rank points from those chests, experience only from
upgrades, level chests with gold and gems. Invented rewards are gone, except
the fragments for wins, which stand in for the shop, quests and season road.

## 16. Tutorial, end to end

The hero picked in the tutorial is read from the battle avatar (+0xb8) and
works. The start is now the base deck (Knight, Barbarian, Archer, Spear
Goblin, Wizard at level 1) with the picked hero, not the hero's starting
chests. The reconnect after the tutorial battle showed "Connection lost"
and waited for the player; the popup's Reload button was traced to the
GUI's reload flag (gui+0x193, CLIENT_HOOK.md "Tutorial") and the hook now
presses it itself (first try: the vtable check used the file offset, the
data segment is mapped 0x1000 higher; the fallback, setting gui+0x193,
reloaded the same way). The tutorial is now on by default.

Small trophy gains (+5 a duel win) are the game's own values. Added from the
game data: the ranked win streak bonus (+3 from the third win in a row) and
the daily limit of 200 battle gold. No experience for battles is original
(BATTLE_END_EXP_REWARD 0); this version has no chests for wins, battle
rewards are the rank reward "chests" shown as gold on the end screen.

## 17. Chests and the daily gold

The ranked screen's "200 gold, refreshes in 23:59" never moved: the home
sent the daily gold state (+0xb4..+0xc4) as zeros, an expired timer, so the
client started a new day on every home. The home now carries the gold left
and the time to midnight.

Chests: the client makes a chest itself for an upgrade (602, 0x7eb554), but
without content, and only opens chests whose content it has (0x313ef4). The
content comes from the server in the home's chest manager; the client opens
it and sends 550. Win fragments, upgrade exp and level chests now go that
way, so they are shown instead of added silently. Server commands found on
the way (by their RTTI names): 373 LogicNewDeliverableCommand, 414
LogicAddChestCommand (VInt chest id, VInt, VInt list, VInt, VInt, base),
442 LogicChangeAvatarScoreCommand, 416 LogicDailyRankingEndedCommand.

First test: the chests were opened (550 came), but silently: the content's
last bool is "show" (0x9443a4 → ChestRewardPopup 0x6bd588; false → 0x8d9c74
sends 550 at once). It is now set from showChestOpening; level chests get
source 6 and the level-up popup (0x8d9c1c).

Next test: still no experience, because there was no 602 at all: after a
chest the client upgrades minis by itself (0x4abb74 from the chest apply,
0x823d1c, through the same upgrade code as 602, which also makes the exp
chest) and tells the server nothing. The server now mirrors that.

Then the exp came, but one home late each step (upgrade → exp chest → level
chest), so it took restarts. The live server filled the client's own chests
with LogicClaimRewardCommand 420 (found by its RTTI name; its decoder makes
the same content object as the home's chests). The server now sends it for
every chest the client makes, so a hero upgraded by hand gets its exp at
once too.

First test of 420: the client echoes the server command in its next
EndClientTurn (as with 411); the server could not read it and dropped the
rest of that batch, the 550 of the same chest. So the chest stayed waiting,
came again with the next home and was counted twice by the client, which
then reached a level the server had not: the level-up popup waited for a
level chest that never came. The echo is read now, and a level chest's
content goes right after the exp chest that reaches the level.

Then the level-up popup still hung until a restart: both 420s arrived and
ran, the exp chest's 550 came, the level chest's never. The popup sends 550
only for the chest of the current level (0x45fd8c, "Invalid current
level"); the client makes it with source 6 and the level in +0x34
(0x82ea2c). Ours had 0 there. It carries the level now. Still hung: the
level chest's 420 went right behind the exp chest's, and ran before the
client had made the level chest (it does when it applies the exp, through
its own 550), so it was dropped. It goes when that 550 arrives now; the
client sends it at once (~10 ms in the log).

## Open

- Bots are always "Jeff" (`OfflineMatchNPC` DisplayName in npcs.csv). A hook
  could rename it per battle.
- Trophies show where the client reads +0x70; the league badge/road and the
  season reset are not driven yet. The exact Rumble places 3/4 and 5–8 are
  not told apart (both count the same).
- Gold/gems show 0; the shop is not driven by this server.
- The meaning of deck slot +8 and of `-mini-level`.
- Real PvP would need SectorState and the UDP battle relay
  (docs/reference/SERVER.md).
