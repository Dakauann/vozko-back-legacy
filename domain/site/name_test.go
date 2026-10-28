package site

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestNameFromBriefTakesTheOpeningWords(t *testing.T) {
	cases := map[string]string{
		"crie uma landing page comercial para minha clínica":    "Crie uma landing page comercial para minha clínica",
		"  Página de inscrição para o evento.\nCom programação": "Página de inscrição para o evento",
		"\n\nLançamento do produto X!!!":                        "Lançamento do produto X",
		"":                                                      "",
		"   \n  ":                                               "",
		"Uma página\tpara   a   Clínica    Sorriso":             "Uma página para a Clínica Sorriso",
		"https://clinicasorriso.com.br":                         "Https://clinicasorriso.com.br",
		strings.Repeat("palavra ", 20):                          "Palavra" + strings.Repeat(" palavra", 6),
	}
	for brief, want := range cases {
		if got := NameFromBrief(brief); got != want {
			t.Errorf("%q: got %q want %q", brief, got, want)
		}
	}
}

func TestNameFromBriefNeverPassesTheLimit(t *testing.T) {
	got := NameFromBrief(strings.Repeat("a", 500))
	if utf8.RuneCountInString(got) != MaxBriefNameRunes {
		t.Fatalf("got %d runes", utf8.RuneCountInString(got))
	}
}
