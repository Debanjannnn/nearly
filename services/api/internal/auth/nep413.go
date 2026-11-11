// Package auth verifies NEP-413 login signatures and issues HS256 session tokens.
package auth

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"math/big"
	"strings"
)

// Login message every client signs (shared contract with the client).
const (
	Message   = "nearly login"
	Recipient = "nearly"
	nep413Tag = 1<<31 + 413
)

var ErrSignature = errors.New("invalid_signature")

// Verify checks sig (base64) over the NEP-413 payload {Message, nonce, Recipient, no callbackUrl}.
func Verify(publicKey, signature string, nonce []byte) error {
	pk, err := ParseKey(publicKey)
	if err != nil || len(nonce) != 32 {
		return ErrSignature
	}
	sig, err := base64.StdEncoding.DecodeString(signature)
	if err != nil || !ed25519.Verify(pk, Hash(Message, Recipient, nonce), sig) {
		return ErrSignature
	}
	return nil
}

// NewNonce is a random 32-byte login challenge.
func NewNonce() []byte { b := make([]byte, 32); rand.Read(b); return b }

// Hash is sha256(borsh(u32 tag) || borsh(payload)).
func Hash(message, recipient string, nonce []byte) []byte {
	b := binary.LittleEndian.AppendUint32(nil, nep413Tag)
	b = borshString(b, message)
	b = append(b, nonce...)
	b = borshString(b, recipient)
	b = append(b, 0) // callbackUrl: None
	h := sha256.Sum256(b)
	return h[:]
}

func borshString(b []byte, s string) []byte {
	return append(binary.LittleEndian.AppendUint32(b, uint32(len(s))), s...)
}

// ParseKey decodes "ed25519:<base58>".
func ParseKey(s string) (ed25519.PublicKey, error) {
	raw, ok := strings.CutPrefix(s, "ed25519:")
	if !ok {
		return nil, ErrSignature
	}
	b, err := base58(raw)
	if err != nil || len(b) != ed25519.PublicKeySize {
		return nil, ErrSignature
	}
	return b, nil
}

const alphabet = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"

func base58(s string) ([]byte, error) {
	n := new(big.Int)
	for _, c := range s {
		i := strings.IndexRune(alphabet, c)
		if i < 0 {
			return nil, ErrSignature
		}
		n.Mul(n, big.NewInt(58)).Add(n, big.NewInt(int64(i)))
	}
	lead := len(s) - len(strings.TrimLeft(s, "1"))
	return append(make([]byte, lead), n.Bytes()...), nil
}

// EncodeKey is the inverse of ParseKey; tests and fixtures use it.
func EncodeKey(pk ed25519.PublicKey) string {
	n := new(big.Int).SetBytes(pk)
	var out []byte
	for m := new(big.Int); n.Sign() > 0; {
		n.DivMod(n, big.NewInt(58), m)
		out = append(out, alphabet[m.Int64()])
	}
	for _, b := range pk {
		if b != 0 {
			break
		}
		out = append(out, '1')
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return "ed25519:" + string(out)
}
