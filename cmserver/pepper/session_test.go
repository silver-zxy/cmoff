package pepper

import (
	"bytes"
	"crypto/rand"
	"testing"

	"golang.org/x/crypto/nacl/box"
	"golang.org/x/crypto/nacl/secretbox"
)

// Emulate the client: build a Login exactly as the client does, then confirm
// OpenLogin recovers it and that the reply round-trips through the stream.
func TestLoginRoundTrip(t *testing.T) {
	srv, _ := GenerateKey()
	cliPub, cliPriv, _ := box.GenerateKey(rand.Reader)

	shNonce := make([]byte, 24)
	_, _ = rand.Read(shNonce)
	var snonce [24]byte
	_, _ = rand.Read(snonce[:])

	// Login plaintext: sessionKey(24) ‖ snonce(24) ‖ payload
	payload := []byte("login-fields-accountId-passToken")
	plain := append(append(append([]byte{}, shNonce...), snonce[:]...), payload...)

	nonce := blake24(cliPub[:], srv.Pub[:])
	ct := box.Seal(nil, plain, &nonce, &srv.Pub, cliPriv)
	body := append(cliPub[:], ct...)

	l, err := OpenLogin(srv, body, shNonce)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(l.SessionKey, shNonce) {
		t.Fatal("sessionKey echo mismatch")
	}
	if l.SNonce != snonce {
		t.Fatal("snonce mismatch")
	}
	if !bytes.Equal(l.Payload, payload) {
		t.Fatalf("payload mismatch: %q", l.Payload)
	}

	// Server reply + stream.
	replyPayload := []byte("login-ok-fields")
	reply, sess, err := l.SealLoginReply(srv, replyPayload)
	if err != nil {
		t.Fatal(err)
	}

	// Client side: open the reply with crypto_box.
	rnonce := blake24(snonce[:], cliPub[:], srv.Pub[:])
	rp, ok := box.Open(nil, reply, &rnonce, &srv.Pub, cliPriv)
	if !ok {
		t.Fatal("client failed to open LoginOk")
	}
	if len(rp) < 56 {
		t.Fatal("reply plaintext too short")
	}
	var clientRNonce, clientSNonce [24]byte
	copy(clientRNonce[:], rp[:24])
	var key [32]byte
	copy(key[:], rp[24:56])
	copy(clientSNonce[:], snonce[:])
	if !bytes.Equal(rp[56:], replyPayload) {
		t.Fatal("reply payload mismatch")
	}

	// First stream message: server seals (encNonce), client opens with rnonce+2.
	msg := []byte("OwnHomeData-goes-here")
	wire := sess.Encrypt(msg)
	inc2(&clientRNonce)
	got, okk := secretbox.Open(nil, wire, &clientRNonce, &key)
	if !okk || !bytes.Equal(got, msg) {
		t.Fatal("client could not open first stream message")
	}

	// Client sends; server opens with decNonce (snonce+2).
	inc2(&clientSNonce)
	up := secretbox.Seal(nil, []byte("command"), &clientSNonce, &key)
	down, err := sess.Decrypt(up)
	if err != nil || !bytes.Equal(down, []byte("command")) {
		t.Fatalf("server could not open client message: %v", err)
	}
}
