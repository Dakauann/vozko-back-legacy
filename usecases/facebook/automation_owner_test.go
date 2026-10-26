package facebook

import (
	"context"
	"testing"

	"vozko/domain/conversation"
	"vozko/usecases/conversation/automationgatetest"
)

func TestOnFacebookOwnershipDecidesWhetherAutomationAnswers(t *testing.T) {
	for _, tc := range automationgatetest.Cases(conversation.Automation{Kind: conversation.AutomationAgent, ID: "agent-1"}) {
		t.Run(tc.Name, func(t *testing.T) {
			f := newGatedMessagingFixture(tc.Gate)

			if err := f.uc.Execute(context.Background(), entry(inbound("m1", "oi"))); err != nil {
				t.Fatal(err)
			}

			if answered := len(f.agents.reqs) == 1; answered != tc.Answers {
				t.Fatalf("agent replies = %d, want answered %v", len(f.agents.reqs), tc.Answers)
			}
			if !tc.Answers && len(f.workflows.events) != 0 {
				t.Fatalf("workflow events = %d on a conversation automation may not touch", len(f.workflows.events))
			}
			if len(f.history.records) != 1 {
				t.Fatalf("records = %d, the message must still be recorded", len(f.history.records))
			}
		})
	}
}
