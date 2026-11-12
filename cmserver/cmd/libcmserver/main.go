// Command libcmserver builds the server as a shared library that runs inside
// the game process (the "standalone" APK: no PC, no adb reverse).
//
//	CGO_ENABLED=1 GOOS=android GOARCH=arm64 CC=<ndk>/aarch64-linux-android21-clang \
//	    go build -buildmode=c-shared -o libcmserver.so ./cmd/libcmserver
//
// The client hook (cmoffline, -DCMOFFLINE_LOCAL_SERVER) dlopens it and calls
// CmServerStart with the app's files folder and the server private key. The
// account (accounts.json) is kept in that folder.
package main

/*
#include <stdlib.h>
*/
import "C"

import (
	"log"
	"net"
	"path/filepath"
	"strings"
	"sync"

	"cmserver/home"
	"cmserver/srv"
)

var once sync.Once

// CmServerStart starts the server on 127.0.0.1:9339 in the background.
// dataDir: writable folder for accounts.json. keyHex: server private key.
// options: space-separated key=value, e.g. "progression=fresh" (see
// cmserver -progression). Returns 0 when listening, -1 on error (see
// logcat, tag cmserver).
//
//export CmServerStart
func CmServerStart(dataDir, keyHex, options *C.char) C.int {
	ret := C.int(0)
	once.Do(func() {
		setupLog()
		dir, hexKey := C.GoString(dataDir), C.GoString(keyHex)
		opts := map[string]string{}
		if options != nil {
			for _, kv := range strings.Fields(C.GoString(options)) {
				if k, v, ok := strings.Cut(kv, "="); ok {
					opts[k] = v
				}
			}
		}
		key, err := srv.KeyFromHex(hexKey)
		if err != nil {
			log.Printf("key: %v", err)
			ret = -1
			return
		}
		home.Dev = opts["dev"] == "on"
		acc, err := home.LoadAccount(filepath.Join(dir, home.AccountFile(home.Dev)))
		if err != nil {
			log.Printf("account: %v", err)
			ret = -1
			return
		}
		ln, err := net.Listen("tcp", "127.0.0.1:9339")
		if err != nil {
			log.Printf("listen: %v", err)
			ret = -1
			return
		}
		if apk := home.FindOwnAPK(); apk == "" {
			log.Printf("game data: APK not found, default rewards")
		} else if d, err := home.LoadGameData(apk); err != nil {
			log.Printf("game data: %v, default rewards", err)
		} else {
			home.Data = d
		}
		home.Tutorial = opts["tutorial"] != "off"
		home.ChestsInHome = opts["chests"] != "off"
		home.Live = opts["live"] != "off"
		home.WinFragments = opts["winfragments"] == "on"
		if msg, err := acc.ApplyProgression(opts["progression"]); err != nil {
			log.Printf("account: %v", err)
		} else if msg != "" {
			log.Printf("%s", msg)
		}
		if msg := acc.CatchUp(); msg != "" {
			log.Printf("%s", msg)
		}
		if home.Dev {
			// dev mode: the season ends at every start, to test it
			log.Printf("dev: %s", acc.DevStart())
		}
		log.Printf("account %q in %s, progression %v, dev %v", acc.Name, dir, acc.Progression, home.Dev)
		o := srv.Options{BattleOnline: true}
		if opts["sectortest"] == "on" {
			// docs/MINIROOM.md: the hook (SECTOR_DUMP=on) writes sector.bin
			// in a bot battle; the next duel gets it as SectorState
			o.SectorFile, o.SectorOnce = filepath.Join(dir, "sector.bin"), true
		}
		s := srv.New(key, acc, o)
		go func() { log.Printf("server stopped: %v", s.Serve(ln)) }()
	})
	return ret
}

func main() {}
