#!/usr/bin/env bash
# Build a patched Clash Mini 1.1143.2 APK from your own clean copy.
#
#   ./build.sh standalone <clashmini.apk>   server runs inside the game (no PC)
#   ./build.sh connect    <clashmini.apk>   game talks to cmserver on 127.0.0.1:9339
#                                           (PC + "adb reverse tcp:9339 tcp:9339",
#                                            or cmserver in Termux on the phone)
#   ./build.sh server                       build cmserver for this machine
#
# PROGRESSION=fresh|unlocked|reset (standalone, default fresh): earn the
# collection (starts once, then resumes) / everything unlocked / start over
# (once, at the first start of this build). The connect-mode server takes
# fresh/unlocked/reset as -progression.
# TUTORIAL=on|off (standalone, default on): new accounts play the game's
# tutorial and pick their starting hero (cmserver -tutorial).
# LIVE=on|off (standalone, default on): quest settings and quest events
# (docs/LIVE_CONTENT.md; cmserver -live).
# DEV=on|off (standalone, default off): dev mode for testing, an account of
#   its own (accounts-dev.json; the real one stays as it is) with every
#   currency at its top, the Adventure Road's fuel for all tiers and every
#   open quest done, refilled with every home. Build again without DEV to
#   play the real account.
# WIN_FRAGMENTS=on|off (standalone, default off): won battles also bring
# fragments; the original gave gold only (cmserver -win-fragments).
# CHESTS=on|off (standalone, default on): rewards (win fragments, upgrade
# exp, level chests) come as chests opened in the home, or straight into the
# account (cmserver -chests).
#
# Output goes to ./out/. Keys stay next to you, never in git:
#   standalone.key   private key baked into the standalone APK
#   cmserver/server.key  key of the external server (connect mode; $SERVER_KEY)
#   cmoffline.keystore   APK signing key (made on first run)
#
# Needs: go, python3, patchelf, apksigner + keytool (JDK), and an arm64
# Android C compiler: $CC, or $NDK (android-ndk path), or Termux's clang.
# `nix run .#standalone -- clashmini.apk` provides all of it.
set -euo pipefail

ROOT=${CMOFF_ROOT:-$(cd "$(dirname "$0")" && pwd)}
OUT=${OUT:-$PWD/out}
mode=${1:-}
apk=${2:-}

die() { echo "error: $*" >&2; exit 1; }

find_cc() {
    if [ -n "${CC:-}" ]; then return; fi
    if [ -n "${NDK:-}" ]; then
        CC=$(echo "$NDK"/toolchains/llvm/prebuilt/*/bin/aarch64-linux-android21-clang)
        [ -x "$CC" ] || die "no aarch64-linux-android21-clang in \$NDK"
    elif [ -n "${TERMUX_VERSION:-}" ] || [ -d /data/data/com.termux ]; then
        CC=clang     # Termux on an arm64 phone builds for Android natively
    else
        die "set NDK=/path/to/android-ndk (or CC), or use: nix run .#$mode"
    fi
}

build_hook() {   # "$@": extra -D flags
    find_cc
    mkdir -p "$OUT"
    "$CC" -shared -fPIC -O2 -Wall -Wextra "$@" \
        -o "$OUT/libcmoffline.so" "$ROOT/cmoffline/src/cmoffline.c" -llog -ldl
}

build_server_lib() {
    find_cc
    mkdir -p "$OUT"
    (cd "$ROOT/cmserver" && CGO_ENABLED=1 GOOS=android GOARCH=arm64 CC="$CC" \
        go build -trimpath -buildmode=c-shared -ldflags="-s -w" \
        -o "$OUT/libcmserver.so" ./cmd/libcmserver)
    rm -f "$OUT/libcmserver.h"
}

pubkey_of() {   # $1: private key file (created if missing)
    (cd "$ROOT/cmserver" && go run . -key "$1" -print-pubkey)
}

sign() {   # $1: unsigned apk, $2: signed apk
    local ks=cmoffline.keystore
    if [ ! -f "$ks" ]; then
        keytool -genkeypair -keystore "$ks" -storepass cmoffline -alias cmoffline \
            -keyalg RSA -keysize 2048 -validity 10000 -dname "CN=cmoffline"
    fi
    apksigner sign --ks "$ks" --ks-pass pass:cmoffline --out "$2" "$1"
    rm -f "$1" "$2.idsig"
    echo "wrote $2"
}

case "$mode" in
standalone)
    [ -f "$apk" ] || die "usage: $0 standalone <clashmini.apk>"
    key=$PWD/standalone.key
    pub=$(pubkey_of "$key")
    priv=$(tr -d ' \n\r' < "$key")
    build_server_lib
    case "${PROGRESSION:=fresh}" in fresh|unlocked|reset) ;; *) die "PROGRESSION must be fresh, unlocked or reset" ;; esac
    case "${TUTORIAL:=on}" in on|off) ;; *) die "TUTORIAL must be on or off" ;; esac
    case "${CHESTS:=on}" in on|off) ;; *) die "CHESTS must be on or off" ;; esac
    case "${LIVE:=on}" in on|off) ;; *) die "LIVE must be on or off" ;; esac
    case "${WIN_FRAGMENTS:=off}" in on|off) ;; *) die "WIN_FRAGMENTS must be on or off" ;; esac
    case "${DEV:=off}" in on|off) ;; *) die "DEV must be on or off" ;; esac
    echo "progression: $PROGRESSION, tutorial: $TUTORIAL, chests: $CHESTS, live: $LIVE, win fragments: $WIN_FRAGMENTS, dev: $DEV"
    opt=$PROGRESSION
    [ "$opt" = reset ] && opt="reset:$(date +%s)"
    build_hook -DCMOFFLINE_NO_CRASH_LOGGER -DCMOFFLINE_LOCAL_SERVER "-DSERVER_KEY_HEX=\"$priv\"" \
        "-DCMSERVER_OPTIONS=\"progression=$opt tutorial=$TUTORIAL chests=$CHESTS live=$LIVE winfragments=$WIN_FRAGMENTS dev=$DEV\""
    python3 "$ROOT/cmoffline/tools/patch_apk.py" "$apk" "$OUT/libcmoffline.so" \
        "$OUT/unsigned.apk" "$pub" --add-lib "$OUT/libcmserver.so" \
        --help-page "$ROOT/cmoffline/help/help.html"
    sign "$OUT/unsigned.apk" "$OUT/clashmini-standalone.apk"
    ;;
connect)
    [ -f "$apk" ] || die "usage: $0 connect <clashmini.apk>"
    pub=$(pubkey_of "${SERVER_KEY:-$PWD/cmserver/server.key}")
    build_hook -DCMOFFLINE_NO_CRASH_LOGGER
    python3 "$ROOT/cmoffline/tools/patch_apk.py" "$apk" "$OUT/libcmoffline.so" \
        "$OUT/unsigned.apk" "$pub" --help-page "$ROOT/cmoffline/help/help.html"
    sign "$OUT/unsigned.apk" "$OUT/clashmini-connect.apk"
    echo "run the server with: cd cmserver && go run ."
    ;;
server)
    (cd "$ROOT/cmserver" && go build -o "$OUT/cmserver" .)
    echo "wrote $OUT/cmserver (run it from cmserver/ so it finds server.key)"
    ;;
*)
    sed -n '2,27p' "$0"
    exit 1
    ;;
esac
