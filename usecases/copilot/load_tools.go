package copilot_usecase

import (
	"context"
	"fmt"
	"strings"

	"vozko/domain/copilot"
	"vozko/domain/tools"
	"vozko/usecases/copilot/copilottools"
)

const (
	loadToolsName       = "load_tools"
	unknownToolAreaCode = "unknown_tool_area"
)

type loadToolsArgs struct {
	Areas []string `json:"areas" req:"true" desc:"chaves exatas das áreas da lista"`
}

type areaLoad struct {
	Areas []string `json:"areas"`
	Tools []string `json:"tools"`
}

type loadToolsTool struct{ areas toolAreas }

func newLoadToolsTool(areas toolAreas) copilot.Tool {
	return &loadToolsTool{areas: areas}
}

func (t *loadToolsTool) Meta() copilot.Meta { return copilottools.ChatReadMeta() }

func (t *loadToolsTool) OfferedOn(view copilot.View) bool {
	return defersOn(view) && len(t.areas.on(view).areas) > 0
}

func (t *loadToolsTool) Definition() tools.Definition {
	return t.index(t.areas)
}

func (t *loadToolsTool) DefinitionOn(view copilot.View) tools.Definition {
	return t.index(t.areas.on(view))
}

func (t *loadToolsTool) index(areas toolAreas) tools.Definition {
	var b strings.Builder
	b.WriteString("Carrega ferramentas por área; as desta tela já estão disponíveis. " +
		"No começo da resposta, carregue numa só chamada todas as áreas de que o pedido precisa. " +
		"Uma área carregada fica até o fim da resposta e nas próximas desta conversa, enquanto ela não for resumida; se uma ferramenta sumir, carregue de novo. Áreas:")
	for _, area := range areas.areas {
		b.WriteString("\n- " + area.Key + ": " + area.Summary + " (" + strings.Join(area.Tools, ", ") + ")")
	}
	def := copilottools.Definition(loadToolsName, b.String(), loadToolsArgs{})
	param := def.Parameters["areas"]
	param.Items.Enum = areas.keys()
	def.Parameters["areas"] = param
	return def
}

func (t *loadToolsTool) Execute(_ context.Context, cc copilot.Context, args map[string]interface{}) copilot.Result {
	offered := t.areas.on(cc.View)
	keys := requestedAreas(args)
	if len(keys) == 0 {
		return refuseAreas(offered, "nenhuma área pedida")
	}
	for _, key := range keys {
		if !offered.known(key) {
			return refuseAreas(offered, "área desconhecida: "+key)
		}
	}
	return copilot.Result{Status: copilot.StatusOK, Data: areaLoad{Areas: keys}}
}

func refuseAreas(offered toolAreas, reason string) copilot.Result {
	return copilot.Result{Status: copilot.StatusError, Message: fmt.Sprintf("%s: %s; nada foi carregado; use uma destas: %s", unknownToolAreaCode, reason, strings.Join(offered.keys(), ", "))}
}

func requestedAreas(args map[string]interface{}) []string {
	var raw []string
	switch v := args["areas"].(type) {
	case []interface{}:
		for _, item := range v {
			if s, ok := item.(string); ok {
				raw = append(raw, s)
			}
		}
	case []string:
		raw = v
	case string:
		raw = []string{v}
	}
	seen := make(map[string]bool, len(raw))
	keys := make([]string, 0, len(raw))
	for _, key := range raw {
		key = strings.TrimSpace(key)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		keys = append(keys, key)
	}
	return keys
}
