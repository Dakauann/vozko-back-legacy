package agentloop

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"testing"

	"vozko/domain/ai"
	"vozko/domain/tools"
	"vozko/usecases/agentloop/looptest"
)

func assertAppendOnly(t *testing.T, inputs []ai.GenerateInput, rewrites ...int) {
	t.Helper()
	looptest.AssertAppendOnly(t, inputs, rewrites...)
}

type recordedRequest struct {
	fakeDriver
	in ai.GenerateInput
}

func (r *recordedRequest) SystemPrompt() string      { return r.in.SystemPrompt }
func (r *recordedRequest) Tools() []tools.Definition { return r.in.Tools }

type callEstimate struct {
	messages int
	images   int
	tokens   int
	fresh    int
}

func estimateCalls(inputs []ai.GenerateInput) []callEstimate {
	out := make([]callEstimate, len(inputs))
	for i, in := range inputs {
		est := callEstimate{messages: len(in.Messages), tokens: estimateRequest(&recordedRequest{in: in}, in.Messages)}
		for _, m := range in.Messages {
			est.images += len(m.Images)
		}
		est.fresh = est.tokens
		if i > 0 && looptest.SameHead(inputs[i-1], in) {
			est.fresh -= estimateRequest(&recordedRequest{in: in}, in.Messages[:looptest.SharedPrefix(inputs[i-1], in)])
		}
		out[i] = est
	}
	return out
}

func logCallEstimates(t *testing.T, label string, inputs []ai.GenerateInput) []callEstimate {
	t.Helper()
	calls := estimateCalls(inputs)
	t.Logf("%s: estimated input per call (repo estimate, %d chars per token, %d per image)", label, charsPerToken, imageTokenEstimate)
	t.Logf("%5s %9s %7s %9s %9s", "call", "messages", "images", "tokens", "new")
	sum, fresh := 0, 0
	for i, c := range calls {
		t.Logf("%5d %9d %7d %9d %9d", i+1, c.messages, c.images, c.tokens, c.fresh)
		sum += c.tokens
		fresh += c.fresh
	}
	t.Logf("%s: %d calls, %d tokens sent, %d past the reusable prefix", label, len(calls), sum, fresh)
	return calls
}

const (
	harnessSystemChars = 14_800
	harnessSkillChars  = 64_000
	harnessGuideChars  = 12_000
	harnessReadChars   = 40_000
	harnessLookChars   = 3_000
	harnessImage       = "data:image/jpeg;base64,/9j/AAAA"
	harnessInterject   = "deixe o título em azul"
	harnessClosingNote = "o texto do rodapé ficou fora da área segura"
)

var harnessToolSizes = map[string]int{
	"finish":                400,
	"load_skill":            23_600,
	"studio_edit_image":     38_800,
	"studio_export":         10_400,
	"studio_library_search": 7_600,
	"studio_look":           12_000,
	"studio_op_guide":       2_400,
	"studio_read":           3_600,
}

func harnessToolset() []tools.Definition {
	names := make([]string, 0, len(harnessToolSizes))
	for name := range harnessToolSizes {
		names = append(names, name)
	}
	sort.Strings(names)
	defs := make([]tools.Definition, len(names))
	for i, name := range names {
		defs[i] = tools.Definition{Name: name, Description: strings.Repeat("d", harnessToolSizes[name])}
	}
	return defs
}

type studioHarness struct {
	fakeDriver
	defs      []tools.Definition
	drains    int
	arrivals  map[int][]ai.Message
	note      string
	noteCalls int
	looks     int
}

func newStudioHarness() *studioHarness {
	return &studioHarness{
		defs:     harnessToolset(),
		arrivals: map[int][]ai.Message{7: {{Role: ai.RoleUser, Content: Interjection(harnessInterject)}}},
		note:     harnessClosingNote,
	}
}

func (h *studioHarness) SystemPrompt() string      { return strings.Repeat("s", harnessSystemChars) }
func (h *studioHarness) Tools() []tools.Definition { return h.defs }
func (h *studioHarness) Reground(iter, _, _ int) string {
	return fmt.Sprintf("OBSERVAÇÃO DO SISTEMA %d", iter)
}

func (h *studioHarness) Drain(context.Context, *Session) []ai.Message {
	h.drains++
	return h.arrivals[h.drains]
}

func (h *studioHarness) ClosingNote(context.Context, string) string {
	h.noteCalls++
	return h.note
}

func (h *studioHarness) Dispatch(_ context.Context, call ai.ToolCall, _ Emit) StepResult {
	h.dispatched = append(h.dispatched, call.Name)
	switch call.Name {
	case "load_skill":
		return StepResult{Result: strings.Repeat("k", harnessSkillChars)}
	case "studio_op_guide":
		return StepResult{Result: strings.Repeat("g", harnessGuideChars)}
	case "studio_read":
		return StepResult{Result: strings.Repeat("r", harnessReadChars)}
	case "studio_look":
		h.looks++
		return StepResult{Result: fmt.Sprintf(`{"look":%d,"frame":"%s"}`, h.looks, strings.Repeat("l", harnessLookChars)), Images: []string{fmt.Sprintf("%s%d", harnessImage, h.looks)}}
	}
	return StepResult{Result: fmt.Sprintf(`{"applied":%q}`, call.ID), Mutated: true}
}

func harnessCall(name string, args map[string]interface{}) ai.ToolCall {
	return ai.ToolCall{Name: name, Arguments: args}
}

func thought(step int) []ai.ReasoningBlock {
	return []ai.ReasoningBlock{{Kind: ai.ReasoningText, Text: fmt.Sprintf("passo %d: planejo o próximo ajuste", step), Signature: fmt.Sprintf("sig-%d", step)}}
}

func studioAnswerScript() []aiTurn {
	edit := func(op string) ai.ToolCall {
		return harnessCall("studio_edit_image", map[string]interface{}{"operations": []interface{}{map[string]interface{}{"op": op, "id": "layer-1"}}})
	}
	look := harnessCall("studio_look", map[string]interface{}{"target": "artboard"})
	return []aiTurn{
		{thinking: thought(1), fullText: "Vou ler o projeto.", tcs: []ai.ToolCall{harnessCall("studio_read", nil), harnessCall("load_skill", map[string]interface{}{"name": "direcao-de-arte"})}},
		{thinking: thought(2), tcs: []ai.ToolCall{harnessCall("load_skill", map[string]interface{}{"name": "design-de-imagem"}), harnessCall("studio_op_guide", map[string]interface{}{"ops": "add_text"})}},
		{thinking: thought(3), tcs: []ai.ToolCall{edit("add_text"), edit("set_fill")}},
		{thinking: thought(4), tcs: []ai.ToolCall{look}},
		{thinking: thought(5), tcs: []ai.ToolCall{edit("move")}},
		{thinking: thought(6), tcs: []ai.ToolCall{look}},
		{thinking: thought(7), finish: "length"},
		{thinking: thought(8), tcs: []ai.ToolCall{edit("set_fill")}},
		{thinking: thought(9), tcs: []ai.ToolCall{look}},
		{thinking: thought(10), tcs: []ai.ToolCall{harnessCall("studio_read", map[string]interface{}{"ids": []interface{}{"layer-1"}})}},
		{thinking: thought(11), tcs: []ai.ToolCall{harnessCall("studio_op_guide", map[string]interface{}{"ops": "add_shape"}), edit("add_shape")}},
		{thinking: thought(12), tcs: []ai.ToolCall{look}},
		{thinking: thought(13), tcs: []ai.ToolCall{edit("align"), edit("resize")}},
		{thinking: thought(14), tcs: []ai.ToolCall{look}},
		{thinking: thought(15), tcs: []ai.ToolCall{edit("set_text")}},
		{thinking: thought(16), tcs: []ai.ToolCall{look}},
		{thinking: thought(17), fullText: "Pronto: título azul, forma alinhada e texto revisado."},
		{thinking: thought(18), tcs: []ai.ToolCall{edit("move")}},
		{thinking: thought(19), fullText: "Ajustei o rodapé para dentro da área segura."},
	}
}

const harnessToolCalls = 21

func harnessConfig() Config {
	return Config{
		WorkspaceID:      "ws-1",
		BillingReference: "aichat:th-1",
		SessionID:        "aichat:th-1",
		FinishToolName:   "finish",
		IterateUntilDone: true,
		ModelLimits:      ai.ModelInfo{ContextLength: 1_000_000, PromptPrice: 3, CompletionPrice: 15},
	}
}

func earlierAnswer() []ai.Message {
	return []ai.Message{
		{Role: ai.RoleUser, Content: UserRequest("crie um anúncio de inverno")},
		{Role: ai.RoleAssistant, ToolCalls: []ai.ToolCall{{ID: "old-1", Name: "studio_read"}}, Reasoning: thought(0)},
		{Role: ai.RoleTool, ToolCallID: "old-1", Content: strings.Repeat("o", 8_000)},
		{Role: ai.RoleAssistant, Content: "Criei a peça."},
	}
}

func runStudioAnswer(t *testing.T) (*fakeAI, *studioHarness, *Session, Outcome) {
	t.Helper()
	prov := &fakeAI{turns: studioAnswerScript()}
	drv := newStudioHarness()
	sess := &Session{History: earlierAnswer()}
	out := (&Engine{AI: prov}).Run(context.Background(), (&capture{}).emit, drv, harnessConfig(), sess, "ajuste o título e o rodapé")
	if out.Kind != OutcomeIdle {
		t.Fatalf("the scripted answer must end with her reply, got %+v", out)
	}
	if len(drv.dispatched) != harnessToolCalls || len(prov.inputs) != len(studioAnswerScript()) {
		t.Fatalf("dispatched %d tools over %d calls, want %d tools over %d calls", len(drv.dispatched), len(prov.inputs), harnessToolCalls, len(studioAnswerScript()))
	}
	return prov, drv, sess, out
}

func TestALongStudioAnswerOnlyEverAppendsToWhatTheModelSaw(t *testing.T) {
	prov, drv, _, _ := runStudioAnswer(t)
	logCallEstimates(t, "studio answer", prov.inputs)
	assertAppendOnly(t, prov.inputs)
	last := prov.inputs[len(prov.inputs)-1]
	images := 0
	for _, m := range last.Messages {
		images += len(m.Images)
	}
	if images != drv.looks {
		t.Fatalf("the last call carries %d of the %d look images; every look stays until the answer ends", images, drv.looks)
	}
}

func TestTheAnswerRecordsWhatTheModelSawAfterTheRequest(t *testing.T) {
	_, _, sess, _ := runStudioAnswer(t)
	seen := sess.History[len(earlierAnswer())+1:]
	if len(sess.Turn) != len(seen) {
		t.Fatalf("the record holds %d messages, the model saw %d after the request", len(sess.Turn), len(seen))
	}
	kinds := map[string]bool{}
	for i, m := range seen {
		if looptest.Wire(KeptForm(m)) != looptest.Wire(sess.Turn[i]) {
			t.Fatalf("record message %d = %s, want %s", i, looptest.Clip(looptest.Wire(sess.Turn[i])), looptest.Clip(looptest.Wire(KeptForm(m))))
		}
		if m.Role == ai.RoleUser {
			kinds[strings.SplitN(m.Content, "\n", 2)[0]] = true
		}
	}
	for _, want := range []string{toolImagesNote, truncatedReplyNudge, strings.TrimSuffix(interjectionPrefix, "\n"), strings.TrimSuffix(appNoticePrefix, "\n")} {
		if !kinds[want] {
			t.Errorf("the record lost the %q message the model saw", want)
		}
	}
	for _, m := range sess.Turn {
		if len(m.Images) > 0 {
			t.Fatalf("look images never outlive their answer, got %+v", m)
		}
	}
}

func TestEveryCallMarksWhereTheReplayedHistoryEnds(t *testing.T) {
	prov, _, _, _ := runStudioAnswer(t)
	for i, in := range prov.inputs {
		if in.ReplayedHistory != len(earlierAnswer()) {
			t.Fatalf("call %d marks %d replayed messages, want %d", i+1, in.ReplayedHistory, len(earlierAnswer()))
		}
	}
}

func TestAFirstAnswerHasNoReplayedHistoryToMark(t *testing.T) {
	prov := &fakeAI{turns: []aiTurn{{tcs: []ai.ToolCall{tcall("read")}}, {fullText: "pronto"}}}
	(&Engine{AI: prov}).Run(context.Background(), (&capture{}).emit, &fakeDriver{}, harnessConfig(), &Session{}, "oi")
	for i, in := range prov.inputs {
		if in.ReplayedHistory != 0 {
			t.Fatalf("call %d marks %d replayed messages in a new thread", i+1, in.ReplayedHistory)
		}
	}
}

func TestASlidingWindowDropsTheReplayMark(t *testing.T) {
	prov := &fakeAI{turns: []aiTurn{{tcs: []ai.ToolCall{tcall("read")}}, {tcs: []ai.ToolCall{tcall("read")}}, {fullText: "pronto"}}}
	sess := &Session{History: earlierAnswer()}
	(&Engine{AI: prov}).Run(context.Background(), (&capture{}).emit, &fakeDriver{}, Config{FinishToolName: "finish", MaxHistoryMsgs: 7}, sess, "oi")
	if prov.inputs[0].ReplayedHistory != len(earlierAnswer()) {
		t.Fatalf("the first call fits the window and marks %d, want %d", prov.inputs[0].ReplayedHistory, len(earlierAnswer()))
	}
	if last := prov.inputs[len(prov.inputs)-1]; last.ReplayedHistory != 0 {
		t.Fatalf("once the window slides the replayed history is no longer a prefix, got mark %d", last.ReplayedHistory)
	}
}

func TestAPromptNothingElseStoresOpensTheRecord(t *testing.T) {
	prov := &fakeAI{turns: []aiTurn{{fullText: "continuo"}}}
	sess := &Session{History: earlierAnswer(), Frame: AppNotice, RecordPrompt: true}
	(&Engine{AI: prov}).Run(context.Background(), (&capture{}).emit, &fakeDriver{}, harnessConfig(), sess, "o editor abriu")
	if len(sess.Turn) != 2 || sess.Turn[0].Content != AppNotice("o editor abriu") || sess.Turn[1].Content != "continuo" {
		t.Fatalf("record = %+v, want the notice that opened the answer and the reply", sess.Turn)
	}
	stored := &Session{History: earlierAnswer()}
	(&Engine{AI: &fakeAI{turns: []aiTurn{{fullText: "ok"}}}}).Run(context.Background(), (&capture{}).emit, &fakeDriver{}, harnessConfig(), stored, "oi")
	if len(stored.Turn) != 1 || stored.Turn[0].Role != ai.RoleAssistant {
		t.Fatalf("a request stored on its own is not part of the record, got %+v", stored.Turn)
	}
}

func TestAGraceAnswerRecordsTheInstructionItAnswered(t *testing.T) {
	prov := &fakeAI{turns: []aiTurn{{tcs: []ai.ToolCall{tcall("read")}}, {fullText: "resumo"}}}
	sess := &Session{}
	(&Engine{AI: prov}).Run(context.Background(), (&capture{}).emit, &fakeDriver{}, Config{FinishToolName: "finish", MaxIterations: 1, GraceInstruction: "responda agora"}, sess, "oi")
	n := len(sess.Turn)
	if n < 2 || sess.Turn[n-2].Content != "responda agora" || sess.Turn[n-1].Content != "resumo" {
		t.Fatalf("record = %+v, want the instruction followed by her answer", sess.Turn)
	}
	assertAppendOnly(t, prov.inputs)
}
