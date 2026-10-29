package automationneeds

import (
	"reflect"
	"testing"

	"vozko/domain/agent"
	"vozko/domain/conversation"
	"vozko/domain/workflow"
)

func TestDeclarationUsersNamesEveryAutomationThatUsesEachNameOnce(t *testing.T) {
	cobrador := &agent.Agent{ID: "a1", Name: "Cobrador", DepartmentID: "d1", MessagingPrompt: "Valor {{valor}} de {{lead_name}}",
		Variables:     []agent.AgentVariable{{Name: "turno", DefaultValue: "Manhã"}},
		InternalTools: []agent.ToolBinding{{Name: workflow.AgentSheetRowTool, Config: map[string]interface{}{workflow.SheetStepVariableKey: "{{campvars.planilha}}"}}}}
	published := &agent.Agent{ID: "a1", Name: "Cobrador", DepartmentID: "d1", MessagingPrompt: "Antigo {{curso}}"}
	flow := &workflow.Workflow{ID: "w1", Name: "Matrículas", Graph: workflow.Graph{
		Nodes:     []workflow.Node{{ID: "n", Type: workflow.NodeTypeActionSendText, Config: map[string]interface{}{"text": "Oi {{campvars.valor}}"}}},
		Variables: []workflow.VariableUse{{Name: "reserva"}},
	}}
	users := DeclarationUsers([]*agent.Agent{cobrador, published}, []*workflow.Workflow{flow, flow})
	agentUser := DeclarationUser{Link: Link{Automation: conversation.Automation{Kind: conversation.AutomationAgent, ID: "a1"}, Name: "Cobrador"}, DepartmentID: "d1"}
	flowUser := DeclarationUser{Link: Link{Automation: conversation.Automation{Kind: conversation.AutomationWorkflow, ID: "w1"}, Name: "Matrículas"}}
	want := map[string][]DeclarationUser{
		"valor":    {agentUser, flowUser},
		"turno":    {agentUser},
		"planilha": {agentUser},
		"curso":    {agentUser},
		"reserva":  {flowUser},
	}
	if !reflect.DeepEqual(users, want) {
		t.Fatalf("users = %+v", users)
	}
}
