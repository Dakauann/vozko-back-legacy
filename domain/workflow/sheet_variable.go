package workflow

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
)

const (
	SheetStepVariableKey        = "sheet_variable"
	RunStateSheetFiles          = "_sheet_files"
	CodeSheetVariableUnresolved = "sheet_variable_unresolved"
	AgentSheetRowTool           = "add_sheet_row"
)

var (
	ErrNodeSheetVariableInvalid = errors.New("workflow: the step that changes spreadsheet per conversation must name one conversation variable of type Planilha whose model holds the step's tab")
	ErrSheetVariableUnresolved  = errors.New("workflow: the spreadsheet of this conversation was not resolved when the run started")
)

type SheetFile struct {
	SpreadsheetID string            `json:"spreadsheet_id"`
	Tabs          map[string]string `json:"tabs"`
}

func SheetVariableToken(name string) string {
	return "{{" + ConversationRef(name) + "}}"
}

func SheetStepVariable(n *Node) (string, bool, error) {
	name, set, err := SheetVariableOf(n.Config[SheetStepVariableKey])
	if err != nil {
		return "", true, fmt.Errorf("%w: node %q", err, n.ID)
	}
	return name, set, nil
}

func SheetVariableOf(raw interface{}) (string, bool, error) {
	if raw == nil {
		return "", false, nil
	}
	text, ok := raw.(string)
	if !ok {
		return "", true, fmt.Errorf("%w: %s is not text", ErrNodeSheetVariableInvalid, SheetStepVariableKey)
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return "", false, nil
	}
	match := interpolationPattern.FindStringSubmatchIndex(text)
	if match == nil || match[0] != 0 || match[1] != len(text) {
		return "", true, fmt.Errorf("%w: %q is not one conversation variable", ErrNodeSheetVariableInvalid, text)
	}
	name, ok := CampaignVarOf(text[match[2]:match[3]])
	if !ok {
		return "", true, fmt.Errorf("%w: %q is not a conversation variable", ErrNodeSheetVariableInvalid, text)
	}
	return name, true, nil
}

func PinSheetFiles(state *RunState, files map[string]SheetFile) {
	pinned := make(map[string]interface{}, len(files))
	for name, file := range files {
		tabs := make(map[string]interface{}, len(file.Tabs))
		for model, tab := range file.Tabs {
			tabs[model] = tab
		}
		pinned[name] = map[string]interface{}{"spreadsheet_id": file.SpreadsheetID, "tabs": tabs}
	}
	state.Set(RunStateSheetFiles, pinned)
}

func PinnedSheetFiles(state *RunState) map[string]SheetFile {
	raw, ok := state.Get(RunStateSheetFiles)
	if !ok {
		return nil
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		return nil
	}
	var files map[string]SheetFile
	if json.Unmarshal(encoded, &files) != nil {
		return nil
	}
	return files
}

func SheetStepTarget(n *Node, state *RunState) (string, error) {
	tab := SheetStepTab(n)
	name, set, err := SheetStepVariable(n)
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrSheetVariableUnresolved, err)
	}
	if !set {
		return tab, nil
	}
	file, ok := PinnedSheetFiles(state)[name]
	if !ok {
		return "", fmt.Errorf("%w: %s", ErrSheetVariableUnresolved, name)
	}
	target, ok := file.Tabs[tab]
	if !ok || target == "" {
		return "", fmt.Errorf("%w: %s has no tab for %s", ErrSheetVariableUnresolved, name, tab)
	}
	return target, nil
}

func SheetVariableLinkProblems(g *Graph, links SheetLinks) []error {
	exported := map[string][]string{}
	for i := range g.Nodes {
		n := &g.Nodes[i]
		if n.Type != NodeTypeActionExportMessagesSheet {
			continue
		}
		name, _, _ := SheetStepVariable(n)
		exported[SheetStepTab(n)] = append(exported[SheetStepTab(n)], name)
	}
	var problems []error
	for i := range g.Nodes {
		n := &g.Nodes[i]
		if n.Type != NodeTypeActionAddSheetRow {
			continue
		}
		name, _, err := SheetStepVariable(n)
		if err != nil {
			continue
		}
		for _, link := range links[SheetStepTab(n)] {
			writers := exported[link.TargetID]
			if len(writers) == 0 || slices.Contains(writers, name) {
				continue
			}
			problems = append(problems, fmt.Errorf("%w: node %q: a coluna %q aponta para a aba %q, que é exportada para outra planilha; o passo que grava a linha e o que exporta as mensagens precisam usar a mesma planilha", ErrNodeSheetVariableInvalid, n.ID, link.Header, link.TargetTitle))
			break
		}
	}
	return problems
}

func SheetStepMapping(n *Node) map[string]string {
	values, _ := n.Config[SheetStepValuesKey].(map[string]interface{})
	mapping := make(map[string]string, len(values))
	for key, value := range values {
		text, ok := value.(string)
		if !ok {
			text = TemplateText(value)
		}
		mapping[key] = text
	}
	return mapping
}
