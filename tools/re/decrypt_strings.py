#!/usr/bin/env python3
"""Decrypt libg.so's string table (docs/reference/RE.md, "String encryption").

    python3 decrypt_strings.py libg.so > strings.tsv

Output: Ghidra address <TAB> length <TAB> text (one line per string; bytes
that are not UTF-8 are replaced). The item table at 0x10746e8 holds entries
of three u32; each is decoded with the previous one:
  pointer = 0x10746e8 + (i32)(c0 - p0 - p1 - p2), length = c0 + p1 - c1
and each byte i of an item is XORed with key[i & 0x7f] (key at 0x10bd037).
"""
import os
import struct
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from libg import load  # noqa: E402

TABLE = 0x10746E8
KEY = 0x10BD037
M = 0xFFFFFFFF


def items(g):
    key = g.read(KEY, 128)
    data_lo, data_hi = None, None
    for va, off, fs in g.segs:  # the writable segment that holds the table
        if va <= TABLE - 0x100000 < va + fs:
            data_lo, data_hi = va + 0x100000, va + fs + 0x100000
    p = struct.unpack("<3I", g.read(TABLE, 12))
    i = 1
    while True:
        c = struct.unpack("<3I", g.read(TABLE + 12 * i, 12))
        rel = (c[0] - p[0] - p[1] - p[2]) & M
        if rel & 0x80000000:
            rel -= 1 << 32
        ptr = TABLE + rel
        n = (c[0] + p[1] - c[1]) & M
        if n == 0 or not (data_lo <= ptr < data_hi) or ptr + n > data_hi:
            return
        raw = g.read(ptr, n)
        yield ptr, bytes(b ^ key[j & 0x7F] for j, b in enumerate(raw))
        p = c
        i += 1


def main():
    g = load(sys.argv, __doc__)
    out = sys.stdout
    count = 0
    for ptr, s in items(g):
        text = s.rstrip(b"\0").decode("utf-8", "replace")
        text = text.replace("\t", "\\t").replace("\n", "\\n")
        out.write(f"{ptr:#x}\t{len(s)}\t{text}\n")
        count += 1
    print(f"{count} strings", file=sys.stderr)


if __name__ == "__main__":
    main()
