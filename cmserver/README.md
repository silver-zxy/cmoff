# cmserver

Local server for Clash Mini 1.1143.2, in Go. Runs on a PC/Termux
(`go run .`) or inside the game (`cmd/libcmserver`, built by `../build.sh
standalone`). No Supercell key or game data: it makes its own key pair and
keeps the account in `accounts.json`.

    go run . [-listen 0.0.0.0:9339] [-key server.key] [-accounts accounts.json] [-trace]
    go run . -print-pubkey        # the key patch_apk.py needs
    go test ./...

Layout and protocol: [../docs/ARCHITECTURE.md](../docs/ARCHITECTURE.md),
[../docs/PROTOCOL.md](../docs/PROTOCOL.md), [../docs/HOME.md](../docs/HOME.md).
