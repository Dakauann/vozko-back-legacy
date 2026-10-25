package agentrun_usecase

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"

	"vozko/domain/agentrun"
	"vozko/domain/ai"
	aa "vozko/domain/ai_attendance"
	"vozko/domain/calltrace"
	"vozko/domain/metrics"
	"vozko/domain/redact"
	"vozko/domain/tools"
	"vozko/domain/tracepayload"
)

const unlistedToolLabel = "unlisted"

const maxToolErrorBytes = 4 * 1024

var spanRedactor = redact.New()

type recorder struct {
	ai.Service
	sessions aa.ToolCallRecorder
	runs     agentrun.Log
	metrics  metrics.AIToolCallMetricsRecorder
	dispatch func(func())
}

func RecordAgentRuns(inner ai.Service, sessions aa.ToolCallRecorder, runs agentrun.Log, metricsRecorder metrics.AIToolCallMetricsRecorder) ai.Service {
	return recordAgentRuns(inner, sessions, runs, metricsRecorder, func(fn func()) { go fn() })
}

func recordAgentRuns(inner ai.Service, sessions aa.ToolCallRecorder, runs agentrun.Log, metricsRecorder metrics.AIToolCallMetricsRecorder, dispatch func(func())) ai.Service {
	if runs == nil || metricsRecorder == nil {
		panic("agent runs: recording needs the run log and the metrics recorder")
	}
	return &recorder{Service: inner, sessions: sessions, runs: runs, metrics: metricsRecorder, dispatch: dispatch}
}

func (c *recorder) Generate(ctx context.Context, input ai.GenerateInput) (*ai.GenerateOutput, error) {
	started := time.Now()
	out, err := c.Service.Generate(ctx, input)
	turn := input.AgentTurn
	source := agentrun.Source(turn.Source)
	if out != nil {
		ai.EnsureToolCallIDs(out.ToolCalls)
	}
	if input.WorkspaceID != "" && turn.AgentID != "" && source.Known() {
		c.recordRun(input, source, out, err, started)
		return out, err
	}
	if err != nil || out == nil || len(out.ToolCalls) == 0 || input.WorkspaceID == "" || !turn.Identified() {
		return out, err
	}
	c.countLive(input, out.ToolCalls)
	c.store(input, nil, assembleSpans(input, "", "", out))
	return out, err
}

func (c *recorder) recordRun(input ai.GenerateInput, source agentrun.Source, out *ai.GenerateOutput, failure error, started time.Time) {
	run := &agentrun.Run{
		ID:           runIDOf(input.AgentTurn),
		WorkspaceID:  input.WorkspaceID,
		AgentID:      input.AgentTurn.AgentID,
		AgentVersion: input.AgentTurn.AgentVersion,
		Source:       source,
		Model:        input.Model,
		DurationMs:   time.Since(started).Milliseconds(),
		ModelCalls:   []ai.ModelCall{},
		Tools:        pinsOf(input.Tools),
		StartedAt:    started.UTC(),
	}
	if !source.Sandboxed() {
		run.EntryID, run.EntryType = input.AgentTurn.EntryID, input.AgentTurn.EntryType
	}
	if failure != nil || out == nil {
		run.Status = agentrun.StatusFailed
		run.ErrorKind = agentrun.ErrorKindModel
		if failure != nil {
			run.Error = tracepayload.MarkedTextWithin(spanRedactor.String(failure.Error()), maxToolErrorBytes)
		}
		c.store(input, run, []aa.ToolCall{})
		return
	}
	spans := assembleSpans(input, run.ID, source, out)
	summarize(run, out, spans)
	run.UserMessage = turnText(lastUserMessage(input.Messages, input.VolatileTail))
	run.Reply = turnText(replyOf(out))
	if !source.Sandboxed() && len(out.ToolCalls) > 0 {
		c.countLive(input, out.ToolCalls)
	}
	c.store(input, run, spans)
}

func lastUserMessage(messages []ai.Message, volatileTail int) string {
	if volatileTail > 0 && volatileTail <= len(messages) {
		messages = messages[:len(messages)-volatileTail]
	}
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == ai.RoleUser && strings.TrimSpace(messages[i].Content) != "" {
			return messages[i].Content
		}
	}
	return ""
}

func replyOf(out *ai.GenerateOutput) string {
	if len(out.Messages) > 0 {
		return strings.Join(out.Messages, "\n")
	}
	return out.Message.Content
}

func turnText(text string) string {
	if strings.TrimSpace(text) == "" {
		return ""
	}
	return tracepayload.MarkedTextWithin(spanRedactor.String(text), maxToolErrorBytes)
}

func runIDOf(turn ai.AgentTurn) string {
	if _, err := uuid.Parse(turn.RunID); err == nil {
		return turn.RunID
	}
	return uuid.NewString()
}

func summarize(run *agentrun.Run, out *ai.GenerateOutput, spans []aa.ToolCall) {
	var cost int64
	if len(out.ModelCalls) > 0 {
		run.ModelCalls = out.ModelCalls
		run.Model = out.ModelCalls[0].Model
	}
	for _, call := range out.ModelCalls {
		run.InputTokens += call.InputTokens
		run.OutputTokens += call.OutputTokens
		run.CachedTokens += call.CachedTokens
		run.ReasoningTokens += call.ReasoningTokens
		cost += call.CostMicros
	}
	if len(out.ModelCalls) == 0 {
		run.InputTokens, run.OutputTokens, run.CachedTokens = out.Usage.PromptTokens, out.Usage.CompletionTokens, out.Usage.CachedTokens
		cost = out.Usage.ProviderCostMicros
	}
	if cost > 0 {
		run.CostMicros = &cost
	}
	run.ToolCalls = len(spans)
	for _, s := range spans {
		if s.Outcome == aa.ToolCallError {
			run.FailedToolCalls++
		}
		if agentrun.FollowedBy(s.FollowedBy).Handled() {
			run.HandledFailures++
		}
	}
	run.Status = agentrun.StatusOf(false, run.FailedToolCalls)
}

func (c *recorder) countLive(input ai.GenerateInput, calls []ai.ToolCall) {
	turn := input.AgentTurn
	if c.sessions != nil && turn.Identified() {
		c.sessions.RecordToolCalls(aa.StartInput{
			WorkspaceID: input.WorkspaceID,
			EntryID:     turn.EntryID,
			EntryType:   turn.EntryType,
			AgentID:     turn.AgentID,
			Channel:     turn.EntryType,
			CampaignID:  turn.CampaignID,
			Model:       input.Model,
		}, len(calls))
	}
	offered := offeredTools(input.Tools)
	for _, call := range calls {
		outcome, _ := toolCallOutcome(call)
		label, isOffered := offered[strings.ToLower(call.Name)]
		if !isOffered {
			label = unlistedToolLabel
		}
		c.metrics.IncAIToolCall(label, string(outcome))
	}
}

func (c *recorder) store(input ai.GenerateInput, run *agentrun.Run, spans []aa.ToolCall) {
	c.dispatch(func() {
		if err := c.runs.Record(run, spans); err != nil {
			slog.Error("agent_runs.record_failed",
				slog.String("workspace_id", input.WorkspaceID),
				slog.String("entry_id", input.AgentTurn.EntryID),
				slog.String("agent_id", input.AgentTurn.AgentID),
				slog.Int("calls", len(spans)),
				slog.String("error", err.Error()))
		}
	})
}

func assembleSpans(input ai.GenerateInput, runID string, source agentrun.Source, out *ai.GenerateOutput) []aa.ToolCall {
	now := time.Now().UTC()
	steps := make([]agentrun.Step, len(out.ToolCalls))
	for i, call := range out.ToolCalls {
		steps[i] = agentrun.Step{Name: call.Name, Failed: call.Result != nil && call.Result.IsError, ModelCall: call.ModelCallIndex}
	}
	followUps := agentrun.FollowUps(steps, strings.TrimSpace(out.Message.Content) != "" || len(out.Messages) > 0)
	spans := make([]aa.ToolCall, 0, len(out.ToolCalls))
	for i, call := range out.ToolCalls {
		outcome, failure := toolCallOutcome(call)
		span := aa.ToolCall{
			ID:             uuid.New().String(),
			ToolCallID:     call.ID,
			WorkspaceID:    input.WorkspaceID,
			AgentID:        input.AgentTurn.AgentID,
			SessionID:      input.SessionID,
			ToolName:       call.Name,
			Args:           tracepayload.Map(spanRedactor.Map(call.Arguments)),
			Outcome:        outcome,
			Error:          tracepayload.MarkedTextWithin(spanRedactor.String(failure), maxToolErrorBytes),
			ErrorCode:      errorCodeOf(call),
			DurationMs:     call.DurationMs,
			RunID:          runStartedBy(call),
			CreatedAt:      now,
			AgentRunID:     runID,
			Source:         string(source),
			Sequence:       i,
			ModelCallIndex: call.ModelCallIndex,
			FollowedBy:     string(followUps[i]),
		}
		if !source.Sandboxed() {
			span.EntryID, span.EntryType = input.AgentTurn.EntryID, input.AgentTurn.EntryType
		}
		withInstance(&span, input.Tools, call.Name)
		if call.Result != nil {
			span.ModelResult = tracepayload.MarkedText(spanRedactor.String(call.Result.ModelPayload()))
			span.Simulated = call.Result.Simulated
			withTrace(&span, call.Result.Trace)
		}
		spans = append(spans, span)
	}
	return spans
}

func withInstance(span *aa.ToolCall, offered []tools.Definition, name string) {
	def, ok := tools.Offered(offered, name)
	if !ok {
		return
	}
	span.ToolType = def.Handler()
	if def.Instance == nil {
		return
	}
	span.ToolInstanceID, span.ToolRevision = def.Instance.ID, def.Instance.Revision
	if def.Instance.Type != "" {
		span.ToolType = def.Instance.Type
	}
}

func withTrace(span *aa.ToolCall, trace *calltrace.ToolTrace) {
	if trace == nil {
		return
	}
	span.ErrorKind = string(trace.ErrorKind)
	span.ErrorChain = spanRedactor.Strings(trace.ErrorChain)
	if len(trace.Exchanges) > 0 {
		exchanges := make([]calltrace.HTTPExchange, len(trace.Exchanges))
		for i, x := range trace.Exchanges {
			exchanges[i] = x.Redact(spanRedactor)
		}
		span.HTTP = calltrace.FitHTTPExchanges(exchanges)
	}
	span.MappedOutput = cappedValue(spanRedactor.Value(trace.MappedOutput))
}

func cappedValue(v interface{}) interface{} {
	if v == nil {
		return nil
	}
	if m, ok := v.(map[string]interface{}); ok {
		return tracepayload.Map(m)
	}
	encoded, err := json.Marshal(v)
	if err == nil && len(encoded) <= tracepayload.MaxBytes {
		return v
	}
	return tracepayload.Map(map[string]interface{}{"value": v})
}

func pinsOf(defs []tools.Definition) []agentrun.ToolPin {
	pins := []agentrun.ToolPin{}
	for _, def := range defs {
		if def.Instance != nil && def.Instance.ID != "" {
			pins = append(pins, agentrun.ToolPin{ID: def.Instance.ID, Revision: def.Instance.Revision})
		}
	}
	return pins
}

func offeredTools(defs []tools.Definition) map[string]string {
	offered := make(map[string]string, len(defs))
	for _, def := range defs {
		label := def.Name
		if def.HandlerName != "" {
			label = def.HandlerName
		}
		offered[strings.ToLower(def.Name)] = label
	}
	return offered
}

func runStartedBy(call ai.ToolCall) string {
	if call.Result == nil {
		return ""
	}
	return call.Result.RunID
}

func errorCodeOf(call ai.ToolCall) string {
	if call.Result == nil || !call.Result.IsError {
		return ""
	}
	return call.Result.ErrorCode
}

func toolCallOutcome(call ai.ToolCall) (aa.ToolCallOutcome, string) {
	if call.Result == nil {
		return aa.ToolCallRequested, ""
	}
	if !call.Result.IsError {
		return aa.ToolCallOK, ""
	}
	if text, ok := call.Result.Result.(string); ok && text != "" {
		return aa.ToolCallError, text
	}
	if call.Result.ContextUpdateText != "" {
		return aa.ToolCallError, call.Result.ContextUpdateText
	}
	encoded, _ := json.Marshal(call.Result.Result)
	return aa.ToolCallError, string(encoded)
}
