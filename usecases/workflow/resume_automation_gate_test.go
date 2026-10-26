package workflow_usecase

import (
	"errors"
	"strings"
	"testing"
	"time"

	"vozko/domain/workflow"
)

type gateStub struct {
	enabled       bool
	mayAct        bool
	err           error
	asked         int
	askedOwner    int
	lastID        string
	lastType      string
	lastWorkspace string
}

func (g *gateStub) AutomationEnabled(entryID, entryType string) (bool, error) {
	g.asked++
	g.lastID, g.lastType = entryID, entryType
	return g.enabled, g.err
}

func (g *gateStub) AutomationMayAct(workspaceID, entryID, entryType string) (bool, error) {
	g.askedOwner++
	g.lastWorkspace, g.lastID, g.lastType = workspaceID, entryID, entryType
	return g.mayAct, g.err
}

type runRepoStub struct {
	workflow.WorkflowRunRepository
	updated *workflow.WorkflowRun
	updates int
}

func (r *runRepoStub) Update(run *workflow.WorkflowRun) error {
	r.updated = run
	r.updates++
	return nil
}

type workflowRepoStub struct {
	workflow.WorkflowRepository
	t       *testing.T
	fetched bool
}

func (w *workflowRepoStub) FindByID(string) (*workflow.Workflow, error) {
	w.fetched = true
	w.t.Error("the workflow graph was loaded for a run that should have been cancelled")
	return nil, nil
}

func parkedRun() *workflow.WorkflowRun {
	return &workflow.WorkflowRun{
		ID:            "run-1",
		WorkflowID:    "wf-1",
		EntryID:       "entry-1",
		EntryType:     "whatsapp",
		Status:        workflow.RunStatusWaiting,
		CurrentNodeID: "node-7",
	}
}

func TestResumeCancelsWhenAutomationWasTurnedOff(t *testing.T) {
	gate := &gateStub{enabled: false}
	engine := testEngine(nil, nil, nil)
	engine.automationGate = gate

	runs := &runRepoStub{}
	wfs := &workflowRepoStub{t: t}
	run := parkedRun()

	if err := resumeRunFromCurrent(wfs, runs, engine, run); err != nil {
		t.Fatalf("cancelling is a normal outcome, not an error: %v", err)
	}

	if run.Status != workflow.RunStatusCancelled {
		t.Errorf("status = %q, want cancelled, since an error would retry and eventually deliver the very message the operator silenced", run.Status)
	}
	if runs.updates != 1 || runs.updated != run {
		t.Errorf("the cancellation must be persisted once, got %d update(s)", runs.updates)
	}
	if wfs.fetched {
		t.Error("no work may happen after the gate says no")
	}
	if gate.asked != 1 {
		t.Errorf("gate consulted %d times, want exactly 1", gate.asked)
	}
	if gate.lastID != "entry-1" || gate.lastType != "whatsapp" {
		t.Errorf("gate asked about %q/%q, want the run's own entry", gate.lastID, gate.lastType)
	}
}

func TestResumeProceedsWhenAutomationIsOn(t *testing.T) {
	engine := testEngine(nil, nil, nil)
	engine.automationGate = &gateStub{enabled: true}

	run := parkedRun()
	runs := &runRepoStub{}
	wfs := &passthroughWorkflowRepo{}

	_ = resumeRunFromCurrent(wfs, runs, engine, run)

	if !wfs.fetched {
		t.Error("the resume stopped at the gate even though automation was on")
	}
	if run.Status == workflow.RunStatusCancelled {
		t.Error("a run was cancelled while automation was on")
	}
}

type passthroughWorkflowRepo struct {
	workflow.WorkflowRepository
	fetched bool
}

func (w *passthroughWorkflowRepo) FindByID(string) (*workflow.Workflow, error) {
	w.fetched = true
	return nil, nil
}

func TestAutomationOffAllowsOnlyWhenThereIsNothingToCheck(t *testing.T) {
	engine := testEngine(nil, nil, nil)
	engine.automationGate = SandboxAutomationGate{}
	if off, err := engine.automationOff(parkedRun()); off || err != nil {
		t.Error("the sandbox gate is the explicit always-on answer for simulations")
	}
	answering := parkedRun()
	answering.TriggerType = workflow.TriggerMessageReceived
	if off, err := engine.automationOff(answering); off || err != nil {
		t.Error("the sandbox gate lets an answering run act too")
	}

	engine.automationGate = &gateStub{enabled: false}
	unattached := parkedRun()
	unattached.EntryID = ""
	if off, err := engine.automationOff(unattached); off || err != nil {
		t.Error("an empty entry id has nothing to check and must be allowed")
	}
	if off, err := engine.automationOff(parkedRun()); !off || err != nil {
		t.Error("an explicit disabled answer must stop the run")
	}

	engine.automationGate = &gateStub{enabled: true, err: errors.New("db down")}
	off, err := engine.automationOff(parkedRun())
	if !errors.Is(err, workflow.ErrAutomationStateUnreadable) {
		t.Errorf("err = %v, want ErrAutomationStateUnreadable", err)
	}
	if off {
		t.Error("an unreadable gate is an error, not an answer")
	}
}

func timeoutParkedRun() *workflow.WorkflowRun {
	run := parkedRun()
	wakeAt := time.Now().Add(-time.Minute).UnixMilli()
	run.WakeAt = &wakeAt
	run.WaitReason = workflow.WaitReasonReply
	run.WorkspaceID = "ws-1"
	return run
}

func TestResumeRetriesLaterWhenTheGateCannotBeReadAndRunsNothing(t *testing.T) {
	engine := testEngine(nil, nil, nil)
	engine.automationGate = &gateStub{enabled: true, err: errors.New("db down")}

	runs := &runRepoStub{}
	wfs := &workflowRepoStub{t: t}
	run := timeoutParkedRun()
	before := time.Now().UnixMilli()

	if err := resumeRunFromCurrent(wfs, runs, engine, run); err != nil {
		t.Fatalf("a deferred retry is a normal outcome, not an error: %v", err)
	}
	if wfs.fetched {
		t.Error("no step may run while automation cannot be read")
	}
	if run.Status != workflow.RunStatusWaiting || run.State.GetInt(workflow.StateKeyGateAttempts) != 1 || run.RetryCount != 0 {
		t.Fatalf("status=%q gate attempts=%d node retries=%d, want waiting with one gate attempt and the node retry budget untouched", run.Status, run.State.GetInt(workflow.StateKeyGateAttempts), run.RetryCount)
	}
	if run.WaitReason != workflow.WaitReasonReply {
		t.Errorf("wait reason = %q, the original wait must be kept so the retry resumes it the same way", run.WaitReason)
	}
	if run.WakeAt == nil || *run.WakeAt <= before {
		t.Error("the retry must be scheduled in the future with backoff")
	}
	if runs.updates != 1 {
		t.Errorf("the deferred retry must be persisted once, got %d", runs.updates)
	}
}

func TestResumeFailsTheRunWhenTheGateStaysUnreadableAfterTheRetries(t *testing.T) {
	engine := testEngine(nil, nil, nil)
	engine.automationGate = &gateStub{enabled: true, err: errors.New("db down")}

	runs := &runRepoStub{}
	run := timeoutParkedRun()
	run.State.Set(workflow.StateKeyGateAttempts, workflow.MaxRetriesPerNode)

	if err := resumeRunFromCurrent(&workflowRepoStub{t: t}, runs, engine, run); err != nil {
		t.Fatalf("failing the run is a normal outcome: %v", err)
	}
	if run.Status != workflow.RunStatusError {
		t.Fatalf("status = %q, want error", run.Status)
	}
	if !strings.Contains(run.Error, workflow.ErrAutomationStateUnreadable.Error()) || !strings.Contains(run.Error, "db down") {
		t.Errorf("error %q must say the automation state was unreadable and why", run.Error)
	}
}

func TestFormResumeRefusesWhenTheGateCannotBeReadSoTheAnswerIsRetried(t *testing.T) {
	engine := testEngine(nil, nil, nil)
	engine.automationGate = &gateStub{enabled: true, err: errors.New("db down")}
	runs := &runRepoStub{}
	run := parkedRun()

	stop, err := cancelWhenAutomationOff(runs, engine, run)
	if !stop || !errors.Is(err, workflow.ErrAutomationStateUnreadable) {
		t.Fatalf("stop=%v err=%v, want a stop with ErrAutomationStateUnreadable", stop, err)
	}
	if runs.updates != 0 || run.Status != workflow.RunStatusWaiting {
		t.Errorf("the run must be left as it was, got status %q after %d updates", run.Status, runs.updates)
	}
}

func TestAReadableGateClearsEarlierGateAttemptsSoALaterOutageGetsAFullBudget(t *testing.T) {
	engine := testEngine(nil, nil, nil)
	engine.automationGate = &gateStub{enabled: true}
	run := timeoutParkedRun()
	run.State.Set(workflow.StateKeyGateAttempts, 2)

	if stop, err := cancelWhenAutomationOff(&runRepoStub{}, engine, run); stop || err != nil {
		t.Fatalf("stop=%v err=%v, want the run to proceed", stop, err)
	}
	if _, ok := run.State.Get(workflow.StateKeyGateAttempts); ok {
		t.Error("the gate attempt counter must be cleared once the gate answers")
	}
}

func TestALeadRunHasNoConversationPauseSwitchToRead(t *testing.T) {
	gate := &gateStub{err: errors.New("lead is not a conversation entry")}
	engine := testEngine(nil, nil, nil)
	engine.automationGate = gate
	run := parkedRun()
	run.EntryID, run.EntryType = "lead-1", workflow.EntryTypeLead

	cancelled, err := cancelWhenAutomationOff(&runRepoStub{}, engine, run)
	if err != nil || cancelled {
		t.Fatalf("cancelled = %v, err = %v", cancelled, err)
	}
	if gate.asked != 0 {
		t.Fatalf("the conversation switch was read %d time(s) for a lead run", gate.asked)
	}
}

func answeringRun(trigger workflow.TriggerType) *workflow.WorkflowRun {
	run := parkedRun()
	run.WorkspaceID = "ws-1"
	run.TriggerType = trigger
	return run
}

func TestAnAnsweringRunStopsWhenAPersonOwnsTheConversation(t *testing.T) {
	for _, trigger := range []workflow.TriggerType{workflow.TriggerMessageReceived, workflow.TriggerFirstMessage, workflow.TriggerAgentCall} {
		t.Run(string(trigger), func(t *testing.T) {
			gate := &gateStub{enabled: true, mayAct: false}
			engine := testEngine(nil, nil, nil)
			engine.automationGate = gate
			run := answeringRun(trigger)

			if err := resumeRunFromCurrent(&workflowRepoStub{t: t}, &runRepoStub{}, engine, run); err != nil {
				t.Fatal(err)
			}

			if run.Status != workflow.RunStatusCancelled {
				t.Fatalf("status = %q, the run would answer over the person", run.Status)
			}
			if gate.askedOwner != 1 || gate.asked != 0 || gate.lastWorkspace != "ws-1" || gate.lastID != "entry-1" {
				t.Fatalf("owner asked %d, switch asked %d, about %q/%q", gate.askedOwner, gate.asked, gate.lastWorkspace, gate.lastID)
			}
		})
	}
}

func TestAnAnsweringRunGoesOnWhileAutomationMayAct(t *testing.T) {
	engine := testEngine(nil, nil, nil)
	engine.automationGate = &gateStub{mayAct: true}
	wfs := &passthroughWorkflowRepo{}
	run := answeringRun(workflow.TriggerMessageReceived)

	_ = resumeRunFromCurrent(wfs, &runRepoStub{}, engine, run)

	if !wfs.fetched || run.Status == workflow.RunStatusCancelled {
		t.Fatalf("fetched %v, status %q; the automation still owns the conversation", wfs.fetched, run.Status)
	}
}

func TestAnAnsweringRunRetriesWhenTheOwnerCannotBeRead(t *testing.T) {
	engine := testEngine(nil, nil, nil)
	engine.automationGate = &gateStub{mayAct: true, err: errors.New("db down")}
	run := answeringRun(workflow.TriggerMessageReceived)

	if err := resumeRunFromCurrent(&workflowRepoStub{t: t}, &runRepoStub{}, engine, run); err != nil {
		t.Fatal(err)
	}
	if run.Status != workflow.RunStatusWaiting || run.State.GetInt(workflow.StateKeyGateAttempts) != 1 {
		t.Fatalf("status %q, gate attempts %d; an unreadable owner runs nothing and retries", run.Status, run.State.GetInt(workflow.StateKeyGateAttempts))
	}
}

func TestARunAPersonsActionStartedOnlyStopsForThePause(t *testing.T) {
	for _, trigger := range []workflow.TriggerType{workflow.TriggerStageAdded, workflow.TriggerFormSubmitted, workflow.TriggerWebhook} {
		t.Run(string(trigger), func(t *testing.T) {
			gate := &gateStub{enabled: true, mayAct: false}
			engine := testEngine(nil, nil, nil)
			engine.automationGate = gate
			wfs := &passthroughWorkflowRepo{}
			run := answeringRun(trigger)

			_ = resumeRunFromCurrent(wfs, &runRepoStub{}, engine, run)

			if !wfs.fetched || run.Status == workflow.RunStatusCancelled || gate.askedOwner != 0 {
				t.Fatalf("fetched %v, status %q, owner asked %d", wfs.fetched, run.Status, gate.askedOwner)
			}
		})
	}
}
