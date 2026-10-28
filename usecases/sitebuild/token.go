package sitebuild

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base32"
	"encoding/binary"
)

const (
	grantIDBytes  = 10
	expiryBytes   = 5
	macBytes      = 16
	tokenBytes    = grantIDBytes + expiryBytes + macBytes
	tokenLength   = (tokenBytes*8 + 4) / 5
	tokenAlphabet = "abcdefghijklmnopqrstuvwxyz234567"
	macDomain     = "site-preview-grant-v1\x00"
)

var tokenEncoding = base32.NewEncoding(tokenAlphabet).WithPadding(base32.NoPadding)

type grant struct {
	Workspace string `json:"w"`
	Project   string `json:"p"`
	Build     string `json:"b"`
	Expires   int64  `json:"e"`
}

type token struct {
	id      []byte
	expires int64
	mac     []byte
}

func tokenMAC(key []byte, id []byte, expires int64, g grant) []byte {
	m := hmac.New(sha256.New, key)
	m.Write([]byte(macDomain))
	m.Write(id)
	var at [8]byte
	binary.BigEndian.PutUint64(at[:], uint64(expires))
	m.Write(at[:])
	for _, part := range []string{g.Workspace, g.Project, g.Build} {
		m.Write([]byte(part))
		m.Write([]byte{0})
	}
	return m.Sum(nil)[:macBytes]
}

func encodeToken(key []byte, id []byte, g grant) string {
	raw := make([]byte, 0, tokenBytes)
	raw = append(raw, id...)
	var at [8]byte
	binary.BigEndian.PutUint64(at[:], uint64(g.Expires))
	raw = append(raw, at[8-expiryBytes:]...)
	raw = append(raw, tokenMAC(key, id, g.Expires, g)...)
	return tokenEncoding.EncodeToString(raw)
}

func decodeToken(label string) (token, bool) {
	if len(label) != tokenLength {
		return token{}, false
	}
	raw, err := tokenEncoding.DecodeString(label)
	if err != nil || len(raw) != tokenBytes || tokenEncoding.EncodeToString(raw) != label {
		return token{}, false
	}
	var at [8]byte
	copy(at[8-expiryBytes:], raw[grantIDBytes:grantIDBytes+expiryBytes])
	return token{id: raw[:grantIDBytes], expires: int64(binary.BigEndian.Uint64(at[:])), mac: raw[grantIDBytes+expiryBytes:]}, true
}

func sameHost(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
