package callsession_usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"vozko/domain/balance"
	"vozko/domain/calls/billing"
	cdr "vozko/domain/calls/cdr"
	"vozko/domain/callsession"
	"vozko/domain/conversation"
	"vozko/domain/messaging"
	"vozko/domain/shared/deps"
)

const (
	reservationLeadDefault  = 10 * time.Second
	reservationRetryDefault = 2 * time.Second
	inflightReservationTTL  = 5 * time.Minute
)

const (
	endReasonInsufficientBalance = "insufficient_balance"
	endReasonBalanceCheckError   = "balance_check_error"
)

type OutboundCallLifecycleInput struct {
	Call        conversation.CRMCall
	Admission   *callsession.CallAdmissionLease
	WorkspaceID string
	StartedAt   time.Time

	OwnerUserID string

	Direction cdr.Direction
	PhoneTo   string

	LeadID         string
	TrunkID        string
	CallListItemID string

	OnStatus func(event conversation.CallEvent)

	OnAudio func(pcm []byte)

	OnEnded func(reason string, duration time.Duration)
}

type OutboundCallLifecycleRunner struct {
	admission            callsession.CallAdmissionCoordinator
	cachedBalanceChecker balance.CachedBalanceChecker
	inflightReserver     balance.InflightReserver
	billingPub           messaging.MessageQueuePub
	cdrStart             cdr.StartCallUseCase
	cdrAnswered          cdr.MarkCallAnsweredUseCase
	cdrComplete          cdr.CompleteCallUseCase
	callListItems        callsession.CallListItems
	billingMinute        time.Duration
	reservationLead      time.Duration
	reservationRetry     time.Duration
	logger               *log.Logger
}

type OutboundCallLifecycleDeps struct {
	Admission     callsession.CallAdmissionCoordinator
	Balance       balance.CachedBalanceChecker
	Inflight      balance.InflightReserver
	BillingPub    messaging.MessageQueuePub
	CDRStart      cdr.StartCallUseCase
	CDRAnswered   cdr.MarkCallAnsweredUseCase
	CDRComplete   cdr.CompleteCallUseCase
	CallListItems callsession.CallListItems
	Logger        *log.Logger
	BillingMinute time.Duration
	Lead          time.Duration
	Retry         time.Duration
}

func NewOutboundCallLifecycleRunner(d OutboundCallLifecycleDeps) (*OutboundCallLifecycleRunner, error) {
	if d.BillingPub == nil {
		return nil, fmt.Errorf("%w: outbound call lifecycle", callsession.ErrBillingNotConfigured)
	}
	if err := deps.Require("outbound call lifecycle", map[string]any{
		"cdr start":       d.CDRStart,
		"cdr answered":    d.CDRAnswered,
		"cdr complete":    d.CDRComplete,
		"call list items": d.CallListItems,
	}); err != nil {
		return nil, err
	}
	if d.Logger == nil {
		d.Logger = log.Default()
	}
	if d.BillingMinute <= 0 {
		d.BillingMinute = billing.BillingMinute
	}
	if d.Lead <= 0 {
		d.Lead = reservationLeadDefault
	}
	if d.Retry <= 0 {
		d.Retry = reservationRetryDefault
	}
	return &OutboundCallLifecycleRunner{
		admission:            d.Admission,
		cachedBalanceChecker: d.Balance,
		inflightReserver:     d.Inflight,
		billingPub:           d.BillingPub,
		cdrStart:             d.CDRStart,
		cdrAnswered:          d.CDRAnswered,
		cdrComplete:          d.CDRComplete,
		callListItems:        d.CallListItems,
		billingMinute:        d.BillingMinute,
		reservationLead:      d.Lead,
		reservationRetry:     d.Retry,
		logger:               d.Logger,
	}, nil
}

func (r *OutboundCallLifecycleRunner) Run(ctx context.Context, input OutboundCallLifecycleInput) {
	if r == nil {
		return
	}

	if input.Call == nil {
		if input.Admission != nil && r.admission != nil {
			if err := r.admission.Release(input.Admission); err != nil {
				r.logger.Printf("[CallLifecycle] admission release error for ws %s (nil call): %v", input.Admission.WorkspaceID, err)
			}
		}
		return
	}

	callRecordID := r.startCDR(input)

	var perMinCost, admittedMicros int64
	if input.Admission != nil {
		perMinCost = input.Admission.PerMinuteCostMicros
		admittedMicros = input.Admission.ReservedMicros
	}
	guard := r.newMinuteGuard(input.WorkspaceID, input.Call.ID(), perMinCost, admittedMicros)
	var answeredAt time.Time
	endReason := string(conversation.CallEventEnded)

	defer func() {
		guard.close()
		r.releaseAdmission(input.Admission, guard.reservedMicros)
		r.publishBilling(input, callRecordID, answeredAt)
		r.completeCDR(input.Call.ID(), answeredAt, endReason)
	}()

	call := input.Call
	audioCh := call.AudioStream()
	events := call.Events()

	sentEnded := false
	emitEnded := func(reason string) {
		if sentEnded {
			return
		}
		sentEnded = true
		endReason = reason
		if input.OnEnded != nil {
			var talk time.Duration
			if !answeredAt.IsZero() {
				talk = time.Now().Sub(answeredAt)
			}
			input.OnEnded(reason, talk)
		}
	}

	for {
		select {
		case <-ctx.Done():
			emitEnded("cancelled")
			return
		case <-call.Done():
			emitEnded(pendingTerminalReason(events))
			return
		case ev, ok := <-events:
			if !ok {
				events = nil
				emitEnded("ended")
				return
			}
			if ev.Type == conversation.CallEventAnswered && answeredAt.IsZero() {
				answeredAt = time.Now()
				r.markAnswered(input.Call.ID())
				guard.answered(answeredAt)
			}
			if ev.IsTerminal() {
				emitEnded(string(ev.Type))
				return
			}
			if input.OnStatus != nil {
				input.OnStatus(ev)
			}
		case pcm, ok := <-audioCh:
			if !ok {
				audioCh = nil
				continue
			}
			if input.OnAudio != nil {
				input.OnAudio(pcm)
			}
		case <-guard.nextC():
			guard.reserveNextMinute()
		case <-guard.stopC():
			emitEnded(guard.failure)
			go func() { _ = call.Hangup() }()
			return
		}
	}
}

func (r *OutboundCallLifecycleRunner) completeCDR(callID string, answeredAt time.Time, reason string) {
	err := r.cdrComplete.Execute(cdr.CompleteCallInput{
		CallID:    callID,
		Status:    cdr.CompletionStatus(!answeredAt.IsZero(), reason),
		EndedAt:   time.Now(),
		EndReason: &reason,
	})
	if err != nil {
		r.logger.Printf("[CallCDR] failed to complete CDR for call %s: %v", callID, err)
	}
}

func (r *OutboundCallLifecycleRunner) releaseAdmission(lease *callsession.CallAdmissionLease, totalReservation int64) {
	if lease == nil {
		return
	}

	lease.ReservedMicros = totalReservation
	if r.admission != nil {
		if err := r.admission.Release(lease); err != nil {
			r.logger.Printf("[CallLifecycle] admission release error for ws %s: %v", lease.WorkspaceID, err)
		}
		return
	}

	if r.inflightReserver != nil && totalReservation > 0 && lease.WorkspaceID != "" {
		_ = r.inflightReserver.Release(lease.WorkspaceID, totalReservation)
	}
}

func (r *OutboundCallLifecycleRunner) startCDR(input OutboundCallLifecycleInput) string {
	if input.Call == nil {
		return ""
	}
	direction := input.Direction
	if direction == "" {
		direction = cdr.DirectionOutbound
	}

	callType := cdr.CallTypeCRM
	phoneFrom, phoneTo := "", input.PhoneTo
	if direction == cdr.DirectionInbound {
		phoneFrom, phoneTo = input.PhoneTo, ""
	}
	var agentIDPtr *string
	if uid := strings.TrimSpace(input.OwnerUserID); uid != "" {
		agentIDPtr = &uid
	}
	rec, err := r.cdrStart.Execute(cdr.StartCallInput{
		CallID:      input.Call.ID(),
		WorkspaceID: input.WorkspaceID,
		Type:        callType,
		Direction:   direction,
		Source:      cdr.SourceForCallID(input.Call.ID()),
		AgentID:     agentIDPtr,
		PhoneFrom:   phoneFrom,
		PhoneTo:     phoneTo,
		LeadID:      optionalID(input.LeadID),
		TrunkID:     optionalID(input.TrunkID),
		StartedAt:   input.StartedAt,
	})
	if err != nil {
		r.logger.Printf("[CallCDR] failed to start CDR for call %s (ws %s): %v", input.Call.ID(), input.WorkspaceID, err)
		return ""
	}
	if rec == nil {
		return ""
	}
	r.stampCallListItem(input, rec.ID)
	return rec.ID
}

const callListStampAttempts = 3

func (r *OutboundCallLifecycleRunner) stampCallListItem(input OutboundCallLifecycleInput, callRecordID string) {
	itemID := strings.TrimSpace(input.CallListItemID)
	if itemID == "" {
		return
	}
	stamp := callsession.CallListItemStamp{WorkspaceID: input.WorkspaceID, UserID: strings.TrimSpace(input.OwnerUserID), ItemID: itemID, CallRecordID: callRecordID}
	var err error
	for attempt := 1; attempt <= callListStampAttempts; attempt++ {
		err = r.callListItems.StampLastCall(context.Background(), stamp)
		if err == nil {
			return
		}
		if errors.Is(err, callsession.ErrCallListStampRefused) {
			r.logger.Printf("[CallCDR] call record %s (call %s) is not stamped on call list item %s (ws %s), the item no longer takes it: %v", callRecordID, input.Call.ID(), itemID, input.WorkspaceID, err)
			return
		}
	}
	r.logger.Printf("[CallCDR] CRITICAL call record %s (call %s) was never stamped on call list item %s (ws %s) after %d attempts: %v", callRecordID, input.Call.ID(), itemID, input.WorkspaceID, callListStampAttempts, err)
}

func optionalID(id string) *string {
	if trimmed := strings.TrimSpace(id); trimmed != "" {
		return &trimmed
	}
	return nil
}

func (r *OutboundCallLifecycleRunner) markAnswered(callID string) {
	if callID == "" {
		return
	}
	if err := r.cdrAnswered.Execute(callID, time.Now()); err != nil {
		r.logger.Printf("[CallCDR] failed to mark answered for call %s: %v", callID, err)
	}
}

func (r *OutboundCallLifecycleRunner) publishBilling(input OutboundCallLifecycleInput, callRecordID string, answeredAt time.Time) {
	if r.billingPub == nil || input.Call == nil {
		return
	}
	if answeredAt.IsZero() {
		r.logger.Printf("[CallBilling] call %s was never answered, nothing to bill", input.Call.ID())
		return
	}
	callEnd := time.Now()
	channel := ""
	if input.Admission != nil {
		channel = input.Admission.CallChannel
	}
	event := billing.CallCompletedEvent{
		CallID:       input.Call.ID(),
		WorkspaceID:  input.WorkspaceID,
		CallSource:   billing.CallSourceWebSocket,
		Channel:      channel,
		CallStart:    answeredAt,
		CallEnd:      callEnd,
		DurationSec:  billing.BillableSeconds(answeredAt, callEnd),
		CallRecordID: callRecordID,
		LeadID:       optionalID(input.LeadID),
	}
	data, err := json.Marshal(event)
	if err != nil {
		r.logger.Printf("[CallBilling] failed to marshal billing event for call %s: %v", event.CallID, err)
		return
	}
	if err := r.billingPub.Publish(billing.TopicCallCompleted, data); err != nil {
		r.logger.Printf("[CallBilling] failed to publish billing event for call %s: %v", event.CallID, err)
		return
	}
	r.logger.Printf("[CallBilling] published billing event for call %s (billable=%ds, channel=%s, workspace=%s)",
		event.CallID, event.DurationSec, channel, input.WorkspaceID)
}

func pendingTerminalReason(events <-chan conversation.CallEvent) string {
	if outcome, ok := conversation.PendingOutcome(events); ok {
		return string(outcome.Type)
	}
	return string(conversation.CallEventEnded)
}
