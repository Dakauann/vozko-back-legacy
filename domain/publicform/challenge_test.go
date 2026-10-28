package publicform

import (
	"bytes"
	"errors"
	"io"
	"strconv"
	"strings"
	"testing"
	"time"
)

var testKey = []byte("challenge-key-for-tests")

func solve(t *testing.T, token string, bits int) string {
	t.Helper()
	for n := 0; n < 1<<24; n++ {
		nonce := strconv.Itoa(n)
		if SolvesChallenge(token, nonce, bits) {
			return nonce
		}
	}
	t.Fatal("no solution found")
	return ""
}

func wrongNonce(token string, bits int) string {
	for n := 0; ; n++ {
		nonce := strconv.Itoa(n)
		if !SolvesChallenge(token, nonce, bits) {
			return nonce
		}
	}
}

func TestChallengeSolutionIsAccepted(t *testing.T) {
	now := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	c, err := NewChallenge(seeded(), testKey, "w1", 8, now)
	if err != nil {
		t.Fatal(err)
	}
	if c.Bits != 8 || !c.ExpiresAt.Equal(now.Add(ChallengeTTL)) || c.ID == "" {
		t.Fatalf("challenge = %+v", c)
	}
	proof, err := VerifyChallenge(testKey, c.Token, solve(t, c.Token, 8), "w1", 8, now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if proof.ID != c.ID {
		t.Fatalf("proof id = %q, want %q", proof.ID, c.ID)
	}
	if !proof.IssuedAt.Equal(now) {
		t.Fatalf("proof issued at = %v, want %v", proof.IssuedAt, now)
	}
}

func TestChallengeTokenKeepsItsWireFormat(t *testing.T) {
	now := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	c, err := NewChallenge(seeded(), testKey, "w1", 16, now)
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(c.Token, ".")
	if len(parts) != 5 || parts[0] != c.ID || parts[1] != "w1" || parts[2] != "16" || parts[3] != strconv.FormatInt(now.Add(ChallengeTTL).Unix(), 10) {
		t.Fatalf("token = %q", c.Token)
	}
}

func TestNewChallengeRefusesBadRequests(t *testing.T) {
	now := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	cases := map[string]struct {
		key        []byte
		scope      string
		difficulty int
		want       error
	}{
		"no key":           {nil, "w1", 8, ErrSigningKeyMissing},
		"no scope":         {testKey, "", 8, ErrChallengeInvalid},
		"dotted scope":     {testKey, "w.1", 8, ErrChallengeInvalid},
		"zero difficulty":  {testKey, "w1", 0, ErrChallengeInvalid},
		"above the limit":  {testKey, "w1", MaxChallengeBits + 1, ErrChallengeInvalid},
		"negative bits":    {testKey, "w1", -1, ErrChallengeInvalid},
		"at the top limit": {testKey, "w1", MaxChallengeBits, nil},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := NewChallenge(seeded(), tc.key, tc.scope, tc.difficulty, now)
			if !errors.Is(err, tc.want) {
				t.Fatalf("NewChallenge = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestChallengeRefusesShortcuts(t *testing.T) {
	now := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	c, _ := NewChallenge(seeded(), testKey, "w1", 8, now)
	easy, _ := NewChallenge(seeded(), testKey, "w1", 1, now)
	foreign, _ := NewChallenge(seeded(), []byte("another-key"), "w1", 8, now)
	nonce := solve(t, c.Token, 8)
	weakened := strings.Replace(c.Token, ".8.", ".1.", 1)

	cases := map[string]struct {
		key                 []byte
		token, nonce, scope string
		at                  time.Time
		want                error
	}{
		"expired":           {testKey, c.Token, nonce, "w1", now.Add(ChallengeTTL + time.Second), ErrChallengeExpired},
		"other scope":       {testKey, c.Token, nonce, "w2", now, ErrChallengeInvalid},
		"weakened bits":     {testKey, weakened, nonce, "w1", now, ErrChallengeInvalid},
		"below the minimum": {testKey, easy.Token, solve(t, easy.Token, 1), "w1", now, ErrChallengeInvalid},
		"wrong nonce":       {testKey, c.Token, wrongNonce(c.Token, 8), "w1", now, ErrChallengeUnsolved},
		"empty nonce":       {testKey, c.Token, "", "w1", now, ErrChallengeUnsolved},
		"long nonce":        {testKey, c.Token, strings.Repeat("1", 40), "w1", now, ErrChallengeUnsolved},
		"garbage":           {testKey, "x.y", nonce, "w1", now, ErrChallengeInvalid},
		"signed elsewhere":  {testKey, foreign.Token, solve(t, foreign.Token, 8), "w1", now, ErrChallengeInvalid},
		"no key":            {nil, c.Token, nonce, "w1", now, ErrSigningKeyMissing},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := VerifyChallenge(tc.key, tc.token, tc.nonce, tc.scope, 8, tc.at)
			if !errors.Is(err, tc.want) {
				t.Fatalf("VerifyChallenge = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestVerifyChallengeNeedsAPositiveMinimum(t *testing.T) {
	now := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	c, _ := NewChallenge(seeded(), testKey, "w1", 1, now)
	nonce := solve(t, c.Token, 1)
	for _, minimum := range []int{0, -4} {
		if _, err := VerifyChallenge(testKey, c.Token, nonce, "w1", minimum, now); !errors.Is(err, ErrChallengeInvalid) {
			t.Fatalf("VerifyChallenge with minimum %d = %v, want ErrChallengeInvalid", minimum, err)
		}
	}
}

func TestLeadingZeroBits(t *testing.T) {
	cases := map[string]struct {
		in   []byte
		want int
	}{
		"none":        {[]byte{0x80}, 0},
		"one byte":    {[]byte{0x00, 0xFF}, 8},
		"partial":     {[]byte{0x00, 0x0F}, 12},
		"all zero":    {[]byte{0x00, 0x00}, 16},
		"single high": {[]byte{0x01}, 7},
	}
	for name, tc := range cases {
		if got := leadingZeroBits(tc.in); got != tc.want {
			t.Errorf("%s: leadingZeroBits = %d, want %d", name, got, tc.want)
		}
	}
}

func seeded() io.Reader {
	return bytes.NewReader(bytes.Repeat([]byte{7}, 64))
}

type brokenReader struct{}

func (brokenReader) Read([]byte) (int, error) {
	return 0, errors.New("no entropy")
}

func TestTheChallengeIDComesFromTheGivenRandomness(t *testing.T) {
	now := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	first, err := NewChallenge(seeded(), testKey, "w1", 8, now)
	if err != nil {
		t.Fatal(err)
	}
	second, _ := NewChallenge(seeded(), testKey, "w1", 8, now)
	if first.ID != second.ID || first.ID != "BwcHBwcHBwcHBwcHBwcHBw" {
		t.Fatalf("ids %q %q", first.ID, second.ID)
	}
	if _, err := NewChallenge(brokenReader{}, testKey, "w1", 8, now); err == nil {
		t.Fatal("a failing source of randomness must refuse")
	}
	if _, err := NewChallenge(nil, testKey, "w1", 8, now); !errors.Is(err, ErrRandomnessMissing) {
		t.Fatalf("no randomness = %v", err)
	}
}
