package tooltest

import (
	"encoding/json"

	"vozko/domain/tools"
)

func paramChars(name string, p tools.Parameter) int {
	total := len(name) + len(p.Description)
	for _, value := range p.Enum {
		total += len(value)
	}
	if p.Items != nil {
		total += len(p.Items.Description)
		for _, value := range p.Items.Enum {
			total += len(value)
		}
		for child, cp := range p.Items.Properties {
			total += paramChars(child, cp)
		}
	}
	return total
}

func SchemaChars(def tools.Definition) int {
	total := len(def.Name) + len(def.Description)
	for name, p := range def.Parameters {
		total += paramChars(name, p)
	}
	return total
}

func WireChars(def tools.Definition) int {
	raw, err := json.Marshal(def.JSONSchema())
	if err != nil {
		return 0
	}
	return len(def.Name) + len(def.Description) + len(raw)
}
