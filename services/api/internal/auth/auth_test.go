package auth

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"strings"
	"testing"
	"time"
)

func sign(t *testing.T, priv ed25519.PrivateKey, nonce []byte) string {
	t.Helper()
	return base64.StdEncoding.EncodeToString(ed25519.Sign(priv, Hash(Message, Recipient, nonce)))
}

func TestVerify(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(nil)
	nonce := bytes.Repeat([]byte{7}, 32)
	pk := EncodeKey(pub)
	if err := Verify(pk, sign(t, priv, nonce), nonce); err != nil {
		t.Fatal(err)
	}
	other := bytes.Repeat([]byte{8}, 32)
	_, priv2, _ := ed25519.GenerateKey(nil)
	for name, c := range map[string][3]any{
		"other nonce": {pk, sign(t, priv, nonce), other},
		"other key":   {pk, sign(t, priv2, nonce), nonce},
		"short nonce": {pk, sign(t, priv, nonce), nonce[:31]},
		"bad key":     {"secp256k1:abc", sign(t, priv, nonce), nonce},
		"bad sig b64": {pk, "!!", nonce},
	} {
		if err := Verify(c[0].(string), c[1].(string), c[2].([]byte)); err != ErrSignature {
			t.Errorf("%s: got %v", name, err)
		}
	}
}

// Borsh layout per NEP-413: tag 2^31+413 LE, len-prefixed strings, raw 32-byte nonce, None byte.
func TestHashLayout(t *testing.T) {
	nonce := make([]byte, 32)
	want := "9d010080" + "0c000000" + hex.EncodeToString([]byte(Message)) + strings.Repeat("00", 32) +
		"06000000" + hex.EncodeToString([]byte(Recipient)) + "00"
	raw, _ := hex.DecodeString(want)
	got := Hash(Message, Recipient, nonce)
	if h := sha(raw); !bytes.Equal(got, h) {
		t.Fatalf("hash mismatch")
	}
}

func TestKeyRoundTrip(t *testing.T) {
	pub := make(ed25519.PublicKey, 32)
	pub[5] = 1 // leading zero bytes map to '1's
	got, err := ParseKey(EncodeKey(pub))
	if err != nil || !bytes.Equal(got, pub) {
		t.Fatal(err, got)
	}
	// well-known: 32 zero bytes
	if EncodeKey(make([]byte, 32)) != "ed25519:"+strings.Repeat("1", 32) {
		t.Fatal("zero key")
	}
}

func TestSessions(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	s := &Sessions{Secret: []byte("k"), TTL: time.Hour, Now: func() time.Time { return now }}
	tok, err := s.Issue("alice.testnet")
	if err != nil {
		t.Fatal(err)
	}
	if a, err := s.Parse(tok); err != nil || a != "alice.testnet" {
		t.Fatal(a, err)
	}
	if _, err := (&Sessions{Secret: []byte("other"), TTL: time.Hour}).Parse(tok); err != ErrToken {
		t.Fatal("wrong secret accepted")
	}
	if _, err := s.Parse(tok[:len(tok)-2] + "xx"); err != ErrToken {
		t.Fatal("tampered accepted")
	}
	now = now.Add(time.Hour)
	if _, err := s.Parse(tok); err != ErrToken {
		t.Fatal("expired accepted")
	}
}

func sha(b []byte) []byte { h := sha256.Sum256(b); return h[:] }
