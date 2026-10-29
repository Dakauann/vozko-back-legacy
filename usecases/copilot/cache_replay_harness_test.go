package copilot_usecase

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"

	"vozko/domain/ai"
	"vozko/domain/copilot"
	"vozko/usecases/agentloop"
	"vozko/usecases/agentloop/looptest"
)

type thinkingStep struct {
	calls []ai.ToolCall
	text  string
}

type thinkingAI struct {
	mu     sync.Mutex
	steps  []thinkingStep
	inputs []ai.GenerateInput
	models []ai.ModelInfo
}

func stepThought(i int) []ai.ReasoningBlock {
	return []ai.ReasoningBlock{{Kind: ai.ReasoningText, Text: fmt.Sprintf("plano %d", i), Signature: fmt.Sprintf("sig-%d", i)}}
}

func (p *thinkingAI) Generate(context.Context, ai.GenerateInput) (*ai.GenerateOutput, error) {
	return nil, context.Canceled
}

func (p *thinkingAI) GenerateStream(_ context.Context, in ai.GenerateInput) (<-chan ai.StreamEvent, error) {
	p.mu.Lock()
	i := len(p.inputs)
	p.inputs = append(p.inputs, in)
	p.mu.Unlock()
	var step thinkingStep
	if i < len(p.steps) {
		step = p.steps[i]
	}
	ch := make(chan ai.StreamEvent, 2)
	if step.text != "" {
		ch <- ai.StreamEvent{Type: ai.StreamEventToken, Token: step.text}
	}
	ch <- ai.StreamEvent{Type: ai.StreamEventDone, FullText: step.text, AllToolCalls: step.calls, Reasoning: stepThought(i), Usage: &ai.Usage{}}
	close(ch)
	return ch, nil
}

func (p *thinkingAI) GetAvaibleModels(context.Context) ([]string, error) { return nil, nil }
func (p *thinkingAI) GetModelsWithPricing(context.Context) ([]ai.ModelInfo, error) {
	return p.models, nil
}

func (p *thinkingAI) recorded() []ai.GenerateInput {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]ai.GenerateInput(nil), p.inputs...)
}

const firstAnswerCalls = 5

func TestTheNextAnswerStartsFromExactlyWhatThePreviousAnswerShowedTheModel(t *testing.T) {
	prov := &thinkingAI{
		models: []ai.ModelInfo{toolModel("m", ai.CapabilityVision)},
		steps: []thinkingStep{
			{text: "Vou ler o projeto.", calls: []ai.ToolCall{call("studio_read", nil), call("load_skill", map[string]interface{}{"name": "design-de-imagem"})}},
			{calls: []ai.ToolCall{call("studio_look", nil)}},
			{calls: []ai.ToolCall{call("studio_op_guide", map[string]interface{}{"ops": "set_fill"})}},
			{calls: []ai.ToolCall{call("studio_look", nil)}},
			{text: "Pronto, título azul."},
			{text: "Rodapé ajustado."},
		},
	}
	var rig *queueRig
	look := &fakeTool{name: "studio_look", meta: readMeta, result: copilot.Result{Status: copilot.StatusOK, Data: "quadro", Images: []string{"data:image/jpeg;base64,AAAA"}}}
	read := &fakeTool{name: "studio_read", meta: readMeta, result: copilot.Result{Status: copilot.StatusOK, Data: strings.Repeat("r", 4_000)}}
	skill := &fakeTool{name: "load_skill", meta: readMeta, result: copilot.Result{Status: copilot.StatusOK, Data: strings.Repeat("k", 6_000)}}
	guide := &hookTool{name: "studio_op_guide", run: func() { rig.enqueue(t, "c1", "deixe o título azul") }}
	rig = newQueueRig(prov, nil, openFunds{}, look, read, skill, guide)
	rig.ms.listCreated = true

	rig.answer(t, rig.send("ajuste o título"))
	first := prov.recorded()
	if len(first) != firstAnswerCalls {
		t.Fatalf("the first answer took %d calls, want %d", len(first), firstAnswerCalls)
	}
	looptest.AssertAppendOnly(t, first)
	if !sees(first[3], agentloop.Interjection("deixe o título azul")) {
		t.Fatal("the message sent during the answer must reach it as an interjection")
	}

	rig.answer(t, rig.send("agora o rodapé"))
	calls := prov.recorded()
	if len(calls) != firstAnswerCalls+1 {
		t.Fatalf("the second answer took %d calls, want 1", len(calls)-firstAnswerCalls)
	}
	next := calls[firstAnswerCalls]
	replay := next.Messages[:len(next.Messages)-2]

	var shown []ai.Message
	for _, m := range looptest.Stable(first[firstAnswerCalls-1]) {
		shown = append(shown, agentloop.KeptForm(m))
	}
	shown = append(shown, ai.Message{Role: ai.RoleAssistant, Content: "Pronto, título azul.", Reasoning: stepThought(firstAnswerCalls - 1)})
	if why := looptest.MessagesBreak(agentloop.PruneToolOutputs(shown), replay); why != "" || len(replay) != len(shown) {
		t.Fatalf("the replay of %d messages differs from the %d the model saw: %s", len(replay), len(shown), why)
	}
	if next.ReplayedHistory != len(replay) {
		t.Fatalf("the second answer marks %d replayed messages, want %d", next.ReplayedHistory, len(replay))
	}
}
