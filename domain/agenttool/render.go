package agenttool

import (
	"net/url"
	"sort"
	"strings"
)

func SecretNames(texts ...string) []string {
	seen := map[string]bool{}
	var names []string
	for _, text := range texts {
		for _, m := range secretReferencePattern.FindAllStringSubmatch(text, -1) {
			if !seen[m[1]] {
				seen[m[1]] = true
				names = append(names, m[1])
			}
		}
	}
	sort.Strings(names)
	return names
}

func RenderSecrets(text string, values map[string]string, escape func(string) string) string {
	if !strings.Contains(text, "{{") {
		return text
	}
	return secretReferencePattern.ReplaceAllStringFunc(text, func(match string) string {
		name := secretReferencePattern.FindStringSubmatch(match)[1]
		value, ok := values[name]
		if !ok {
			return match
		}
		if escape != nil {
			return escape(value)
		}
		return value
	})
}

func RenderURL(raw string, values map[string]string) string {
	base, query, fragment, ok := splitQuery(raw)
	rendered := RenderSecrets(base, values, url.PathEscape)
	if !ok {
		return rendered
	}
	return rendered + "?" + RenderSecrets(query, values, url.QueryEscape) + fragment
}
