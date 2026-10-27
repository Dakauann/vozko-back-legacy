package workflow_usecase

import (
	"fmt"
	"log/slog"
	"time"

	"vozko/domain/cache"
	"vozko/domain/shared/deps"
	"vozko/domain/workflow"
)

type triggerEvaluator struct {
	workflowRepo   workflow.WorkflowRepository
	runRepo        workflow.WorkflowRunRepository
	engine         *RunEngine
	sharedState    cache.SharedState
	waiters        *ReplyWaiters
	responsibility workflow.ConversationResponsibility
}

func NewTriggerEvaluator(
	workflowRepo workflow.WorkflowRepository,
	runRepo workflow.WorkflowRunRepository,
	engine *RunEngine,
	sharedState cache.SharedState,
	responsibility workflow.ConversationResponsibility,
) workflow.CheckedTriggerEvaluator {
	if err := deps.Require("workflow trigger evaluator", map[string]any{"conversation responsibility": responsibility}); err != nil {
		panic(err.Error())
	}
	return &triggerEvaluator{
		workflowRepo:   workflowRepo,
		runRepo:        runRepo,
		engine:         engine,
		sharedState:    sharedState,
		waiters:        NewReplyWaiters(workflowRepo, runRepo, engine),
		responsibility: responsibility,
	}
}

func (te *triggerEvaluator) WaitingForReply(event workflow.TriggerEvent) (*workflow.WorkflowRun, error) {
	return te.waiters.WaitingForReply(event)
}

func (te *triggerEvaluator) WakeForReply(run *workflow.WorkflowRun, event workflow.TriggerEvent) bool {
	return te.waiters.WakeForReply(run, event)
}

var _ workflow.ReplyWaiters = (*triggerEvaluator)(nil)

func eventLogAttrs(event workflow.TriggerEvent) []any {
	return []any{
		slog.String("workspace_id", event.WorkspaceID),
		slog.String("entry_id", event.EntryID),
		slog.String("entry_type", event.EntryType),
		slog.String("trigger", string(event.TriggerType)),
	}
}

func (te *triggerEvaluator) Evaluate(event workflow.TriggerEvent) {
	_ = te.EvaluateChecked(event)
}

func (te *triggerEvaluator) EvaluateChecked(event workflow.TriggerEvent) error {
	slog.Debug("workflow.trigger.evaluate", eventLogAttrs(event)...)

	if _, known := event.TriggerType.Scope(); !known {
		slog.Error("workflow.trigger.unknown_scope", append(eventLogAttrs(event), slog.String("outcome", "not_fired"))...)
		return nil
	}

	handledReplyRun := ""
	handledReplyWorkflow := ""
	if event.TriggerType == workflow.TriggerMessageReceived && event.EntryID != "" {
		routed, ok := te.routeReply(event)
		if !ok {
			return nil
		}
		handledReplyRun, handledReplyWorkflow = routed.RunID, routed.WorkflowID
	}

	responsible, scoped, err := te.responsibleWorkflow(event)
	if err != nil {
		slog.Error("workflow.trigger.responsibility_unreadable", append(eventLogAttrs(event), slog.String("outcome", "not_fired"), slog.String("error", err.Error()))...)
		return fmt.Errorf("responsible workflow for %s %s: %w", event.EntryType, event.EntryID, err)
	}
	if scoped && responsible == "" {
		slog.Info("workflow.trigger.no_responsible_workflow", append(eventLogAttrs(event), slog.String("outcome", "not_fired"))...)
		return nil
	}

	workflows, err := te.workflowRepo.FindActiveByTrigger(event.WorkspaceID, event.TriggerType)
	if err != nil {
		slog.Error("workflow.trigger.find_workflows_failed", append(eventLogAttrs(event), slog.String("error", err.Error()))...)
		return fmt.Errorf("find workflows for %s: %w", event.TriggerType, err)
	}

	slog.Debug("workflow.trigger.candidates", append(eventLogAttrs(event), slog.Int("workflows", len(workflows)))...)

	for _, w := range workflows {

		if scoped && w.ID != responsible {
			slog.Debug("workflow.trigger.skip_not_responsible", append(eventLogAttrs(event), slog.String("workflow_id", w.ID), slog.String("responsible_workflow_id", responsible))...)
			continue
		}

		if handledReplyWorkflow != "" && w.ID == handledReplyWorkflow {
			slog.Info("workflow.trigger.message_already_consumed", append(eventLogAttrs(event), slog.String("workflow_id", w.ID), slog.String("run_id", handledReplyRun))...)
			continue
		}

		if !event.MayStart(w.ID) {
			slog.Info("workflow.trigger.own_move_skipped", append(eventLogAttrs(event), slog.String("workflow_id", w.ID))...)
			continue
		}

		if !te.matchesTriggerConfig(w, event) {
			continue
		}

		trigger := w.Graph.TriggerNodeByType(event.TriggerType)
		if trigger == nil {
			continue
		}

		existing, err := te.blockingRun(w, trigger, event, responsible)
		if err != nil {
			slog.Error("workflow.trigger.existing_run_check_failed", append(eventLogAttrs(event), slog.String("workflow_id", w.ID), slog.String("error", err.Error()))...)
			continue
		}
		if existing != nil {

			if existing.ID == handledReplyRun {
				continue
			}
			wakesOnMessage := event.WakesWaitingRun() && existing.Status == workflow.RunStatusWaiting && (existing.WaitReason == workflow.WaitReasonReply || existing.WaitReason == workflow.WaitReasonDuration)
			if !wakesOnMessage {
				slog.Info("workflow.trigger.existing_run_blocks", append(runLogAttrs(existing, existing.CurrentNodeID), slog.String("status", string(existing.Status)))...)
				continue
			}
			pinned, err := pinnedWorkflow(te.workflowRepo, existing, w)
			if err != nil || pinned == nil {
				slog.Error("workflow.trigger.pinned_version_unreadable", append(runLogAttrs(existing, existing.CurrentNodeID), slog.Any("error", err))...)
				continue
			}
			if existing.WaitReason == workflow.WaitReasonReply {
				te.waiters.wake(existing, pinned, event)
			} else if existing.WaitReason == workflow.WaitReasonDuration {

				te.wakeRunForMessage(existing, pinned, event)
			} else {
				slog.Info("workflow.trigger.existing_run_blocks", append(runLogAttrs(existing, existing.CurrentNodeID), slog.String("status", string(existing.Status)))...)
			}
			continue
		}

		if !tryAcquireWorkspaceSlot(te.sharedState, event.WorkspaceID) {
			slog.Warn("workflow.trigger.workspace_at_capacity", append(eventLogAttrs(event), slog.String("workflow_id", w.ID), slog.Int("limit", maxConcurrentRunsPerWorkspace))...)
			continue
		}

		run := newTriggeredRun(w, trigger, event)
		if err := te.runRepo.Create(run); err != nil {
			slog.Error("workflow.trigger.create_run_failed", append(eventLogAttrs(event), slog.String("workflow_id", w.ID), slog.String("error", err.Error()))...)
			releaseWorkspaceSlot(te.sharedState, event.WorkspaceID)
			continue
		}

		slog.Info("workflow.trigger.run_created", append(runLogAttrs(run, run.CurrentNodeID), slog.String("workflow_id", w.ID), slog.String("started_by", run.StartedBy))...)

		executeLocked(te.engine, run, w)
		releaseWorkspaceSlot(te.sharedState, event.WorkspaceID)
	}
	return nil
}

func (te *triggerEvaluator) responsibleWorkflow(event workflow.TriggerEvent) (string, bool, error) {
	if !event.TriggerType.FollowsResponsibleWorkflow() {
		return "", false, nil
	}
	if named, ok := event.NamedResponsibleWorkflow(); ok {
		return named, true, nil
	}
	id, _, err := te.responsibility.ResponsibleWorkflow(event.WorkspaceID, event.EntryID, event.EntryType)
	if err != nil {
		return "", true, err
	}
	return id, true, nil
}

func (te *triggerEvaluator) blockingRun(w *workflow.Workflow, trigger *workflow.Node, event workflow.TriggerEvent, responsible string) (*workflow.WorkflowRun, error) {
	if event.StartsRunPerEvent() {
		return nil, nil
	}
	run, err := te.runRepo.FindActiveByEntryAndTrigger(w.ID, event.EntryID, trigger.ID)
	if err != nil {
		return nil, err
	}
	return conversationRunCurrent(te.runRepo, te.engine, run, w, event, responsible)
}

func (te *triggerEvaluator) routeReply(event workflow.TriggerEvent) (workflow.ReplyRouting, bool) {
	if event.Reply != nil {
		return *event.Reply, true
	}
	waiters := te.waiters
	run, err := waiters.WaitingForReply(event)
	if err != nil {
		return workflow.ReplyRouting{}, false
	}
	if run == nil || !waiters.WakeForReply(run, event) {
		return workflow.ReplyRouting{}, true
	}
	return workflow.ReplyRouting{RunID: run.ID, WorkflowID: run.WorkflowID}, true
}

func (te *triggerEvaluator) wakeRunForMessage(run *workflow.WorkflowRun, w *workflow.Workflow, event workflow.TriggerEvent) {
	if !te.engine.TryLockRun(run.ID) {
		slog.Info("workflow.trigger.message_wake_locked", runLogAttrs(run, run.CurrentNodeID)...)
		return
	}
	defer te.engine.UnlockRun(run.ID)

	slog.Info("workflow.trigger.message_cancels_wait", runLogAttrs(run, run.CurrentNodeID)...)

	node := w.Graph.FindNode(run.CurrentNodeID)
	if node == nil || (node.Type != workflow.NodeTypeWaitDuration && node.Type != workflow.NodeTypeWaitSchedule) {
		return
	}
	edges := w.Graph.OutgoingEdges(run.CurrentNodeID)
	nextID := findExactEdgeByLabel(edges, "message_received")
	if nextID == "" {
		slog.Info("workflow.trigger.wait_has_no_message_edge", runLogAttrs(run, run.CurrentNodeID)...)
		return
	}

	run.State.Set("_wait_outcome", "message_received")
	workflow.ForgetTriggerMessage(&run.State)
	for k, v := range event.Data {
		run.State.Set(k, v)
	}
	run.SetRunning()
	run.State.Set("_prev_node_id", run.CurrentNodeID)
	run.CurrentNodeID = nextID
	run.UpdatedAt = time.Now().UTC()

	if err := te.runRepo.Update(run); err != nil {
		slog.Error("workflow.trigger.message_wake_save_failed", append(runLogAttrs(run, run.CurrentNodeID), slog.String("error", err.Error()))...)
		return
	}

	if err := te.engine.Execute(run, w); err != nil {
		slog.Error("workflow.trigger.engine_error", append(runLogAttrs(run, run.CurrentNodeID), slog.String("error", outbound.String(err.Error())))...)
	}
}

func (te *triggerEvaluator) matchesTriggerConfig(w *workflow.Workflow, event workflow.TriggerEvent) bool {
	triggerNode := w.Graph.TriggerNodeByType(event.TriggerType)
	var nodeConfig map[string]interface{}
	if triggerNode != nil {
		nodeConfig = triggerNode.Config
	}
	cfg := workflow.EffectiveTriggerConfig(nodeConfig, w.TriggerConfig)
	if event.TriggerType == workflow.TriggerFormSubmitted {
		formID, _ := event.Data[workflow.DataKeyFormID].(string)
		return workflow.FormTriggerAccepts(cfg, formID)
	}
	if len(cfg) == 0 {
		return true
	}

	switch event.TriggerType {
	case workflow.TriggerStageAdded:
		StageID, _ := cfg[workflow.DataKeyStageID].(string)
		if StageID == "" {
			return true
		}
		eventTagID, _ := event.Data[workflow.DataKeyStageID].(string)
		return StageID == eventTagID

	default:
		return true
	}
}
