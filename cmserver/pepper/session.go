// Package pepper implements the Supercell "Pepper" transport for Clash Mini
// 1.1143.2: a NaCl crypto_box handshake keyed by a static server keypair,
// then a secretbox-encrypted message stream.
//
// We hold the server private key and patch the client to use our public key,
// so no Supercell key is involved. The wire scheme below is the standard
// Titan/Pepper layout, confirmed against the client's encoders:
//
//	ClientHello (cleartext)
//	ServerHello (cleartext): 24 random "session" bytes
//	Login:   client_pk(32) ‖ box(sessionKey(24) ‖ snonce(24) ‖ payload)
//	         nonce = Blake2b-24(client_pk ‖ server_pk)
//	LoginOk: box(rnonce(24) ‖ key(32) ‖ payload)
//	         nonce = Blake2b-24(snonce ‖ client_pk ‖ server_pk)
//	stream:  secretbox(key); server seals with rnonce, opens with snonce;
//	         each message adds 2 to its 24-byte little-endian nonce first.
//
// box() is NaCl crypto_box: X25519(priv, peer_pub) then HSalsa20 to a
// secretbox key, which is what golang.org/x/crypto/nacl/box does.
package pepper

import (
	"crypto/rand"
	"encoding/binary"
	"errors"

	"golang.org/x/crypto/blake2b"
	"golang.org/x/crypto/curve25519"
	"golang.org/x/crypto/nacl/box"
	"golang.org/x/crypto/nacl/secretbox"
)

type Keypair struct {
	Pub, Priv [32]byte
}

func GenerateKey() (*Keypair, error) {
	pub, priv, err := box.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	return &Keypair{Pub: *pub, Priv: *priv}, nil
}

// PubFromPriv fills Pub from Priv (X25519 base-point mult).
func (k *Keypair) PubFromPriv() {
	pub, _ := curve25519.X25519(k.Priv[:], curve25519.Basepoint)
	copy(k.Pub[:], pub)
}

// blake24 is Blake2b with a 24-byte digest over the concatenation of parts.
func blake24(parts ...[]byte) [24]byte {
	h, _ := blake2b.New(24, nil)
	for _, p := range parts {
		h.Write(p)
	}
	var n [24]byte
	copy(n[:], h.Sum(nil))
	return n
}

// Login is the decrypted content of a Login (10101) body.
type Login struct {
	ClientPub  [32]byte // client's ephemeral public key (from the wire)
	SessionKey []byte   // 24 bytes the client echoes back from ServerHello
	SNonce     [24]byte // client's session nonce (client->server base)
	Payload    []byte   // the LoginMessage fields (accountId, passToken, ...)
}

// OpenLogin decrypts a Login body given the server keypair and the 24-byte
// ServerHello nonce we sent. shNonce is only used to sanity-check the echoed
// sessionKey; decryption does not depend on it.
func OpenLogin(server *Keypair, payload, shNonce []byte) (*Login, error) {
	if len(payload) < 32+box.Overhead+48 {
		return nil, errors.New("login too short")
	}
	var cp [32]byte
	copy(cp[:], payload[:32])

	nonce := blake24(cp[:], server.Pub[:])
	plain, ok := box.Open(nil, payload[32:], &nonce, &cp, &server.Priv)
	if !ok {
		return nil, errors.New("box open failed (wrong server key or scheme)")
	}
	if len(plain) < 48 {
		return nil, errors.New("login plaintext too short")
	}
	l := &Login{ClientPub: cp, SessionKey: plain[:24], Payload: plain[48:]}
	copy(l.SNonce[:], plain[24:48])
	return l, nil
}

// SealLoginReply builds the crypto_box'd body of a LoginOk/LoginFailed reply
// and returns it together with the ready session stream. key and rnonce are
// freshly generated; the client uses them for every message afterwards.
func (l *Login) SealLoginReply(server *Keypair, payload []byte) (body []byte, sess *Session, err error) {
	var key [32]byte
	var rnonce [24]byte
	if _, err = rand.Read(key[:]); err != nil {
		return nil, nil, err
	}
	if _, err = rand.Read(rnonce[:]); err != nil {
		return nil, nil, err
	}

	plain := make([]byte, 0, 24+32+len(payload))
	plain = append(plain, rnonce[:]...)
	plain = append(plain, key[:]...)
	plain = append(plain, payload...)

	nonce := blake24(l.SNonce[:], l.ClientPub[:], server.Pub[:])
	body = box.Seal(nil, plain, &nonce, &l.ClientPub, &server.Priv)

	sess = &Session{key: key, encNonce: rnonce, decNonce: l.SNonce, ready: true}
	return body, sess, nil
}

// Session is the per-connection secretbox stream after LoginOk.
type Session struct {
	key      [32]byte
	encNonce [24]byte // server->client (rnonce), stepped +2
	decNonce [24]byte // client->server (snonce), stepped +2
	ready    bool
}

func (s *Session) Ready() bool { return s != nil && s.ready }

// inc2 adds 2 to a 24-byte little-endian nonce.
func inc2(n *[24]byte) {
	var carry uint16 = 2
	for i := 0; i < 24 && carry != 0; i++ {
		carry += uint16(n[i])
		n[i] = byte(carry)
		carry >>= 8
	}
}

func (s *Session) Encrypt(plain []byte) []byte {
	inc2(&s.encNonce)
	return secretbox.Seal(nil, plain, &s.encNonce, &s.key)
}

func (s *Session) Decrypt(ct []byte) ([]byte, error) {
	inc2(&s.decNonce)
	out, ok := secretbox.Open(nil, ct, &s.decNonce, &s.key)
	if !ok {
		return nil, errors.New("secretbox open failed")
	}
	return out, nil
}

// PutLong writes a LogicLong (two big-endian int32).
func PutLong(hi, lo int32) []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint32(b[0:], uint32(hi))
	binary.BigEndian.PutUint32(b[4:], uint32(lo))
	return b
}
