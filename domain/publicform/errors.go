package publicform

import "errors"

var (
	ErrOriginInvalid        = errors.New("publicform: the site must be an origin such as https://www.example.com")
	ErrOriginInsecure       = errors.New("publicform: the site must use https (http is accepted only for localhost)")
	ErrChallengeInvalid     = errors.New("publicform: challenge is not valid")
	ErrChallengeExpired     = errors.New("publicform: challenge has expired")
	ErrChallengeUnsolved    = errors.New("publicform: challenge solution is wrong")
	ErrChallengeReused      = errors.New("publicform: challenge was already used")
	ErrChallengeUnavailable = errors.New("publicform: the store of spent challenges cannot be reached")
	ErrRandomnessMissing    = errors.New("publicform: no source of randomness was given")
	ErrSigningKeyMissing    = errors.New("publicform: signing key is not configured")
	ErrRateLimited          = errors.New("publicform: too many requests, try again shortly")
	ErrHoneypotFilled       = errors.New("publicform: a field people never see was filled")
	ErrFilledTooFast        = errors.New("publicform: the form was sent faster than a person can fill it")
)
