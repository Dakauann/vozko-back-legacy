package agentrun_usecase

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"vozko/domain/agentrun"
	"vozko/domain/ai"
	aa "vozko/domain/ai_attendance"
	"vozko/domain/calltrace"
	"vozko/domain/redact"
	"vozko/domain/tools"
)

type scriptedAI struct {
	ai.Service
	out *ai.GenerateOutput
	err error
}

func (s scriptedAI) Generate(context.Context, ai.GenerateInput) (*ai.GenerateOutput, error) {
	return s.out, s.err
}

type recordedToolCalls struct {
	in    aa.StartInput
	count int
}

type sessionSpy struct {
	calls []recordedToolCalls
}

func (s *sessionSpy) RecordToolCalls(in aa.StartInput, count int) {
	s.calls = append(s.calls, recordedToolCalls{in: in, count: count})
}

type runLogSpy struct {
	mu        sync.Mutex
	runs      []*agentrun.Run
	batches   [][]aa.ToolCall
	recordErr error
}

func (s *runLogSpy) Record(run *agentrun.Run, spans []aa.ToolCall) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.runs = append(s.runs, run)
	s.batches = append(s.batches, spans)
	return s.recordErr
}

type toolMetricsSpy struct {
	mu    sync.Mutex
	calls []string
}

func (s *toolMetricsSpy) IncAIToolCall(tool, outcome string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, tool+"/"+outcome)
}

func syncDispatch(fn func()) { fn() }

func counting(inner ai.Service, sessions aa.ToolCallRecorder) ai.Service {
	return recordAgentRuns(inner, sessions, &runLogSpy{}, &toolMetricsSpy{}, syncDispatch)
}

func agentTurnInput() ai.GenerateInput {
	return ai.GenerateInput{
		WorkspaceID: "ws-1",
		Model:       "model-x",
		AgentTurn:   ai.AgentTurn{EntryID: "entry-1", EntryType: "instagram", AgentID: "agent-1", CampaignID: "camp-1"},
	}
}

func twoToolCalls() *ai.GenerateOutput {
	return &ai.GenerateOutput{ToolCalls: []ai.ToolCall{{Name: "http_request"}, {Name: "transfer_to_human"}}}
}

func executedTurn() *ai.GenerateOutput {
	return &ai.GenerateOutput{ToolCalls: []ai.ToolCall{
		{Name: "http_request", Arguments: map[string]interface{}{"url": "https://api", "headers": map[string]interface{}{"X-Webhook-Token": "tok-9"}, "cpf": "52998224725"}, Result: &tools.ExecutionResult{Result: map[string]interface{}{"ok": true}}, DurationMs: 120},
		{Name: "transfer_to_human", Arguments: map[string]interface{}{}, Result: &tools.ExecutionResult{Result: "tool transfer_to_human failed: 409 O workflow não está ativo", IsError: true}, DurationMs: 30},
		{Name: "route_to_sales", Arguments: map[string]interface{}{"reason": "quer comprar"}},
	}}
}

func turnWithTools() ai.GenerateInput {
	in := agentTurnInput()
	in.SessionID = "sess-1"
	in.Tools = []tools.Definition{{Name: "http_request"}, {Name: "transfer_to_human"}, {Name: "route_to_sales"}}
	return in
}

func TestEveryAgentTurnCountsItsToolCallsOnTheAttendanceSessionWhateverTheChannel(t *testing.T) {
	spy := &sessionSpy{}
	counted := counting(scriptedAI{out: twoToolCalls()}, spy)

	if _, err := counted.Generate(context.Background(), agentTurnInput()); err != nil {
		t.Fatal(err)
	}
	if len(spy.calls) != 1 || spy.calls[0].count != 2 {
		t.Fatalf("recorded %+v, want one record of two tool calls", spy.calls)
	}
	want := aa.StartInput{WorkspaceID: "ws-1", EntryID: "entry-1", EntryType: "instagram", AgentID: "agent-1", Channel: "instagram", CampaignID: "camp-1", Model: "model-x"}
	if spy.calls[0].in != want {
		t.Fatalf("session input = %+v, want %+v", spy.calls[0].in, want)
	}
}

func TestToolCallsOutsideAnIdentifiedAgentTurnAreNotCounted(t *testing.T) {
	notATurn := agentTurnInput()
	notATurn.AgentTurn = ai.AgentTurn{}
	noAgent := agentTurnInput()
	noAgent.AgentTurn.AgentID = ""
	noWorkspace := agentTurnInput()
	noWorkspace.WorkspaceID = ""
	simulated := agentTurnInput()
	simulated.AgentTurn.Source = string(agentrun.SourceSimulator)

	cases := []struct {
		name  string
		input ai.GenerateInput
		inner scriptedAI
	}{
		{"background generation", notATurn, scriptedAI{out: twoToolCalls()}},
		{"turn without an agent", noAgent, scriptedAI{out: twoToolCalls()}},
		{"turn without a workspace", noWorkspace, scriptedAI{out: twoToolCalls()}},
		{"no tool calls", agentTurnInput(), scriptedAI{out: &ai.GenerateOutput{}}},
		{"generation failed", agentTurnInput(), scriptedAI{out: twoToolCalls(), err: errors.New("provider down")}},
		{"no output", agentTurnInput(), scriptedAI{}},
		{"a simulation", simulated, scriptedAI{out: twoToolCalls()}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			spy := &sessionSpy{}
			_, _ = counting(c.inner, spy).Generate(context.Background(), c.input)
			if len(spy.calls) != 0 {
				t.Fatalf("unexpected record: %+v", spy.calls)
			}
		})
	}
}

func TestCountingKeepsTheGenerationResultUntouched(t *testing.T) {
	out := twoToolCalls()
	got, err := counting(scriptedAI{out: out}, nil).Generate(context.Background(), agentTurnInput())
	if err != nil || got != out {
		t.Fatalf("got %v, %v; want the inner output", got, err)
	}
}

func TestEveryToolCallOfAnAgentTurnIsRecordedOnceWithItsOutcome(t *testing.T) {
	log, metrics, sessions := &runLogSpy{}, &toolMetricsSpy{}, &sessionSpy{}
	counted := recordAgentRuns(scriptedAI{out: executedTurn()}, sessions, log, metrics, syncDispatch)

	if _, err := counted.Generate(context.Background(), turnWithTools()); err != nil {
		t.Fatal(err)
	}
	if len(sessions.calls) != 1 || sessions.calls[0].count != 3 {
		t.Fatalf("the session counter must count the same three calls once, got %+v", sessions.calls)
	}
	if len(log.batches) != 1 || len(log.batches[0]) != 3 || log.runs[0] != nil {
		t.Fatalf("batches = %+v runs = %+v", log.batches, log.runs)
	}
	got := log.batches[0]
	first := got[0]
	if first.WorkspaceID != "ws-1" || first.EntryID != "entry-1" || first.EntryType != "instagram" || first.AgentID != "agent-1" || first.SessionID != "sess-1" {
		t.Fatalf("identity = %+v", first)
	}
	if first.ToolName != "http_request" || first.Outcome != aa.ToolCallOK || first.DurationMs != 120 || first.ID == "" || first.CreatedAt.IsZero() {
		t.Fatalf("first = %+v", first)
	}
	headers, _ := first.Args["headers"].(map[string]interface{})
	if headers["X-Webhook-Token"] != redact.Mask || first.Args["cpf"] != "•••••••••25" || first.Args["url"] != "https://api" {
		t.Fatalf("args not redacted: %+v", first.Args)
	}
	if got[1].Outcome != aa.ToolCallError || !strings.Contains(got[1].Error, "não está ativo") {
		t.Fatalf("failed call = %+v", got[1])
	}
	if got[2].Outcome != aa.ToolCallRequested {
		t.Fatalf("a call the model made but nobody executed here = %+v", got[2])
	}
	if strings.Join(metrics.calls, ",") != "http_request/ok,transfer_to_human/error,route_to_sales/requested" {
		t.Fatalf("metrics = %v", metrics.calls)
	}
}

func TestTheRecordAndTheReturnedCallShareOneToolCallID(t *testing.T) {
	log := &runLogSpy{}
	out := &ai.GenerateOutput{ToolCalls: []ai.ToolCall{
		{ID: "call_model_1", Name: "http_request", Result: &tools.ExecutionResult{Result: "ok"}},
		{Name: "transfer_to_human", Result: &tools.ExecutionResult{Result: "ok"}},
	}}
	counted := recordAgentRuns(scriptedAI{out: out}, nil, log, &toolMetricsSpy{}, syncDispatch)

	got, err := counted.Generate(context.Background(), turnWithTools())
	if err != nil {
		t.Fatal(err)
	}
	records := log.batches[0]
	if records[0].ToolCallID != "call_model_1" || got.ToolCalls[0].ID != "call_model_1" {
		t.Fatalf("the model's id must reach the record unchanged: record %q, call %q", records[0].ToolCallID, got.ToolCalls[0].ID)
	}
	if records[1].ToolCallID == "" || records[1].ToolCallID != got.ToolCalls[1].ID {
		t.Fatalf("a call without a model id gets one id for both: record %q, call %q", records[1].ToolCallID, got.ToolCalls[1].ID)
	}
	if records[1].ID == records[1].ToolCallID {
		t.Fatal("the record keeps its own row id")
	}
}

func TestAToolNameTheAgentWasNotOfferedIsNotAMetricLabel(t *testing.T) {
	metrics := &toolMetricsSpy{}
	out := &ai.GenerateOutput{ToolCalls: []ai.ToolCall{{Name: "made_up_tool_42"}}}
	counted := recordAgentRuns(scriptedAI{out: out}, nil, &runLogSpy{}, metrics, syncDispatch)
	if _, err := counted.Generate(context.Background(), turnWithTools()); err != nil {
		t.Fatal(err)
	}
	if len(metrics.calls) != 1 || metrics.calls[0] != unlistedToolLabel+"/requested" {
		t.Fatalf("metrics = %v", metrics.calls)
	}
}

func TestARunStartedByAToolIsLinkedOnItsRecordAndABoundToolCountsUnderItsHandler(t *testing.T) {
	log, metrics := &runLogSpy{}, &toolMetricsSpy{}
	out := &ai.GenerateOutput{ToolCalls: []ai.ToolCall{
		{Name: "run_workflow_wf-1", Arguments: map[string]interface{}{}, Result: &tools.ExecutionResult{Result: "Fluxo iniciado.", RunID: "run-9"}},
		{Name: "http_request", Arguments: map[string]interface{}{}, Result: &tools.ExecutionResult{Result: "ok"}},
	}}
	in := turnWithTools()
	in.Tools = append(in.Tools, tools.Definition{Name: "run_workflow_wf-1", HandlerName: "run_workflow"})
	counted := recordAgentRuns(scriptedAI{out: out}, nil, log, metrics, syncDispatch)

	if _, err := counted.Generate(context.Background(), in); err != nil {
		t.Fatal(err)
	}
	got := log.batches[0]
	if got[0].RunID != "run-9" || got[0].ToolName != "run_workflow_wf-1" || got[1].RunID != "" {
		t.Fatalf("records = %+v", got)
	}
	if strings.Join(metrics.calls, ",") != "run_workflow/ok,http_request/ok" {
		t.Fatalf("metrics = %v", metrics.calls)
	}
}

func TestAFailedRecordNeverFailsTheReply(t *testing.T) {
	log := &runLogSpy{recordErr: errors.New("db down")}
	out := executedTurn()
	counted := recordAgentRuns(scriptedAI{out: out}, nil, log, &toolMetricsSpy{}, syncDispatch)
	got, err := counted.Generate(context.Background(), turnWithTools())
	if err != nil || got != out {
		t.Fatalf("got %v, %v", got, err)
	}
}

func TestToolCallsOutsideAnAgentTurnAreNotRecorded(t *testing.T) {
	log := &runLogSpy{}
	in := turnWithTools()
	in.AgentTurn = ai.AgentTurn{}
	counted := recordAgentRuns(scriptedAI{out: executedTurn()}, nil, log, &toolMetricsSpy{}, syncDispatch)
	if _, err := counted.Generate(context.Background(), in); err != nil {
		t.Fatal(err)
	}
	if len(log.batches) != 0 {
		t.Fatalf("batches = %+v", log.batches)
	}
}

type blockingLog struct {
	release chan struct{}
	done    chan struct{}
}

func (b *blockingLog) Record(*agentrun.Run, []aa.ToolCall) error {
	<-b.release
	close(b.done)
	return nil
}

func TestRecordingRunsOffTheReplyPath(t *testing.T) {
	release := make(chan struct{})
	done := make(chan struct{})
	log := &blockingLog{release: release, done: done}
	counted := RecordAgentRuns(scriptedAI{out: executedTurn()}, nil, log, &toolMetricsSpy{})

	finished := make(chan struct{})
	go func() {
		_, _ = counted.Generate(context.Background(), turnWithTools())
		close(finished)
	}()
	select {
	case <-finished:
	case <-time.After(2 * time.Second):
		t.Fatal("the reply waited for the tool call record")
	}
	close(release)
	<-done
}

func TestARefusalCodeIsRecordedSoTheRuntimeAlertCanTellAnInactiveWorkflowApart(t *testing.T) {
	log := &runLogSpy{}
	out := &ai.GenerateOutput{ToolCalls: []ai.ToolCall{
		{Name: "run_workflow_wf-1", Arguments: map[string]interface{}{}, Result: &tools.ExecutionResult{Result: "Fluxo inativo.", IsError: true, ErrorCode: "workflow_not_active"}},
	}}
	counted := recordAgentRuns(scriptedAI{out: out}, nil, log, &toolMetricsSpy{}, syncDispatch)
	if _, err := counted.Generate(context.Background(), turnWithTools()); err != nil {
		t.Fatal(err)
	}
	if got := log.batches[0][0]; got.Outcome != aa.ToolCallError || got.ErrorCode != "workflow_not_active" {
		t.Fatalf("record = %+v", got)
	}
}

func savedAgentTurn(source agentrun.Source) ai.GenerateInput {
	in := agentTurnInput()
	in.AgentTurn.Source = string(source)
	in.AgentTurn.AgentVersion = 7
	in.Tools = []tools.Definition{
		{Name: "consultar_pedido", HandlerName: "http_request", Instance: &tools.InstanceRef{ID: "tool-1", Type: "http_request", Revision: 3}},
		{Name: "transferir", HandlerName: "transfer_to_human", Instance: &tools.InstanceRef{ID: "tool-2", Type: "transfer_to_human", Revision: 1}},
	}
	return in
}

func spannedTurn() *ai.GenerateOutput {
	failing := &tools.ExecutionResult{
		Result:  map[string]interface{}{"status": 502, "cpf": "52998224725"},
		IsError: true,
		Trace: &calltrace.ToolTrace{
			ErrorKind:  calltrace.ErrorKindHTTPStatus,
			ErrorChain: []string{"a API respondeu 502"},
			Exchanges: []calltrace.HTTPExchange{{
				Method: "GET", URL: "https://api.test/pedidos/42", Status: 502,
				RequestHeaders: map[string]string{"Authorization": "Bearer tok-secreto"},
				ResponseBody:   `{"cpf":"52998224725"}`,
			}},
		},
	}
	ok := &tools.ExecutionResult{Result: map[string]interface{}{"status": "enviado"}, Trace: &calltrace.ToolTrace{MappedOutput: map[string]interface{}{"status": "enviado"}}}
	return &ai.GenerateOutput{
		Message: ai.Message{Role: ai.RoleAssistant, Content: "Seu pedido foi enviado."},
		ToolCalls: []ai.ToolCall{
			{ID: "c1", Name: "consultar_pedido", Arguments: map[string]interface{}{"numero": "42"}, Result: failing, DurationMs: 40, ModelCallIndex: 0},
			{ID: "c2", Name: "consultar_pedido", Arguments: map[string]interface{}{"numero": "42"}, Result: ok, DurationMs: 35, ModelCallIndex: 1},
		},
		ModelCalls: []ai.ModelCall{
			{Index: 0, Model: "openai/gpt-4o", InputTokens: 100, OutputTokens: 10, CachedTokens: 80, CostMicros: 300},
			{Index: 1, Model: "openai/gpt-4o", InputTokens: 140, OutputTokens: 12, ReasoningTokens: 3, CostMicros: 400},
			{Index: 2, Model: "openai/gpt-4o", InputTokens: 160, OutputTokens: 20},
		},
	}
}

func TestASavedAgentTurnIsOneRunWithModelAndToolSpans(t *testing.T) {
	log := &runLogSpy{}
	counted := recordAgentRuns(scriptedAI{out: spannedTurn()}, nil, log, &toolMetricsSpy{}, syncDispatch)
	if _, err := counted.Generate(context.Background(), savedAgentTurn(agentrun.SourceConversation)); err != nil {
		t.Fatal(err)
	}
	if len(log.runs) != 1 || log.runs[0] == nil {
		t.Fatalf("runs %+v", log.runs)
	}
	run := log.runs[0]
	if run.ID == "" || run.WorkspaceID != "ws-1" || run.AgentID != "agent-1" || run.AgentVersion != 7 || run.EntryID != "entry-1" || run.EntryType != "instagram" || run.Source != agentrun.SourceConversation {
		t.Fatalf("identity %+v", run)
	}
	if run.Status != agentrun.StatusToolFailed || run.ToolCalls != 2 || run.FailedToolCalls != 1 || run.HandledFailures != 1 {
		t.Fatalf("outcome %+v", run)
	}
	if run.InputTokens != 400 || run.OutputTokens != 42 || run.CachedTokens != 80 || run.ReasoningTokens != 3 || run.CostMicros == nil || *run.CostMicros != 700 || len(run.ModelCalls) != 3 || run.Model != "openai/gpt-4o" {
		t.Fatalf("usage %+v", run)
	}
	if len(run.Tools) != 2 || run.Tools[0] != (agentrun.ToolPin{ID: "tool-1", Revision: 3}) || run.Tools[1] != (agentrun.ToolPin{ID: "tool-2", Revision: 1}) {
		t.Fatalf("pins %+v", run.Tools)
	}
	spans := log.batches[0]
	if len(spans) != 2 {
		t.Fatalf("spans %+v", spans)
	}
	failed, retried := spans[0], spans[1]
	if failed.AgentRunID != run.ID || failed.ToolInstanceID != "tool-1" || failed.ToolType != "http_request" || failed.ToolRevision != 3 || failed.Sequence != 0 || failed.ModelCallIndex != 0 || failed.Source != "conversation" {
		t.Fatalf("failed span identity %+v", failed)
	}
	if failed.ErrorKind != "http_status" || len(failed.ErrorChain) != 1 || failed.FollowedBy != string(agentrun.FollowedRetry) || len(failed.HTTP) != 1 || failed.HTTP[0].Status != 502 {
		t.Fatalf("failed span outcome %+v", failed)
	}
	if retried.Sequence != 1 || retried.ModelCallIndex != 1 || retried.FollowedBy != "" || retried.Outcome != aa.ToolCallOK {
		t.Fatalf("retried span %+v", retried)
	}
	if mapped, _ := retried.MappedOutput.(map[string]interface{}); mapped["status"] != "enviado" {
		t.Fatalf("mapped %+v", retried.MappedOutput)
	}
	if !strings.Contains(retried.ModelResult, `"status":"enviado"`) {
		t.Fatalf("model result %s", retried.ModelResult)
	}
}

func TestRecordedSpansNeverKeepCredentialsOrDocuments(t *testing.T) {
	log := &runLogSpy{}
	counted := recordAgentRuns(scriptedAI{out: spannedTurn()}, nil, log, &toolMetricsSpy{}, syncDispatch)
	if _, err := counted.Generate(context.Background(), savedAgentTurn(agentrun.SourceConversation)); err != nil {
		t.Fatal(err)
	}
	failed := log.batches[0][0]
	exchange := failed.HTTP[0]
	if strings.Contains(exchange.RequestHeaders["Authorization"], "tok-secreto") || strings.Contains(exchange.ResponseBody, "52998224725") {
		t.Fatalf("exchange %+v", exchange)
	}
	if strings.Contains(failed.ModelResult, "52998224725") || strings.Contains(failed.Error, "52998224725") {
		t.Fatalf("payload %q / %q", failed.ModelResult, failed.Error)
	}
}

func TestASimulatedTurnIsARunButNeverLiveTraffic(t *testing.T) {
	log, metrics, sessions := &runLogSpy{}, &toolMetricsSpy{}, &sessionSpy{}
	out := spannedTurn()
	out.ToolCalls[1].Result.Simulated = true
	in := savedAgentTurn(agentrun.SourceSimulator)
	in.AgentTurn.EntryID, in.AgentTurn.EntryType = "", ""
	counted := recordAgentRuns(scriptedAI{out: out}, sessions, log, metrics, syncDispatch)
	if _, err := counted.Generate(context.Background(), in); err != nil {
		t.Fatal(err)
	}
	if len(log.runs) != 1 || log.runs[0].Source != agentrun.SourceSimulator || log.runs[0].EntryID != "" {
		t.Fatalf("runs %+v", log.runs)
	}
	if spans := log.batches[0]; spans[0].Simulated || !spans[1].Simulated || spans[1].Source != "simulator" {
		t.Fatalf("spans %+v", spans)
	}
	if len(sessions.calls) != 0 || len(metrics.calls) != 0 {
		t.Fatalf("a simulation counted as live traffic: sessions %+v metrics %v", sessions.calls, metrics.calls)
	}
}

func TestATurnWithoutToolCallsIsStillARun(t *testing.T) {
	log := &runLogSpy{}
	out := &ai.GenerateOutput{Message: ai.Message{Content: "Olá!"}, ModelCalls: []ai.ModelCall{{Index: 0, Model: "m", InputTokens: 5, OutputTokens: 2}}}
	counted := recordAgentRuns(scriptedAI{out: out}, nil, log, &toolMetricsSpy{}, syncDispatch)
	if _, err := counted.Generate(context.Background(), savedAgentTurn(agentrun.SourceWorkflow)); err != nil {
		t.Fatal(err)
	}
	if len(log.runs) != 1 || log.runs[0].Status != agentrun.StatusOK || log.runs[0].CostMicros != nil || len(log.batches[0]) != 0 {
		t.Fatalf("runs %+v spans %+v", log.runs, log.batches)
	}
}

func TestAFailedModelCallIsAFailedRunAndTheErrorReachesTheCaller(t *testing.T) {
	log := &runLogSpy{}
	boom := errors.New("openrouter: provider down for token sk-live-123456789012345678901234")
	counted := recordAgentRuns(scriptedAI{err: boom}, nil, log, &toolMetricsSpy{}, syncDispatch)
	if _, err := counted.Generate(context.Background(), savedAgentTurn(agentrun.SourceConversation)); !errors.Is(err, boom) {
		t.Fatalf("err %v", err)
	}
	if len(log.runs) != 1 || log.runs[0].Status != agentrun.StatusFailed || log.runs[0].ErrorKind != agentrun.ErrorKindModel || log.runs[0].Error == "" {
		t.Fatalf("runs %+v", log.runs)
	}
}

func TestARunKeepsTheIDItsCallerAssigned(t *testing.T) {
	log := &runLogSpy{}
	input := agentTurnInput()
	input.AgentTurn.Source, input.AgentTurn.RunID = "scenario", "6f1c2a0e-4b7d-4e55-9a51-0c3f6b2d8e11"
	recordAgentRuns(scriptedAI{out: executedTurn()}, nil, log, &toolMetricsSpy{}, syncDispatch).Generate(context.Background(), input)
	if len(log.runs) != 1 || log.runs[0].ID != input.AgentTurn.RunID || log.runs[0].EntryID != "" {
		t.Fatalf("runs %+v", log.runs)
	}
	input.AgentTurn.RunID = "not-a-uuid"
	recordAgentRuns(scriptedAI{out: executedTurn()}, nil, log, &toolMetricsSpy{}, syncDispatch).Generate(context.Background(), input)
	if log.runs[1].ID == "not-a-uuid" || log.runs[1].ID == "" {
		t.Fatalf("an invalid id must be replaced: %q", log.runs[1].ID)
	}
}

func TestARunKeepsTheRedactedCustomerMessageAndReplyButNotTheContextNote(t *testing.T) {
	log := &runLogSpy{}
	input := agentTurnInput()
	input.AgentTurn.Source = "conversation"
	input.Messages = []ai.Message{{Role: ai.RoleUser, Content: "meu cpf é 123.456.789-09, cadê o pedido?"}, {Role: ai.RoleUser, Content: "trechos da base"}}
	input.VolatileTail = 1
	out := executedTurn()
	out.Messages = []string{"Já foi enviado."}
	recordAgentRuns(scriptedAI{out: out}, nil, log, &toolMetricsSpy{}, syncDispatch).Generate(context.Background(), input)
	run := log.runs[0]
	if strings.Contains(run.UserMessage, "123.456.789-09") || !strings.Contains(run.UserMessage, "pedido") || run.Reply != "Já foi enviado." {
		t.Fatalf("user %q reply %q", run.UserMessage, run.Reply)
	}
}
