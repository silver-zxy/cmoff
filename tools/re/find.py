#!/usr/bin/env python3
"""Small static-analysis helpers for libg.so (needs numpy; dis needs capstone).

    python3 find.py libg.so dis     ADDR [COUNT]   disassemble (stops at ret)
    python3 find.py libg.so callers ADDR           bl/b instructions that target ADDR
    python3 find.py libg.so xref    ADDR           adrp+add/ldr pairs that point at ADDR
    python3 find.py libg.so vtable  ADDR [SLOTS]   function pointers of a vtable

ADDR is a Ghidra address (hex). A string's address comes from
decrypt_strings.py, a vtable's from msgnames.py. These are the scans that
found the deletion warning (LoginOk +0x14c), the skin store and the name
dialog, see docs/JOURNAL.md.
"""
import os
import sys

import numpy as np

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from libg import GHIDRA, load  # noqa: E402


def words(g):
    w = np.frombuffer(g.b[:len(g.b) // 4 * 4], dtype="<u4").astype(np.int64)
    pc = np.arange(len(w), dtype=np.int64) * 4 + GHIDRA  # .text: file offset == vaddr
    return w, pc


def callers(g, target):
    w, pc = words(g)
    imm = w & 0x3FFFFFF
    imm = np.where(imm & 0x2000000, imm - 0x4000000, imm)
    branch = ((w >> 26) == 0x25) | ((w >> 26) == 0x05)
    return [int(x) for x in pc[np.nonzero(branch & (pc + imm * 4 == target))[0]]]


def xrefs(g, target):
    w, pc = words(g)
    adrp = (w & 0x9F000000) == 0x90000000
    imm = (((w >> 5) & 0x7FFFF) << 2) | ((w >> 29) & 3)
    imm = np.where(imm & (1 << 20), imm - (1 << 21), imm)
    page = (pc & ~0xFFF) + (imm << 12)
    out = []
    for i in np.nonzero(adrp & (page == (target & ~0xFFF)))[0]:
        rd = int(w[i]) & 31
        for j in range(1, 8):
            x = int(w[i + j])
            if (x & 0xFFC00000) == 0x91000000 and (x >> 5) & 31 == rd \
                    and (x >> 10) & 0xFFF == target & 0xFFF:          # add
                out.append(int(pc[i]))
                break
            # ldr/str x (scale 8), ldr/str w (scale 4), ldrb/strb (scale 1)
            hit = False
            for base, scale in ((0xF9400000, 8), (0xF9000000, 8), (0xB9400000, 4),
                                (0xB9000000, 4), (0x39400000, 1), (0x39000000, 1)):
                if (x & 0xFFC00000) == base and (x >> 5) & 31 == rd \
                        and ((x >> 10) & 0xFFF) * scale == target & 0xFFF:
                    hit = True
            if hit:
                out.append(int(pc[i]))
                break
    return out


def dis(g, addr, count):
    import capstone
    md = capstone.Cs(capstone.CS_ARCH_ARM64, capstone.CS_MODE_LITTLE_ENDIAN)
    o = g.off(addr)
    for a, _, mn, op in md.disasm_lite(g.b[o:o + count * 4], addr):
        print(f"{a:#x}: {mn} {op}")
        if mn == "ret":
            break


def vtable(g, addr, slots):
    rel = g.relocs()
    va = addr - GHIDRA
    for k in range(slots):
        t = rel.get(va + 8 * k)
        print(f"  slot {k} (+{8 * k:#x}): {t + GHIDRA:#x}" if t is not None else f"  slot {k}: -")


def main():
    if len(sys.argv) < 4:
        sys.exit(__doc__)
    g = load(sys.argv, __doc__)
    cmd, addr = sys.argv[2], int(sys.argv[3], 16)
    n = int(sys.argv[4]) if len(sys.argv) > 4 else None
    if cmd == "dis":
        dis(g, addr, n or 80)
    elif cmd == "callers":
        print("\n".join(hex(a) for a in callers(g, addr)) or "none")
    elif cmd == "xref":
        print("\n".join(hex(a) for a in xrefs(g, addr)) or "none")
    elif cmd == "vtable":
        vtable(g, addr, n or 8)
    else:
        sys.exit(__doc__)


if __name__ == "__main__":
    main()
