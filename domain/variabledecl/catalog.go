package variabledecl

import (
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

type Catalog struct {
	byName map[string]Declaration
}

func NewCatalog(declarations []Declaration) Catalog {
	c := Catalog{byName: make(map[string]Declaration, len(declarations))}
	for _, d := range declarations {
		c.byName[d.Name] = d
	}
	return c
}

func (c Catalog) Find(name string) (Declaration, bool) {
	d, ok := c.byName[name]
	return d, ok
}

func (c Catalog) LabelOf(name string) string {
	if label, ok := SystemLabel(name); ok {
		return label
	}
	if d, ok := c.byName[name]; ok && d.Label != "" {
		return d.Label
	}
	return HumanLabel(name)
}

func HumanLabel(name string) string {
	text := strings.Join(strings.Fields(strings.ReplaceAll(name, "_", " ")), " ")
	first, size := utf8.DecodeRuneInString(text)
	if size == 0 {
		return name
	}
	return string(unicode.ToUpper(first)) + text[size:]
}

func (c Catalog) All() []Declaration {
	out := make([]Declaration, 0, len(c.byName))
	for _, d := range c.byName {
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
