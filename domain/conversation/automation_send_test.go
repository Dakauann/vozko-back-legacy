package conversation

import "testing"

func TestAnAutomationAuthorBecomesItsSender(t *testing.T) {
	agent, ok := SentByAutomation("ai:agent-1")
	if !ok || agent.Kind() != SenderAI || agent.Participant() != "agent-1" {
		t.Fatalf("agent = %+v %v", agent, ok)
	}
	flow, ok := SentByAutomation("workflow:wf-1")
	if !ok || flow.Kind() != SenderWorkflow || flow.Participant() != "wf-1" {
		t.Fatalf("workflow = %+v %v", flow, ok)
	}
	for _, id := range []string{"", "ai:", "workflow:", "user-1", "campaign:c-1"} {
		if _, ok := SentByAutomation(id); ok {
			t.Fatalf("%q must not be an automation author", id)
		}
	}
}

func TestASendOriginSpeaksForItsAuthor(t *testing.T) {
	person := SendOrigin{SenderUserID: "user-1"}
	if !person.ByPerson() || person.MessageType() != MessageTypeOperator || person.Author().Participant() != "user-1" {
		t.Fatalf("person origin = %+v", person)
	}
	agent, _ := SentByAutomation("ai:agent-1")
	automated := SendOrigin{Automation: agent}
	if automated.ByPerson() || automated.MessageType() != MessageTypeAIResponse || automated.Author().Kind() != SenderAI {
		t.Fatalf("automation origin = %+v", automated)
	}
}
