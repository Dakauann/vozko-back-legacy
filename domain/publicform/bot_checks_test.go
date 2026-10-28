package publicform

import (
	"errors"
	"testing"
	"time"
)

func TestHoneypotMustStayEmpty(t *testing.T) {
	cases := map[string]struct {
		value string
		want  error
	}{
		"empty passes":       {"", nil},
		"text is a bot":      {"https://spam.example", ErrHoneypotFilled},
		"spaces are a bot":   {"   ", ErrHoneypotFilled},
		"a newline is a bot": {"\n", ErrHoneypotFilled},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if err := CheckHoneypot(tc.value); !errors.Is(err, tc.want) {
				t.Fatalf("CheckHoneypot(%q) = %v, want %v", tc.value, err, tc.want)
			}
		})
	}
}

func TestFillTimeRefusesFormsFilledFasterThanAPerson(t *testing.T) {
	started := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
	cases := map[string]struct {
		started   time.Time
		submitted time.Time
		minimum   time.Duration
		want      error
	}{
		"slow enough":                 {started, started.Add(5 * time.Second), 4 * time.Second, nil},
		"exactly the minimum":         {started, started.Add(4 * time.Second), 4 * time.Second, nil},
		"too fast":                    {started, started.Add(3 * time.Second), 4 * time.Second, ErrFilledTooFast},
		"instant":                     {started, started, 4 * time.Second, ErrFilledTooFast},
		"submitted before starting":   {started, started.Add(-time.Minute), 4 * time.Second, ErrFilledTooFast},
		"unknown start":               {time.Time{}, started, 4 * time.Second, ErrFilledTooFast},
		"zero minimum uses default":   {started, started.Add(DefaultMinimumFillTime - time.Millisecond), 0, ErrFilledTooFast},
		"negative minimum uses dflt":  {started, started.Add(DefaultMinimumFillTime - time.Millisecond), -time.Hour, ErrFilledTooFast},
		"default minimum is reached":  {started, started.Add(DefaultMinimumFillTime), 0, nil},
		"small minimum keeps default": {started, started.Add(time.Second), time.Millisecond, ErrFilledTooFast},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if err := CheckFillTime(tc.started, tc.submitted, tc.minimum); !errors.Is(err, tc.want) {
				t.Fatalf("CheckFillTime = %v, want %v", err, tc.want)
			}
		})
	}
}
