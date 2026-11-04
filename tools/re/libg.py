"""Shared helpers for the libg.so (Clash Mini 1.1143.2, arm64) RE scripts.

Addresses are Ghidra addresses (image base 0x100000) unless said otherwise:
  ELF vaddr = Ghidra - 0x100000.  .text: file offset = vaddr.
  .data/.data.rel.ro: file offset = vaddr - 0x1000.
Use your own libg.so from your own APK (lib/arm64-v8a/libg.so).
"""
import struct
import sys

GHIDRA = 0x100000


class Libg:
    def __init__(self, path):
        self.b = open(path, "rb").read()
        phoff = struct.unpack_from("<Q", self.b, 0x20)[0]
        phn = struct.unpack_from("<H", self.b, 0x38)[0]
        self.segs = []
        for i in range(phn):
            o = phoff + i * 56
            if struct.unpack_from("<I", self.b, o)[0] != 1:  # PT_LOAD
                continue
            off, va, _, fs = struct.unpack_from("<QQQQ", self.b, o + 8)
            self.segs.append((va, off, fs))
        self._rel = None

    def off(self, ghidra):
        """File offset of a Ghidra address."""
        va = ghidra - GHIDRA
        for sva, soff, fs in self.segs:
            if sva <= va < sva + fs:
                return soff + va - sva
        raise ValueError(hex(ghidra))

    def read(self, ghidra, n):
        o = self.off(ghidra)
        return self.b[o:o + n]

    def u32(self, ghidra):
        return struct.unpack_from("<I", self.b, self.off(ghidra))[0]

    def relocs(self):
        """R_AARCH64_RELATIVE: slot vaddr -> target vaddr."""
        if self._rel is None:
            self._rel = {}
            shoff = struct.unpack_from("<Q", self.b, 0x28)[0]
            shn = struct.unpack_from("<H", self.b, 0x3C)[0]
            for i in range(shn):
                o = shoff + i * 64
                _, typ, _, _, off, size = struct.unpack_from("<IIQQQQ", self.b, o)
                if typ != 4:  # SHT_RELA
                    continue
                for j in range(size // 24):
                    ro, ri, ra = struct.unpack_from("<QQq", self.b, off + j * 24)
                    if ri & 0xFFFFFFFF == 0x403:
                        self._rel[ro] = ra
        return self._rel


def load(argv, usage):
    if len(argv) < 2:
        sys.exit(usage)
    return Libg(argv[1])
