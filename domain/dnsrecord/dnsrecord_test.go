package dnsrecord

import (
	"context"
	"errors"
	"testing"
)

func TestObserveReadsTheAnswerForTheName(t *testing.T) {
	answers := map[string]Answer{
		"_vz.www.loja.com.br": {Values: []string{"other", "token-1"}},
		"broken.loja.com.br":  {Failed: true},
		"empty.loja.com.br":   {},
	}
	matches := func(v string) bool { return v == "token-1" }
	cases := map[string]Status{
		"_vz.www.loja.com.br.": Verified,
		"_VZ.WWW.LOJA.COM.BR":  Verified,
		"broken.loja.com.br":   Unknown,
		"missing.loja.com.br":  Unknown,
		"empty.loja.com.br":    Pending,
	}
	for name, want := range cases {
		if got := Observe(answers, name, matches); got != want {
			t.Errorf("%s = %s, want %s", name, got, want)
		}
	}
}

func TestStatusesAreKnown(t *testing.T) {
	for _, s := range []Status{Verified, Pending, Unknown} {
		if !s.Valid() {
			t.Errorf("%s must be valid", s)
		}
	}
	if Status("nope").Valid() {
		t.Error("an unknown status is invalid")
	}
}

func TestRelativeHost(t *testing.T) {
	cases := map[[2]string]string{
		{"loja.com.br", "loja.com.br"}:         "@",
		{"_vz.www.loja.com.br", "loja.com.br"}: "_vz.www",
		{"outra.com", "loja.com.br"}:           "outra.com",
	}
	for in, want := range cases {
		if got := RelativeHost(in[0], in[1]); got != want {
			t.Errorf("RelativeHost(%q, %q) = %q, want %q", in[0], in[1], got, want)
		}
	}
}

func TestLookupMarksFailuresAndAsksEachNameOnce(t *testing.T) {
	asked := map[string]int{}
	var failedNames []string
	answers := Lookup(context.Background(), func(_ context.Context, name string) ([]string, error) {
		asked[name]++
		if name == "down.loja.com.br" {
			return nil, errors.New("timeout")
		}
		return []string{"v"}, nil
	}, []string{"ok.loja.com.br", "OK.loja.com.br.", "down.loja.com.br"}, func(name string, _ error) { failedNames = append(failedNames, name) })
	if asked["ok.loja.com.br"] != 1 || !answers["down.loja.com.br"].Failed || len(answers["ok.loja.com.br"].Values) != 1 {
		t.Fatalf("asked %v answers %v", asked, answers)
	}
	if len(failedNames) != 1 || failedNames[0] != "down.loja.com.br" {
		t.Fatalf("failures reported: %v", failedNames)
	}
}
