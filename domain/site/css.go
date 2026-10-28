package site

import (
	"path"
	"strings"
)

var allowedAtRules = map[string]bool{
	"theme": true, "layer": true, "media": true, "supports": true, "keyframes": true, "-webkit-keyframes": true,
	"font-face": true, "utility": true, "variant": true, "custom-variant": true, "apply": true, "container": true,
	"property": true, "starting-style": true, "page": true, "font-feature-values": true, "counter-style": true,
	"scope": true, "view-transition": true, "charset": true, "slot": true,
	"import": true, "reference": true, "source": true,
}

const tailwindPackage = "tailwindcss"

type cssScanner struct {
	src   string
	i     int
	file  string
	p     Project
	found []Issue
}

func CheckCSS(p Project, file string) []Issue {
	s := &cssScanner{src: p.Files[file].Content, file: file, p: p}
	s.scan()
	return s.found
}

func (s *cssScanner) refuse(detail string) {
	s.found = append(s.found, Issue{Code: CodeCSSDirectiveForbidden, Path: s.file, Detail: detail})
}

func (s *cssScanner) scan() {
	for s.i < len(s.src) {
		switch c := s.src[s.i]; {
		case strings.HasPrefix(s.src[s.i:], "/*"):
			if !s.skipComment() {
				s.refuse("a comment is not closed")
				return
			}
		case c == '"' || c == '\'':
			if !s.skipString() {
				s.refuse("a string is not closed")
				return
			}
		case c == '\\':
			s.i += 2
		case c == '@':
			if !s.atRule() {
				return
			}
		default:
			s.i++
		}
	}
}

func (s *cssScanner) skipComment() bool {
	end := strings.Index(s.src[s.i+2:], "*/")
	if end < 0 {
		return false
	}
	s.i += 2 + end + 2
	return true
}

func (s *cssScanner) skipString() bool {
	quote := s.src[s.i]
	for j := s.i + 1; j < len(s.src); j++ {
		switch s.src[j] {
		case '\\':
			j++
		case '\n':
			return false
		case quote:
			s.i = j + 1
			return true
		}
	}
	return false
}

func identByte(c byte) bool {
	return c == '-' || c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
}

func (s *cssScanner) atRule() bool {
	j := s.i + 1
	for j < len(s.src) && identByte(s.src[j]) {
		j++
	}
	if j < len(s.src) && s.src[j] == '\\' {
		s.refuse("an escaped at-rule name")
		return false
	}
	name := strings.ToLower(s.src[s.i+1 : j])
	s.i = j
	prelude, ok := s.prelude()
	if !ok {
		s.refuse("@" + name + " is not terminated")
		return false
	}
	if !allowedAtRules[name] {
		s.refuse("@" + name)
		return true
	}
	switch name {
	case "import", "reference":
		if !s.stylesheetTarget(prelude) {
			s.refuse("@" + name + " " + strings.TrimSpace(prelude))
		}
	case "source":
		if !s.sourceTarget(prelude) {
			s.refuse("@source " + strings.TrimSpace(prelude))
		}
	}
	return true
}

func (s *cssScanner) prelude() (string, bool) {
	start := s.i
	depth := 0
	for s.i < len(s.src) {
		switch c := s.src[s.i]; {
		case strings.HasPrefix(s.src[s.i:], "/*"):
			if !s.skipComment() {
				return "", false
			}
			continue
		case c == '"' || c == '\'':
			if !s.skipString() {
				return "", false
			}
			continue
		case c == '(':
			depth++
		case c == ')':
			depth--
		case depth <= 0 && (c == ';' || c == '{' || c == '}'):
			return s.src[start:s.i], true
		}
		s.i++
	}
	return s.src[start:], depth <= 0
}

func firstTarget(prelude string) (string, bool) {
	rest := strings.TrimSpace(stripCSSComments(prelude))
	if lower := strings.ToLower(rest); strings.HasPrefix(lower, "url(") {
		end := strings.IndexByte(rest, ')')
		if end < 0 {
			return "", false
		}
		inner := strings.TrimSpace(rest[4:end])
		return strings.Trim(inner, `"'`), inner != ""
	}
	if rest == "" || (rest[0] != '"' && rest[0] != '\'') {
		return "", false
	}
	end := strings.IndexByte(rest[1:], rest[0])
	if end < 0 {
		return "", false
	}
	return rest[1 : 1+end], true
}

func relativeTarget(target string) bool {
	return strings.HasPrefix(target, "./") || strings.HasPrefix(target, "../")
}

func insideProject(resolved string) bool {
	return resolved != ".." && !strings.HasPrefix(resolved, "../") && !strings.HasPrefix(resolved, "/")
}

func (s *cssScanner) stylesheetTarget(prelude string) bool {
	target, ok := firstTarget(prelude)
	if !ok {
		return false
	}
	if target == tailwindPackage || (strings.HasPrefix(target, tailwindPackage+"/") && !strings.Contains(target, "..")) {
		return true
	}
	if !relativeTarget(target) {
		return false
	}
	resolved := path.Join(path.Dir(s.file), target)
	if !insideProject(resolved) || resolved == s.file || !s.p.Has(resolved) {
		return false
	}
	kind, err := KindOf(s.p.Stack, resolved)
	return err == nil && kind == KindStyle
}

func (s *cssScanner) sourceTarget(prelude string) bool {
	rest := strings.TrimSpace(stripCSSComments(prelude))
	if after, negated := strings.CutPrefix(rest, "not "); negated {
		rest = strings.TrimSpace(after)
	}
	if strings.HasPrefix(rest, "inline(") {
		return true
	}
	target, ok := firstTarget(rest)
	if !ok || !relativeTarget(target) {
		return false
	}
	return insideProject(path.Join(path.Dir(s.file), target))
}

func stripCSSComments(css string) string {
	var b strings.Builder
	for {
		start := strings.Index(css, "/*")
		if start < 0 {
			b.WriteString(css)
			return b.String()
		}
		b.WriteString(css[:start])
		end := strings.Index(css[start+2:], "*/")
		if end < 0 {
			return b.String()
		}
		css = css[start+2+end+2:]
	}
}
