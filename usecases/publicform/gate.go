package publicform

import (
	"crypto/rand"
	"fmt"
	"io"
	"time"

	"vozko/domain/cache"
	pfdomain "vozko/domain/publicform"
)

const spentChallengeTTL = pfdomain.ChallengeTTL + time.Minute

type OneShot interface {
	SetNX(key string, value string, ttl time.Duration) (bool, error)
}

type ChallengeGate struct {
	Key       []byte
	Spent     OneShot
	KeyPrefix string
	Bits      int
	Random    io.Reader
}

func (g ChallengeGate) random() io.Reader {
	if g.Random == nil {
		return rand.Reader
	}
	return g.Random
}

func (g ChallengeGate) Issue(scope string, now time.Time) (pfdomain.Challenge, error) {
	return pfdomain.NewChallenge(g.random(), g.Key, scope, g.Bits, now)
}

func (g ChallengeGate) Admit(token, nonce, scope string, now time.Time) (pfdomain.Proof, error) {
	proof, err := pfdomain.VerifyChallenge(g.Key, token, nonce, scope, g.Bits, now)
	if err != nil {
		return pfdomain.Proof{}, err
	}
	if g.Spent == nil {
		return pfdomain.Proof{}, pfdomain.ErrChallengeUnavailable
	}
	fresh, err := g.Spent.SetNX(g.KeyPrefix+proof.ID, "1", spentChallengeTTL)
	if err != nil {
		return pfdomain.Proof{}, fmt.Errorf("%w: %w", pfdomain.ErrChallengeUnavailable, err)
	}
	if !fresh {
		return pfdomain.Proof{}, pfdomain.ErrChallengeReused
	}
	return proof, nil
}

func Allow(limiter cache.RateLimiter, key string) error {
	if limiter == nil {
		return pfdomain.ErrRateLimited
	}
	allowed, _, err := limiter.Allow(key)
	if err != nil || !allowed {
		return pfdomain.ErrRateLimited
	}
	return nil
}
