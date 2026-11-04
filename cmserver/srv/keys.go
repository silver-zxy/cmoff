package srv

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"

	"cmserver/pepper"
)

// LoadOrMakeKey reads the server's private key (64 hex chars) from path, or
// creates and saves a new one. The client is patched to trust its public key
// (cmoffline/tools/patch_apk.py); no Supercell key is ever used.
func LoadOrMakeKey(path string) (*pepper.Keypair, error) {
	if b, err := os.ReadFile(path); err == nil {
		return KeyFromHex(string(bytes.TrimSpace(b)))
	}
	k, err := pepper.GenerateKey()
	if err != nil {
		return nil, err
	}
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	if err := os.WriteFile(path, []byte(hex.EncodeToString(k.Priv[:])), 0o600); err != nil {
		return nil, err
	}
	return k, nil
}

// KeyFromHex builds the keypair from a private key in hex.
func KeyFromHex(s string) (*pepper.Keypair, error) {
	raw, err := hex.DecodeString(s)
	if err != nil || len(raw) != 32 {
		return nil, fmt.Errorf("server key must be 32 bytes of hex")
	}
	var k pepper.Keypair
	copy(k.Priv[:], raw)
	k.PubFromPriv()
	return &k, nil
}
