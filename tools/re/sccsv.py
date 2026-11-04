#!/usr/bin/env python3
"""Decompress the game's CSV files (Supercell LZMA) from your own APK.

    python3 sccsv.py clashmini.apk outdir/     every assets/**/*.csv -> outdir

The files start with the 5 LZMA property bytes and a 4-byte size; inserting
four zero bytes makes them a standard .lzma ("alone") stream. Rows whose name
is random junk and whose other cells are empty are content stripped from this
build ("obfuscated rows"), e.g. skins and minis that never shipped.
"""
import lzma
import os
import sys
import zipfile


def decode(b):
    if b[:1] in (b'"', b"N") or b[:4].isascii() and b[:4].decode(errors="ignore").isprintable():
        try:
            return b.decode("utf-8")
        except UnicodeDecodeError:
            pass
    return lzma.decompress(b[:9] + b"\0" * 4 + b[9:], format=lzma.FORMAT_ALONE).decode("utf-8", "replace")


def main():
    if len(sys.argv) != 3:
        sys.exit(__doc__)
    apk, out = sys.argv[1:]
    with zipfile.ZipFile(apk) as z:
        for name in z.namelist():
            if name.startswith("assets/") and name.endswith(".csv"):
                dst = os.path.join(out, name[len("assets/"):])
                os.makedirs(os.path.dirname(dst), exist_ok=True)
                with open(dst, "w", encoding="utf-8") as f:
                    f.write(decode(z.read(name)))
                print(dst)


if __name__ == "__main__":
    main()
