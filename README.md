# cmoff — Clash Mini 1.1143.2, playable again on your own phone

Supercell shut Clash Mini's servers down in April 2024. This project brings
the last build (1.1143.2, arm64) back to life for **one player on their own
device**: a small game server written from scratch in Go, and a tiny hook
library that points the game at it. The server can run on a PC, in Termux, or
**inside the game itself** (the "standalone" APK — no PC needed).

What works:

| | |
|---|---|
| Login, home, full collection (all minis and heroes at max, 13 skins) | ✅ |
| Decks, heroes, skins and your name are saved | ✅ |
| Duel and Rumble against the game's built-in bot, at your levels | ✅ |
| In-battle upgrades, Exit back to the home | ✅ |
| Trophies (rank points) and gold as the game data gives them; casual stays casual; bots get stronger with your league | ✅ |
| Profile statistics, win streaks, player level (EXP from upgrades, as in the original) | ✅ |
| Progression: start small, earn gold and fragments, unlock and upgrade minis and heroes (or everything unlocked: `PROGRESSION=unlocked`) | 🧪 first version |
| Chest openings in the home (quest rewards, upgrade exp, level chests; `CHESTS=off`: straight into the account) | 🧪 first version |
| Shop, quests | ❌ |
| Real PvP | ❌ |

Nothing of Supercell's is in this repository: no APK, no game code, no game
data, no Supercell keys. You patch **your own copy** of the game. The server
makes its own key pair and the patcher changes the game's server key to it.

## Quick start

You need your own clean `clashmini.apk` (1.1143.2, versionCode 11160, the
APK that contains `lib/arm64-v8a/libg.so`).

**Standalone** (server inside the game):

    nix run .#standalone -- clashmini.apk        # or: NDK=... ./build.sh standalone clashmini.apk
    adb install out/clashmini-standalone.apk

**Connect** (server on your PC, handy for development — every message is logged):

    nix run .#connect -- clashmini.apk
    adb install out/clashmini-connect.apk
    cd cmserver && go run .                      # first run creates server.key
    adb reverse tcp:9339 tcp:9339

Build `connect` *after* running the server once, so the APK gets its key.
Details, Termux and troubleshooting: [docs/BUILDING.md](docs/BUILDING.md).

## Layout

    cmserver/            the server (Go)
      srv/               connection, handshake, message handling, commands
      home/              OwnHomeData encoder (mirrors libg's encoders), account, skins
      pepper/            Pepper handshake crypto (crypto_box + secretbox stream)
      proto/             frames, Titan byte stream, message ids
      cmd/libcmserver/   the server as libcmserver.so for the standalone APK
    cmoffline/
      src/cmoffline.c    the hook loaded into the game
      tools/patch_apk.py adds the hook (and server) to your APK, sets the server key
    tools/re/            scripts used for the reverse engineering (bring your own libg.so)
    docs/                how everything works, see below
    build.sh, flake.nix  builds

## Documentation

- [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md): the pieces and how a session runs
- [docs/PROTOCOL.md](docs/PROTOCOL.md): frames, crypto, login, the messages and commands we handle
- [docs/HOME.md](docs/HOME.md): OwnHomeData and the account model (decks, heroes, skins, name)
- [docs/CLIENT_HOOK.md](docs/CLIENT_HOOK.md): what the hook does, every libg offset it uses
- [docs/JOURNAL.md](docs/JOURNAL.md): how each problem was found and solved, including dead ends
- [docs/RE_TOOLS.md](docs/RE_TOOLS.md): the analysis scripts
- [docs/ids.tsv](docs/ids.tsv): every message and command id of 1.1143.2 with its class name
- [docs/reference/](docs/reference/): the original RE notes this builds on (by .gnoks)

## Rules for this repo

Keep it to your own copy of the game and your own device. Never commit or
share APKs, `libg.so`, game data, keys (`*.key`, keystores) or saves
(`accounts.json`); `.gitignore` covers them. Modifying the game breaks
Supercell's terms even with their code untouched.
