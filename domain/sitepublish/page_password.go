package sitepublish

import (
	"crypto/hmac"
	"crypto/pbkdf2"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"strconv"
	"strings"
	"time"
)

const (
	MaxPagePasswordLength = 256
	PassTokenLifetime     = 7 * 24 * time.Hour
	maxNextLength         = 512
)

func VerifyPagePassword(password, stored string) bool {
	if password == "" || len(password) > MaxPagePasswordLength || !validPasswordHash(stored) {
		return false
	}
	parts := strings.Split(stored, "$")
	iterations, err := strconv.Atoi(parts[1])
	if err != nil || iterations != minHashIterations {
		return false
	}
	salt, okSalt := decodeHashPart(parts[2])
	expected, okHash := decodeHashPart(parts[3])
	if !okSalt || !okHash {
		return false
	}
	derived, err := pbkdf2.Key(sha256.New, password, salt, iterations, len(expected))
	return err == nil && hmac.Equal(derived, expected)
}

func decodeHashPart(text string) ([]byte, bool) {
	normal := strings.TrimRight(strings.NewReplacer("-", "+", "_", "/").Replace(text), "=")
	out, err := base64.RawStdEncoding.DecodeString(normal)
	return out, err == nil && len(out) > 0
}

func PassToken(secret []byte, host string, s ServingSettings, now time.Time) string {
	expires := strconv.FormatInt(now.Add(PassTokenLifetime).Unix(), 10)
	return expires + "." + passSignature(secret, host, s, expires)
}

func ValidPassToken(secret []byte, host string, s ServingSettings, token string, now time.Time) bool {
	expires, signature, ok := strings.Cut(token, ".")
	if !ok {
		return false
	}
	at, err := strconv.ParseInt(expires, 10, 64)
	if err != nil || at < now.Unix() {
		return false
	}
	return hmac.Equal([]byte(signature), []byte(passSignature(secret, host, s, expires)))
}

func passSignature(secret []byte, host string, s ServingSettings, expires string) string {
	return hex.EncodeToString(sign(secret, "pass|"+host+"|"+s.DeploymentID+"|"+s.PasswordHash+"|"+expires))
}

func SafeNext(raw string) string {
	if !strings.HasPrefix(raw, "/") || strings.HasPrefix(raw, "//") || strings.Contains(raw, "\\") {
		return "/"
	}
	return raw[:min(len(raw), maxNextLength)]
}
