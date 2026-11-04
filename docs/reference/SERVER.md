# Server and network protocol

`crates/cm-server` lets the original client play on your own server. Its
core is the protocol compatibility layer (`compat.rs`): one
`ServerConnection` per TCP client detects which protocol the client speaks
and gives the game logic plaintext messages, so the handlers do not care
about crypto.

Addresses are Ghidra's (image base 0x100000), as in [RE.md](RE.md).
How all of this was found, and how to repeat it for another client
version, is in [PORTING.md](PORTING.md).

## Quick start

```
cargo run -p cm-server -- keygen              # prints the public key, writes cm-server.key
cargo run -p cm-server -- run --addr 0.0.0.0:9339
python scripts/patch_apk.py "Clash+Mini_1.1143.2.apk" out.apk --key <public key> --host <your ip>
cargo run -p cm-server -- probe 127.0.0.1:9339 --key <public key>   # login, home, keep-alive
cargo run -p cm-server -- probe 127.0.0.1:9339 --legacy             # same over legacy RC4
```

`run` options: `--key <file>`, `--major <n>` (refuse other client versions),
`--pepper-only` (refuse legacy RC4), `--legacy-rekey <nonce>` (send
SetEncryption after a legacy login).

## Framing

Every message is a 7-byte header and a body: type id (u16 BE), body length
(u24 BE), message version (u16 BE). The header is never encrypted. The
receive path of `Messaging` (0x606dd8) checks id and length, then decrypts
the body according to its state.

## Message ids

This build shuffles the ids of nearly all messages (LoginOk is 20375, not
the usual 20104). `scripts/message_ids.py` reads them from libg.so through
RTTI and the RELATIVE relocations (`getMessageType` is vtable slot 5). The
table, 252 classes, is `crates/cm-server/src/messages.rs`. Ids that keep the
Titan defaults: ClientHello 10100, Login 10101, KeepAlive 10108,
SetEncryption 20000, ServerHello 20100, LoginFailed 20103, KeepAliveServer
20108.

Ids differ per build, so the layer holds one `Profile` per supported build,
picked from `ClientHello.major`. A build without a profile still gets a
readable LoginFailed ("not supported yet").

## Protocols

### Pepper (what the shipped client uses)

Messaging states: 3 = wait for ServerHello, 4 = Login sent, 5 = encrypted.

1. Client → ClientHello 10100 in the clear: int protocol, int keyVersion,
   int major, int minor, int build, string contentHash, int deviceType,
   int appStore (encoder 0x49f8cc).
2. Server → ServerHello 20100 in the clear: bytes sessionKey (24 random
   bytes, encoder 0x5577fc). In state 3 a LoginFailed 20103 is also read
   in the clear.
3. Client → Login 10101: `client_pk(32) ‖ crypto_box(sessionKey ‖ snonce(24)
   ‖ payload)` with nonce Blake2b-24(client_pk ‖ server_pk). TODO(RE): the
   sender (0x55a1b0) is OLLVM-flattened; this is the standard Pepper layout
   and is consistent with the reply below.
4. Server → LoginOk 20375 or LoginFailed 20103: `crypto_box(rnonce(24) ‖
   key(32) ‖ payload)` with nonce Blake2b-24(snonce ‖ client_pk ‖ server_pk).
   Confirmed by the receiver 0x3d3b68 (blake2b of the 24-byte snonce and the
   64 bytes client_pk ‖ server_pk, then `open_afternm`).
5. From then on both directions are XSalsa20-Poly1305 secretbox with `key`.
   The client encrypts with snonce and decrypts with rnonce
   (`PepperEncrypter`, 0x8e0704); each message first adds 2 to its nonce
   (little-endian, 0x7a4020).

The server's public key and host are compiled into libg.so;
`scripts/patch_apk.py` points the client at your server, see
[Patching the client](#patching-the-client).

### Legacy RC4

The pre-Pepper Titan protocol is still in the client: on connect
`Messaging` gets two `RC4Encrypter`s (0x342b7c) with key
`fhsd6f86f67rt8fw78fw789we78r9789wer6re` ‖ `"nonce"`. RC4 init (0x3111e4) is
the standard key schedule followed by dropping len(key) keystream bytes.
SetEncryptionMessage 20000 (bytes nonce) replaces both with `RC4_KEY ‖
nonce` (0x75d2f0). Bodies are encrypted, headers are not.

The layer treats every connection whose first frame is not ClientHello as
legacy RC4. `ServerConnection::set_encryption` re-keys; call it only while
the client is idle, or its in-flight messages still use the old key.

### UDP battle channel

`PepperPerMessageEncrypter` (vtable 0xe99690), built with prefix 4 and tag
4 by the UDP packet handlers (0x83569c, 0x8e8ef4): packet = 4 random prefix
bytes ‖ the last 4 bytes of the Poly1305 tag ‖ XSalsa20 ciphertext. The
nonce is the 24-byte base nonce XORed with the prefix repeated. Key and
base nonces come from `UdpConnectionInfoMessage` 23036 (not decoded yet).
`crypto::PerMessage` implements the cipher. TODO(RE): the truncated tag
check (0x38537c).

## Handshake message layouts

From the encoders (writeInt 0x79547c, writeVInt 0x4d29b0, writeString
0x5ab65c/0x836210, writeBoolean 0x3257a8, writeBytes 0x686d84; LogicLong =
two ints). See `proto.rs`; field names are borrowed from other Titan games
where the shape matches.

- Login (0x31d45c): long accountId, string passToken, vint major, minor,
  build, string resourceSha, then device data the server keeps raw.
- LoginOk (0x78fdf4): 2 longs, 3 strings, 4 vints, string, 3 vints,
  3 strings, vint, 6 strings, 3 vints, 2 string lists, bytes, 3 booleans,
  vint, string.
- LoginFailed (0x533590): vint code, 4 strings, vint, boolean, string,
  string list.

## OwnHomeData (25091)

Sent right after LoginOk; the client stays on the loading screen until it
arrives. Body = LogicClientHome ‖ LogicClientAvatar ‖ 4 VInts (message
encoder 0x94f060, decoder 0x48b264). The server writes it with
`own_home.rs`, which mirrors the client's own encoders write by write.
`scripts/ghidra/DecompileTree.java` dumps such an encoder with everything
it calls.

- LogicClientHome::encode 0x596f24 (decoder 0x3d4c94): LogicLong home id,
  then 44 steps of scalars, lists and sub-objects (0x8197fc, 0x4f61d8,
  0x31b774, 0x7ec5fc, 0x7ee56c, ...). Empty, it is 196 bytes. One field
  has no count on the wire: a 10-bit bitset (0x7f05f4) written as one int.
- LogicClientAvatar::encode 0x5cb380 (vtable 0xea8290 slot 3, flags 0/0):
  three LogicLongs as VInt pairs (account, home, one unknown), name, 11
  VInts, a LogicLong, then 8 lists of (data reference, amount) and a fixed
  33-byte tail.

Stream details that matter: booleans pack 8 to a byte, and a run continues
across sub-encoders until the next non-boolean write; data references are
either `index + 1` as one VInt (0x37cc70) or `(class, instance)` VInts
(0x8e9860); 0x720dac writes fixed-length arrays as VInt deltas. Stream
vtable slots: 0x20/0x28 writeString, 0x38 writeString(String*), 0x40
writeBoolean, 0x48 writeInt, 0x80 writeVInt, 0x90/0x98 LogicLong as two
ints; readers 0xd8 readInt, 0xf8 readVInt, 0x110 readLong.

The server fills it from `server-data/accounts.json` (`accounts.rs`):
owned minis, deck slots, saved decks, heroes, and the collectible levels.
With `unlock_all` (the default) an account gets every mini and hero of the
game data at its highest level.

Collectible levels (collection screen, stars, hero level) are the third of
three maps at home +0x200 (0x39d64c; each a VInt count of (key, value)
VInt pairs, 0x827c4c). Key = `collectibleId` from
`chests/collectibles.json` (the global id: class 36 row for minis, class
107 row for heroes), value = level, at most the number of
`upgradeFragments` (4 for minis: base and three stars; 10 for heroes).
Without an entry the collection shows the mini as locked. Found through the
hero level lookup 0x5f5c60 → 0x308864 → 0x473440(home + 0x200).

## Patching the client

libg.so protects itself: changing its code, its `.rodata` strings or its
encrypted `.data` strings makes it crash at load. Only the key half k0 can
change. So `patch_apk.py`:

- Key: writes the server key into k0 only (`limb_i = rotr16(k0_i, 3 + i % 8)
  - k1_i`, Ghidra 0x83ec4c, k1 untouched).
- Host: adds `android/cmredirect` as `lib/arm64-v8a/libcmredirect.so`,
  loaded by `GameApp.onCreate` right after libg (one smali line via
  apktool). Its constructor overwrites libg's GOT slot for `getaddrinfo`
  (libg + 0xdf4fc8, a JUMP_SLOT in the RELRO segment; `mprotect` around the
  write) with a function that swaps `game.prod.clashminigame.com` for the
  configured host. This is what CM Reborn does too, with a Frida gadget
  hooking libc's resolver.

Tested on BlueStacks 5 (Android 9, x86_64 with houdini ARM translation) and
on a phone: the stock client logs in, shows the account's home and plays
bot duels (below). Notes from getting there:

- Patching code at run time (libc's `getaddrinfo` entry, Frida's inline
  hooks) crashes inside `libhoudini.so` with fault address 0xdead0000: the
  translator's code cache does not survive it. A pointer write in the GOT is
  data and works. The Frida gadget did not start under houdini at all.
- Google's emulator images (`libndk_translation`) cannot run libg.so at all,
  not even unmodified.
- On real arm64 phones no translator is involved; it works there too.
  Effects show graphical glitches on BlueStacks only.

## Bot duels

`patch_apk.py --offline-battles after-home` makes Duel start the client's
own offline duel against `OfflineMatchNPC` (npcs.csv) with the server's
deck. The client stays online on the home screen, so its commands reach the
server (`commands.rs`: name, selected deck, hero, deck positions). Duel
sends LogicStartMatchmakeCommand 582; the server answers with a
KeepAliveServer no KeepAlive asked for, and on that the hook turns the
client's offline flag (0x118abd8) on and queues the match (game mode +0x15c
= 2, +0x160 = 0, as listener 0x59e8a0 would; 0x467e78 then calls 0x940664).
A second into the battle the flag goes off again, so that Exit asks the
server for the home (`GoHomeMessage`, answered with OwnHomeData). A
SectorState as the signal does not work: with the flag set the client keeps
messages queued and handles them once it is online again, and the server's
battle then replaces the duel.

The avatar's +0x135 ("name chosen") and +0x138 (name changes) are set once
an account has a name, so the client stops asking for one.

`patch_apk.py --local-server <secret key file>` puts the server itself into
the APK (`crates/cm-server-android`, `lib/arm64-v8a/libcmserver.so`): the
hook starts it on a thread of the game process on 127.0.0.1:9339, with the
game data from the installed base.apk and accounts in the app's files
folder.
Details in [PORTING.md](PORTING.md#6-bot-duelle-der-eigentliche-trick).

Building the battle on the server instead (`SectorStateMessage`, see
`sector.rs`) gets as far as a running battle, but the client fixes decks,
shop and timings when a battle is created, so a captured battle keeps the
tutorial's rules. Battle results are not stored on the server yet.

## Playing against everyone: what is missing

The goal is that anyone with a Clash Mini APK can join and play. Where it
stands:

| Piece | State |
|---|---|
| Connect, Pepper + RC4 handshake, keep-alive (1.1143.2) | done, tested over TCP |
| Point a stock 1.1143.2 APK at the server | `patch_apk.py`; real client logs in on BlueStacks |
| Per-build message id profiles | done for 1.1143; table per build from `message_ids.py` |
| `OwnHomeDataMessage` 25091 | done: home, decks, collection, levels from `accounts.json` |
| Duel against bots | done: the client's own offline duel, see [Bot duels](#bot-duels) |
| Battle results stored on the server | TODO |
| PvP: matchmaking, `SectorState`/battle relay over TCP + UDP | TODO (`sector.rs` has the findings) |
| Other builds | see below |

Builds found so far:

- **1.1143.2** (stock, fingerprint 1.1143.1): fully mapped above.
- **44.291.1** ("CM Reborn 1.2.0" APK): a different, larger libg.so plus an
  injected hook library (`libColette.so` with a config and a compiled
  script). From its strings, the script redirects the game to another
  private server (`clashminigame.pingfanh.top:9339`) and patches libg.so in
  memory (`Interceptor.replace`, `Memory.protect`, `writeByteArray`), most
  likely the crypto. Supporting it needs: decrypting its strings (the
  `decrypt_libg.py` addresses are for 1.1143), a profile from
  `message_ids.py`, and finding out what the script patches; players would
  also need an APK that points at this server instead.

Battles are lockstep (client and server compare checksums), so two
players can only fight if they run the same battle logic and data:
matchmaking has to pair players of the same build. Social features (home,
friends, chat, rankings) can be shared across builds once each build's
messages are translated.
