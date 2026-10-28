package publicform

import (
	"bytes"
	"errors"
	"strconv"
	"testing"
	"time"

	pfdomain "vozko/domain/publicform"
)

type spentKeys struct {
	keys map[string]time.Duration
	err  error
}

func (s *spentKeys) SetNX(key, _ string, ttl time.Duration) (bool, error) {
	if s.err != nil {
		return false, s.err
	}
	if _, taken := s.keys[key]; taken {
		return false, nil
	}
	s.keys[key] = ttl
	return true, nil
}

type fixedLimiter struct {
	allowed bool
	err     error
}

func (f fixedLimiter) Allow(string) (bool, time.Duration, error) {
	return f.allowed, 0, f.err
}

var gateNow = time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)

func solved(t *testing.T, c pfdomain.Challenge) string {
	t.Helper()
	for n := 0; ; n++ {
		if nonce := strconv.Itoa(n); pfdomain.SolvesChallenge(c.Token, nonce, c.Bits) {
			return nonce
		}
	}
}

func newGate(spent OneShot) ChallengeGate {
	return ChallengeGate{Key: []byte("gate-key"), Spent: spent, KeyPrefix: "test:challenge:", Bits: 8}
}

func TestAChallengeIsAdmittedOnceAndRemembered(t *testing.T) {
	spent := &spentKeys{keys: map[string]time.Duration{}}
	gate := newGate(spent)
	c, err := gate.Issue("scope-1", gateNow)
	if err != nil {
		t.Fatal(err)
	}
	if c.Bits != 8 {
		t.Fatalf("bits = %d, want the gate difficulty", c.Bits)
	}
	nonce := solved(t, c)
	proof, err := gate.Admit(c.Token, nonce, "scope-1", gateNow.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if proof.ID != c.ID || !proof.IssuedAt.Equal(gateNow) {
		t.Fatalf("proof = %+v", proof)
	}
	if ttl, ok := spent.keys["test:challenge:"+c.ID]; !ok || ttl != pfdomain.ChallengeTTL+time.Minute {
		t.Fatalf("spent keys = %v", spent.keys)
	}
	if _, err := gate.Admit(c.Token, nonce, "scope-1", gateNow.Add(2*time.Second)); !errors.Is(err, pfdomain.ErrChallengeReused) {
		t.Fatalf("replay = %v, want ErrChallengeReused", err)
	}
}

func TestAdmitRefusesWhenTheChallengeCannotBeSpent(t *testing.T) {
	issuer := newGate(&spentKeys{keys: map[string]time.Duration{}})
	c, err := issuer.Issue("scope-1", gateNow)
	if err != nil {
		t.Fatal(err)
	}
	nonce := solved(t, c)
	fresh := func() OneShot { return &spentKeys{keys: map[string]time.Duration{}} }
	inTime := gateNow.Add(time.Second)
	cases := map[string]struct {
		spent OneShot
		token string
		nonce string
		scope string
		at    time.Time
		want  error
	}{
		"store failure":   {&spentKeys{err: errors.New("redis down")}, c.Token, nonce, "scope-1", inTime, pfdomain.ErrChallengeUnavailable},
		"no store":        {nil, c.Token, nonce, "scope-1", inTime, pfdomain.ErrChallengeUnavailable},
		"invalid token":   {fresh(), "x.y", nonce, "scope-1", inTime, pfdomain.ErrChallengeInvalid},
		"other scope":     {fresh(), c.Token, nonce, "scope-2", inTime, pfdomain.ErrChallengeInvalid},
		"missing token":   {fresh(), "", nonce, "scope-1", inTime, pfdomain.ErrChallengeInvalid},
		"unsolved nonce":  {fresh(), c.Token, "", "scope-1", inTime, pfdomain.ErrChallengeUnsolved},
		"stale challenge": {fresh(), c.Token, nonce, "scope-1", gateNow.Add(pfdomain.ChallengeTTL + time.Second), pfdomain.ErrChallengeExpired},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := newGate(tc.spent).Admit(tc.token, tc.nonce, tc.scope, tc.at); !errors.Is(err, tc.want) {
				t.Fatalf("Admit = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestAnInvalidStoreAnswerNeverSpendsTheChallenge(t *testing.T) {
	spent := &spentKeys{err: errors.New("redis down")}
	gate := newGate(spent)
	c, _ := gate.Issue("scope-1", gateNow)
	if _, err := gate.Admit(c.Token, solved(t, c), "scope-1", gateNow); !errors.Is(err, pfdomain.ErrChallengeUnavailable) {
		t.Fatalf("Admit = %v", err)
	}
	if len(spent.keys) != 0 {
		t.Fatal("nothing is remembered when the store fails")
	}
}

func TestIssueRefusesWithoutAKey(t *testing.T) {
	gate := ChallengeGate{Bits: 8}
	if _, err := gate.Issue("scope-1", gateNow); !errors.Is(err, pfdomain.ErrSigningKeyMissing) {
		t.Fatalf("Issue = %v", err)
	}
}

func TestLimiterErrorsRefuseInsteadOfAllowing(t *testing.T) {
	cases := map[string]struct {
		limiter fixedLimiter
		absent  bool
		want    error
	}{
		"allowed":         {limiter: fixedLimiter{allowed: true}, want: nil},
		"over the limit":  {limiter: fixedLimiter{allowed: false}, want: pfdomain.ErrRateLimited},
		"limiter failure": {limiter: fixedLimiter{allowed: true, err: errors.New("redis down")}, want: pfdomain.ErrRateLimited},
		"no limiter":      {absent: true, want: pfdomain.ErrRateLimited},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			var err error
			if tc.absent {
				err = Allow(nil, "k")
			} else {
				err = Allow(tc.limiter, "k")
			}
			if !errors.Is(err, tc.want) {
				t.Fatalf("Allow = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestAStoreFailureIsNotReportedAsAReusedChallenge(t *testing.T) {
	c, _ := newGate(nil).Issue("scope-1", gateNow)
	_, err := newGate(&spentKeys{err: errors.New("redis down")}).Admit(c.Token, solved(t, c), "scope-1", gateNow)
	if !errors.Is(err, pfdomain.ErrChallengeUnavailable) || errors.Is(err, pfdomain.ErrChallengeReused) {
		t.Fatalf("Admit = %v", err)
	}
}

func TestIssueDrawsTheChallengeIDFromTheGateRandomness(t *testing.T) {
	gate := newGate(nil)
	gate.Random = bytes.NewReader(bytes.Repeat([]byte{7}, 16))
	c, err := gate.Issue("scope-1", gateNow)
	if err != nil || c.ID != "BwcHBwcHBwcHBwcHBwcHBw" {
		t.Fatalf("challenge %+v err %v", c, err)
	}
	other, err := newGate(nil).Issue("scope-1", gateNow)
	if err != nil || other.ID == "" || other.ID == c.ID {
		t.Fatalf("without a source the gate uses the system randomness: %+v %v", other, err)
	}
}
