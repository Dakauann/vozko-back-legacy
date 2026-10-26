package instagram

import (
	"context"
	"testing"
	"time"

	"vozko/domain/conversation"
	igdomain "vozko/domain/instagram"
	"vozko/usecases/conversation/automationgatetest"
)

func TestOnInstagramOwnershipDecidesWhetherAutomationAnswers(t *testing.T) {
	agent, flow := "agent-1", "wf-1"
	account := &igdomain.Account{ID: "acc-1", WorkspaceID: "ws-1", AgentID: &agent, EnableAgentResponses: true}
	flowAccount := &igdomain.Account{ID: "acc-1", WorkspaceID: "ws-1", WorkflowID: &flow, EnableWorkflow: true}
	conv := &igdomain.Conversation{ID: "conv-1"}

	for _, tc := range automationgatetest.Cases(conversation.Automation{Kind: conversation.AutomationAgent, ID: agent}) {
		t.Run(tc.Name+": agent", func(t *testing.T) {
			replies, triggers := &automationgatetest.Replies{}, &automationgatetest.Triggers{}
			uc := NewHandleWebhookUseCase(HandleWebhookDeps{AutomationGate: tc.Gate, AIReply: replies, Workflows: triggers})

			if err := uc.automation.Dispatch(context.Background(), automationInput(account, nil, conv, "oi", "mid-1", time.Now(), nil)); err != nil {
				t.Fatal(err)
			}

			if answered := replies.Count() == 1; answered != tc.Answers {
				t.Fatalf("agent replies = %d, want answered %v", replies.Count(), tc.Answers)
			}
		})
	}
	for _, tc := range automationgatetest.Cases(conversation.Automation{Kind: conversation.AutomationWorkflow, ID: flow}) {
		t.Run(tc.Name+": workflow", func(t *testing.T) {
			replies, triggers := &automationgatetest.Replies{}, &automationgatetest.Triggers{}
			uc := NewHandleWebhookUseCase(HandleWebhookDeps{AutomationGate: tc.Gate, AIReply: replies, Workflows: triggers})

			if err := uc.automation.Dispatch(context.Background(), automationInput(flowAccount, nil, conv, "oi", "mid-1", time.Now(), nil)); err != nil {
				t.Fatal(err)
			}

			if triggered := triggers.Count() > 0; triggered != tc.Answers {
				t.Fatalf("workflow events = %d, want triggered %v", triggers.Count(), tc.Answers)
			}
		})
	}
}
