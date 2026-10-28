package publicform

import "time"

const DefaultMinimumFillTime = 3 * time.Second

func CheckHoneypot(value string) error {
	if value != "" {
		return ErrHoneypotFilled
	}
	return nil
}

func CheckFillTime(startedAt, submittedAt time.Time, minimum time.Duration) error {
	if startedAt.IsZero() || submittedAt.Before(startedAt) {
		return ErrFilledTooFast
	}
	if submittedAt.Sub(startedAt) < max(minimum, DefaultMinimumFillTime) {
		return ErrFilledTooFast
	}
	return nil
}
