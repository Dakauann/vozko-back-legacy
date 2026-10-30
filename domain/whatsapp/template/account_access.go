package template

import "errors"

type accountAccessLoss interface {
	AccountAccessLost() bool
}

func AccountAccessLost(err error) bool {
	var loss accountAccessLoss
	return errors.As(err, &loss) && loss.AccountAccessLost()
}
