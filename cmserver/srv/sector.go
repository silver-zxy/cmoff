package srv

// Mini Room work (docs/MINIROOM.md): a test of a server-run battle. With
// Options.SectorFile set and present, the server answers a duel with a
// SectorState the client wrote itself (hook built with SECTOR_DUMP=on)
// instead of the bot signal, and then sends heartbeats.

import (
	"bufio"
	"encoding/hex"
	"fmt"
	"log"
	"net"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"cmserver/pepper"
	"cmserver/proto"
)

const (
	idSectorState     = 22681
	idSectorHeartbeat = 27199
)

var sectorLine = regexp.MustCompile(`sector (\d{5}) ([0-9a-f]+)\s*$`)

// sectorState returns the SectorState for the sector test, or nil (no
// test, or no dump yet: a bot battle, which makes one).
func (s *Server) sectorState() []byte {
	if s.opts.SectorFile == "" {
		return nil
	}
	if _, err := os.Stat(s.opts.SectorFile); err != nil {
		log.Printf("   sector test: no %s yet, a bot battle (it writes one)", s.opts.SectorFile)
		return nil
	}
	var b []byte
	var err error
	if strings.HasSuffix(s.opts.SectorFile, ".bin") {
		b, err = os.ReadFile(s.opts.SectorFile)
	} else {
		b, err = LoadSectorDump(s.opts.SectorFile)
	}
	if err == nil && len(b) == 0 {
		err = fmt.Errorf("empty")
	}
	if err != nil {
		log.Printf("   sector test: %s: %v", s.opts.SectorFile, err)
		return nil
	}
	if s.opts.SectorOnce {
		os.Rename(s.opts.SectorFile, s.opts.SectorFile+"-used")
	}
	return b
}

// LoadSectorDump reads the last SectorState in a log of the hook's sector
// dump (`adb logcat | grep sector`): the lines `sector <offset> <hex>`.
func LoadSectorDump(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var last, cur []byte
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		m := sectorLine.FindStringSubmatch(sc.Text())
		if m == nil {
			continue
		}
		off, _ := strconv.Atoi(m[1])
		b, err := hex.DecodeString(m[2])
		if err != nil {
			return nil, fmt.Errorf("%s: %v", path, err)
		}
		if off == 0 {
			if len(cur) > 0 {
				last = cur
			}
			cur = nil
		}
		if off != len(cur) {
			return nil, fmt.Errorf("%s: offset %d, expected %d", path, off, len(cur))
		}
		cur = append(cur, b...)
	}
	if len(cur) > 0 {
		last = cur
	}
	if len(last) == 0 {
		return nil, fmt.Errorf("%s: no sector dump lines", path)
	}
	return last, sc.Err()
}

// startSectorTest sends the SectorState and then a heartbeat every
// SectorInterval until stop is closed. A heartbeat (decoder 0x341f9c):
// VInt end (the client simulates up to end*10, 0x5b76c4 -> 0x51e518), VInt
// (+0x84, passed along; unknown, 0 here), VInt command count (0).
func (s *Server) startSectorTest(c net.Conn, sess *pepper.Session, state []byte, stop <-chan struct{}) {
	if s.opts.SectorInterval <= 0 {
		s.opts.SectorInterval = 100 * time.Millisecond
	}
	if s.opts.SectorStep <= 0 {
		s.opts.SectorStep = 1
	}
	log.Printf("-> sector test: SectorState %d bytes, heartbeat every %v, +%d each",
		len(state), s.opts.SectorInterval, s.opts.SectorStep)
	s.send(c, sess, idSectorState, state)
	go func() {
		t := time.NewTicker(s.opts.SectorInterval)
		defer t.Stop()
		for n := 1; ; n++ {
			select {
			case <-stop:
				log.Printf("   sector test: stopped after %d heartbeats", n-1)
				return
			case <-t.C:
			}
			w := proto.NewWriter()
			w.VInt(int32(n * s.opts.SectorStep))
			w.VInt(0)
			w.VInt(0)
			s.send(c, sess, idSectorHeartbeat, w.Bytes())
			if n%50 == 0 {
				log.Printf("   sector test: heartbeat %d (end %d)", n, n*s.opts.SectorStep)
			}
		}
	}()
}
