package agenttool

import (
	"regexp"
	"strconv"
	"strings"

	"vozko/domain/redact"
	"vozko/domain/shared"
)

const secretNamePrefix = "agent_"

var (
	secretReferencePattern = regexp.MustCompile(`\{\{\s*secret\.([a-z0-9_]{1,64})\s*\}\}`)
	authSchemePattern      = regexp.MustCompile(`(?i)^\s*(bearer|basic|token|bot|apikey)\s+`)
	credentialNameParts    = []string{"auth", "key", "signature", "session"}
)

func IsCredentialName(name string) bool {
	if redact.IsSensitiveKey(name) {
		return true
	}
	normalized := strings.NewReplacer("_", "", "-", "", " ", "").Replace(strings.ToLower(name))
	for _, part := range credentialNameParts {
		if strings.Contains(normalized, part) {
			return true
		}
	}
	return false
}

func IsSealed(value string) bool {
	rest := authSchemePattern.ReplaceAllString(value, "")
	rest = secretReferencePattern.ReplaceAllString(rest, "")
	return strings.TrimSpace(rest) == ""
}

func Reference(name string) string {
	return "{{secret." + name + "}}"
}

func SecretNameFor(agentID, toolName, field string) string {
	parts := []string{}
	if short := Slug(strings.ReplaceAll(firstSegment(agentID), "-", "")); short != "" {
		parts = append(parts, short)
	}
	for _, p := range []string{Slug(toolName), Slug(field)} {
		if p != "" {
			parts = append(parts, p)
		}
	}
	return trimTo(secretNamePrefix+strings.Join(parts, "_"), shared.MaxIdentifierNameLength)
}

func WithSuffix(name string, n int) string {
	suffix := "_" + strconv.Itoa(n)
	return trimTo(name, shared.MaxIdentifierNameLength-len(suffix)) + suffix
}

func firstSegment(id string) string {
	id = strings.TrimSpace(id)
	if head, _, ok := strings.Cut(id, "-"); ok {
		return head
	}
	if len(id) > 8 {
		return id[:8]
	}
	return id
}
