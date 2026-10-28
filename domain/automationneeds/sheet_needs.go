package automationneeds

import (
	"sort"

	"vozko/domain/sheetfit"
	"vozko/domain/variabledecl"
	"vozko/domain/workflow"
)

func sheetStepUse(node workflow.Node) (string, sheetfit.Use, bool) {
	kind := sheetfit.UseAddRow
	switch node.Type {
	case workflow.NodeTypeActionAddSheetRow:
	case workflow.NodeTypeActionExportMessagesSheet:
		kind = sheetfit.UseExport
	default:
		return "", sheetfit.Use{}, false
	}
	name, set, err := workflow.SheetStepVariable(&node)
	if err != nil || !set {
		return "", sheetfit.Use{}, false
	}
	return name, sheetfit.Use{ModelSheetID: workflow.SheetStepTab(&node), Kind: kind, Mapping: workflow.SheetStepMapping(&node)}, true
}

func (w *walker) agentSheetTool(config map[string]interface{}, path []Link) {
	name, set, err := workflow.SheetVariableOf(config[workflow.SheetStepVariableKey])
	if err != nil || !set {
		return
	}
	tab, _ := config[workflow.SheetStepSheetKey].(string)
	w.add(name, "", path)
	w.addUse(name, sheetfit.Use{ModelSheetID: tab, Kind: sheetfit.UseAgentRow})
}

func (w *walker) addUse(name string, use sheetfit.Use) {
	if w.uses == nil {
		w.uses = map[string][]sheetfit.Use{}
	}
	w.uses[name] = append(w.uses[name], use)
}

func (n Needs) SheetNeeds() Needs {
	var out Needs
	for _, need := range n {
		if len(need.SheetUses) > 0 {
			out = append(out, need)
		}
	}
	return out
}

func (v Variables) WithDeclaredDefaults(catalog variabledecl.Catalog) Variables {
	out := Variables{Optional: append(Needs(nil), v.Optional...)}
	for _, need := range v.Required {
		d, declared := catalog.Find(need.Name)
		if fallback := variabledecl.DefaultOf(d); declared && fallback != "" {
			need.Default = fallback
			out.Optional = append(out.Optional, need)
			continue
		}
		out.Required = append(out.Required, need)
	}
	sort.Slice(out.Optional, func(i, j int) bool { return out.Optional[i].Name < out.Optional[j].Name })
	return out
}
