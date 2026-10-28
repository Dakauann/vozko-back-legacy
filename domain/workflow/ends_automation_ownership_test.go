package workflow

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

var endsAutomationOwnership = map[NodeType]bool{
	NodeTypeEnd:                      true,
	NodeTypeActionFinishConversation: true,
	NodeTypeActionTransferDepartment: true,
	NodeTypeActionAssignMember:       true,
	NodeTypeTransferToQueue:          true,
	NodeTypeActionReplyToAgent:       true,

	NodeTypeTriggerFirstMessage:    false,
	NodeTypeTriggerMessageReceived: false,
	NodeTypeTriggerStageAdded:      false,
	NodeTypeTriggerWebhook:         false,
	NodeTypeTriggerCallReceived:    false,
	NodeTypeTriggerFormSubmitted:   false,
	NodeTypeTriggerAgentCall:       false,
	NodeTypeTriggerBirthday:        false,

	NodeTypeActionPlayAudio: false,
	NodeTypeWaitDTMF:        false,

	NodeTypeActionSendText:                  false,
	NodeTypeActionSendTemplate:              false,
	NodeTypeActionSendEmail:                 false,
	NodeTypeActionSendCarrier:               false,
	NodeTypeActionSendPlatformEmail:         false,
	NodeTypeActionSendInteractive:           false,
	NodeTypeActionSendForm:                  false,
	NodeTypeActionSendMedia:                 false,
	NodeTypeActionAIAgent:                   false,
	NodeTypeActionAIExtract:                 false,
	NodeTypeActionSetVariable:               false,
	NodeTypeActionHTTPRequest:               false,
	NodeTypeActionRunTool:                   false,
	NodeTypeActionFormatDate:                false,
	NodeTypeActionCode:                      false,
	NodeTypeActionRunWorkflow:               false,
	NodeTypeActionLoop:                      false,
	NodeTypeActionGetCurrentTime:            false,
	NodeTypeActionCreateCalendarEvent:       false,
	NodeTypeActionRescheduleCalendarEvent:   false,
	NodeTypeActionCheckCalendarAvailability: false,
	NodeTypeActionAddSheetRow:               false,
	NodeTypeActionExportMessagesSheet:       false,
	NodeTypeActionAssignLabel:               false,
	NodeTypeActionMoveStage:                 false,
	NodeTypeActionManageOpportunity:         false,
	NodeTypeActionUpdateLead:                false,
	NodeTypeActionConsent:                   false,
	NodeTypeActionScheduleMessage:           false,
	NodeTypeActionCancelScheduledMessages:   false,

	NodeTypeWaitDuration:              false,
	NodeTypeWaitForReply:              false,
	NodeTypeWaitSchedule:              false,
	NodeTypeConditionBranch:           false,
	NodeTypeConditionAIClassfy:        false,
	NodeTypeConditionTextMatch:        false,
	NodeTypeConditionFilter:           false,
	NodeTypeConditionCheckLabel:       false,
	NodeTypeConditionCheckStage:       false,
	NodeTypeConditionCheckOpportunity: false,
	NodeTypeConditionChannel:          false,
	NodeTypeConditionConsent:          false,

	NodeTypeDecorationBackground: false,

	NodeTypeActionSendWhatsappButtonLegacy: false,
	NodeTypeActionScheduleMeetingLegacy:    false,
	NodeTypeActionRescheduleMeetingLegacy:  false,
}

func declaredNodeTypes(t *testing.T) []NodeType {
	t.Helper()
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	var types []NodeType
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		for _, decl := range file.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.CONST {
				continue
			}
			for _, spec := range gen.Specs {
				value := spec.(*ast.ValueSpec)
				typed, ok := value.Type.(*ast.Ident)
				if !ok || typed.Name != "NodeType" {
					continue
				}
				for _, literal := range value.Values {
					basic, ok := literal.(*ast.BasicLit)
					if !ok || basic.Kind != token.STRING {
						continue
					}
					raw, err := strconv.Unquote(basic.Value)
					if err != nil {
						t.Fatal(err)
					}
					types = append(types, NodeType(raw))
				}
			}
		}
	}
	return types
}

func TestEveryNodeTypeDecidesWhetherItEndsTheAutomationsOwnership(t *testing.T) {
	declared := declaredNodeTypes(t)
	if len(declared) < len(endsAutomationOwnership) {
		t.Fatalf("found %d declared node types, the table lists %d", len(declared), len(endsAutomationOwnership))
	}
	for _, nodeType := range declared {
		want, decided := endsAutomationOwnership[nodeType]
		if !decided {
			t.Errorf("node type %q must decide whether a tool path ending on it ends the run: add it to the table", nodeType)
			continue
		}
		if got := nodeType.EndsAutomationOwnership(); got != want {
			t.Errorf("%s.EndsAutomationOwnership() = %v, want %v", nodeType, got, want)
		}
	}
}
