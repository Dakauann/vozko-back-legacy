package conversation_usecase

import (
	"context"
	"errors"

	aa "vozko/domain/ai_attendance"
	"vozko/domain/conversation"
	"vozko/domain/lead"
	"vozko/domain/leadexit"
	"vozko/domain/shared"
	wc "vozko/domain/whatsapp_campaign"
	"vozko/domain/workflow"
	"vozko/usecases/agentturn"
	"vozko/usecases/conversation/loopguard"
)

type channelAIQuietAttendance struct{}

func (channelAIQuietAttendance) RecordAIReply(aa.StartInput, string) {}

type channelAINoLeads struct{}

func (channelAINoLeads) LeadOfEntry(context.Context, string, shared.EntryRef) (string, error) {
	return "", lead.ErrLeadNotFound
}

func (channelAINoLeads) FindByID(string, string) (*lead.Lead, error) {
	return nil, lead.ErrLeadNotFound
}

type channelAIEveryValue struct{}

func (channelAIEveryValue) AgentValues(context.Context, string, string, shared.EntryType, string) (map[string]string, bool) {
	return map[string]string{}, true
}

type channelAINoNames struct{}

func (channelAINoNames) ResolveNames(...string) (map[string]string, error) { return nil, nil }

func channelAIWith(d ChannelAIReplyDeps) *ChannelAIReplyService {
	if d.Assembler == nil {
		d.Assembler = agentturn.New(nil, nil, nil, nil)
	}
	if d.TurnLock == nil {
		d.TurnLock = NewAgentTurnLock(newLockState())
	}
	if d.Attendance == nil {
		d.Attendance = channelAIQuietAttendance{}
	}
	if d.Leads == nil {
		d.Leads = channelAINoLeads{}
	}
	if d.WorkflowNames == nil {
		d.WorkflowNames = channelAINoNames{}
	}
	if d.Variables == nil {
		d.Variables = channelAIEveryValue{}
	}
	return NewChannelAIReplyService(d)
}

func channelAIDefaults(s *ChannelAIReplyService) *ChannelAIReplyService {
	if s.assembler == nil {
		s.assembler = agentturn.New(nil, nil, nil, nil)
	}
	if s.turnLock == nil {
		s.turnLock = NewAgentTurnLock(newLockState())
	}
	if s.aiAttendance == nil {
		s.aiAttendance = channelAIQuietAttendance{}
	}
	if s.entryLeads == nil {
		s.entryLeads = channelAINoLeads{}
	}
	if s.workflowNames == nil {
		s.workflowNames = channelAINoNames{}
	}
	if s.variables == nil {
		s.variables = channelAIEveryValue{}
	}
	return s
}

type whatsappNoAssignment struct{}

func (whatsappNoAssignment) EnsureAssignment(string, string, string) string { return "" }

type whatsappQuietTriggers struct{}

func (whatsappQuietTriggers) Evaluate(workflow.TriggerEvent) {}
func (whatsappQuietTriggers) WaitingForReply(workflow.TriggerEvent) (*workflow.WorkflowRun, error) {
	return nil, nil
}
func (whatsappQuietTriggers) WakeForReply(*workflow.WorkflowRun, workflow.TriggerEvent) bool {
	return false
}

type whatsappNoDelegations struct {
	conversation.DelegationRepository
}

func (whatsappNoDelegations) Find(context.Context, string, shared.EntryType) (*conversation.Delegation, error) {
	return nil, nil
}

type whatsappNoReceptive struct{}

func (whatsappNoReceptive) Execute(string, string, string) (*wc.Campaign, bool, error) {
	return nil, false, errors.New("no receptive container in this test")
}

type whatsappLiveOff struct{}

func (whatsappLiveOff) Acts(context.Context, string) bool { return false }

type whatsappNoForms struct{}

func (whatsappNoForms) ReceiveFormReply(context.Context, InboundFormReply) (ReceivedFormReply, error) {
	return ReceivedFormReply{}, errors.New("no form receiver in this test")
}

type whatsappNoExit struct{}

func (whatsappNoExit) Run(context.Context, leadexit.Inbound, leadexit.Replier) (leadexit.Result, error) {
	return leadexit.Result{}, nil
}

func whatsappDefaults(uc *handleWhatsAppMessageUseCase) *handleWhatsAppMessageUseCase {
	if uc.assignmentService == nil {
		uc.assignmentService = whatsappNoAssignment{}
	}
	if uc.aiAttendance == nil {
		uc.aiAttendance = channelAIQuietAttendance{}
	}
	if uc.triggerEvaluator == nil {
		uc.triggerEvaluator = whatsappQuietTriggers{}
	}
	if uc.inboundRouter == nil {
		uc.inboundRouter = InboundRouterOf(uc.triggerEvaluator)
	}
	if uc.turnLock == nil {
		uc.turnLock = NewAgentTurnLock(newLockState())
	}
	if uc.automationGate == nil {
		uc.automationGate = gateWith(ownedBy(""), whatsappNoDelegations{})
	}
	if uc.receptive == nil {
		uc.receptive = whatsappNoReceptive{}
	}
	if uc.loopGuard == nil {
		uc.loopGuard = loopguard.AlwaysAllow{}
	}
	if uc.live == nil {
		uc.live = whatsappLiveOff{}
	}
	if uc.agentVariables == nil {
		uc.agentVariables = channelAIEveryValue{}
	}
	if uc.turnAssembler == nil {
		uc.turnAssembler = agentturn.New(uc.toolRegistry, uc.ragService, nil, nil)
	}
	if uc.workflowNames == nil {
		uc.workflowNames = channelAINoNames{}
	}
	if uc.formReplies == nil {
		uc.formReplies = whatsappNoForms{}
	}
	if uc.exitStep == nil {
		uc.exitStep = whatsappNoExit{}
	}
	return uc
}
