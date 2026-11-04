# Reverse-engineering scripts (`tools/re/`)

Python 3 with `numpy`; `find.py dis` also needs `capstone`. All take your own
`libg.so` (unzip `lib/arm64-v8a/libg.so` from your APK) and Ghidra addresses
(image base 0x100000). Run with `python3 -I` when the input is a downloaded
file.

| Script | Does |
|---|---|
| `decrypt_strings.py libg.so > strings.tsv` | decrypts the 15 232 encrypted `.data` strings (TIDs, CSV columns, RTTI class names) with their addresses |
| `msgnames.py libg.so strings.tsv > ids.tsv` | every message id (vtable slot 5) and command id (slot 4) with its class name and vtable — this is [ids.tsv](ids.tsv) |
| `find.py libg.so xref ADDR` | code that loads an address (adrp + add/ldr) — use on a string from strings.tsv |
| `find.py libg.so callers ADDR` | bl/b to a function |
| `find.py libg.so dis ADDR [N]` | disassemble until ret |
| `find.py libg.so vtable ADDR [N]` | function pointers of a vtable (from msgnames.py) |
| `sccsv.py clashmini.apk outdir/` | unpacks the game's LZMA CSVs |

## Recipes that worked

- **What does the server have to answer?** Find the message class in
  `ids.tsv`, `vtable` it: slot 2 encodes, slot 3 decodes. Read the decoder
  with `dis`; stream calls go through the stream vtable (`ldr x8,[x8,#0xf8]`
  = readVInt, `#0xd8` readInt, `#0xd0` readBoolean). The receive handler
  0x4549a8 switches on the message id (`mov w8,#id; cmp w21,w8`).
- **Where is a text shown?** `grep TID_... strings.tsv` → `xref` → read around
  it → `callers` of the check it depends on. This found the deletion warning,
  the empty hand and the skin store.
- **What does a command change?** Command vtable slot 3 is execute; follow
  its calls with `dis` to the field it writes (hero: +0x10 of the hero
  collection; skin: entry +0x18).
- capstone over the whole `.text` must use `disasm_lite` with
  `skipdata = True`; detail mode runs out of memory and without skipdata it
  stops at the first literal pool. The numpy scans in `find.py` avoid
  disassembling at all.
