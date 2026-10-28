package publicform

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"io"
	"math/bits"
	"strconv"
	"strings"
	"time"
)

const (
	ChallengeTTL         = 2 * time.Minute
	MaxChallengeBits     = 32
	MaxChallengeNonceLen = 20
	challengeIDBytes     = 16
)

type Challenge struct {
	ID        string    `json:"id"`
	Token     string    `json:"token"`
	Bits      int       `json:"bits"`
	ExpiresAt time.Time `json:"expiresAt"`
}

type Proof struct {
	ID       string
	IssuedAt time.Time
}

func NewChallenge(random io.Reader, key []byte, scope string, difficulty int, now time.Time) (Challenge, error) {
	if len(key) == 0 {
		return Challenge{}, ErrSigningKeyMissing
	}
	if scope == "" || strings.Contains(scope, ".") || difficulty < 1 || difficulty > MaxChallengeBits {
		return Challenge{}, ErrChallengeInvalid
	}
	id, err := randomID(random)
	if err != nil {
		return Challenge{}, err
	}
	expires := now.UTC().Add(ChallengeTTL).Truncate(time.Second)
	head := strings.Join([]string{id, scope, strconv.Itoa(difficulty), strconv.FormatInt(expires.Unix(), 10)}, ".")
	return Challenge{
		ID:        id,
		Token:     head + "." + sign(key, head),
		Bits:      difficulty,
		ExpiresAt: expires,
	}, nil
}

func VerifyChallenge(key []byte, token, nonce, scope string, minimumBits int, now time.Time) (Proof, error) {
	if len(key) == 0 {
		return Proof{}, ErrSigningKeyMissing
	}
	if minimumBits < 1 {
		return Proof{}, ErrChallengeInvalid
	}
	parts := strings.Split(strings.TrimSpace(token), ".")
	if len(parts) != 5 {
		return Proof{}, ErrChallengeInvalid
	}
	head := strings.Join(parts[:4], ".")
	if !hmac.Equal([]byte(parts[4]), []byte(sign(key, head))) {
		return Proof{}, ErrChallengeInvalid
	}
	if parts[1] != scope {
		return Proof{}, ErrChallengeInvalid
	}
	difficulty, err := strconv.Atoi(parts[2])
	if err != nil || difficulty < minimumBits {
		return Proof{}, ErrChallengeInvalid
	}
	expiresUnix, err := strconv.ParseInt(parts[3], 10, 64)
	if err != nil {
		return Proof{}, ErrChallengeInvalid
	}
	expires := time.Unix(expiresUnix, 0).UTC()
	if now.After(expires) {
		return Proof{}, ErrChallengeExpired
	}
	if nonce == "" || len(nonce) > MaxChallengeNonceLen {
		return Proof{}, ErrChallengeUnsolved
	}
	if !SolvesChallenge(token, nonce, difficulty) {
		return Proof{}, ErrChallengeUnsolved
	}
	return Proof{ID: parts[0], IssuedAt: expires.Add(-ChallengeTTL)}, nil
}

func SolvesChallenge(token, nonce string, difficulty int) bool {
	sum := sha256.Sum256([]byte(token + ":" + nonce))
	return leadingZeroBits(sum[:]) >= difficulty
}

func leadingZeroBits(digest []byte) int {
	total := 0
	for _, b := range digest {
		if b == 0 {
			total += 8
			continue
		}
		return total + bits.LeadingZeros8(b)
	}
	return total
}

func sign(key []byte, data string) string {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(data))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func randomID(random io.Reader) (string, error) {
	if random == nil {
		return "", ErrRandomnessMissing
	}
	raw := make([]byte, challengeIDBytes)
	if _, err := io.ReadFull(random, raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}
