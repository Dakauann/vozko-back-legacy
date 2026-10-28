package copilottools

import (
	"encoding/json"
	"strings"
	"unicode"
)

const (
	closingNoteOpening = "Antes de encerrar: "
	closingNoteLeftOut = ", e a sua resposta não fala delas:\n"
	closingNoteRewrite = "Escreva de novo a resposta final, completa: diga ao usuário o que falta e peça esses dados numa só pergunta, ou corrija o que depende só de você. "
)

func JSONEndingWith(body map[string]interface{}, key string, value interface{}) ([]byte, error) {
	tail, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	name, err := json.Marshal(key)
	if err != nil {
		return nil, err
	}
	field := append(append(name, ':'), tail...)
	if len(body) == 0 {
		return append(append([]byte("{"), field...), '}'), nil
	}
	head, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	return append(append(append(head[:len(head)-1], ','), field...), '}'), nil
}

func ReplyMentions(reply string, keys []string) bool {
	lower := strings.ToLower(reply)
	words := strings.FieldsFunc(lower, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
	for _, key := range keys {
		key = strings.ToLower(strings.TrimSpace(key))
		if key == "" {
			continue
		}
		if strings.IndexFunc(key, func(r rune) bool { return !unicode.IsLetter(r) }) >= 0 {
			if strings.Contains(lower, key) {
				return true
			}
			continue
		}
		for _, word := range words {
			if strings.HasPrefix(word, key) {
				return true
			}
		}
	}
	return false
}

func Bullets(lines []string) string {
	var b strings.Builder
	for _, line := range lines {
		b.WriteString("- " + line + "\n")
	}
	return b.String()
}

func ClosingNote(pending string, left []string, never string) string {
	return closingNoteOpening + pending + closingNoteLeftOut + Bullets(left) + closingNoteRewrite + never
}
