package site

import (
	"regexp"
	"strconv"
	"strings"
)

const tailwindEntryImport = `@import\s+["']tailwindcss["']`

var (
	tailwindEntry = regexp.MustCompile(tailwindEntryImport)
	tokenName     = regexp.MustCompile(`^--[A-Za-z0-9_-]+$`)
)

type Token struct {
	Name  string `json:"name" example:"--color-brand"`
	Value string `json:"value" example:"oklch(0.56 0.15 162)"`
	Path  string `json:"path" example:"theme.css"`
}

type KnobState struct {
	Knob
	Number *float64 `json:"number,omitempty" example:"1.25"`
	Color  *string  `json:"color,omitempty" example:"#ff0066"`
}

type declaration struct {
	name  string
	value string
}

func styleFiles(p Project) []string {
	var out []string
	for _, path := range p.Paths() {
		if kind, err := KindOf(p.Stack, path); err == nil && kind == KindStyle {
			out = append(out, path)
		}
	}
	return out
}

func ThemeFile(p Project) (string, bool) {
	if p.Stack == StackHTML {
		return ThemePath, p.Has(ThemePath)
	}
	candidates := append([]string{"src/index.css"}, styleFiles(p)...)
	for _, path := range candidates {
		if content, ok := p.Content(path); ok && len(themeBlocks(content)) > 0 {
			return path, true
		}
	}
	for _, path := range candidates {
		if content, ok := p.Content(path); ok && tailwindEntry.MatchString(content) {
			return path, true
		}
	}
	return "", false
}

func styleOrder(p Project) []string {
	theme, ok := ThemeFile(p)
	var out []string
	if ok {
		out = append(out, theme)
	}
	for _, path := range styleFiles(p) {
		if path != theme {
			out = append(out, path)
		}
	}
	return out
}

func ThemeTokens(p Project) []Token {
	tokens := []Token{}
	seen := map[string]bool{}
	for _, path := range styleOrder(p) {
		content := p.Files[path].Content
		for _, block := range themeBlocks(content) {
			for _, d := range declarations(content, block[0], block[1], true) {
				if seen[d.name] {
					continue
				}
				seen[d.name] = true
				tokens = append(tokens, Token{Name: d.name, Value: d.value, Path: path})
			}
		}
	}
	return tokens
}

func variableValue(p Project, variable string) (string, bool) {
	for _, path := range styleOrder(p) {
		content := p.Files[path].Content
		for _, block := range themeBlocks(content) {
			for _, d := range declarations(content, block[0], block[1], true) {
				if d.name == variable {
					return d.value, true
				}
			}
		}
		for _, d := range declarations(content, 0, len(content), false) {
			if d.name == variable {
				return d.value, true
			}
		}
	}
	return "", false
}

func KnobStates(p Project) ([]KnobState, error) {
	states := []KnobState{}
	raw, ok := p.Content(KnobsPath)
	if !ok {
		return states, nil
	}
	knobs, err := DecodeKnobs([]byte(raw))
	if err != nil {
		return nil, err
	}
	for _, knob := range knobs.Knobs {
		state := KnobState{Knob: knob}
		if value, found := variableValue(p, knob.Variable); found {
			state.Number, state.Color = knob.current(value)
		}
		states = append(states, state)
	}
	return states, nil
}

func (k Knob) current(value string) (*float64, *string) {
	switch k.Kind {
	case KnobRange:
		digits, ok := strings.CutSuffix(value, k.Unit)
		if !ok {
			return nil, nil
		}
		n, err := strconv.ParseFloat(strings.TrimSpace(digits), 64)
		if err != nil {
			return nil, nil
		}
		if _, err := k.CSSValue(KnobValue{Number: &n}); err != nil {
			return nil, nil
		}
		return &n, nil
	case KnobColor:
		if _, err := k.CSSValue(KnobValue{Text: &value}); err != nil {
			return nil, nil
		}
		return nil, &value
	}
	return nil, nil
}

func skipCSS(css string, i int) int {
	switch {
	case strings.HasPrefix(css[i:], "/*"):
		end := strings.Index(css[i+2:], "*/")
		if end < 0 {
			return len(css)
		}
		return i + 2 + end + 2
	case css[i] == '"' || css[i] == '\'':
		for j := i + 1; j < len(css); j++ {
			switch css[j] {
			case '\\':
				j++
			case css[i], '\n':
				return j + 1
			}
		}
		return len(css)
	}
	return i
}

func themeBlocks(css string) [][2]int {
	var blocks [][2]int
	for i := 0; i < len(css); i++ {
		if next := skipCSS(css, i); next != i {
			i = next - 1
			continue
		}
		if !strings.HasPrefix(css[i:], "@theme") || (i+6 < len(css) && identByte(css[i+6])) {
			continue
		}
		open := strings.IndexByte(css[i:], '{')
		semicolon := strings.IndexByte(css[i:], ';')
		if open < 0 || (semicolon >= 0 && semicolon < open) {
			continue
		}
		open += i
		close := matchingBrace(css, open)
		if close < 0 {
			continue
		}
		blocks = append(blocks, [2]int{open + 1, close})
		i = close
	}
	return blocks
}

func matchingBrace(css string, open int) int {
	depth := 0
	for i := open; i < len(css); i++ {
		if next := skipCSS(css, i); next != i {
			i = next - 1
			continue
		}
		switch css[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

func declarations(css string, from, to int, topOnly bool) []declaration {
	var out []declaration
	var segment strings.Builder
	depth, parens := 0, 0
	end := func(keep bool) {
		if keep {
			if d, ok := parseDeclaration(segment.String()); ok {
				out = append(out, d)
			}
		}
		segment.Reset()
	}
	for i := from; i < to; i++ {
		if strings.HasPrefix(css[i:], "/*") {
			i = skipCSS(css, i) - 1
			continue
		}
		if next := skipCSS(css, i); next != i {
			segment.WriteString(css[i:min(next, to)])
			i = next - 1
			continue
		}
		c := css[i]
		switch {
		case c == '(':
			parens++
		case c == ')':
			parens--
		case parens > 0:
		case c == ';':
			end(!topOnly || depth == 0)
			continue
		case c == '{':
			end(false)
			depth++
			continue
		case c == '}':
			end(!topOnly || depth == 0)
			depth--
			if topOnly && depth == 0 {
				segment.Reset()
			}
			continue
		}
		segment.WriteByte(c)
	}
	end(!topOnly || depth == 0)
	return out
}

func parseDeclaration(segment string) (declaration, bool) {
	name, value, found := strings.Cut(strings.TrimSpace(segment), ":")
	name = strings.TrimSpace(name)
	value = strings.TrimSpace(value)
	if !found || !tokenName.MatchString(name) || value == "" {
		return declaration{}, false
	}
	return declaration{name: name, value: value}, true
}
