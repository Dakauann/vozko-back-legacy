package workflow

import "fmt"

const (
	LintSheetLinkExportMissing   LintIssueCode = "SHEET_LINK_EXPORT_MISSING"
	LintSheetLinkExportSomePaths LintIssueCode = "SHEET_LINK_EXPORT_SOME_PATHS"
)

const (
	SheetStepSheetKey        = "sheet_id"
	SheetStepValuesKey       = "values"
	SheetStepWritten         = "sucesso"
	SheetStepRefused         = "erro"
	SheetStepAlreadyRecorded = "ja_registrada"

	sheetExportScope = "_sheet_export."
	exportStepLabel  = "Exportar mensagens para planilha"
)

type SheetLink struct {
	Column      string
	Header      string
	TargetID    string
	TargetTitle string
}

type SheetLinks map[string][]SheetLink

func SheetStepTab(n *Node) string {
	return configText(n, SheetStepSheetKey)
}

func sheetExportRef(tabID string) string {
	return sheetExportScope + tabID
}

func sheetExportProvisions(np nodeProvisions, n *Node) {
	if tab := SheetStepTab(n); tab != "" {
		np.on(onlyEdge(SheetStepWritten)).hidden(sheetExportRef(tab), matchExact)
	}
}

func SheetLinkExportIssues(g *Graph, catalog []NodeDefinition, links SheetLinks) []LintIssue {
	if len(links) == 0 {
		return nil
	}
	flow := newVariableFlow(g, NodeCatalogMap(catalog), VariableSources{Specs: TriggerInputSpecsOf(g, TriggerInputSources{})})
	var issues []LintIssue
	for i := range g.Nodes {
		n := &g.Nodes[i]
		if !flow.reached[i] || n.Type != NodeTypeActionAddSheetRow {
			continue
		}
		for _, link := range links[SheetStepTab(n)] {
			switch flow.availabilityAt(i, sheetExportRef(link.TargetID)) {
			case availableNever:
				issues = append(issues, sheetLinkIssue(n.ID, link, LintSheetLinkExportMissing, SeverityBlocking,
					fmt.Sprintf("O passo %q grava a coluna %q, um link para a aba %q, mas nenhum passo %s dessa aba roda antes dele; sem a exportação o link não existe e a linha segue pela saída de erro", flow.nodeName(i), link.Header, link.TargetTitle, exportStepLabel),
					fmt.Sprintf("Coloque antes deste passo, no mesmo caminho, um passo %s (%s) com a aba %q (sheet_id %q), seguindo pela saída %q.", exportStepLabel, NodeTypeActionExportMessagesSheet, link.TargetTitle, link.TargetID, SheetStepWritten)))
			case AvailableOnSomePaths:
				issues = append(issues, sheetLinkIssue(n.ID, link, LintSheetLinkExportSomePaths, SeverityAdvisory,
					fmt.Sprintf("O passo %q grava a coluna %q, um link para a aba %q, mas só alguns caminhos até ele passam por %s dessa aba; nos outros a linha segue pela saída de erro", flow.nodeName(i), link.Header, link.TargetTitle, exportStepLabel),
					fmt.Sprintf("Faça todos os caminhos até este passo passarem por um passo %s (%s) com a aba %q (sheet_id %q), seguindo pela saída %q.", exportStepLabel, NodeTypeActionExportMessagesSheet, link.TargetTitle, link.TargetID, SheetStepWritten)))
			}
		}
	}
	return issues
}

func sheetLinkIssue(nodeID string, link SheetLink, code LintIssueCode, severity LintSeverity, message, hint string) LintIssue {
	return LintIssue{
		Code:     code,
		Severity: severity,
		NodeID:   nodeID,
		Field:    SheetStepValuesKey,
		Path:     link.Column,
		Message:  message,
		Hint:     hint,
		Values:   map[string]string{"column": link.Header, "tab": link.TargetTitle, "tab_id": link.TargetID},
	}
}
