package workflow

import (
	"strings"
	"testing"
)

const (
	paymentsTab = "tab-pagamentos"
	dealsTab    = "tab-negociacoes"
	otherTab    = "tab-outra"
)

func paymentLinks() SheetLinks {
	return SheetLinks{paymentsTab: {{Column: "negociacao", Header: "Negociação", TargetID: dealsTab, TargetTitle: "Negociações"}}}
}

func paymentRow(id string) Node {
	return Node{ID: id, Type: NodeTypeActionAddSheetRow, Config: map[string]interface{}{SheetStepSheetKey: paymentsTab, "display_name": "Registrar pagamento"}}
}

func exportOf(id, tab string) Node {
	return Node{ID: id, Type: NodeTypeActionExportMessagesSheet, Config: map[string]interface{}{SheetStepSheetKey: tab}}
}

func messageTrigger(id string) Node {
	return Node{ID: id, Type: NodeTypeTriggerMessageReceived, Config: map[string]interface{}{}}
}

func paymentAgent() Node {
	return Node{ID: "n1", Type: NodeTypeActionAIAgent, Config: map[string]interface{}{}}
}

func endNode() Node {
	return Node{ID: "end", Type: NodeTypeEnd}
}

func flowOf(nodes []Node, edges ...Edge) *Graph {
	return &Graph{Nodes: append(nodes, endNode()), Edges: edges}
}

func linkIssues(g *Graph) []LintIssue {
	return SheetLinkExportIssues(g, catalogDefs(), paymentLinks())
}

func TestAnExportOfTheLinkedTabEarlierOnThePathLetsTheRowLink(t *testing.T) {
	g := flowOf([]Node{messageTrigger("t"), exportOf("n7", dealsTab), paymentRow("n8")},
		Edge{Source: "t", Target: "n7"},
		Edge{Source: "n7", Target: "n8", Label: SheetStepWritten},
		Edge{Source: "n8", Target: "end", Label: SheetStepWritten})

	if issues := linkIssues(g); len(issues) != 0 {
		t.Fatalf("issues = %+v", issues)
	}
}

func TestAnExportAfterTheRowCannotLinkItSoPublishingIsBlocked(t *testing.T) {
	g := flowOf([]Node{messageTrigger("t"), paymentRow("n8"), exportOf("n7", dealsTab)},
		Edge{Source: "t", Target: "n8"},
		Edge{Source: "n8", Target: "n7", Label: SheetStepWritten},
		Edge{Source: "n7", Target: "end", Label: SheetStepWritten})

	issues := linkIssues(g)

	if len(issues) != 1 {
		t.Fatalf("issues = %+v", issues)
	}
	got := issues[0]
	if got.Code != LintSheetLinkExportMissing || got.Severity != SeverityBlocking || got.NodeID != "n8" || got.Field != SheetStepValuesKey || got.Path != "negociacao" {
		t.Fatalf("issue = %+v", got)
	}
	if got.Values["column"] != "Negociação" || got.Values["tab"] != "Negociações" || got.Values["tab_id"] != dealsTab {
		t.Fatalf("values = %v", got.Values)
	}
	for _, name := range []string{"Registrar pagamento", "Negociação", "Negociações", "Exportar mensagens para planilha"} {
		if !strings.Contains(got.Message, name) {
			t.Fatalf("message %q does not name %q", got.Message, name)
		}
	}
	for _, builderNeeds := range []string{string(NodeTypeActionExportMessagesSheet), dealsTab, SheetStepWritten} {
		if !strings.Contains(got.Hint, builderNeeds) {
			t.Fatalf("hint %q does not tell the builder %q", got.Hint, builderNeeds)
		}
	}
	if (LintReport{Issues: issues}).IsGreen() {
		t.Fatal("a row that can never link must block publishing")
	}
}

func TestAnExportOnOnlyOneBranchIsAWarning(t *testing.T) {
	g := flowOf([]Node{messageTrigger("t"), {ID: "c", Type: NodeTypeConditionBranch, Config: map[string]interface{}{}}, exportOf("n7", dealsTab), paymentRow("n8")},
		Edge{Source: "t", Target: "c"},
		Edge{Source: "c", Target: "n7", Label: "true"},
		Edge{Source: "c", Target: "n8", Label: "false"},
		Edge{Source: "n7", Target: "n8", Label: SheetStepWritten},
		Edge{Source: "n8", Target: "end", Label: SheetStepWritten})

	issues := linkIssues(g)

	if len(issues) != 1 || issues[0].Code != LintSheetLinkExportSomePaths || issues[0].Severity != SeverityAdvisory || issues[0].NodeID != "n8" {
		t.Fatalf("issues = %+v", issues)
	}
	if issues[0].Values["tab"] != "Negociações" || issues[0].Values["column"] != "Negociação" || issues[0].Hint == "" {
		t.Fatalf("issue = %+v", issues[0])
	}
}

func TestAnExportOfAnotherTabDoesNotProvideTheLink(t *testing.T) {
	g := flowOf([]Node{messageTrigger("t"), exportOf("n7", otherTab), paymentRow("n8")},
		Edge{Source: "t", Target: "n7"},
		Edge{Source: "n7", Target: "n8", Label: SheetStepWritten},
		Edge{Source: "n8", Target: "end", Label: SheetStepWritten})

	if issues := linkIssues(g); len(issues) != 1 || issues[0].Code != LintSheetLinkExportMissing {
		t.Fatalf("issues = %+v", issues)
	}
}

func TestAFailedExportMarksNothingSoItsErrorOutputDoesNotProvideTheLink(t *testing.T) {
	g := flowOf([]Node{messageTrigger("t"), exportOf("n7", dealsTab), paymentRow("n8")},
		Edge{Source: "t", Target: "n7"},
		Edge{Source: "n7", Target: "n8", Label: SheetStepRefused},
		Edge{Source: "n8", Target: "end", Label: SheetStepWritten})

	if issues := linkIssues(g); len(issues) != 1 || issues[0].Code != LintSheetLinkExportMissing {
		t.Fatalf("issues = %+v", issues)
	}
}

func TestACodeNodeWritingComputedKeysCannotStandInForTheExport(t *testing.T) {
	g := flowOf([]Node{messageTrigger("t"), {ID: "js", Type: NodeTypeActionCode, Config: map[string]interface{}{"code": "state.set(nome, 1)"}}, paymentRow("n8")},
		Edge{Source: "t", Target: "js"},
		Edge{Source: "js", Target: "n8"},
		Edge{Source: "n8", Target: "end", Label: SheetStepWritten})

	if issues := linkIssues(g); len(issues) != 1 || issues[0].Code != LintSheetLinkExportMissing {
		t.Fatalf("issues = %+v", issues)
	}
}

func TestARowOnATabWithoutLinkColumnsNeedsNoExport(t *testing.T) {
	row := paymentRow("n8")
	row.Config[SheetStepSheetKey] = otherTab
	g := flowOf([]Node{messageTrigger("t"), row}, Edge{Source: "t", Target: "n8"}, Edge{Source: "n8", Target: "end", Label: SheetStepWritten})

	if issues := linkIssues(g); len(issues) != 0 {
		t.Fatalf("issues = %+v", issues)
	}
}

func TestTheExportOnAnotherTriggerPathDoesNotReachTheAgentToolRoute(t *testing.T) {
	g := flowOf([]Node{
		messageTrigger("t"), paymentAgent(), paymentRow("n8"),
		{ID: "n5", Type: NodeTypeTriggerStageAdded, Config: map[string]interface{}{"stage_id": "pago"}}, exportOf("n7", dealsTab),
	},
		Edge{Source: "t", Target: "n1"},
		Edge{Source: "n1", Target: "n8", Label: "pagamento_realizado"},
		Edge{Source: "n8", Target: "end", Label: SheetStepWritten},
		Edge{Source: "n5", Target: "n7"},
		Edge{Source: "n7", Target: "end", Label: SheetStepWritten})

	issues := linkIssues(g)

	if len(issues) != 1 || issues[0].Code != LintSheetLinkExportMissing || issues[0].NodeID != "n8" {
		t.Fatalf("issues = %+v", issues)
	}
}

func TestAnAgentLoopingBackThroughTheExportOnlyCoversTheLaterTurns(t *testing.T) {
	g := flowOf([]Node{messageTrigger("t"), paymentAgent(), exportOf("n7", dealsTab), paymentRow("n8")},
		Edge{Source: "t", Target: "n1"},
		Edge{Source: "n1", Target: "n7", Label: "exportar"},
		Edge{Source: "n7", Target: "n1", Label: SheetStepWritten},
		Edge{Source: "n1", Target: "n8", Label: "pagamento_realizado"},
		Edge{Source: "n8", Target: "end", Label: SheetStepWritten})

	if issues := linkIssues(g); len(issues) != 1 || issues[0].Code != LintSheetLinkExportSomePaths {
		t.Fatalf("issues = %+v", issues)
	}
}

func TestAnAgentRoutingBackToItselfWithoutAnExportNeverLinks(t *testing.T) {
	g := flowOf([]Node{messageTrigger("t"), paymentAgent(), paymentRow("n8")},
		Edge{Source: "t", Target: "n1"},
		Edge{Source: "n1", Target: "n1", Label: "continuar"},
		Edge{Source: "n1", Target: "n8", Label: "pagamento_realizado"},
		Edge{Source: "n8", Target: "end", Label: SheetStepWritten})

	if issues := linkIssues(g); len(issues) != 1 || issues[0].Code != LintSheetLinkExportMissing {
		t.Fatalf("issues = %+v", issues)
	}
}

func TestAnExportBeforeTheAgentLoopCoversEveryTurn(t *testing.T) {
	g := flowOf([]Node{messageTrigger("t"), exportOf("n7", dealsTab), paymentAgent(), paymentRow("n8")},
		Edge{Source: "t", Target: "n7"},
		Edge{Source: "n7", Target: "n1", Label: SheetStepWritten},
		Edge{Source: "n1", Target: "n1", Label: "continuar"},
		Edge{Source: "n1", Target: "n8", Label: "pagamento_realizado"},
		Edge{Source: "n8", Target: "end", Label: SheetStepWritten})

	if issues := linkIssues(g); len(issues) != 0 {
		t.Fatalf("issues = %+v", issues)
	}
}

func TestARowNoTriggerReachesIsLeftToTheOrphanLint(t *testing.T) {
	g := flowOf([]Node{messageTrigger("t"), paymentRow("n8")}, Edge{Source: "t", Target: "end"})

	if issues := linkIssues(g); len(issues) != 0 {
		t.Fatalf("issues = %+v", issues)
	}
}

func TestTheExportMarkerIsNeverOfferedAsAVariable(t *testing.T) {
	g := flowOf([]Node{messageTrigger("t"), exportOf("n7", dealsTab), sendText("s", "oi")},
		Edge{Source: "t", Target: "n7"},
		Edge{Source: "n7", Target: "s", Label: SheetStepWritten},
		Edge{Source: "s", Target: "end"})

	for _, v := range VariableCatalogAt(g, catalogDefs(), VariableSources{}, "s").Variables {
		if strings.HasPrefix(v.Ref, sheetExportScope) {
			t.Fatalf("catalog offers %q", v.Ref)
		}
	}
}
