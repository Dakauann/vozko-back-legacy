package workflow

import "testing"

func TestOnlyARunStartedByTheConversationsOwnAutomationAnswersIt(t *testing.T) {
	cases := map[TriggerType]bool{
		TriggerMessageReceived: true,
		TriggerFirstMessage:    true,
		TriggerAgentCall:       true,
		TriggerStageAdded:      false,
		TriggerFormSubmitted:   false,
		TriggerWebhook:         false,
		TriggerBirthday:        false,
		TriggerCallReceived:    false,
		"":                     false,
	}
	for trigger, want := range cases {
		if got := (&WorkflowRun{TriggerType: trigger}).AnswersConversation(); got != want {
			t.Errorf("a run started by %q answers the conversation = %v, want %v", trigger, got, want)
		}
	}
}
