package site

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"
)

const (
	CodeDesignSystemInvalid = "design_system_invalid"
	SnippetKeyPrefix        = "snippet-"
)

var ErrDesignSystemInvalid = errors.New("site: the design system is invalid")

type FontRole string

const (
	FontHeading FontRole = "heading"
	FontBody    FontRole = "body"
	FontAccent  FontRole = "accent"
	FontMono    FontRole = "mono"
)

func FontRoles() []FontRole {
	return []FontRole{FontHeading, FontBody, FontAccent, FontMono}
}

type DesignFont struct {
	Family  string   `json:"family"`
	Role    FontRole `json:"role"`
	Weights []int    `json:"weights,omitempty"`
}

type DesignSnippet struct {
	Key     string `json:"key"`
	Name    string `json:"name"`
	Stack   Stack  `json:"stack"`
	Content string `json:"content"`
}

type DesignSystem struct {
	Name        string            `json:"name"`
	Tokens      map[string]string `json:"tokens,omitempty"`
	Fonts       []DesignFont      `json:"fonts,omitempty"`
	LogoMediaID string            `json:"logo_media_id,omitempty"`
	Voice       string            `json:"voice,omitempty"`
	Snippets    []DesignSnippet   `json:"snippets,omitempty"`
}

const (
	maxDesignNameRunes  = 120
	maxDesignVoiceRunes = 2000
	maxFontFamilyRunes  = 80
)

var (
	fontFamily = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9 ]*$`)
	snippetKey = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)
)

func designError(reason string) error {
	return fmt.Errorf("%w: %s", ErrDesignSystemInvalid, reason)
}

func validDesignName(name string) bool {
	trimmed := strings.TrimSpace(name)
	return trimmed != "" && trimmed == name && utf8.RuneCountInString(name) <= maxDesignNameRunes
}

func (d DesignSystem) Validate() error {
	if !validDesignName(d.Name) {
		return designError("name must be 1 to 120 characters without outer spaces")
	}
	for name, value := range d.Tokens {
		if !cssVariable.MatchString(name) || !validTokenValue(value) {
			return designError("token " + name + " is not a css variable with a plain value")
		}
	}
	roles := map[FontRole]bool{}
	for _, font := range d.Fonts {
		if !fontFamily.MatchString(font.Family) || utf8.RuneCountInString(font.Family) > maxFontFamilyRunes {
			return designError("font family " + font.Family + " is not a font name")
		}
		if !slices.Contains(FontRoles(), font.Role) || roles[font.Role] {
			return designError("font role " + string(font.Role) + " is unknown or repeats")
		}
		roles[font.Role] = true
		for _, weight := range font.Weights {
			if weight < 100 || weight > 900 || weight%100 != 0 {
				return designError("font weights go from 100 to 900 in steps of 100")
			}
		}
	}
	if d.LogoMediaID != "" && !referenceID.MatchString(d.LogoMediaID) {
		return designError("the logo is not a media id")
	}
	if utf8.RuneCountInString(d.Voice) > maxDesignVoiceRunes {
		return designError("the voice notes are too long")
	}
	return validateSnippets(d.Snippets)
}

func validateSnippets(snippets []DesignSnippet) error {
	keys := map[string]bool{}
	total := 0
	for _, snippet := range snippets {
		if !snippetKey.MatchString(snippet.Key) || keys[snippet.Key] {
			return designError("snippet key " + snippet.Key + " is malformed or repeats")
		}
		keys[snippet.Key] = true
		if !validDesignName(snippet.Name) || !snippet.Stack.Valid() || strings.TrimSpace(snippet.Content) == "" {
			return designError("snippet " + snippet.Key + " needs a name, a stack and content")
		}
		total += len(snippet.Content)
	}
	if total > MaxProjectBytes {
		return designError("the snippets are larger than a project")
	}
	return nil
}
