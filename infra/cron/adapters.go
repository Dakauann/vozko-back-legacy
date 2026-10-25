package cron

import (
	"context"
	"log"

	"vozko/domain/shared/deps"
)

type Executable interface {
	Execute(ctx context.Context) error
}

func Executes(job Executable) Run {
	if deps.IsNil(job) {
		return nil
	}
	return job.Execute
}

func Method(receiver any, run Run) Run {
	if deps.IsNil(receiver) || run == nil {
		return nil
	}
	return run
}

func Plain(fn func() error) Run {
	if fn == nil {
		return nil
	}
	return func(context.Context) error { return fn() }
}

func Counted[N int | int64](format string, fn func() (N, error)) Run {
	if fn == nil {
		return nil
	}
	return func(context.Context) error {
		n, err := fn()
		if err != nil {
			return err
		}
		if n > 0 {
			log.Printf("[cron] "+format, n)
		}
		return nil
	}
}
