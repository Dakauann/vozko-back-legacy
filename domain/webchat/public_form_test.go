package webchat

import (
	"errors"
	"fmt"
	"testing"

	"vozko/domain/publicform"
)

func TestPublicFormRefusalsKeepTheWebchatErrors(t *testing.T) {
	other := errors.New("disk full")
	cases := []struct {
		name string
		in   error
		want error
	}{
		{"origin invalid", publicform.ErrOriginInvalid, ErrOriginInvalid},
		{"origin insecure", publicform.ErrOriginInsecure, ErrOriginInsecure},
		{"challenge invalid", publicform.ErrChallengeInvalid, ErrChallengeInvalid},
		{"challenge expired", publicform.ErrChallengeExpired, ErrChallengeExpired},
		{"challenge unsolved", publicform.ErrChallengeUnsolved, ErrChallengeUnsolved},
		{"challenge reused", publicform.ErrChallengeReused, ErrChallengeReused},
		{"challenge store unavailable", publicform.ErrChallengeUnavailable, ErrChallengeUnavailable},
		{"signing key missing", publicform.ErrSigningKeyMissing, ErrSigningKeyMissing},
		{"rate limited", publicform.ErrRateLimited, ErrRateLimited},
		{"wrapped refusal", fmt.Errorf("spend: %w", publicform.ErrChallengeReused), ErrChallengeReused},
		{"unrelated error passes through", other, other},
		{"no error", nil, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := PublicFormRefusal(tc.in)
			if got != tc.want {
				t.Fatalf("PublicFormRefusal(%v) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestWidgetOriginsUseTheWebchatErrors(t *testing.T) {
	cases := map[string]struct {
		raw  string
		want error
	}{
		"a valid site":    {"https://loja.example.com", nil},
		"a wildcard site": {"https://*.example.com", nil},
		"not an origin":   {"loja.example.com", ErrOriginInvalid},
		"plain http":      {"http://loja.example.com", ErrOriginInsecure},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if err := validateOrigins([]string{tc.raw}); err != tc.want {
				t.Fatalf("validateOrigins(%q) = %v, want %v", tc.raw, err, tc.want)
			}
		})
	}
}
