package agenttool

import (
	"sort"
	"strings"

	"vozko/domain/agent"
	"vozko/domain/tools"
)

func ConcealBindings(defs []tools.Definition, bindings []agent.ToolBinding) []agent.ToolBinding {
	index := make(map[string]tools.Definition, len(defs))
	for _, def := range defs {
		index[strings.ToLower(def.Name)] = def
	}
	out := make([]agent.ToolBinding, len(bindings))
	for i, b := range bindings {
		if len(b.Config) > 0 {
			b.Config = ConcealPlain(sensitiveParamsOf(index, b.Name, b.Config), b.Config)
		}
		out[i] = b
	}
	return out
}

func sensitiveParamsOf(index map[string]tools.Definition, toolType string, config map[string]interface{}) []string {
	if def, ok := index[strings.ToLower(toolType)]; ok {
		return def.SensitiveConfig()
	}
	var params []string
	for key, value := range config {
		if _, isMap := value.(map[string]interface{}); isMap {
			params = append(params, key)
		}
	}
	sort.Strings(params)
	return params
}
