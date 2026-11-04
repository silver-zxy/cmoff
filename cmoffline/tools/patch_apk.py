#!/usr/bin/env python3
"""
patch_apk.py - add cmoffline to your own copy of Clash Mini 1.1143.2

    python3 tools/patch_apk.py <input.apk> <libcmoffline.so> <output.apk> [server_pubkey_hex]
                               [--add-lib libcmserver.so ...]

What it changes in the APK:
  + adds   lib/arm64-v8a/libcmoffline.so
  ~ edits  lib/arm64-v8a/libsentry-android.so (third-party, MIT licensed):
           one extra NEEDED entry so Android loads libcmoffline.so with it
  ~ with server_pubkey_hex: sets libg's server key so it reconstructs to yours
  + with --add-lib: adds more libraries (the in-game server libcmserver.so)
  - drops  the old v1 signature files (you re-sign afterwards)
Every Supercell file (classes*.dex, libg.so, assets, ...) is copied unchanged.

Needs: Python 3.8+, patchelf (Termux: pkg install patchelf, PC: pip install patchelf)
"""
import argparse
import os
import shutil
import struct
import subprocess
import sys
import tempfile
import zipfile

LIB_DIR = "lib/arm64-v8a/"
LIBG = LIB_DIR + "libg.so"
SENTRY = LIB_DIR + "libsentry-android.so"
OURS = LIB_DIR + "libcmoffline.so"
LIBG_BUILD_ID = "128b8fb1640843833d20eb35438f69decd98f8df"  # Clash Mini 1.1143.2
FOREIGN_PATCH_MARKERS = (b"cmredirect", b"cmserver", b"cmoffline")

# libg does NOT store the server public key directly. It reconstructs it from
# two halves at load time: k0 (clear, in .rodata) and k1 (encrypted, in .data):
#
#     pub_i = (rotr16(k0_i, 3 + i%8) - k1_i) mod 2**16      (16 little-endian limbs)
#
# So to point the client at your server we must write k0 such that the
# reconstruction yields YOUR public key — writing the key raw (the old bug)
# makes the client encrypt to a key nobody holds. We can only change k0; k1 and
# all code/strings are left untouched (libg checks its own bytes and crashes if
# they change).
SERVER_KEY_OFFSET = 0xBC3DE0  # k0, file offset in libg.so
# k0 as shipped (the Supercell build's clear half) — a guard against a wrong file.
SUPERCELL_K0 = bytes.fromhex(
    "ac47249905d40199e1d9373ccd061c86d3b6a5196d511d3b000f0d8d3377ba40")
# k1, the encrypted half decrypted with libg's own string-decryption pass
# (see RE.md). Constant for this build.
SERVER_K1 = bytes.fromhex(
    "63137adaadb716d445fa4bddf76d5486ce10b4747a0f2c253ba0b0a33ba4e41d")

SO_ALIGN = 16384   # uncompressed .so files: page aligned (works for 4K and 16K pages)
ALIGN = 4          # other uncompressed files (resources.arsc etc.)


def die(msg):
    sys.exit("error: " + msg)


def _limbs(b):
    return list(struct.unpack("<16H", b))


def _pack(ls):
    return struct.pack("<16H", *[x & 0xFFFF for x in ls])


def _rotl16(v, r):
    r &= 15
    return ((v << r) | (v >> (16 - r))) & 0xFFFF


def _rotr16(v, r):
    r &= 15
    return ((v >> r) | (v << (16 - r))) & 0xFFFF


def reconstruct_pubkey(k0, k1=SERVER_K1):
    """pub_i = rotr16(k0_i, 3+i%8) - k1_i (for verifying)."""
    a, b = _limbs(k0), _limbs(k1)
    return _pack([(_rotr16(a[i], 3 + i % 8) - b[i]) & 0xFFFF for i in range(16)])


def solve_k0(pub, k1=SERVER_K1):
    """k0_i = rotl16(pub_i + k1_i, 3+i%8), so reconstruct_pubkey(k0)==pub."""
    t, b = _limbs(pub), _limbs(k1)
    return _pack([_rotl16((t[i] + b[i]) & 0xFFFF, 3 + i % 8) for i in range(16)])


def gnu_build_id(elf):
    """Return the GNU Build ID of a 64-bit little-endian ELF, or None."""
    if elf[:4] != b"\x7fELF" or elf[4] != 2 or elf[5] != 1:
        return None
    phoff, = struct.unpack_from("<Q", elf, 0x20)
    phentsize, phnum = struct.unpack_from("<HH", elf, 0x36)
    for i in range(phnum):
        p_type, _, p_offset, _, _, p_filesz = struct.unpack_from(
            "<IIQQQQ", elf, phoff + i * phentsize)
        if p_type != 4:  # PT_NOTE
            continue
        pos, end = p_offset, p_offset + p_filesz
        while pos + 12 <= end:
            namesz, descsz, ntype = struct.unpack_from("<III", elf, pos)
            name = elf[pos + 12:pos + 12 + namesz]
            desc_pos = pos + 12 + ((namesz + 3) & ~3)
            if ntype == 3 and name == b"GNU\0":
                return elf[desc_pos:desc_pos + descsz].hex()
            pos = desc_pos + ((descsz + 3) & ~3)
    return None


def is_signature_file(name):
    if not name.startswith("META-INF/") or name.count("/") != 1:
        return False
    return name.endswith((".SF", ".RSA", ".DSA", ".EC")) or name == "META-INF/MANIFEST.MF"


def check_input(zin):
    names = set(zin.namelist())
    for need in (LIBG, SENTRY):
        if need not in names:
            die(f"{need} not found. Pass the APK that contains the arm64 libraries "
                "(for split installs that is split_config.arm64_v8a.apk).")
    if OURS in names:
        die("this APK already contains libcmoffline.so")

    build_id = gnu_build_id(zin.read(LIBG))
    if build_id != LIBG_BUILD_ID:
        die(f"libg.so Build ID is {build_id}, expected {LIBG_BUILD_ID}. "
            "Only Clash Mini 1.1143.2 (versionCode 11160) is supported.")

    for name in names:
        if name.startswith("classes") and name.endswith(".dex"):
            dex = zin.read(name)
            for marker in FOREIGN_PATCH_MARKERS:
                if marker in dex:
                    die(f"{name} mentions '{marker.decode()}': this APK was modified by "
                        "another patch. Use an unmodified copy of the game.")
        if name.startswith(LIB_DIR) and any(
                m in os.path.basename(name).encode() for m in FOREIGN_PATCH_MARKERS):
            die(f"{name} looks like a library from another patch. Use an unmodified copy.")


def add_needed(sentry_bytes, tmp):
    if not shutil.which("patchelf"):
        die("patchelf not found (Termux: pkg install patchelf, PC: pip install patchelf)")
    path = os.path.join(tmp, "libsentry-android.so")
    with open(path, "wb") as f:
        f.write(sentry_bytes)
    subprocess.run(["patchelf", "--add-needed", "libcmoffline.so", path], check=True)
    out = subprocess.run(["patchelf", "--print-needed", path],
                         check=True, capture_output=True, text=True).stdout.split()
    if "libcmoffline.so" not in out:
        die("patchelf did not add the dependency")
    with open(path, "rb") as f:
        return f.read()


def aligned_info(zout, src_info, name):
    """ZipInfo for `name`, padded so stored data starts on an aligned offset."""
    zi = zipfile.ZipInfo(name, date_time=src_info.date_time)
    zi.compress_type = src_info.compress_type
    zi.external_attr = src_info.external_attr
    if zi.compress_type != zipfile.ZIP_STORED:
        return zi
    align = SO_ALIGN if name.endswith(".so") else ALIGN
    header_end = zout.fp.tell() + 30 + len(name.encode())
    pad = (-(header_end + 6)) % align + 6   # 6 = smallest padding extra field
    # extra field 0xD935 (Android zipalign): u16 alignment, then zero padding
    zi.extra = struct.pack("<HHH", 0xD935, pad - 4, align) + b"\0" * (pad - 6)
    return zi


def patch_server_key(libg, key_hex):
    pub = bytes.fromhex(key_hex)
    if len(pub) != 32:
        die("server key must be 32 bytes (64 hex chars)")
    cur = libg[SERVER_KEY_OFFSET:SERVER_KEY_OFFSET + 32]
    if cur != SUPERCELL_K0:
        die(f"unexpected bytes at the k0 offset ({cur.hex()}); wrong libg/version")
    k0 = solve_k0(pub)
    # sanity: the reconstruction must round-trip to the requested key
    if reconstruct_pubkey(k0) != pub:
        die("internal: k0 reconstruction mismatch")
    return libg[:SERVER_KEY_OFFSET] + k0 + libg[SERVER_KEY_OFFSET + 32:]


def main():
    ap = argparse.ArgumentParser(usage=__doc__)
    ap.add_argument("src")
    ap.add_argument("lib")
    ap.add_argument("dst")
    ap.add_argument("server_key", nargs="?")
    ap.add_argument("--add-lib", action="append", default=[], metavar="SO",
                    help="extra arm64 library to put into lib/arm64-v8a/")
    a = ap.parse_args()
    src, lib, dst, server_key = a.src, a.lib, a.dst, a.server_key
    if os.path.abspath(src) == os.path.abspath(dst):
        die("output must be a different file")

    def read_arm64(path):
        with open(path, "rb") as f:
            b = f.read()
        if b[:4] != b"\x7fELF" or struct.unpack_from("<H", b, 0x12)[0] != 183:
            die(f"{path} is not an arm64 ELF library")
        return b

    ours = read_arm64(lib)
    extras = [(LIB_DIR + os.path.basename(p), read_arm64(p)) for p in a.add_lib]

    with zipfile.ZipFile(src) as zin, tempfile.TemporaryDirectory() as tmp:
        check_input(zin)
        names = set(zin.namelist())
        for name, _ in extras:
            if name in names:
                die(f"the APK already contains {name}")
        sentry_info = zin.getinfo(SENTRY)
        new_sentry = add_needed(zin.read(SENTRY), tmp)

        with zipfile.ZipFile(dst, "w") as zout:
            for info in zin.infolist():
                if is_signature_file(info.filename):
                    continue
                if info.filename == SENTRY:
                    data = new_sentry
                elif info.filename == LIBG and server_key:
                    data = patch_server_key(zin.read(info), server_key)
                else:
                    data = zin.read(info)
                zout.writestr(aligned_info(zout, info, info.filename), data)
            # our libraries go in with the same storage mode as Sentry's
            zout.writestr(aligned_info(zout, sentry_info, OURS), ours)
            for name, data in extras:
                zout.writestr(aligned_info(zout, sentry_info, name), data)

    print(f"wrote {dst} (aligned, unsigned). Sign it next:")
    print(f"  apksigner sign --ks cmoffline.keystore --out signed.apk {dst}")


if __name__ == "__main__":
    main()
