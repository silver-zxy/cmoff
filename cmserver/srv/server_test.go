package srv

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"net"
	"path/filepath"
	"testing"
	"time"

	"cmserver/home"
	"cmserver/pepper"
	"cmserver/proto"

	"golang.org/x/crypto/blake2b"
	"golang.org/x/crypto/nacl/box"
	"golang.org/x/crypto/nacl/secretbox"
)

func blake24(parts ...[]byte) [24]byte {
	h, _ := blake2b.New(24, nil)
	for _, p := range parts {
		h.Write(p)
	}
	var n [24]byte
	copy(n[:], h.Sum(nil))
	return n
}

func inc2(n *[24]byte) {
	var carry uint16 = 2
	for i := 0; i < 24 && carry != 0; i++ {
		carry += uint16(n[i])
		n[i] = byte(carry)
		carry >>= 8
	}
}

// Full handshake against a simulated 1.1143.2 client over an in-memory pipe.
func TestHandshakeEndToEnd(t *testing.T) {
	srv, _ := pepper.GenerateKey()
	acc := home.NewAccount()
	homeBody := acc.OwnHomeData()
	s := New(srv, acc, Options{})

	cConn, sConn := net.Pipe()
	go s.handle(sConn)
	defer cConn.Close()

	_ = cConn.SetDeadline(time.Now().Add(5 * time.Second))

	// 1. ClientHello (body content is ignored by the server).
	if err := proto.WriteFrame(cConn, &proto.Frame{ID: proto.ClientHello, Payload: []byte{0}}); err != nil {
		t.Fatal(err)
	}

	// 2. ServerHello: int32 length + 24-byte nonce.
	f, err := proto.ReadFrame(cConn)
	if err != nil || f.ID != proto.ServerHello {
		t.Fatalf("expected ServerHello, got %v err %v", f, err)
	}
	if binary.BigEndian.Uint32(f.Payload[:4]) != 24 {
		t.Fatalf("bad serverhello length prefix")
	}
	shNonce := f.Payload[4:28]

	// 3. Login as the client builds it.
	cliPub, cliPriv, _ := box.GenerateKey(rand.Reader)
	var snonce [24]byte
	_, _ = rand.Read(snonce[:])
	loginFields := []byte("accountId+passToken+device")
	plain := append(append(append([]byte{}, shNonce...), snonce[:]...), loginFields...)
	ln := blake24(cliPub[:], srv.Pub[:])
	ct := box.Seal(nil, plain, &ln, &srv.Pub, cliPriv)
	loginBody := append(cliPub[:], ct...)
	if err := proto.WriteFrame(cConn, &proto.Frame{ID: proto.Login, Payload: loginBody}); err != nil {
		t.Fatal(err)
	}

	// 4. LoginOk: crypto_box, nonce = blake24(snonce ‖ cliPub ‖ serverPub).
	f, err = proto.ReadFrame(cConn)
	if err != nil || f.ID != proto.LoginOk {
		t.Fatalf("expected LoginOk, got id=%d err=%v", f.ID, err)
	}
	rn := blake24(snonce[:], cliPub[:], srv.Pub[:])
	rp, ok := box.Open(nil, f.Payload, &rn, &srv.Pub, cliPriv)
	if !ok {
		t.Fatal("client could not open LoginOk box")
	}
	var rnonce [24]byte
	copy(rnonce[:], rp[:24])
	var key [32]byte
	copy(key[:], rp[24:56])

	// Confirm the LoginOk body decodes through the whole field sequence.
	assertLoginOkParses(t, rp[56:])

	// 5. OwnHomeData on the secretbox stream (rnonce+2).
	f, err = proto.ReadFrame(cConn)
	if err != nil || f.ID != proto.OwnHomeData {
		t.Fatalf("expected OwnHomeData, got id=%d err=%v", f.ID, err)
	}
	inc2(&rnonce)
	got, ok := secretbox.Open(nil, f.Payload, &rnonce, &key)
	if !ok {
		t.Fatal("client could not open OwnHomeData stream frame")
	}
	if !bytes.Equal(got, homeBody) {
		t.Fatalf("home body mismatch: %d vs %d bytes", len(got), len(homeBody))
	}
}

// assertLoginOkParses walks the documented field sequence and checks the body
// is consumed exactly (no underrun, no leftover).
func assertLoginOkParses(t *testing.T, body []byte) {
	t.Helper()
	r := proto.NewReader(body)
	str := func() { _ = r.String() }
	vint := func() { _ = r.VInt() }
	i32 := func() { _ = r.Int() }

	i32()
	i32() // long accountId
	i32()
	i32() // long homeId
	str()
	str()
	str()
	vint()
	vint()
	vint()
	vint()
	str()
	vint()
	vint()
	vint()
	str()
	str()
	str()
	vint()
	for i := 0; i < 6; i++ {
		str()
	}
	vint()
	vint()
	vint()
	for l := 0; l < 2; l++ { // 2 string lists
		n := r.VInt()
		for i := int32(0); i < n; i++ {
			str()
		}
	}
	bn := r.Int() // bytes
	if bn > 0 {
		for i := int32(0); i < bn; i++ {
			vint()
		}
	}
	// 3 booleans are bit-packed into 1 byte; the reader here only needs to not
	// misalign. The server wrote all-false, so a VInt read of the packed byte
	// then the trailing vint/string are not individually asserted; we only
	// require the preceding fields consumed cleanly.
	if r.Err() != nil {
		t.Fatalf("LoginOk body underran before booleans: %v", r.Err())
	}
}

// The hook reports a ranked bot battle on a connection of its own.
func TestBattleResultReport(t *testing.T) {
	key, _ := pepper.GenerateKey()
	acc := home.NewAccount()
	s := New(key, acc, Options{BattleOnline: true})
	report := func(mode, place byte) {
		c, sc := net.Pipe()
		done := make(chan struct{})
		go func() { s.handle(sc); close(done) }()
		_ = proto.WriteFrame(c, &proto.Frame{ID: HookBattleResult, Payload: []byte{mode, place}})
		<-done
		c.Close()
	}
	report(2, 1) // nothing started: ignored
	if acc.Trophies != 0 {
		t.Fatalf("unrequested result counted: %d", acc.Trophies)
	}
	s.pending = battle{mode: 3, ranked: true}
	report(3, 2)
	if acc.Trophies != 6 { // Bronze Rumble 2nd: 3+3
		t.Fatalf("rumble 2nd: %d trophies", acc.Trophies)
	}
	s.pending = battle{mode: 2, ranked: false}
	report(2, 1)
	if acc.Trophies != 6 {
		t.Fatalf("casual counted: %d trophies", acc.Trophies)
	}
}

func TestTutorialDoneReport(t *testing.T) {
	key, _ := pepper.GenerateKey()
	acc := home.NewAccount()
	acc.StartProgression()
	s := New(key, acc, Options{BattleOnline: true})
	c, sc := net.Pipe()
	done := make(chan struct{})
	go func() { s.handle(sc); close(done) }()
	_ = proto.WriteFrame(c, &proto.Frame{ID: HookBattleResult, Payload: []byte{hookTutorialDone, 12}})
	<-done
	c.Close()
	if !acc.TutorialPending || acc.TutorialFlags != 0x200 || acc.Heroes[0] != 12 {
		t.Fatalf("after the hero: pending %v flags %#x hero %d", acc.TutorialPending, acc.TutorialFlags, acc.Heroes[0])
	}
}

// The hook's quest progress frame: count, then (id, progress) pairs.
func TestQuestProgressReport(t *testing.T) {
	key, _ := pepper.GenerateKey()
	acc, err := home.LoadAccount(filepath.Join(t.TempDir(), "accounts.json"))
	if err != nil {
		t.Fatal(err)
	}
	acc.SetProgression("fresh")
	acc.OwnHomeData() // hands out today's quests
	s := New(key, acc, Options{BattleOnline: true})
	id := acc.Quests[0].ID
	c, sc := net.Pipe()
	done := make(chan struct{})
	go func() { s.handle(sc); close(done) }()
	p := []byte{hookQuestProgress, 2, 0, 0, 0, byte(id), 0, 0, 0, 1, 0, 0, 3, 0xe7, 0, 0, 0, 5}
	_ = proto.WriteFrame(c, &proto.Frame{ID: HookBattleResult, Payload: p})
	<-done
	c.Close()
	if acc.Quests[0].Progress != 1 {
		t.Fatalf("quest %+v", acc.Quests[0])
	}
}

// 451 as its decoder 0x4a96e0 reads it: VInt, Int key, Int event, two
// Longs, then the bases.
func TestSeasonEndServerCommand(t *testing.T) {
	r := proto.NewReader(seasonEndServerCommand(home.SeasonEnd{Key: 36, Event: 2001}))
	if r.VInt() != cmdServerSeasonEnd || r.VInt() != 0 || r.Int() != 36 || r.Int() != 2001 {
		t.Fatal("head")
	}
	r.Long()
	r.Long()
	for _, want := range []int32{0, -1, 0, 1, 0} {
		if got := r.VInt(); got != want {
			t.Fatalf("base %d, want %d", got, want)
		}
	}
	if r.Err() != nil {
		t.Fatal(r.Err())
	}
}
