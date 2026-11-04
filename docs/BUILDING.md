# Building and installing

Everything is driven by `build.sh`; `flake.nix` only provides the tools.

| Command | Result |
|---|---|
| `./build.sh standalone clashmini.apk` | `out/clashmini-standalone.apk`: hook + server inside the game |
| `./build.sh connect clashmini.apk` | `out/clashmini-connect.apk`: hook only, talks to `127.0.0.1:9339` |
| `./build.sh server` | `out/cmserver` for this machine |

With Nix, `nix run .#standalone -- clashmini.apk` (and `.#connect`, `.#server`)
runs the same with Go, the NDK, patchelf and apksigner supplied. `nix develop`
gives a shell with all of them.

## The input APK

Clash Mini 1.1143.2, versionCode 11160, arm64-v8a. `patch_apk.py` checks the
Build ID of `libg.so` (`128b8fb1640843833d20eb35438f69decd98f8df`) and refuses
anything else, including already patched APKs. From a split install (APKMirror
`.apks`) use `split_config.arm64_v8a.apk` and sign every split with the same key.

## What the patch changes

- adds `lib/arm64-v8a/libcmoffline.so` (and `libcmserver.so` for standalone)
- adds one `DT_NEEDED` entry to `libsentry-android.so` (Sentry, MIT) so Android
  loads the hook together with it; no smali/dex change
- rewrites 32 bytes of `libg.so` (k0, half of the server public key) so the game
  trusts our server key — see [PROTOCOL.md](PROTOCOL.md#server-key)
- drops the old signatures; `build.sh` re-signs with `cmoffline.keystore`
  (created on first run)

## Keys and saves

| File | What | Where |
|---|---|---|
| `standalone.key` | server private key baked into the standalone APK | where you run `build.sh` |
| `cmserver/server.key` | key of the PC/Termux server (connect mode) | created by the first `go run .` |
| `cmoffline.keystore` | APK signing key | where you run `build.sh` |
| `accounts.json` | your account (name, decks, heroes, skins) | standalone: the app's files folder; PC: next to the server |

Keep the keystore: an update signed with another key needs an uninstall,
which also deletes the standalone account. Rebuilding with the same
`standalone.key` and keystore installs over the old APK and keeps it.

## Toolchain without Nix

- Go ≥ 1.22, Python 3, `patchelf`, `apksigner` + `keytool` (JDK 17)
- the Android NDK: `NDK=/path/to/android-ndk-r26 ./build.sh standalone ...`
  (or set `CC` to an `aarch64-linux-android21-clang`)
- **Termux on the phone**: `pkg install golang clang patchelf python apksigner openjdk-17`;
  `build.sh` uses Termux's clang, which builds for Android natively.

## Install

Uninstall any other copy first (from every user profile on GrapheneOS), then
`adb install out/clashmini-*.apk`. The game needs its network permission even
standalone: the server listens on 127.0.0.1 inside the app.

## Logs

    adb logcat -s cmoffline cmserver

- `cmoffline`: hook (`bot battles ready`, `game state 4 -> 5`, `battle signal`, ...)
- `cmserver`: the in-game server (standalone). The PC server logs to its terminal,
  with every frame in hex when `-trace` (default on).

## Server flags (PC)

    go run . [-listen 0.0.0.0:9339] [-key server.key] [-accounts accounts.json]
             [-trace=false] [-print-pubkey]
             [-battle-online=false] [-signal-empty] [-slot-value N] [-mini-level N]

The last four are experiment switches kept for debugging; the defaults are the
working setup.

## Troubleshooting

| Symptom | Cause |
|---|---|
| `login: ... (is the APK patched with this server's key?)` | APK built with another key; rebuild `connect` after the server created `server.key` |
| stuck on the loading screen, nothing in the server log | no `adb reverse`, or the server is not running |
| `unsupported libg.so` in logcat | not 1.1143.2 |
| standalone: `local server failed` | network permission revoked, or port 9339 taken |
| `address already in use` on the PC | an old server still holds 9339 |
