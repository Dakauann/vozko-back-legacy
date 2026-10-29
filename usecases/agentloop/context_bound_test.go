package agentloop

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"

	"vozko/domain/ai"
)

const (
	studioPromptChars  = 136_000
	studioOutputChars  = 32_000
	scenarioAnswers    = 3
	scenarioCallsEach  = 20
	scenarioCompletion = 300
)

type studioScenarioDriver struct {
	*checkpointDriver
	produced int
}

func (d *studioScenarioDriver) SystemPrompt() string { return strings.Repeat("s", studioPromptChars) }

func newStudioScenarioDriver() *studioScenarioDriver {
	d := &studioScenarioDriver{checkpointDriver: &checkpointDriver{fakeDriver: &fakeDriver{}}}
	d.dispatchFn = func(call ai.ToolCall) StepResult {
		result := call.ID + " " + strings.Repeat("d", studioOutputChars)
		d.produced += estimateTokens(result)
		return StepResult{Result: result}
	}
	return d
}

type sizingAI struct {
	fakeAI
	drv       Driver
	window    int
	mu        sync.Mutex
	step      int
	serial    int
	prompts   []int
	starts    []int
	summaries int
	answers   [][]ai.GenerateInput
	rewrites  [][]int
	compacted bool
	pruned    bool
}

func (s *sizingAI) GenerateStream(_ context.Context, in ai.GenerateInput) (<-chan ai.StreamEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	prompt := estimateRequest(s.drv, in.Messages)
	ch := make(chan ai.StreamEvent, 1)
	defer close(ch)
	if prompt > s.window {
		ch <- ai.StreamEvent{Type: ai.StreamEventError, Error: fmt.Errorf("%w: %d tokens", ai.ErrContextOverflow, prompt)}
		return ch, nil
	}
	usage := &ai.Usage{PromptTokens: prompt, CompletionTokens: scenarioCompletion, TotalTokens: prompt + scenarioCompletion}
	if in.ToolChoice == "none" {
		s.summaries++
		s.compacted = true
		ch <- ai.StreamEvent{Type: ai.StreamEventDone, FullText: filledSummary, Usage: usage}
		return ch, nil
	}
	s.prompts = append(s.prompts, prompt)
	last := len(s.answers) - 1
	if s.compacted {
		s.rewrites[last] = append(s.rewrites[last], len(s.answers[last]))
		s.compacted = false
	}
	s.answers[last] = append(s.answers[last], in)
	if s.step == 0 {
		s.starts = append(s.starts, prompt)
	}
	s.pruned = s.pruned || containsText(in.Messages, "[resultado antigo de studio_read")
	if s.step == scenarioCallsEach {
		ch <- ai.StreamEvent{Type: ai.StreamEventDone, FullText: "pronto", Usage: usage}
		return ch, nil
	}
	s.step++
	s.serial++
	call := ai.ToolCall{ID: fmt.Sprintf("call%d", s.serial), Name: "studio_read", Arguments: map[string]interface{}{"n": s.serial}}
	ch <- ai.StreamEvent{Type: ai.StreamEventDone, AllToolCalls: []ai.ToolCall{call}, Usage: usage}
	return ch, nil
}

func (s *sizingAI) nextAnswer() {
	s.mu.Lock()
	s.step = 0
	s.compacted = false
	s.answers = append(s.answers, nil)
	s.rewrites = append(s.rewrites, nil)
	s.mu.Unlock()
}

func TestALongStudioSessionStaysWithinTheWorkingBudget(t *testing.T) {
	drv := newStudioScenarioDriver()
	cfg := compactingConfig()
	cfg.ModelLimits = ai.ModelInfo{ContextLength: 1_000_000}
	prov := &sizingAI{drv: drv, window: int(cfg.ModelLimits.ContextLength)}
	e := Engine{AI: prov}
	sess := &Session{}
	for answer := range scenarioAnswers {
		prov.nextAnswer()
		sess.History = PruneToolOutputs(sess.History)
		sess.Turn = nil
		out := e.Run(t.Context(), (&capture{}).emit, drv, cfg, sess, fmt.Sprintf("pedido %d", answer))
		if out.Kind != OutcomeIdle || out.Halt != nil {
			t.Fatalf("answer %d ended with %+v", answer, out)
		}
	}
	if prov.serial != scenarioAnswers*scenarioCallsEach {
		t.Fatalf("%d tool calls, want %d", prov.serial, scenarioAnswers*scenarioCallsEach)
	}
	peak := 0
	for _, p := range prov.prompts {
		peak = max(peak, p)
	}
	unbounded := estimateTokens(drv.SystemPrompt()) + drv.produced
	t.Logf("first call %d, answer starts %v, peak %d, summaries %d, unbounded %d", prov.prompts[0], prov.starts, peak, prov.summaries, unbounded)
	if peak >= ai.ContextBudgetCapTokens {
		t.Fatalf("the context reached %d tokens, over the %d budget", peak, ai.ContextBudgetCapTokens)
	}
	if prov.summaries == 0 || len(drv.saved) == 0 {
		t.Fatal("a session this long must compact at least once")
	}
	if !prov.pruned {
		t.Fatal("old tool outputs must be pruned at an answer boundary")
	}
	for answer, inputs := range prov.answers {
		assertAppendOnly(t, inputs, prov.rewrites[answer]...)
	}
	if unbounded <= 2*ai.ContextBudgetCapTokens {
		t.Fatalf("the scenario must produce well over the budget to prove the bound, produced %d", unbounded)
	}
}
