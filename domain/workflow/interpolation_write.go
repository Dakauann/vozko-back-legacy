package workflow

import (
	"slices"
	"strings"
)

const unresolvedWritePrefix = "variáveis sem valor neste ponto do fluxo, nada foi gravado: "

func InterpolateForWrite(text string, state *RunState) (string, []string) {
	resolved := Interpolate(text, state, nil)
	return resolved, PlaceholdersIn(resolved)
}

func PlaceholdersIn(text string) []string {
	if !strings.Contains(text, "{{") {
		return nil
	}
	var refs []string
	for _, match := range interpolationPattern.FindAllStringSubmatch(text, -1) {
		refs = append(refs, normalizeIndexing(strings.TrimSpace(match[1])))
	}
	slices.Sort(refs)
	return slices.Compact(refs)
}

func UnresolvedWriteReason(refs []string) string {
	sorted := slices.Clone(refs)
	slices.Sort(sorted)
	return unresolvedWritePrefix + strings.Join(slices.Compact(sorted), ", ")
}
