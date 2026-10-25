package container

import (
	"context"
	"log"

	"vozko/domain/cache"
	conversation_domain "vozko/domain/conversation"
	"vozko/domain/leadexit"
	"vozko/domain/shared/deps"
	workflow_domain "vozko/domain/workflow"
	conversation_usecase "vozko/usecases/conversation"
)

func (c *Container) agentTurnLock() *conversation_usecase.AgentTurnLock {
	if c.turnLock == nil {
		sharedState := c.redisProvider.SharedState()
		if _, guarded := sharedState.(cache.OwnedKeyDeleter); !guarded {
			log.Fatalf("[agent-turns] the shared state cannot release a turn lock only for its holder")
		}
		c.turnLock = conversation_usecase.NewAgentTurnLock(sharedState)
	}
	return c.turnLock
}

func (c *Container) inboundTriggersHandle() *deps.Handle[conversation_usecase.InboundTriggers] {
	if c.inboundTriggers == nil {
		c.inboundTriggers = deps.Tracked[conversation_usecase.InboundTriggers](&c.handles, "official inbound workflow triggers")
	}
	return c.inboundTriggers
}

type lateInboundTriggers struct {
	handle *deps.Handle[conversation_usecase.InboundTriggers]
}

func (t lateInboundTriggers) Evaluate(event workflow_domain.TriggerEvent) {
	triggers, err := t.handle.Get()
	if err != nil {
		log.Printf("[official-inbound] workflow triggers for entry %s not evaluated: %v", event.EntryID, err)
		return
	}
	triggers.Evaluate(event)
}

func (t lateInboundTriggers) WaitingForReply(event workflow_domain.TriggerEvent) (*workflow_domain.WorkflowRun, error) {
	triggers, err := t.handle.Get()
	if err != nil {
		return nil, err
	}
	return triggers.WaitingForReply(event)
}

func (t lateInboundTriggers) WakeForReply(run *workflow_domain.WorkflowRun, event workflow_domain.TriggerEvent) bool {
	triggers, err := t.handle.Get()
	if err != nil {
		log.Printf("[official-inbound] run %s not woken for entry %s: %v", run.ID, event.EntryID, err)
		return false
	}
	return triggers.WakeForReply(run, event)
}

func (c *Container) leadExitHandle() *deps.Handle[leadexit.Step] {
	if c.leadExitSteps == nil {
		c.leadExitSteps = deps.Tracked[leadexit.Step](&c.handles, "official inbound lead exit step")
	}
	return c.leadExitSteps
}

type lateLeadExit struct{ handle *deps.Handle[leadexit.Step] }

func (l lateLeadExit) Run(ctx context.Context, in leadexit.Inbound, replier leadexit.Replier) (leadexit.Result, error) {
	step, err := l.handle.Get()
	if err != nil {
		return leadexit.Result{}, err
	}
	return step.Run(ctx, in, replier)
}

func (c *Container) formRepliesHandle() *deps.Handle[conversation_usecase.FormReplyReceiver] {
	if c.formReplyReceivers == nil {
		c.formReplyReceivers = deps.Tracked[conversation_usecase.FormReplyReceiver](&c.handles, "official inbound form replies")
	}
	return c.formReplyReceivers
}

type lateFormReplies struct {
	handle *deps.Handle[conversation_usecase.FormReplyReceiver]
}

func (f lateFormReplies) ReceiveFormReply(ctx context.Context, in conversation_usecase.InboundFormReply) (conversation_usecase.ReceivedFormReply, error) {
	receiver, err := f.handle.Get()
	if err != nil {
		return conversation_usecase.ReceivedFormReply{}, err
	}
	return receiver.ReceiveFormReply(ctx, in)
}

func (c *Container) inboundCallsHandle() *deps.Handle[conversation_domain.WhatsAppInboundCallHandler] {
	if c.inboundCalls == nil {
		c.inboundCalls = deps.Tracked[conversation_domain.WhatsAppInboundCallHandler](&c.handles, "whatsapp inbound calls")
	}
	return c.inboundCalls
}

type lateInboundCalls struct {
	handle *deps.Handle[conversation_domain.WhatsAppInboundCallHandler]
}

func (l lateInboundCalls) HandleInboundConnect(connect conversation_domain.WhatsAppInboundConnect) {
	inbound, err := l.handle.Get()
	if err != nil {
		log.Printf("[whatsapp-calls] inbound call %s refused: %v", connect.CallID, err)
		return
	}
	inbound.HandleInboundConnect(connect)
}
