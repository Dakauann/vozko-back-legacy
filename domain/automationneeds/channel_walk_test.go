package automationneeds

import (
	"errors"
	"testing"

	"vozko/domain/agent"
	"vozko/domain/shared"
	"vozko/domain/workflow"
)

func flow(id, name string, list []workflow.Node, edges ...workflow.Edge) *workflow.Workflow {
	return &workflow.Workflow{ID: id, WorkspaceID: "ws", Name: name, Graph: workflow.Graph{Nodes: list, Edges: edges}}
}

func messageStart() workflow.Node {
	return workflow.Node{ID: "t", Type: workflow.NodeTypeTriggerMessageReceived, Config: map[string]interface{}{}}
}

func agentCallStart() workflow.Node {
	return workflow.Node{ID: "t", Type: workflow.NodeTypeTriggerAgentCall, Config: map[string]interface{}{}}
}

func boletoTemplate() workflow.Node {
	return workflow.Node{ID: "tpl", Type: workflow.NodeTypeActionSendTemplate, Config: map[string]interface{}{"template_id": "boleto", "display_name": "Enviar boleto"}}
}

func branchByChannel() workflow.Node {
	return workflow.Node{ID: "c", Type: workflow.NodeTypeConditionChannel, Config: map[string]interface{}{}}
}

func channelWorld() fakeSource {
	return fakeSource{
		agents: map[string]*agent.Agent{
			"bia": {ID: "bia", WorkspaceID: "ws", Name: "Bia", MessagingPrompt: "Oi", InternalTools: []agent.ToolBinding{{Name: workflow.AgentRunWorkflowTool, Config: map[string]interface{}{"workflow_id": "cobranca"}}}},
		},
		workflows: map[string]*workflow.Workflow{
			"cobranca": flow("cobranca", "Cobrança", []workflow.Node{agentCallStart(), boletoTemplate()}, workflow.Edge{Source: "t", Target: "tpl"}),
			"triagem": flow("triagem", "Triagem", []workflow.Node{messageStart(), branchByChannel(), subFlowNode("sub", "cobranca"), textNode("txt", "oi")},
				workflow.Edge{Source: "t", Target: "c"},
				workflow.Edge{Source: "c", Target: "sub", Label: "whatsapp"},
				workflow.Edge{Source: "c", Target: "txt", Label: workflow.ChannelBranchDefault}),
			"direto": flow("direto", "Direto", []workflow.Node{messageStart(), boletoTemplate()}, workflow.Edge{Source: "t", Target: "tpl"}),
		},
	}
}

func instagram() []shared.EntryType { return []shared.EntryType{shared.EntryTypeInstagram} }

func TestAWorkflowWithATemplateCannotRunOnInstagram(t *testing.T) {
	walk, err := ChannelWalkOf(channelWorld(), "ws", workflowRef("direto"), instagram())
	if err != nil {
		t.Fatal(err)
	}
	if len(walk.Blocks) != 1 {
		t.Fatalf("blocks = %+v", walk.Blocks)
	}
	block := walk.Blocks[0]
	if block.Automation.Name != "Direto" || block.Step != "Enviar boleto" || block.Fact != workflow.ChannelFactTemplates || len(block.Channels) != 1 || block.Channels[0] != shared.EntryTypeInstagram {
		t.Fatalf("block = %+v", block)
	}
}

func TestAnAgentOnInstagramCannotCallAWorkflowThatSendsATemplate(t *testing.T) {
	walk, err := ChannelWalkOf(channelWorld(), "ws", agentRef("bia"), instagram())
	if err != nil {
		t.Fatal(err)
	}
	if len(walk.Blocks) != 1 || walk.Blocks[0].Automation.ID != "cobranca" {
		t.Fatalf("blocks = %+v", walk.Blocks)
	}
	if got := walk.Reached[workflowRef("cobranca")]; len(got) != 1 || got[0] != shared.EntryTypeInstagram {
		t.Fatalf("reached = %v", walk.Reached)
	}
}

func TestASubWorkflowBehindTheWhatsAppBranchIsNeverReachedFromInstagram(t *testing.T) {
	walk, err := ChannelWalkOf(channelWorld(), "ws", workflowRef("triagem"), instagram())
	if err != nil {
		t.Fatal(err)
	}
	if len(walk.Blocks) != 0 {
		t.Fatalf("blocks = %+v", walk.Blocks)
	}
	if _, reached := walk.Reached[workflowRef("cobranca")]; reached {
		t.Fatalf("reached = %v", walk.Reached)
	}
}

func TestTheOfficialNumberRunsTheTemplate(t *testing.T) {
	walk, err := ChannelWalkOf(channelWorld(), "ws", workflowRef("triagem"), []shared.EntryType{shared.EntryTypeWhatsApp})
	if err != nil || len(walk.Blocks) != 0 {
		t.Fatalf("walk = %+v, %v", walk, err)
	}
	if got := walk.Reached[workflowRef("cobranca")]; len(got) != 1 || got[0] != shared.EntryTypeWhatsApp {
		t.Fatalf("reached = %v", walk.Reached)
	}
}

func TestAnUnreadableAutomationFailsTheWalk(t *testing.T) {
	src := channelWorld()
	src.failing = map[string]bool{"cobranca": true}
	if _, err := ChannelWalkOf(src, "ws", agentRef("bia"), instagram()); !errors.Is(err, ErrUnreadable) {
		t.Fatalf("err = %v", err)
	}
}
