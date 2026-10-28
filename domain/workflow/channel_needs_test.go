package workflow

import (
	"strings"
	"testing"

	"vozko/domain/googlesheet"
	"vozko/domain/shared"
)

func TestEveryBranchChannelDeclaresEveryFact(t *testing.T) {
	for _, channel := range ChannelBranchOrder {
		for _, fact := range ChannelFacts() {
			_, derived := derivedChannelFacts[fact]
			if _, declared := channelGuarantees[channel][fact]; !declared && !derived {
				t.Errorf("%s does not declare %s", channel, fact)
			}
		}
	}
}

func TestAChannelNobodyDeclaredProvidesNothing(t *testing.T) {
	for _, fact := range ChannelFacts() {
		if level := ChannelProvides(shared.EntryType("pombo_correio"), fact); level != ChannelNever {
			t.Fatalf("%s = %s", fact, level)
		}
	}
}

func TestPhoneChannelsAlwaysHaveAPhoneAndEmailAlwaysHasAnAddress(t *testing.T) {
	for _, channel := range []shared.EntryType{shared.EntryTypeWhatsApp, shared.EntryTypeUnofficialWhatsApp, shared.EntryTypeSMS, shared.EntryTypeRCS} {
		if level := ChannelProvides(channel, ChannelFactContactPhone); level != ChannelGuaranteed {
			t.Errorf("%s phone = %s", channel, level)
		}
	}
	if level := ChannelProvides(shared.EntryTypeEmail, ChannelFactContactEmail); level != ChannelGuaranteed {
		t.Errorf("email address = %s", level)
	}
	if level := ChannelProvides(shared.EntryTypeInstagram, ChannelFactContactPhone); level != ChannelMaybe {
		t.Errorf("instagram phone = %s", level)
	}
	if level := ChannelProvides(shared.EntryTypeEmail, ChannelFactTemplates); level != ChannelNever {
		t.Errorf("email templates = %s", level)
	}
}

func TestOnlyTheOfficialNumberSendsTemplatesAndForms(t *testing.T) {
	for _, fact := range []ChannelFact{ChannelFactTemplates, ChannelFactForms} {
		got := ChannelsWith(fact, ChannelGuaranteed)
		if len(got) != 1 || got[0] != shared.EntryTypeWhatsApp {
			t.Fatalf("%s = %v", fact, got)
		}
	}
}

func phoneSheet(required bool) SheetChannelColumns {
	return SheetChannelColumns{"tab-leads": SheetChannelColumnsOf([]googlesheet.Column{
		{Key: "nome", Header: "Nome", Source: googlesheet.SourceLeadName},
		{Key: "telefone", Header: "Telefone", Source: googlesheet.SourceLeadPhone, Required: required},
	})}
}

func sheetRow(id string) Node {
	return Node{ID: id, Type: NodeTypeActionAddSheetRow, Config: map[string]interface{}{SheetStepSheetKey: "tab-leads", "display_name": "Registrar lead"}}
}

func TestASheetColumnFilledWithThePhoneNeedsTheContactPhone(t *testing.T) {
	row := sheetRow("n1")
	needs := NodeChannelNeeds(&row, phoneSheet(true))
	if len(needs) != 1 {
		t.Fatalf("needs = %+v", needs)
	}
	need := needs[0]
	if need.Fact != ChannelFactContactPhone || !need.Required || need.Path != "telefone" || need.Subject != "Telefone" || need.Fallback != SheetStepRefused {
		t.Fatalf("need = %+v", need)
	}
}

func TestATemplateStepNeedsTemplates(t *testing.T) {
	step := Node{ID: "n1", Type: NodeTypeActionSendTemplate, Config: map[string]interface{}{"template_id": "tpl"}}
	needs := NodeChannelNeeds(&step, nil)
	if len(needs) != 1 || needs[0].Fact != ChannelFactTemplates || !needs[0].Required {
		t.Fatalf("needs = %+v", needs)
	}
}

func TestReadingTheContactNumberIsAnOptionalChannelNeed(t *testing.T) {
	step := Node{ID: "n1", Type: NodeTypeActionHTTPRequest, Config: map[string]interface{}{"body": `{"fone":"{{contact_number}}","n":"{{campvars.lead_number}}"}`}}
	needs := NodeChannelNeeds(&step, nil)
	if len(needs) != 2 {
		t.Fatalf("needs = %+v", needs)
	}
	for _, need := range needs {
		if need.Required || need.Field != "body" {
			t.Fatalf("need = %+v", need)
		}
	}
	if needs[0].Fact != ChannelFactPhoneHandle || needs[1].Fact != ChannelFactContactPhone {
		t.Fatalf("needs = %+v", needs)
	}
}

func channelBranch(id string) Node {
	return Node{ID: id, Type: NodeTypeConditionChannel, Config: map[string]interface{}{}}
}

func templateStep(id string) Node {
	return Node{ID: id, Type: NodeTypeActionSendTemplate, Config: map[string]interface{}{"template_id": "tpl", "display_name": "Enviar boleto"}}
}

func textStep(id string) Node {
	return Node{ID: id, Type: NodeTypeActionSendText, Config: map[string]interface{}{"text": "oi"}}
}

var whatsAppAndInstagram = []shared.EntryType{shared.EntryTypeInstagram, shared.EntryTypeWhatsApp}

func TestEveryLinkedChannelReachesAStepWithoutABranch(t *testing.T) {
	g := flowOf([]Node{messageTrigger("t"), templateStep("n1")}, Edge{Source: "t", Target: "n1"}, Edge{Source: "n1", Target: "end"})
	got := ChannelReachOf(g, whatsAppAndInstagram)["n1"]
	if len(got) != 2 || got[0] != shared.EntryTypeWhatsApp || got[1] != shared.EntryTypeInstagram {
		t.Fatalf("reach = %v", got)
	}
}

func TestAChannelBranchNarrowsWhatReachesEachSide(t *testing.T) {
	g := flowOf([]Node{messageTrigger("t"), channelBranch("c"), templateStep("n1"), textStep("n2")},
		Edge{Source: "t", Target: "c"},
		Edge{Source: "c", Target: "n1", Label: "whatsapp"},
		Edge{Source: "c", Target: "n2", Label: ChannelBranchDefault},
		Edge{Source: "n1", Target: "end"}, Edge{Source: "n2", Target: "end"})
	reach := ChannelReachOf(g, whatsAppAndInstagram)
	if got := reach["n1"]; len(got) != 1 || got[0] != shared.EntryTypeWhatsApp {
		t.Fatalf("template reach = %v", got)
	}
	if got := reach["n2"]; len(got) != 1 || got[0] != shared.EntryTypeInstagram {
		t.Fatalf("default reach = %v", got)
	}
}

func TestALeadTriggerCarriesNoChannel(t *testing.T) {
	birthday := Node{ID: "t", Type: NodeTypeTriggerBirthday, Config: map[string]interface{}{}}
	g := flowOf([]Node{birthday, templateStep("n1")}, Edge{Source: "t", Target: "n1"}, Edge{Source: "n1", Target: "end"})
	if reach := ChannelReachOf(g, whatsAppAndInstagram); len(reach["n1"]) != 0 {
		t.Fatalf("reach = %v", reach)
	}
}

func issueOf(t *testing.T, issues []LintIssue, nodeID string) LintIssue {
	t.Helper()
	for _, issue := range issues {
		if issue.NodeID == nodeID {
			return issue
		}
	}
	t.Fatalf("no issue on %s in %+v", nodeID, issues)
	return LintIssue{}
}

func TestATemplateOnInstagramBlocksPublishing(t *testing.T) {
	g := flowOf([]Node{messageTrigger("t"), templateStep("n1")}, Edge{Source: "t", Target: "n1"}, Edge{Source: "n1", Target: "end"})
	issue := issueOf(t, ChannelGapIssues(g, catalogDefs(), whatsAppAndInstagram, nil), "n1")
	if issue.Code != LintChannelStepUnsupported || issue.Severity != SeverityBlocking {
		t.Fatalf("issue = %+v", issue)
	}
	if issue.Values["channels"] != "Instagram" || !strings.Contains(issue.Message, "Enviar boleto") || strings.Contains(issue.Message, "{{") {
		t.Fatalf("issue = %+v", issue)
	}
}

func TestATemplateBehindTheWhatsAppBranchIsFine(t *testing.T) {
	g := flowOf([]Node{messageTrigger("t"), channelBranch("c"), templateStep("n1"), textStep("n2")},
		Edge{Source: "t", Target: "c"},
		Edge{Source: "c", Target: "n1", Label: "whatsapp"},
		Edge{Source: "c", Target: "n2", Label: ChannelBranchDefault},
		Edge{Source: "n1", Target: "end"}, Edge{Source: "n2", Target: "end"})
	if issues := ChannelGapIssues(g, catalogDefs(), whatsAppAndInstagram, nil); len(issues) != 0 {
		t.Fatalf("issues = %+v", issues)
	}
}

func TestAWorkflowLinkedNowhereHasNoChannelGaps(t *testing.T) {
	g := flowOf([]Node{messageTrigger("t"), templateStep("n1")}, Edge{Source: "t", Target: "n1"}, Edge{Source: "n1", Target: "end"})
	if issues := ChannelGapIssues(g, catalogDefs(), nil, nil); len(issues) != 0 {
		t.Fatalf("issues = %+v", issues)
	}
}

func TestAFormWithItsFailurePathWiredOnlyWarns(t *testing.T) {
	form := Node{ID: "n1", Type: NodeTypeActionSendForm, Config: map[string]interface{}{FormNodeFormIDKey: "f1"}}
	g := flowOf([]Node{messageTrigger("t"), form, textStep("n2")},
		Edge{Source: "t", Target: "n1"},
		Edge{Source: "n1", Target: "end", Label: FormHandleCompleted},
		Edge{Source: "n1", Target: "n2", Label: FormHandleSendFailed},
		Edge{Source: "n2", Target: "end"})
	issue := issueOf(t, ChannelGapIssues(g, catalogDefs(), whatsAppAndInstagram, nil), "n1")
	if issue.Code != LintChannelStepFallsBack || issue.Severity != SeverityAdvisory || issue.Values["handle"] != FormHandleSendFailed {
		t.Fatalf("issue = %+v", issue)
	}
}

func TestARequiredPhoneColumnOnInstagramWarnsThatTheStepMayFail(t *testing.T) {
	g := flowOf([]Node{messageTrigger("t"), sheetRow("n1")}, Edge{Source: "t", Target: "n1"}, Edge{Source: "n1", Target: "end", Label: SheetStepWritten})
	issue := issueOf(t, ChannelGapIssues(g, catalogDefs(), whatsAppAndInstagram, phoneSheet(true)), "n1")
	if issue.Code != LintChannelDataMayFail || issue.Severity != SeverityAdvisory || issue.Values["subject"] != "Telefone" || issue.Path != "telefone" {
		t.Fatalf("issue = %+v", issue)
	}
}

func TestAnOptionalPhoneColumnOnInstagramSaysTheCellMayBeEmpty(t *testing.T) {
	g := flowOf([]Node{messageTrigger("t"), sheetRow("n1")}, Edge{Source: "t", Target: "n1"}, Edge{Source: "n1", Target: "end", Label: SheetStepWritten})
	issue := issueOf(t, ChannelGapIssues(g, catalogDefs(), whatsAppAndInstagram, phoneSheet(false)), "n1")
	if issue.Code != LintChannelDataMayBeEmpty || issue.Severity != SeverityInfo || issue.Values["channels"] != "Instagram" {
		t.Fatalf("issue = %+v", issue)
	}
}

func TestAPhoneColumnOnWhatsAppOnlyIsAlwaysFilled(t *testing.T) {
	g := flowOf([]Node{messageTrigger("t"), sheetRow("n1")}, Edge{Source: "t", Target: "n1"}, Edge{Source: "n1", Target: "end", Label: SheetStepWritten})
	if issues := ChannelGapIssues(g, catalogDefs(), []shared.EntryType{shared.EntryTypeWhatsApp, shared.EntryTypeSMS}, phoneSheet(true)); len(issues) != 0 {
		t.Fatalf("issues = %+v", issues)
	}
}

func runOn(entryType string) *WorkflowRun {
	return &WorkflowRun{ID: "r1", EntryID: "e1", EntryType: entryType}
}

func TestATemplateStepRunOnInstagramFailsWithACode(t *testing.T) {
	g := flowOf([]Node{messageTrigger("t"), templateStep("n1")}, Edge{Source: "t", Target: "n1"}, Edge{Source: "n1", Target: "end"})
	result, refused := OffChannelStep(g, g.FindNode("n1"), runOn("instagram").EntryType)
	if !refused || result.Error == "" || result.Output["error_code"] != CodeChannelUnsupported {
		t.Fatalf("result = %+v", result)
	}
}

func TestAFormRunOnInstagramTakesItsFailurePath(t *testing.T) {
	form := Node{ID: "n1", Type: NodeTypeActionSendForm, Config: map[string]interface{}{FormNodeFormIDKey: "f1"}}
	g := flowOf([]Node{messageTrigger("t"), form, textStep("n2")},
		Edge{Source: "t", Target: "n1"},
		Edge{Source: "n1", Target: "n2", Label: FormHandleSendFailed},
		Edge{Source: "n2", Target: "end"})
	result, refused := OffChannelStep(g, g.FindNode("n1"), "instagram")
	if !refused || result.Error != "" || result.NextNodeID != "n2" || result.ErrorKind != ErrorKindHandled || result.Output["error_code"] != CodeChannelUnsupported {
		t.Fatalf("result = %+v", result)
	}
}

func TestStepsRunWhereTheChannelProvidesWhatTheyNeed(t *testing.T) {
	g := flowOf([]Node{messageTrigger("t"), templateStep("n1"), sheetRow("n2")}, Edge{Source: "t", Target: "n1"}, Edge{Source: "n1", Target: "n2"}, Edge{Source: "n2", Target: "end"})
	for _, entryType := range []string{"whatsapp", EntryTypeLead, ""} {
		if result, refused := OffChannelStep(g, g.FindNode("n1"), entryType); refused {
			t.Fatalf("%q: %+v", entryType, result)
		}
	}
	if result, refused := OffChannelStep(g, g.FindNode("n2"), "instagram"); refused {
		t.Fatalf("data needs are checked by the step itself, got %+v", result)
	}
}
