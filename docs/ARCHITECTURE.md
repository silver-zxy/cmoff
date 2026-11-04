# Architecture

```
 your phone
 ┌──────────────────────────────────────────────────────────────┐
 │ Clash Mini 1.1143.2 (Supercell code unchanged)               │
 │   libg.so ──getaddrinfo──▶ 127.0.0.1   (k0 rewritten: trusts │
 │      │                                   our server key)     │
 │      │ TCP 9339, Pepper crypto                               │
 │      ▼                                                       │
 │   libcmoffline.so (hook) ── watches frame headers, starts    │
 │      │                     offline bot battles, renaming on  │
 │      ▼                                                       │
 │   libcmserver.so (standalone)  or  cmserver on PC/Termux     │
 │      handshake · LoginOk · OwnHomeData · commands · saves    │
 └──────────────────────────────────────────────────────────────┘
```

## A session

1. **Connect.** The hook has redirected the game host, so the game connects to
   127.0.0.1:9339 (standalone: the server in the same process; connect mode:
   `adb reverse` to the PC).
2. **Handshake.** ClientHello → ServerHello (nonce) → Login (crypto_box to our
   key) → LoginOk (crypto_box, carries the stream key). From here every body is
   secretbox-encrypted. [PROTOCOL.md](PROTOCOL.md)
3. **Home.** OwnHomeData built from `accounts.json`: full collection, the saved
   decks/heroes/skins/name. [HOME.md](HOME.md)
4. **Playing in the home.** The client executes its own commands (swap mini,
   pick hero, skin, deck) and reports them in EndClientTurn every few
   seconds; the server applies them to the account and saves it.
5. **Battle.** Duel/Rumble sends StartMatchmake; the server answers with the
   battle signal; the hook starts the client's own offline battle against the
   built-in bot and goes back online after a second. [CLIENT_HOOK.md](CLIENT_HOOK.md)
6. **Exit.** GoHome → the server sends OwnHomeData again.

The server never simulates a battle and keeps no game data: the client
already has everything needed for offline battles, and the account is just
a few ids.

## Code map

| Path | Role |
|---|---|
| `cmserver/srv/server.go` | accept loop, handshake, per-message handling |
| `cmserver/srv/commands.go` | EndClientTurn parser, command → account |
| `cmserver/srv/loginok.go` | LoginOk body |
| `cmserver/srv/keys.go` | server key file |
| `cmserver/pepper/` | crypto_box handshake, secretbox stream |
| `cmserver/proto/` | frames, byte stream (VInt, bools, strings), message ids |
| `cmserver/home/ownhome.go` | OwnHomeData encoder (mirror of libg's) |
| `cmserver/home/account.go` | the saved account, commands applied to it |
| `cmserver/home/skins.go` | skin table, skin store encoding |
| `cmserver/cmd/libcmserver/` | `CmServerStart` for the in-game server, logs to logcat |
| `cmserver/main.go` | PC/Termux server |
| `cmoffline/src/cmoffline.c` | the hook |
| `cmoffline/tools/patch_apk.py` | APK patcher (key, DT_NEEDED, libs, alignment) |
| `build.sh`, `flake.nix` | builds |
| `tools/re/` | reverse-engineering scripts |

## Tests

    cd cmserver && go test ./...

Covers the crypto round trip, a full simulated handshake, the home encoder
against .gnoks's reference body, captured EndClientTurn batches, account
persistence and the skin store.
