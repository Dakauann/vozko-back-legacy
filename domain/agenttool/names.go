package agenttool

import (
	"regexp"
	"strings"

	"vozko/domain/shared"
)

const (
	MaxNameLength     = shared.MaxIdentifierNameLength
	proposedNameWords = 5
	numericNamePrefix = "tool_"
	mcpNameSeparator  = "__"
)

var namePattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)

func ValidName(name string) bool {
	return namePattern.MatchString(name) && !strings.Contains(name, mcpNameSeparator)
}

func ProposeName(toolType, description string, taken func(string) bool) string {
	words := strings.Fields(Slug(strings.Join(strings.Fields(description), " ")))
	base := ""
	if len(words) > 0 {
		parts := strings.Split(words[0], "_")
		if len(parts) > proposedNameWords {
			parts = parts[:proposedNameWords]
		}
		base = strings.Join(parts, "_")
	}
	if base == "" {
		base = Slug(toolType)
	}
	if base == "" {
		base = "tool"
	}
	if base[0] >= '0' && base[0] <= '9' {
		base = numericNamePrefix + base
	}
	base = trimTo(strings.ReplaceAll(base, mcpNameSeparator, "_"), MaxNameLength)
	if !taken(base) {
		return base
	}
	for n := 2; ; n++ {
		candidate := WithSuffix(base, n)
		if !taken(candidate) {
			return candidate
		}
	}
}
