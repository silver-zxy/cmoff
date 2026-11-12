# Mini Room: what the client expects

Work notes for roadmap A10 (Mini Room with real players). Addresses are
Ghidra addresses of libg 1.1143.2 (file offset + 0x100000).

## The plan

Every phone keeps its own server and account. A meeting point (way A: one
phone's server in the Wi-Fi; way B: a relay on a PC or VPS) passes the room
and the battle between the servers. For testing without a second phone, a
small program on the PC plays the second player (joins, starts, plays
nothing).

## Reading messages in libg

- Message vtables: slot 2 encode, slot 3 decode, slot 5 getMessageType
  (`tools/re`, `vtslots`).
- Byte stream, direct calls: 0x728214 readInt, 0x5d4f38 readBoolean,
  0x5ae28c readVInt, 0x6d2944 readString(max), 0x79b6bc a new LogicLong
  from the stream (two readInts), 0x79547c writeInt, 0x3257a8
  writeBoolean, 0x508e70 / 0x836210 writeLong (pointer). 0x3d33dc and
  0x7b721c are the Message base encode / decode.
- The main message handler is 0x4549a8; it switches on the message type.

## Room messages

The client has the whole room UI (texts `TID_PRIVATE_LOBBY_*`): create with
a mode choice, join with a room code, copy the code, and the errors "Room
<Code> does not exist", "Room <Code> is full", "You're already in a room",
"The host has left. The room no longer exists.", "You've been removed from
the room.", and "Reach level <level> to unlock Mini Room" (level 5).

Client to server:

| Id | Message | Encoder | Content |
|---|---|---|---|
| 10181 | CreateGameRoom | 0x4c1700 | int (+0x84), probably the mode |
| 10186 | JoinGameRoom | 0x8d914c | bool, then long room (+0x88) if set |
| 10190 | StartGameRoomMatch | 0x975f58 | long room (+0x88) |
| 14761 | LeaveGameRoom | 0x5e6d68 | long room (+0x88) |

Server to client:

| Id | Message | Decoder | Content | Handler |
|---|---|---|---|---|
| 24566 | CreateGameRoomResponse | 0x58b39c | int status; bool + string (+0x88, the room code?); bool + long (+0x90) | 0x4558bc → 0x7db5c8 |
| 24568 | JoinGameRoomResponse | 0x33bd04 | int status; long (+0x88) | 0x4558e8 → 0x3c2c7c |
| 24571 | LeaveGameRoomResponse | | | 0x455914 |
| 24572 | GameRoomShutdown | | | 0x455944 → 0x69df98 |
| 24573 | StartGameRoomMatchResponse | 0x832574 | long (+0x88) | 0x455964 |
| 20188 | GameRoomMembers | 0x28e9c8 | long room (+0x90); VInt n × {long account, string name} (entry 0x754820); int (+0x98); long (+0xa0) | 0x454b98 → 0x475f78 |

The room responses go to the GUI object `*(0x11851c0)` (jump table at
0x454c14). Open: the status values (which one is which error), the last
int and long of GameRoomMembers.

## The battle

- SectorState 22681 has no encoder or decoder of its own (slots 2 and 3 are
  `ret`): the handler reads the message's bytes itself. Handler 0x4f29f0:
  game state 5 (battle), then the game mode (0x5952bc) reads the state,
  0x603558 → 0x69922c → 0x605eb8 (obfuscated, 0x9a6a98), and the client
  answers with a 0x88-byte message (0x3bf6f8, probably SectorStateOk
  17239) through 0x87641c.
- The content is the whole start of the battle (players, decks, shop,
  timings), not a small header. A captured battle kept the tutorial's
  rules (reference/SERVER.md).
- Idea: the client builds battles itself (0x940664 for the bot battles).
  If it can also write a battle in SectorState's format, the host's client
  builds the battle and the meeting point hands the same bytes to both.
- SectorHeartbeat 27199: handler 0x454d20 → 0x559ee4(game mode, message).
- Lockstep checksums: 0x994008 ("Check sum mismatch. Request full
  update.").
- UdpConnectionInfo 23036 (decoder 0x7f93f0): a VInt, a string, an int,
  bytes; handler 0x4559bc. Open: whether battles also run over TCP.

## Waydroid as a second phone (dropped)

On an Intel PC with libhoudini (14.0.0_z.GoogleGame) the game, the
unchanged APK too, crashes 0.4 s after the start in Supercell's obfuscated
code: a computed jump (`blr x8` at 0x3e3f90, table at 0x114e918) lands
4 GB off. Not tried: libndk.
