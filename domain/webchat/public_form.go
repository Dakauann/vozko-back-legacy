package webchat

import (
	"errors"

	"vozko/domain/publicform"
)

var publicFormRefusals = []struct {
	from error
	to   error
}{
	{publicform.ErrOriginInvalid, ErrOriginInvalid},
	{publicform.ErrOriginInsecure, ErrOriginInsecure},
	{publicform.ErrChallengeInvalid, ErrChallengeInvalid},
	{publicform.ErrChallengeExpired, ErrChallengeExpired},
	{publicform.ErrChallengeUnsolved, ErrChallengeUnsolved},
	{publicform.ErrChallengeReused, ErrChallengeReused},
	{publicform.ErrChallengeUnavailable, ErrChallengeUnavailable},
	{publicform.ErrSigningKeyMissing, ErrSigningKeyMissing},
	{publicform.ErrRateLimited, ErrRateLimited},
}

func PublicFormRefusal(err error) error {
	for _, r := range publicFormRefusals {
		if errors.Is(err, r.from) {
			return r.to
		}
	}
	return err
}
