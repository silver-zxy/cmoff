# Eine andere Clash-Mini-Version anbinden

Diese Anleitung beschreibt, wie der Server und der APK-Patch für Clash Mini
**1.1143.2** entstanden sind, und wie man dasselbe für eine andere Version
(ein anderes Update) wiederholt. Sie ist bewusst auf Deutsch; die
technischen Details stehen ausführlicher in [SERVER.md](SERVER.md),
[RE.md](RE.md) und [STRINGS.md](STRINGS.md).

Alle Adressen sind **Ghidra-Adressen** (Image-Base 0x100000). Im Speicher des
Handys ist es `libg-Basis + (Ghidra-Adresse − 0x100000)`; der Hook rechnet
das mit dem Makro `G(addr)` um.

## Das Ergebnis in einem Satz

Der originale Client (unverändertes `libg.so`, nur ein anderer
Server-Schlüssel) verbindet sich über eine kleine Hook-Bibliothek mit dem
eigenen Server (`crates/cm-server`). Der Server liefert Login und Home
(mit allen Minis und Helden auf Maximalstufe) und speichert Name und Decks.
Ein Druck auf **Duel** startet das eingebaute Bot-Duell des Clients gegen
`OfflineMatchNPC`, mit dem Deck vom Server. Der Server kann auf einem PC
laufen oder **in der APK selbst** (`--local-server`); dann braucht das Spiel
weder PC noch Internet.

## Bausteine

| Teil | Datei | Was es tut |
|---|---|---|
| Server | `crates/cm-server` | Protokoll (Pepper/RC4), Login, OwnHomeData, Accounts, Befehle (Name, Deck), GoHome |
| Server in der APK | `crates/cm-server-android` | derselbe Server als `libcmserver.so`, startet im Spielprozess |
| Accounts | `server-data/accounts.json` (nicht im Repo) | ein Eintrag pro Spieler, von Hand änderbar, wird live nachgeladen |
| APK-Patch | `scripts/patch_apk.py` | Server-Schlüssel in libg, Hook-Bibliothek dazu, neu signieren |
| Hook | `android/cmredirect/cmredirect.c` | Host umleiten (GOT), Offline-Flag steuern, Duel auslösen, Absturzbericht |
| Ghidra-Skripte | `scripts/ghidra/*.java` | Dekompilieren, Aufrufer, Referenzen, Vtables, Befehlssuche |
| Strings | `scripts/decrypt_libg.py` | entschlüsselt die Strings in libg (Grundlage fürs Suchen) |
| Message-IDs | `scripts/message_ids.py` | liest die (pro Version gemischten) Nachrichten-IDs aus libg |

## Werkzeuge

- **Ghidra** (hier 12.1.4) mit einem Projekt, in das `libg.so` (arm64-v8a)
  importiert und analysiert ist. Headless aufrufen, z. B.:

  ```
  analyzeHeadless <projekt-ordner> <projekt> -process libg.so -noanalysis -readOnly \
    -scriptPath scripts/ghidra -postScript DecompileTree.java 0x596f24
  ```

  - `Decompile.java <adr>…`: die Funktionen, die diese Adressen enthalten
  - `DecompileTree.java <adr>` mit `CM_OUT=ordner CM_DEPTH=n`: eine Funktion
    und alles, was sie aufruft
  - `Callers.java <adr>`: wer ruft diese Funktion auf
  - `RefsTo.java <adr>`: wer verweist auf eine Adresse (z. B. einen String)
  - `Vtable.java <adr>:<n>`: Funktionszeiger einer Vtable
  - `InsnGrep.java "<regex>" …`: Funktionen mit passenden Befehlen, z. B.
    `"str w.*#0x15c\]"` für alle Schreibzugriffe auf Offset 0x15c
- **Android SDK** mit build-tools (zipalign, apksigner), **NDK** (baut den
  Hook) und **adb**; **Java** und **apktool** (`~/tools/apktool`).
- **Python 3** (mit Pillow für Screenshot-Collagen).
- **BlueStacks 5**, Instanz *Pie64* (Android 9, arm-Übersetzung „houdini“),
  adb unter `127.0.0.1:5555`. Googles Emulator-Images können libg.so nicht
  ausführen. Grafikfehler bei Effekten gibt es nur in BlueStacks.

## Schritt für Schritt für eine neue Version

### 1. APK und Spieldaten

APK besorgen und nach `apk/` entpacken (oder `CLASH_MINI_GAME` setzen).
`cargo xtask check` prüft, dass alle Daten lesbar sind. CSVs: `csv/`
(`scripts/decompress_csv.py`). **Nichts davon ins Repo committen.**

### 2. libg.so verstehen

1. `libg.so` in Ghidra importieren und analysieren lassen.
2. **Strings entschlüsseln.** Der erste Konstruktor in `.init_array`
   entschlüsselt beim Laden ~15 000 Strings in `.data` (XOR mit einem
   128-Byte-Schlüssel, Tabelle mit 12-Byte-Einträgen). In
   `decrypt_libg.py` stehen dafür `TABLE` und `KEY`. Bei einer neuen
   Version: den ersten `.init_array`-Eintrag dekompilieren, darin die
   Tabelle und den Schlüssel finden, die Konstanten anpassen. Ergebnis:
   `strings.tsv` (Adresse, Länge, Text). Damit findet man Code über
   Strings: `RefsTo.java <string-adresse>`.
3. **Message-IDs.** `message_ids.py libg_decrypted.so strings.tsv` liefert
   `id → Name` (über RTTI und Vtable-Slot 5 `getMessageType`). Die Tabelle
   kommt als neues Profil nach `crates/cm-server/src/messages.rs`.
   Fast alle IDs sind pro Version neu gemischt.

### 3. Server-Schlüssel und Umleitung (`patch_apk.py`)

libg prüft beim Laden seine eigenen Bytes (Code, `.rodata`, verschlüsselte
`.data`-Strings). Jede Änderung dort führt zum Absturz (Fehleradresse
`0xdead0000`). **Nur die Schlüsselhälfte k0 darf geändert werden.**

- **Schlüssel:** Der öffentliche Server-Schlüssel ist aus zwei Hälften
  zusammengesetzt: `limb_i = rotr16(k0_i, 3 + i % 8) − k1_i`. k0 liegt im
  Klartext in `.rodata`, k1 verschlüsselt in `.data`. Gefunden über
  `_INIT_4` (kopiert beide) und die Funktion, die daraus den Schlüssel
  baut. In `patch_apk.py`: `K0`, `K1`, `STR_KEY`, `HOST` (Adresse des
  Strings `game.prod.clashminigame.com`, zum Prüfen der Version).
- **Host:** Wird nicht in libg geändert. Die Hook-Bibliothek
  `libcmredirect.so` wird per apktool direkt nach `System.loadLibrary("g")`
  in `GameApp.smali` geladen (`LOAD_G` in `patch_apk.py`; die Zeilennummer
  `.line 79` kann sich ändern). Ihr Konstruktor überschreibt den
  **GOT-Eintrag** von `getaddrinfo` in libg.
- **GOT-Offsets** findet man ohne Ghidra:
  `llvm-readelf -r libg.so | grep -E "JUMP_SLOT.* (getaddrinfo|read|recvfrom|write|sendto)@"`
  (JUMP_SLOT-Relocations; `llvm-readelf` liegt im NDK unter
  `toolchains/llvm/prebuilt/*/bin`). In `cmredirect.c`: `GOT_OFFSET`, `GOT_READ`,
  `GOT_RECVFROM`, `GOT_WRITE`, `GOT_SENDTO`.
- Warum der Umweg über die GOT: Code-Patches zur Laufzeit (Frida, Inline
  Hooks) stürzen unter BlueStacks' houdini ab. Ein Zeiger in der GOT ist
  Daten, das funktioniert überall.

### 4. Protokoll und Login

Bei 1.1143.2: Pepper-Handshake (Curve25519 `crypto_box`, Nonces per
Blake2b, danach `secretbox` mit Nonce +2 pro Nachricht) und das ältere RC4.
Siehe [SERVER.md](SERVER.md). Neue Version: Login (`0x31d45c`) und LoginOk
(`0x78fdf4`) neu prüfen. Die Message-Klassen findet man über ihre Vtables
(`message_ids.py` gibt sie mit aus; Slot 5 = `getMessageType`), Encoder und
Decoder über `Vtable.java` und die Funktionen in den Slots davor.

### 5. OwnHomeData (das Home)

Der Client bleibt im Ladebildschirm, bis OwnHomeData passt. Das Format ist
`LogicClientHome ‖ LogicClientAvatar ‖ 4 VInts`. `own_home.rs` bildet die
Encoder des Clients **Schreibaufruf für Schreibaufruf** nach:

- Home-Encoder `0x596f24`, Avatar-Encoder `0x5cb380` (Avatar-Vtable
  `0xea8290`, Slot 3 encode, Slot 4 decode). Mit `DecompileTree.java`
  ausgeben und jeden `writeVInt`/`writeBoolean`/… übernehmen.
- Stream-Vtable (`0xecba90`): `0x40` writeBoolean, `0x80` writeVInt,
  `0xd0` readBoolean, `0xd8` readInt, `0xf8` readVInt, `0x108` readLong,
  `0xa8` readString. Booleans werden zu 8 pro Byte gepackt, bis zum
  nächsten Nicht-Boolean.
- Wichtige Felder: Home +0x28 Minis (characters-Zeile, Level), +0x38
  Deck-Slots, +0x80 gespeicherte Decks, +0x90 Held pro Deck, +0x20
  eigene Helden. Avatar +0x130 Tutorial-Flags (0x100 = Tutorial-Kampf
  überspringen), +0xbc Erfahrungsstufe (mindestens 1).
- **Collectible-Level** (Sammlung, Sterne, Heldenlevel): die dritte von
  drei Maps bei Home +0x200 (`0x39d64c`), Schlüssel = `collectibleId` aus
  `chests/collectibles.json` (= globale ID: Klasse × 1 000 000 + Zeile,
  Minis Klasse 36, Helden Klasse 107), Wert = Level. Ohne Eintrag zeigt die
  Sammlung „Locked“. Maximum = Anzahl der `upgradeFragments` (Minis 4 =
  Basis + 3 Sterne, Helden 10). Gefunden über die Heldenlevel-Abfrage
  `0x5f5c60 → 0x308864 → 0x473440 (home + 0x200)`.

Testen ohne Handy: `cargo run -p cm-server -- probe 127.0.0.1:9339 --key …`.

### 6. Bot-Duelle (der eigentliche Trick)

Einen Kampf vom Server aus zu bauen (SectorStateMessage) ist sehr
aufwendig: Der Client legt Deck, Shop und Zeiten beim Erstellen des Kampfes
fest. Die Ansätze und Funde dazu stehen in `crates/cm-server/src/sector.rs`.
Viel einfacher: Der Client hat ein **eigenes Offline-Duell** gegen den NPC
`OfflineMatchNPC` (npcs.csv) mit echter Bot-KI. Der Hook schaltet es zur
richtigen Zeit ein (Modus `--offline-battles after-home`):

1. **Offline-Flag:** ein Byte (1.1143.2: `0x118abd8`, `.bss`), gelesen von
   `0x3b4300`. Gefunden über `MessageManager`/Offline-Strings
   (`offline_home.json`, `OfflineMatchNPC` in `strings.tsv`, dann
   `RefsTo.java`).
2. **Home online:** Das Flag bleibt im Home aus. Ist es beim Laden an,
   nimmt der Client seinen lokalen Offline-Spielstand (leeres Deck nach dem
   Tutorial), und Befehle gehen nicht an den Server.
3. **Duel-Knopf:** `0x7985ec` ruft `0x973448(…, Modus, 0)` auf. Diese
   Funktion setzt das Flag auf den letzten Parameter (0) und führt sofort
   `LogicStartMatchmakeCommand` (582) aus. Dessen Listener `0x59e8a0`
   merkt sich das Match nur bei gesetztem Flag, in **Spielmodus +0x15c**
   (Modus) und **+0x160**. Weil das Flag im Home aus ist (damit Name und
   Deck beim Server ankommen, siehe 6b), geht der Befehl an den Server.
4. **Signal:** Der Server antwortet auf 582 mit einem **KeepAliveServer**
   (20108), um das der Client nicht gebeten hat. Der Hook zählt die
   KeepAlives des Clients (10108; Header sind unverschlüsselt) und erkennt
   so das Signal. Dann setzt er das Flag und trägt `+0x15c = 2` (Modus aus
   den Befehls-Bytes `8609 … 02 7f`) und `+0x160 = 0` ein.
   Wichtig: Ein **SectorState** als Signal geht nicht. Bei gesetztem Flag
   verwirft der Client Nachrichten nicht, sondern hält sie zurück und
   verarbeitet sie, sobald er wieder online ist. Dann ersetzt der
   Server-Kampf das Bot-Duell (leerer Shop, „You have deployed all your
   minis“).
5. **Start:** `0x467e78` (Home-Update) sieht +0x15c und ruft `0x940664` mit
   `OfflineMatchNPC` auf, der eingebaute Aufbau eines Offline-Kampfes.
6. **Zurück:** Eine Sekunde nach Kampfbeginn (Spielzustand 5) schaltet der
   Hook das Flag wieder aus. So schickt „Exit“ ein `GoHomeMessage`, und der
   Server antwortet mit OwnHomeData. Mit Flag würde der lokale Spielstand
   geladen (leeres Deck nach dem Tutorial).

Hilfsadressen (1.1143.2): Spielzustand `*(*0x1178728 + 8)` (4 = Home,
5 = Kampf), Verbindungszustand `**0x118a928` (6 = eingeloggt), Spielmodus
`0x5952bc()`, MessageManager `*0x1184d48`.

### 6b. Name und Deck speichern (Befehle)

Was der Spieler im Home ändert, führt der Client selbst aus und schickt es
als **Befehl** in `EndClientTurnMessage` (17306) an den Server:
`tick, checksum, Anzahl, [Typ + Daten]…`. Der Server (`commands.rs`) liest
die Befehle und übernimmt sie in den Account; beim nächsten Home (Neustart,
Exit nach dem Kampf) schickt er sie zurück.

Jeder Befehl beginnt mit der Basis von `LogicCommand` (Encoder `0x8f2a40`:
zwei VInts, dann die Account-ID als zwei VInts). Die Layouts stehen in den
Encodern der Befehls-Vtables (Slot 5; Slot 3 führt aus, Slot 4 = Typ-ID):

| Typ | Befehl | Daten nach der Basis |
|---|---|---|
| 500 | LogicSwapChessCommand | Mini (Zeile in seasonal_character_mapping, −1 = keins), Slot 0–4, zweiter Slot (−1 = keiner): einsetzen, tauschen oder leeren |
| 549 | LogicChangeAvatarNameByUserCommand | String Name (steht **vor** der Basis) |
| 582 | LogicStartMatchmakeCommand | Modus, VInt, 2 Bools |
| 583 | LogicSelectSupportHeroCommand | globale Helden-ID |
| 595 | LogicAddTroopToDeckCommand | globale Mini-ID, Bool hinzufügen/entfernen |
| 597 | LogicSelectDeckCommand | Deck-Index 0–2 |

Die Vtables findet man wie die der Nachrichten über RTTI (Klassenname aus
`strings.tsv`, mit dem Code aus `message_ids.py`). Die Typ-IDs ändern sich
zwischen Versionen vermutlich wie die Message-IDs.

**Namensdialog:** Der Namensbefehl setzt im Avatar **+0x135 = 1** („Name vom
Spieler gesetzt“, `0x8eb1dc`) und erhöht **+0x138** (Anzahl der
Namensänderungen). Solange +0x135 im OwnHomeData 0 ist, fragt der Client bei
jedem Start nach einem Namen. Der Server setzt beides, sobald ein Name
gespeichert ist.

### 7. Testen auf BlueStacks

- Emulator-APK bauen und installieren:
  `patch_apk.py <apk> out.apk --key <hex> --host 10.0.2.2 --offline-battles after-home --trace`
  (`10.0.2.2` ist in BlueStacks der PC).
- `--trace` loggt Spiel-/Verbindungszustand und Offline-Flag
  (`adb logcat | grep cmredirect`). `--dump-sector` schreibt die Bytes
  jedes Kampfstarts ins Log.
- **Absturzbericht:** Der Hook fängt SIGSEGV ab und loggt `pc`, `lr` und
  die Frame-Kette als Ghidra-Adressen. Die Adressen direkt mit
  `Decompile.java` öffnen.
- Stolperfallen:
  - Der alte Server hält Port 9339 manchmal noch kurz; dann startet der neue
    nicht („Adresse wird bereits verwendet“), und der Client hängt am alten.
    Im Server-Log auf `listening` achten.
  - Screenshots: `adb exec-out screencap -p > bild.png`; Taps:
    `adb shell input tap x y` (Bildschirm 900×1600).
  - Nach einem PC-Neustart BlueStacks neu starten:
    `HD-Player.exe --instance Pie64`.

### 8. Handy-APK

Zwei Varianten:

**Eigenständig (Server in der APK, kein PC nötig):**

```
python scripts/patch_apk.py <apk> clashmini-standalone.apk --local-server server-data/cm-server.key
```

**Mit Server am PC** (mehrere Geräte teilen sich die Accounts):

```
python scripts/patch_apk.py <apk> clashmini-handy.apk --key <öffentlicher schlüssel> --host <PC-IP im WLAN> --offline-battles after-home
```

Server am PC: `cargo run -p cm-server --release -- run --addr 0.0.0.0:9339
--key server-data/cm-server.key --game . --accounts server-data/accounts.json`.
Windows-Firewall für privates Netz erlauben.

Eine Play-Store-Version vorher deinstallieren (andere Signatur).

**Andere Sprache** (z. B. Russisch):

```
python scripts/patch_apk.py <apk> clashmini-standalone-ru.apk --local-server server-data/standalone.key --texts ru.csv --text "TID_SELECT_GAMEMODE_BUTTON=РЕЖИМЫ"
```

1.1143.2 kennt nur Englisch und Spanisch (`assets/csv_client/texts.csv`:
`Name,EN,ES`). `--texts` ersetzt die englische Spalte durch eine
Übersetzung im Format der Sprachdateien späterer Versionen
(`Name,RU` plus Typzeile); fehlende Schlüssel bleiben englisch (bei
`ru.csv` 62 von 1636, vor allem Heldenfähigkeiten). Die Datei wird wieder
im Format des Spiels gepackt (LZMA mit 9-Byte-Kopf: 5 Byte Eigenschaften,
4 Byte Größe). `--text KEY=TEXT` ersetzt einzelne Texte, etwa einen
Knopftext, der sonst mitten im Wort umbricht. Kyrillisch ist in den beiden
Hauptschriften (`supercell-magic_0.ttf`, `sgb_20170216.ttf`) enthalten; für
andere Schriftsysteme vorher die Schriften prüfen (z. B. mit fontTools).

### 9. Server in der APK (`--local-server`)

So funktioniert die eigenständige Variante:

1. `crates/cm-server-android` ist derselbe Server als Bibliothek
   (`cdylib`). Gebaut wird sie für `aarch64-linux-android` mit dem Linker
   aus dem NDK (`aarch64-linux-android21-clang`); `patch_apk.py` erledigt
   das. Einmalig nötig: `rustup target add aarch64-linux-android`.
2. Der geheime Server-Schlüssel wird nach dem Bauen in einen Platzhalter
   der Bibliothek geschrieben (Text `CMSERVER_SECRET` + Nullbyte, danach 64
   Hex-Zeichen). Den öffentlichen Schlüssel für libg berechnet
   `patch_apk.py` daraus selbst (X25519). Die Variable darf nicht wie der
   Platzhalter heißen, sonst steht der Text auch in den Symboltabellen.
3. `libcmserver.so` kommt neben `libcmredirect.so` nach `lib/arm64-v8a/`.
   Mit der Option `local` lädt der Hook sie per `dlopen` und ruft
   `cm_server_start(apk, ordner)` auf. Der Server läuft als Thread im
   Spielprozess auf `127.0.0.1:9339`, und die Umleitung zeigt dorthin.
4. Die Spieldaten liest der Server direkt aus der installierten APK
   (`/data/app/<paket>-<id>/base.apk`; der Hook leitet den Pfad aus dem
   Pfad von `libg.so` ab). Accounts und Log liegen im privaten Ordner der
   App: `/data/data/<paket>/files/cm-server/` (`accounts.json`,
   `server.log`). Deinstallieren löscht sie.
5. Für eine neue Version ist hier nichts zusätzlich anzupassen; es gilt
   alles, was der Server ohnehin braucht (Message-IDs, Home-Format).

## Checkliste: versionsabhängige Werte

Für eine neue Version sind genau diese Werte neu zu bestimmen
(1.1143.2 als Vorlage):

| Wert | 1.1143.2 | Wo | Wie finden |
|---|---|---|---|
| String-Tabelle / XOR-Schlüssel | `0xF746E8` / `0xFBD037` (vaddr) | `decrypt_libg.py`, `patch_apk.py STR_KEY` | erster `.init_array`-Konstruktor |
| Host-String | `0xF5E104` (vaddr) | `patch_apk.py HOST` | `strings.tsv`: `game.prod.clashminigame.com` |
| k0 / k1 | `0xBC3DE0` / `0xF7342C` (vaddr) | `patch_apk.py K0/K1` | `_INIT_4`, Schlüsselaufbau (`rotr16`) |
| loadLibrary("g") in smali | `.line 79` | `patch_apk.py LOAD_G` | `GameApp.smali` nach apktool |
| GOT getaddrinfo, read, recvfrom, write, sendto | `0xDF4FC8`, `0xDF4C08`, `0xDF43E8`, `0xDF6D18`, `0xDF4BF0` (Datei-Offset) | `cmredirect.c` | `llvm-readelf -r` |
| Message-IDs | `messages.rs` Profil `V1_1143` | `messages.rs` | `message_ids.py` |
| Offline-Flag | `0x118abd8` (libg + `0x108ABD8`) | `cmredirect.c OFFLINE_FLAG` | Leser `0x3b4300`, Schreiber `0x973448` |
| Spielzustand, Verbindung | `0x1178728`, `0x118a928` | `cmredirect.c` (trace) | Message-Handler `0x4549a8` prüft `*0x118a928 != 6` |
| Spielmodus-Getter | `0x5952bc` | `cmredirect.c` | häufig aufgerufen, liest Zustand 4/5 |
| Duel-Match: Offset / Modus | `+0x15c`, `+0x160` / 2 | `cmredirect.c start_offline_duel` | Listener `0x59e8a0`, Befehl 582 |
| Offline-Duell-Start | `0x467e78` → `0x940664` | (nur zum Verständnis) | `RefsTo` auf String `OfflineMatchNPC` |
| Home-/Avatar-Encoder | `0x596f24` / `0x5cb380` | `own_home.rs` | OwnHomeData-Encoder `0x94f060` |
| Collectible-Map | Home +0x200 (3. Map) | `own_home.rs encode_maps` | `0x39d64c` im Home-Encoder |
| Befehls-Typen und -Layouts | 500, 549, 582, 583, 595, 597 | `commands.rs` | RTTI-Vtables, Slot 4 (Typ) und 5 (encode) |
| Name gesetzt / Namensänderungen | Avatar +0x135 / +0x138 | `own_home.rs` | Ausführung von 549 (`0x3286a0`) |

Sinnvolle Reihenfolge bei einer neuen Version:
**Strings → Message-IDs → k0/k1 + GOT → Login → OwnHomeData → Offline-Flag
→ Duel.** Jede Stufe lässt sich einzeln testen (Probe, Logcat, Screenshot).

## Accounts

`server-data/accounts.json`, ein Eintrag pro Account (`"0-1"`, …):
`name`, `exp_level`, `minis` (Name + Level), `heroes`, `decks`,
`selected_deck`, `skip_tutorial_battle`, `unlock_all`.

- `unlock_all` (Standard **an**): alle Minis und Helden der Spieldaten auf
  Maximalstufe, unabhängig von `minis`/`heroes`. Auf `false` setzen, um die
  eigenen Listen zu verwenden.
- Änderungen an der Datei lädt der Server automatisch nach; ist der Spieler
  online, wird er kurz getrennt und lädt neu.
- Name, gewähltes Deck, Held und Minis im Deck speichert der Server selbst,
  wenn der Spieler sie im Spiel ändert.
- In der eigenständigen APK liegt die Datei auf dem Handy
  (`/data/data/com.supercell.clashmini/files/cm-server/accounts.json`) und
  ist ohne Root nicht von außen änderbar; `unlock_all` ist dort immer an.

## Was noch fehlt

- Kampfergebnisse (Gold, Trophäen, Truhen) werden nicht auf dem Server
  gespeichert; der Client rechnet sie lokal.
- `LogicRandomizeSelectedDeckCommand` (596, Würfel im Deck-Editor) wird
  nicht nachgebildet; ein so gewürfeltes Deck geht beim nächsten Home
  verloren.
- Spieler gegeneinander (PvP) braucht das Kampfprotokoll (SectorState,
  UDP); die Vorarbeit steht in `sector.rs` und SERVER.md.
