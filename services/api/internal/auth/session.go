package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

var ErrToken = errors.New("unauthorized")

var b64 = base64.RawURLEncoding

const jwtHeader = `{"alg":"HS256","typ":"JWT"}`

// Sessions signs and checks HS256 JWTs whose sub is the NEAR account id.
type Sessions struct {
	Secret []byte
	TTL    time.Duration
	Now    func() time.Time // nil = time.Now
}

type claims struct {
	Sub string `json:"sub"`
	Iat int64  `json:"iat"`
	Exp int64  `json:"exp"`
}

func (s *Sessions) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func (s *Sessions) Issue(account string) (string, error) {
	t := s.now()
	c, err := json.Marshal(claims{account, t.Unix(), t.Add(s.TTL).Unix()})
	if err != nil {
		return "", err
	}
	msg := b64.EncodeToString([]byte(jwtHeader)) + "." + b64.EncodeToString(c)
	return msg + "." + b64.EncodeToString(s.mac(msg)), nil
}

// Parse returns the account id of a valid, unexpired token.
func (s *Sessions) Parse(tok string) (string, error) {
	i := strings.LastIndexByte(tok, '.')
	if i < 0 || len(s.Secret) == 0 {
		return "", ErrToken
	}
	msg, sig := tok[:i], tok[i+1:]
	got, err := b64.DecodeString(sig)
	if err != nil || !hmac.Equal(got, s.mac(msg)) {
		return "", ErrToken
	}
	h, body, ok := strings.Cut(msg, ".")
	hb, err1 := b64.DecodeString(h)
	cb, err2 := b64.DecodeString(body)
	var c claims
	if !ok || err1 != nil || err2 != nil || string(hb) != jwtHeader || json.Unmarshal(cb, &c) != nil {
		return "", ErrToken
	}
	if c.Sub == "" || s.now().Unix() >= c.Exp {
		return "", ErrToken
	}
	return c.Sub, nil
}

func (s *Sessions) mac(msg string) []byte {
	m := hmac.New(sha256.New, s.Secret)
	m.Write([]byte(msg))
	return m.Sum(nil)
}
