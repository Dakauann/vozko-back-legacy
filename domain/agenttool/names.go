package agenttool

import (
	"regexp"
	"strings"

	"vozko/domain/shared"
)

const (
	MaxNameLength     = shared.MaxIdentifierNameLength
	numericNamePrefix = "tool_"
	mcpNameSeparator  = "__"
)

var namePattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)

func ValidName(name string) bool {
	return namePattern.MatchString(name) && !strings.Contains(name, mcpNameSeparator)
}

func ProposeName(basis string, taken func(string) bool) string {
	base := Slug(basis)
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
