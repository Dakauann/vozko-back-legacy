package agent

import (
	"encoding/json"
	"strings"
)

type legacyBinding struct {
	Name       string                 `json:"name"`
	Visibility []string               `json:"visibility,omitempty"`
	Config     map[string]interface{} `json:"config,omitempty"`
}

type legacyBindings struct {
	Bindings []legacyBinding `json:"bindings"`
}

func EncodeLegacyBindings(bindings []ToolBinding) string {
	stored := legacyBindings{Bindings: make([]legacyBinding, 0, len(bindings))}
	for _, b := range bindings {
		lb := legacyBinding{Name: b.Name}
		if len(b.Visibility) > 0 {
			lb.Visibility = make([]string, len(b.Visibility))
			for i, v := range b.Visibility {
				lb.Visibility[i] = string(v)
			}
		}
		if len(b.Config) > 0 {
			lb.Config = b.Config
		}
		stored.Bindings = append(stored.Bindings, lb)
	}
	raw, _ := json.Marshal(stored)
	return string(raw)
}

func DecodeLegacyBindings(raw string) []ToolBinding {
	if strings.TrimSpace(raw) == "" {
		return []ToolBinding{}
	}
	var stored legacyBindings
	if err := json.Unmarshal([]byte(raw), &stored); err == nil && len(stored.Bindings) > 0 {
		out := make([]ToolBinding, 0, len(stored.Bindings))
		for _, lb := range stored.Bindings {
			b := ToolBinding{Name: lb.Name}
			if len(lb.Visibility) > 0 {
				b.Visibility = make([]ToolVisibility, len(lb.Visibility))
				for i, v := range lb.Visibility {
					b.Visibility[i] = ToolVisibility(v)
				}
			}
			if len(lb.Config) > 0 {
				b.Config = lb.Config
			}
			out = append(out, b)
		}
		return out
	}
	names := make(map[string]string)
	if err := json.Unmarshal([]byte(raw), &names); err == nil && len(names) > 0 {
		out := make([]ToolBinding, 0, len(names))
		for name := range names {
			out = append(out, ToolBinding{Name: name})
		}
		return out
	}
	return []ToolBinding{}
}
