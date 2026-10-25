package template

import (
	"bytes"
	"encoding/json"
)

func (p ParameterFormat) OrDefault() ParameterFormat {
	if p == "" {
		return ParameterFormatPositional
	}
	return p
}

func (t *Template) SameContent(other *Template) bool {
	if t == nil || other == nil {
		return t == other
	}
	return t.WABAId == other.WABAId &&
		t.Name == other.Name &&
		t.Language == other.Language &&
		t.Category == other.Category &&
		t.Status == other.Status &&
		t.Quality == other.Quality &&
		t.ParameterFormat.OrDefault() == other.ParameterFormat.OrDefault() &&
		sameOptionalText(t.HeaderMediaURL, other.HeaderMediaURL) &&
		sameOptionalText(t.HeaderMediaID, other.HeaderMediaID) &&
		sameComponents(t.Components, other.Components)
}

func sameOptionalText(a, b *string) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func sameComponents(a, b []TemplateComponent) bool {
	if len(a) != len(b) {
		return false
	}
	if len(a) == 0 {
		return true
	}
	left, err := json.Marshal(a)
	if err != nil {
		return false
	}
	right, err := json.Marshal(b)
	if err != nil {
		return false
	}
	return bytes.Equal(left, right)
}
