package agenttool

import (
	"strings"

	"vozko/domain/shared"
)

func Slug(text string) string {
	folded := shared.FoldForMatch(text)
	var b strings.Builder
	b.Grow(len(folded))
	pendingUnderscore := false
	for _, r := range folded {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			if pendingUnderscore && b.Len() > 0 {
				b.WriteByte('_')
			}
			pendingUnderscore = false
			b.WriteRune(r)
			continue
		}
		pendingUnderscore = true
	}
	return b.String()
}

func trimTo(name string, limit int) string {
	if len(name) <= limit {
		return name
	}
	return strings.TrimRight(name[:limit], "_")
}
