package site

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

const MaxBriefNameRunes = 60

func NameFromBrief(brief string) string {
	line := ""
	for _, candidate := range strings.Split(brief, "\n") {
		if line = strings.Join(strings.Fields(candidate), " "); line != "" {
			break
		}
	}
	if sentence, _, found := strings.Cut(line, ". "); found {
		line = sentence
	}
	line = strings.TrimRightFunc(line, func(r rune) bool { return unicode.IsPunct(r) && r != ')' })
	if utf8.RuneCountInString(line) > MaxBriefNameRunes {
		runes := []rune(line)[:MaxBriefNameRunes]
		cut := string(runes)
		if space := strings.LastIndex(cut, " "); space > 0 && runes[MaxBriefNameRunes-1] != ' ' {
			cut = cut[:space]
		}
		line = strings.TrimSpace(cut)
	}
	first, size := utf8.DecodeRuneInString(line)
	if size == 0 {
		return ""
	}
	return string(unicode.ToUpper(first)) + line[size:]
}
