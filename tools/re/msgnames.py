#!/usr/bin/env python3
"""List message and command ids with their class names.

    python3 msgnames.py libg.so strings.tsv > ids.tsv

Every message class has getMessageType() in vtable slot 5 and every command
getCommandType() in slot 4, each a two-instruction "mov w0,#id; ret". The
vtable's typeinfo name ("29AvatarNameCheckRequestMessage") is one of the
decrypted strings (decrypt_strings.py). Output: id <TAB> class <TAB> vtable
(Ghidra address of the vtable's address point).
"""
import os
import re
import sys

import numpy as np

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from libg import GHIDRA, load  # noqa: E402

RET = 0xD65F03C0


def main():
    g = load(sys.argv, __doc__)
    if len(sys.argv) < 3:
        sys.exit(__doc__)
    names = {}
    for line in open(sys.argv[2], encoding="utf-8", errors="replace"):
        p = line.rstrip("\n").split("\t")
        if len(p) == 3 and re.fullmatch(r"\d+[A-Za-z_]\w*", p[2]):
            names[int(p[0], 16)] = re.sub(r"^\d+", "", p[2])
    rel = g.relocs()
    rev = {}
    for slot, tgt in rel.items():
        rev.setdefault(tgt, []).append(slot)

    def class_of(address_point):
        ti = rel.get(address_point - 8)
        nm = rel.get(ti + 8) if ti is not None else None
        return names.get(nm + GHIDRA) if nm is not None else None

    w = np.frombuffer(g.b[:len(g.b) // 4 * 4], dtype="<u4")
    movs = np.nonzero(((w & 0xFFE0001F) == 0x52800000) & (np.roll(w, -1) == RET))[0]
    rows = set()
    for i in movs:
        ident = int((w[i] >> 5) & 0xFFFF)
        for slot in rev.get(int(i) * 4, []):
            for k in (5, 4):
                cls = class_of(slot - 8 * k)
                if cls and (cls.endswith("Message") if k == 5 else cls.endswith("Command")):
                    rows.add((ident, cls, slot - 8 * k + GHIDRA))
    for ident, cls, vt in sorted(rows):
        print(f"{ident}\t{cls}\t{vt:#x}")


if __name__ == "__main__":
    main()
