# Reference notes

The original reverse-engineering notes by **.gnoks**, from the project this
one builds on (Rust server `cm-server`, `cmredirect` hook, Ghidra scripts).
They are kept unchanged; file and script names in them refer to his
repository, not this one. Where this repo found something different, the
docs one level up say so (e.g. game state is at GameMain+0x28, not +8).

| File | About |
|---|---|
| RE.md | libg.so layout, string encryption, findings from the decrypted strings |
| SERVER.md | protocol, OwnHomeData, bot duels, what PvP would need |
| PORTING.md | (German) step-by-step porting guide and per-version offsets |
| STRINGS.md | notable decrypted strings |
| CRYPTO_NOTES.md | handshake crypto investigation |
| PROGRESS.md | his progress log |
