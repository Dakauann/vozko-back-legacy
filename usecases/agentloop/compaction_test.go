package agentloop

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"vozko/domain/ai"
)

const filledSummary = "## Objetivo\n- marcador retrô\n\n## Próximo passo\n1. montar o mar"

type checkpointDriver struct {
	*fakeDriver
	saved []string
	err   error
}

func (d *checkpointDriver) SaveCheckpoint(_ string, text string) error {
	d.saved = append(d.saved, text)
	return d.err
}

func full(prompt int) *ai.Usage { return &ai.Usage{PromptTokens: prompt, TotalTokens: prompt} }

func compactingConfig() Config {
	return Config{FinishToolName: "finish", IterateUntilDone: true, NoProgressStop: 100, ModelLimits: ai.ModelInfo{ContextLength: 40_000}}
}

func TestCompactionFiresAtTheWorkingBudgetOfTheModel(t *testing.T) {
	cases := map[int64]int{20_000: 18_000, 40_000: 24_000, 128_000: 112_000, 200_000: 180_000, 1_000_000: 200_000, 2_000_000: 200_000}
	for window, want := range cases {
		got, ok := Config{ModelLimits: ai.ModelInfo{ContextLength: window}}.compactionCeiling()
		if !ok || got != want {
			t.Errorf("window %d: compacts at %d, want %d", window, got, want)
		}
	}
	if _, ok := (Config{}).compactionCeiling(); ok {
		t.Error("an unknown window never compacts")
	}
}

func TestAMillionTokenWindowCompactsMidAnswerOnceTheRealUsageReachesTheBudget(t *testing.T) {
	cfg := compactingConfig()
	cfg.ModelLimits = ai.ModelInfo{ContextLength: 1_000_000}
	prov := &fakeAI{turns: []aiTurn{
		{tcs: []ai.ToolCall{tcall("read")}, usage: &ai.Usage{PromptTokens: 150_000, CompletionTokens: 2_000, CachedTokens: 140_000}},
		{tcs: []ai.ToolCall{tcall("look")}, usage: &ai.Usage{PromptTokens: 195_000, CompletionTokens: 5_000, CachedTokens: 150_000}},
		{fullText: filledSummary},
		{fullText: "pronto"},
	}}
	drv := &checkpointDriver{fakeDriver: reading()}
	out, _, sess := runWith(t, prov, drv, cfg)
	if out.Halt != nil || len(prov.inputs) != 4 {
		t.Fatalf("outcome %+v after %d calls, want one summary between the steps", out, len(prov.inputs))
	}
	if prov.inputs[1].ToolChoice == "none" || prov.inputs[2].ToolChoice != "none" {
		t.Fatal("150 mil tokens stay under the budget; the summary comes only after the call that reached 200 mil")
	}
	if len(drv.saved) != 1 || !isCheckpoint(sess.History[0]) {
		t.Fatalf("saved %d checkpoints, history opens with %q", len(drv.saved), sess.History[0].Content)
	}
}

func bulkyExchanges(n, chars int) []ai.Message {
	var h []ai.Message
	for i := 0; i < n; i++ {
		h = append(h,
			ai.Message{Role: ai.RoleUser, Content: fmt.Sprintf("pedido %d", i)},
			ai.Message{Role: ai.RoleAssistant, Content: fmt.Sprintf("resposta %d ", i) + strings.Repeat("x", chars)},
		)
	}
	return h
}

func TestASummaryRequestLongerThanTheWindowIsFlattenedAndDropsTheOldest(t *testing.T) {
	cfg := compactingConfig()
	cfg.CompactKeepTokens = 1_000
	prov := &fakeAI{turns: []aiTurn{{fullText: filledSummary}, {fullText: "pronto"}}}
	drv := &checkpointDriver{fakeDriver: reading()}
	sess := &Session{History: bulkyExchanges(10, 20_000)}
	e := Engine{AI: prov}
	out := e.Run(t.Context(), (&capture{}).emit, drv, cfg, sess, "continue")
	if out.Halt != nil {
		t.Fatalf("outcome %+v", out)
	}
	summary := prov.inputs[0]
	ceiling, _ := cfg.ModelLimits.ContextCeiling()
	if size := estimateRequest(drv, summary.Messages); size > ceiling {
		t.Fatalf("the summary request has %d tokens, over the %d the model takes", size, ceiling)
	}
	if len(summary.Messages) != 2 || summary.Messages[0].Role != ai.RoleUser {
		t.Fatalf("the summary request must be the flattened conversation and the summary prompt, got %d messages", len(summary.Messages))
	}
	flat := summary.Messages[0].Content
	if strings.Contains(flat, "[Usuário]: pedido 0\n") || !strings.Contains(flat, "[Usuário]: pedido 8") || !strings.Contains(flat, "parte mais antiga da conversa ficou de fora") {
		t.Fatalf("the oldest exchanges are dropped and the newest summarized ones kept: %.300s", flat)
	}
	if !strings.Contains(summary.Messages[1].Content, "resumo estruturado") {
		t.Fatal("the summary prompt stays last")
	}
}

func TestASummaryRequestThatFitsKeepsTheConversationAsItWasForTheCache(t *testing.T) {
	cfg := compactingConfig()
	cfg.CompactKeepTokens = 1_000
	cfg.ModelLimits = ai.ModelInfo{ContextLength: 1_000_000}
	prov := &fakeAI{turns: []aiTurn{{fullText: filledSummary}, {fullText: "pronto"}}}
	sess := &Session{History: bulkyExchanges(4, 240_000)}
	e := Engine{AI: prov}
	e.Run(t.Context(), (&capture{}).emit, &checkpointDriver{fakeDriver: reading()}, cfg, sess, "continue")
	summary := prov.inputs[0].Messages
	if len(summary) < 3 || summary[0].Content != "pedido 0" {
		t.Fatalf("a summary request that fits reuses the conversation messages, got %d messages starting %.40q", len(summary), summary[0].Content)
	}
}

func TestASummaryTheProviderRejectsAsTooLongIsRetriedSmaller(t *testing.T) {
	cfg := compactingConfig()
	cfg.CompactKeepTokens = 1_000
	overflow := fmt.Errorf("%w: prompt is too long", ai.ErrContextOverflow)
	prov := &fakeAI{turns: []aiTurn{
		{errEvent: overflow},
		{errEvent: overflow},
		{fullText: filledSummary},
		{fullText: "pronto"},
	}}
	drv := &checkpointDriver{fakeDriver: reading()}
	sess := &Session{History: bulkyExchanges(6, 8_000)}
	e := Engine{AI: prov}
	out := e.Run(t.Context(), (&capture{}).emit, drv, cfg, sess, "continue")
	if out.Halt != nil || len(drv.saved) != 1 {
		t.Fatalf("outcome %+v saved %d, want the reply to go on after a smaller summary", out, len(drv.saved))
	}
	first, retried := prov.inputs[1], prov.inputs[2]
	if retried.ToolChoice != "none" || estimateRequest(drv, retried.Messages) >= estimateRequest(drv, first.Messages) {
		t.Fatal("the retried summary request must be smaller than the one the provider refused")
	}
	if containsText(retried.Messages, "[Usuário]: pedido 0\n") || !containsText(retried.Messages, "[Usuário]: pedido 4") {
		t.Fatal("the retry drops the oldest exchanges and keeps the newest summarized ones")
	}
}

func TestASummaryThatNeverFitsStopsTheReply(t *testing.T) {
	cfg := compactingConfig()
	cfg.CompactKeepTokens = 1_000
	overflow := fmt.Errorf("%w: prompt is too long", ai.ErrContextOverflow)
	turns := []aiTurn{{errEvent: overflow}}
	for range maxOverflowCompactions + 1 {
		turns = append(turns, aiTurn{errEvent: overflow})
	}
	prov := &fakeAI{turns: turns}
	drv := &checkpointDriver{fakeDriver: reading()}
	sess := &Session{History: bulkyExchanges(6, 8_000)}
	e := Engine{AI: prov}
	out := e.Run(t.Context(), (&capture{}).emit, drv, cfg, sess, "continue")
	if !errors.Is(out.Halt, ErrCompactionFailed) || len(drv.saved) != 0 {
		t.Fatalf("outcome %+v saved %d, want the reply stopped", out, len(drv.saved))
	}
	if len(prov.inputs) != maxOverflowCompactions+2 {
		t.Fatalf("%d calls, want the summary tried %d times", len(prov.inputs), maxOverflowCompactions+1)
	}
}

func TestAFullContextIsSummarizedIntoACheckpointAndTheReplyGoesOn(t *testing.T) {
	prov := &fakeAI{turns: []aiTurn{
		{tcs: []ai.ToolCall{tcall("read")}, usage: full(25_000)},
		{fullText: filledSummary},
		{fullText: "pronto"},
	}}
	drv := &checkpointDriver{fakeDriver: reading()}
	out, cp, sess := runWith(t, prov, drv, compactingConfig())
	if out.Kind != OutcomeIdle || out.Halt != nil {
		t.Fatalf("outcome %+v", out)
	}
	summaryCall := prov.inputs[1]
	last := summaryCall.Messages[len(summaryCall.Messages)-1]
	if summaryCall.ToolChoice != "none" || !strings.Contains(last.Content, "resumo estruturado") {
		t.Fatalf("the summary call must ask for the structured summary without tools: %+v", last)
	}
	after := prov.inputs[2].Messages
	if !isCheckpoint(after[0]) || !strings.Contains(after[0].Content, "marcador retrô") {
		t.Fatalf("the next call must open with the checkpoint: %+v", after[0])
	}
	if len(drv.saved) != 1 || drv.saved[0] != after[0].Content {
		t.Fatalf("the checkpoint must be saved for the next replies, saved %d", len(drv.saved))
	}
	if len(sess.Turn) != 1 || sess.Turn[0].Content != "pronto" {
		t.Fatalf("the reply transcript restarts after the checkpoint, got %+v", sess.Turn)
	}
	if cp.count(EventAssistantDone) != 2 {
		t.Fatalf("the summary call must stay silent, got %d answer events", cp.count(EventAssistantDone))
	}
}

func TestTheNewestExchangesStayVerbatimInsideTheCheckpoint(t *testing.T) {
	cfg := compactingConfig()
	cfg.CompactKeepTokens = 1_000
	cfg.ModelLimits = ai.ModelInfo{ContextLength: 1_000_000}
	prov := &fakeAI{turns: []aiTurn{{fullText: filledSummary}, {fullText: "pronto"}}}
	sess := &Session{History: []ai.Message{
		{Role: ai.RoleUser, Content: "primeiro pedido"},
		{Role: ai.RoleAssistant, Content: "feito " + strings.Repeat("antigo ", 115_000)},
		{Role: ai.RoleUser, Content: "segundo pedido"},
		{Role: ai.RoleAssistant, Content: "também feito"},
	}}
	e := Engine{AI: prov}
	e.Run(t.Context(), (&capture{}).emit, &checkpointDriver{fakeDriver: reading()}, cfg, sess, "terceiro")
	older := prov.inputs[0].Messages
	if !strings.Contains(older[0].Content, "primeiro pedido") || containsText(older, "segundo pedido") {
		t.Fatalf("only the older exchange goes to the summary, got %+v", older)
	}
	checkpoint := prov.inputs[1].Messages[0].Content
	if !strings.Contains(checkpoint, "<recent-context>") || !strings.Contains(checkpoint, "[Usuário]: segundo pedido") || !strings.Contains(checkpoint, "[Assistente]: também feito") {
		t.Fatalf("the recent exchanges must stay as text in the checkpoint: %s", checkpoint)
	}
}

func TestASummaryThatSkipsTheTemplateGetsOneReminder(t *testing.T) {
	prov := &fakeAI{turns: []aiTurn{
		{tcs: []ai.ToolCall{tcall("read")}, usage: full(25_000)},
		{fullText: "resumo solto"},
		{fullText: filledSummary},
		{fullText: "pronto"},
	}}
	out, _, _ := runWith(t, prov, &checkpointDriver{fakeDriver: reading()}, compactingConfig())
	nudge := prov.inputs[2].Messages[len(prov.inputs[2].Messages)-1]
	if out.Halt != nil || !strings.Contains(nudge.Content, "títulos exatos") {
		t.Fatalf("outcome %+v, reminder %q", out, nudge.Content)
	}
}

func TestACompactionThatNeverFillsTheTemplateStopsTheReply(t *testing.T) {
	prov := &fakeAI{turns: []aiTurn{
		{tcs: []ai.ToolCall{tcall("read")}, usage: full(25_000)},
		{fullText: "resumo solto"},
		{fullText: "de novo solto"},
	}}
	drv := &checkpointDriver{fakeDriver: reading()}
	out, _, _ := runWith(t, prov, drv, compactingConfig())
	if !errors.Is(out.Halt, ErrCompactionFailed) || len(drv.saved) != 0 {
		t.Fatalf("outcome %+v saved %d, want the reply stopped and nothing saved", out, len(drv.saved))
	}
}

func TestACheckpointThatCannotBeSavedStopsTheReply(t *testing.T) {
	prov := &fakeAI{turns: []aiTurn{{tcs: []ai.ToolCall{tcall("read")}, usage: full(25_000)}, {fullText: filledSummary}, {fullText: "pronto"}}}
	out, _, _ := runWith(t, prov, &checkpointDriver{fakeDriver: reading(), err: errors.New("db down")}, compactingConfig())
	if !errors.Is(out.Halt, ErrCompactionFailed) {
		t.Fatalf("outcome %+v, want the reply stopped when the checkpoint is not saved", out)
	}
}

func TestAContextTheProviderRejectsAsTooLongIsCompactedAndRetried(t *testing.T) {
	prov := &fakeAI{turns: []aiTurn{
		{errEvent: fmt.Errorf("%w: prompt is too long", ai.ErrContextOverflow)},
		{fullText: filledSummary},
		{fullText: "pronto"},
	}}
	out, _, sess := runWith(t, prov, &checkpointDriver{fakeDriver: reading()}, compactingConfig())
	if out.Halt != nil || sess.History[len(sess.History)-1].Content != "pronto" {
		t.Fatalf("outcome %+v, want the reply to go on after compacting", out)
	}
}

func TestAnUnknownWindowNeverCompacts(t *testing.T) {
	cfg := compactingConfig()
	cfg.ModelLimits = ai.ModelInfo{}
	prov := &fakeAI{turns: []aiTurn{{tcs: []ai.ToolCall{tcall("read")}, usage: full(900_000)}, {fullText: "pronto"}}}
	runWith(t, prov, &checkpointDriver{fakeDriver: reading()}, cfg)
	if len(prov.inputs) != 2 {
		t.Fatalf("%d calls, want no summary call", len(prov.inputs))
	}
}

func TestASecondCompactionUpdatesTheCheckpoint(t *testing.T) {
	prov := &fakeAI{turns: []aiTurn{{tcs: []ai.ToolCall{tcall("read")}, usage: full(25_000)}, {fullText: filledSummary}, {fullText: "pronto"}}}
	sess := &Session{History: []ai.Message{{Role: ai.RoleUser, Content: checkpointText("## Objetivo\n- antes", "")}}}
	e := Engine{AI: prov}
	e.Run(t.Context(), (&capture{}).emit, &checkpointDriver{fakeDriver: reading()}, compactingConfig(), sess, "continue")
	prompt := prov.inputs[1].Messages[len(prov.inputs[1].Messages)-1].Content
	if !strings.Contains(prompt, "Atualize o resumo anterior") {
		t.Fatalf("an existing checkpoint must be updated, got %q", prompt)
	}
}

func runWith(t *testing.T, prov *fakeAI, drv Driver, cfg Config) (Outcome, *capture, *Session) {
	t.Helper()
	e := Engine{AI: prov}
	cp := &capture{}
	sess := &Session{}
	out := e.Run(t.Context(), cp.emit, drv, cfg, sess, "faça X")
	return out, cp, sess
}

func containsText(msgs []ai.Message, text string) bool {
	for _, m := range msgs {
		if strings.Contains(m.Content, text) {
			return true
		}
	}
	return false
}
