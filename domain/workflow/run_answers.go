package workflow

func (r *WorkflowRun) AnswersConversation() bool {
	switch r.TriggerType {
	case TriggerMessageReceived, TriggerFirstMessage, TriggerAgentCall:
		return true
	}
	return false
}
