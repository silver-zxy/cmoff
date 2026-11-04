# cmoffline

The client side: `src/cmoffline.c` (the hook loaded into the game) and
`tools/patch_apk.py` (puts it, and optionally the in-game server, into your
own APK and sets the server key). Build with `../build.sh`; how it works is
in [../docs/CLIENT_HOOK.md](../docs/CLIENT_HOOK.md).

    python3 tools/patch_apk.py clashmini.apk libcmoffline.so out.apk [server_pubkey_hex] \
        [--add-lib libcmserver.so]

Only Supercell's `libg.so` k0 bytes and Sentry's dependency list change;
every other file is copied as is. Sign the result (`build.sh` does).
