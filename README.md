# cmoff — Clash Mini 1.1143.2, playable again on your own phone

Supercell shut Clash Mini's servers down in April 2024. This project brings
the Clashmas update (1.1143.2, December 2021, arm64) back to life for **one
player on their own device**: a small game server written from scratch in Go,
and a tiny hook library that points the game at it. The server can run on a
PC, in Termux, or **inside the game itself** (the "standalone" APK — no PC
needed).

1.1143.2 is not the last build, but the one we keep: the game stays a
snapshot of Clashmas 2021 (Season 1, the Clashmas shop and quests), close to
the original, with real-money items free ([docs/ROADMAP.md](docs/ROADMAP.md)).

What works (the offline mode is done, tested on the phone):

| | |
|---|---|
| Login, home, decks, heroes, skins and your name are saved | ✅ |
| Duel and Rumble against the game's bots, with their own decks and levels for your league, on the league's boards | ✅ |
| In-battle upgrades, Exit back to the home | ✅ |
| Trophies (rank points) and gold as the game data gives them; casual stays casual | ✅ |
| Profile statistics, win streaks, player level (EXP from upgrades, as in the original) | ✅ |
| Progression: start with the game's starting values and chests, earn gold and fragments, unlock and upgrade minis and heroes (or everything unlocked: `PROGRESSION=unlocked`); fragments past max level become Crystal Coins | ✅ |
| Chest openings in the home (quest rewards, shop, road, upgrade exp, level chests; `CHESTS=off`: straight into the account) | ✅ |
| Daily quests: six a day, hero quests, reroll, counted in battle by the game itself | ✅ |
| Mini quests for every newly unlocked mini, at once; the tutorial quests for new players; "Win 5 rounds" for a Crystal Coin, again and again | ✅ |
| Clashmas quests with the Candy Elf skin after 12 | ✅ |
| Adventure Road: Season 1 with 90 tiers, the Mini Pass lane free | ✅ |
| Season end every two months: the "Season has ended!" popup, the star chest, trophy reset | ✅ |
| Skins locked as in the original, unlocked through the road, the Clashmas quests and the shop | ✅ |
| Shop: daily minis and heroes, the Clashmas skins, Crystal Coin skins, gold for gems, gems for gold | ✅ |
| Help and Support: our own help page | ✅ |
| Dev mode for testing (`DEV=on`) | ✅ |
| Mini Room | ❌ (next) |
| Real PvP | ❌ (much later) |

Nothing of Supercell's is in this repository: no APK, no game code, no game
data, no Supercell keys. You patch **your own copy** of the game. The server
makes its own key pair and the patcher changes the game's server key to it.

## Quick start

You need your own clean `clashmini.apk` (1.1143.2, versionCode 11160, the
APK that contains `lib/arm64-v8a/libg.so`).

**Standalone** (server inside the game):

    nix run .#standalone -- clashmini.apk        # or: NDK=... ./build.sh standalone clashmini.apk
    adb install out/clashmini-standalone.apk

**Dev mode** for testing: an account of its own (your real one stays as it
is) with every currency at its top, the Adventure Road's fuel for all
tiers and every open quest done, refilled with every home. Build again
without `DEV` to play your real account.

    DEV=on TUTORIAL=off nix run .#standalone -- clashmini.apk
    adb install -r out/clashmini-standalone.apk

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
      home/              OwnHomeData encoder (mirrors libg's encoders), account, skins,
                         quests, road, shop; reads the game data from your APK
      pepper/            Pepper handshake crypto (crypto_box + secretbox stream)
      proto/             frames, Titan byte stream, message ids
      cmd/libcmserver/   the server as libcmserver.so for the standalone APK
    cmoffline/
      src/cmoffline.c    the hook loaded into the game
      tools/patch_apk.py adds the hook (and server) to your APK, sets the server key
      help/help.html     the page "Help and Support" opens
    tools/re/            scripts used for the reverse engineering (bring your own libg.so)
    docs/                how everything works, see below
    build.sh, flake.nix  builds

## Documentation

- [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md): the pieces and how a session runs
- [docs/PROTOCOL.md](docs/PROTOCOL.md): frames, crypto, login, the messages and commands we handle
- [docs/HOME.md](docs/HOME.md): OwnHomeData and the account model (decks, heroes, skins, name)
- [docs/LIVE_CONTENT.md](docs/LIVE_CONTENT.md): quests, Adventure Road and shop (the live server's content)
- [docs/ORIGINAL.md](docs/ORIGINAL.md): what is known of the original game around 1.1143.2, with sources
- [docs/ROADMAP.md](docs/ROADMAP.md): what is done, what comes next, what is still unknown
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
