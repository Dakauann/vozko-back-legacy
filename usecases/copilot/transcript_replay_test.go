package copilot_usecase

import (
	"context"
	"strings"
	"testing"

	"vozko/domain/ai"
	"vozko/domain/aichat"
	"vozko/domain/copilot"
	"vozko/usecases/agentloop"
)

func TestAReplyStoresItsTranscriptForTheNextOne(t *testing.T) {
	th := &fakeThreads{thread: testThread()}
	ms := &fakeMessages{}
	rt := &fakeTool{name: "list_agents", meta: readMeta}
	prov := &scriptAI{turns: [][]ai.ToolCall{{call("list_agents", nil)}, {}}, texts: []string{"", "Você tem 3 agentes."}}
	if err := newService(prov, th, ms, rt).Stream(context.Background(), th.thread, copilot.UserMessage{Content: "quantos agentes?"}, ownerCtx, (&capture{}).emit); err != nil {
		t.Fatal(err)
	}
	turn, err := aichat.DecodeTranscript(ms.last().Transcript)
	if err != nil {
		t.Fatal(err)
	}
	if len(turn) != 3 || turn[0].ToolCalls[0].Name != "list_agents" || turn[1].Role != ai.RoleTool || turn[2].Content != "Você tem 3 agentes." {
		t.Fatalf("transcript = %+v, want the call, its result and the answer", turn)
	}
}

func storedTurn(t *testing.T, model string) *aichat.Message {
	t.Helper()
	raw, err := aichat.EncodeTranscript([]ai.Message{
		{Role: ai.RoleAssistant, ToolCalls: []ai.ToolCall{{ID: "c1", Name: "list_agents"}}, Reasoning: []ai.ReasoningBlock{{Kind: ai.ReasoningText, Text: "plano", Signature: "sig"}}},
		{Role: ai.RoleTool, ToolCallID: "c1", Content: `{"total":3}`},
		{Role: ai.RoleAssistant, Content: "Você tem 3 agentes."},
	})
	if err != nil {
		t.Fatal(err)
	}
	return &aichat.Message{Role: aichat.RoleAssistant, Content: "Você tem 3 agentes.", Model: model, Transcript: raw}
}

func TestTheNextReplyReplaysThePreviousTranscript(t *testing.T) {
	ms := &fakeMessages{list: []*aichat.Message{{Role: aichat.RoleUser, Content: "quantos agentes?"}, storedTurn(t, "m")}}
	h, err := newService(&scriptAI{}, &fakeThreads{thread: testThread()}, ms).buildHistory("t1", "m", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(h) != 4 || h[1].ToolCalls[0].Name != "list_agents" || len(h[1].Reasoning) != 1 || h[2].Content != `{"total":3}` || h[3].Content != "Você tem 3 agentes." {
		t.Fatalf("history = %+v, want the request and the whole previous reply", h)
	}
}

func TestAnotherModelGetsThePreviousReplyWithoutItsReasoning(t *testing.T) {
	ms := &fakeMessages{list: []*aichat.Message{{Role: aichat.RoleUser, Content: "quantos agentes?"}, storedTurn(t, "outro")}}
	h, err := newService(&scriptAI{}, &fakeThreads{thread: testThread()}, ms).buildHistory("t1", "m", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(h) != 4 || len(h[1].Reasoning) != 0 || h[1].ToolCalls[0].Name != "list_agents" {
		t.Fatalf("history = %+v, want the tool steps without reasoning", h)
	}
}

func TestAnUnreadableTranscriptFailsTheRebuildInsteadOfGuessing(t *testing.T) {
	broken := &aichat.Message{Role: aichat.RoleAssistant, Content: "x", Model: "m", Transcript: []byte("{broken")}
	ms := &fakeMessages{list: []*aichat.Message{broken}}
	if _, err := newService(&scriptAI{}, &fakeThreads{thread: testThread()}, ms).buildHistory("t1", "m", true); err == nil {
		t.Fatal("an unreadable transcript must fail closed")
	}
}

func storedSegment(t *testing.T, turn ...ai.Message) *aichat.Message {
	t.Helper()
	raw, err := aichat.EncodeTranscript(turn)
	if err != nil {
		t.Fatal(err)
	}
	return &aichat.Message{Role: aichat.RoleAssistant, Model: "m", Transcript: raw}
}

func replayed(t *testing.T, list ...*aichat.Message) []ai.Message {
	t.Helper()
	h, err := newService(&scriptAI{}, &fakeThreads{thread: testThread()}, &fakeMessages{list: list}).buildHistory("t1", "m", true)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func contents(h []ai.Message) []string {
	out := make([]string, len(h))
	for i, m := range h {
		out[i] = string(m.Role) + ": " + m.Content
	}
	return out
}

func TestAMessageDeliveredBetweenSegmentsReplaysAsTheInterjectionTheModelSaw(t *testing.T) {
	read := ai.Message{Role: ai.RoleAssistant, ToolCalls: []ai.ToolCall{{ID: "c1", Name: "studio_read"}}}
	h := replayed(t,
		&aichat.Message{Role: aichat.RoleUser, Content: "monte o anúncio"},
		storedSegment(t, read, ai.Message{Role: ai.RoleTool, ToolCallID: "c1", Content: "lido"}),
		&aichat.Message{Role: aichat.RoleUser, Content: "use azul", Attachments: []byte(`[{"mediaId":"md1","name":"logo.png","kind":"image","url":"https://cdn/logo.png"}]`)},
		storedSegment(t, ai.Message{Role: ai.RoleUser, Content: agentloop.Interjection("use azul")}, ai.Message{Role: ai.RoleAssistant, Content: "Feito."}),
	)
	want := []string{"user: " + agentloop.UserRequest("monte o anúncio"), "assistant: ", "tool: lido", "user: " + userMessage(&aichat.Message{Content: "use azul", Attachments: []byte(`[{"mediaId":"md1","name":"logo.png","kind":"image","url":"https://cdn/logo.png"}]`)}, agentloop.Interjection, true).Content, "assistant: Feito."}
	if got := contents(h); strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("history = %q, want %q", got, want)
	}
	if len(h[3].Images) != 1 || h[3].Images[0] != "https://cdn/logo.png" {
		t.Fatalf("the delivered message keeps its attachment, got %+v", h[3])
	}
}

func TestAMessageDeliveredBeforeAnythingWasSavedReplaysInsideTheReply(t *testing.T) {
	h := replayed(t,
		&aichat.Message{Role: aichat.RoleUser, Content: "monte o anúncio"},
		&aichat.Message{Role: aichat.RoleUser, Content: "use azul"},
		storedSegment(t,
			ai.Message{Role: ai.RoleAssistant, ToolCalls: []ai.ToolCall{{ID: "c1", Name: "studio_read"}}},
			ai.Message{Role: ai.RoleTool, ToolCallID: "c1", Content: "lido"},
			ai.Message{Role: ai.RoleUser, Content: agentloop.Interjection("use azul")},
			ai.Message{Role: ai.RoleAssistant, Content: "Feito."},
		),
	)
	want := []string{"user: " + agentloop.UserRequest("monte o anúncio"), "assistant: ", "tool: lido", "user: " + agentloop.Interjection("use azul"), "assistant: Feito."}
	if got := contents(h); strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("history = %q, want %q", got, want)
	}
}

func TestTheNotesAReplySawReplayWhereTheModelSawThem(t *testing.T) {
	h := replayed(t,
		&aichat.Message{Role: aichat.RoleUser, Content: "monte o e-mail"},
		storedSegment(t,
			ai.Message{Role: ai.RoleAssistant, Content: "Pronto."},
			ai.Message{Role: ai.RoleUser, Content: agentloop.AppNotice("o botão não tem link")},
			ai.Message{Role: ai.RoleAssistant, Content: "Falta o link do botão."},
		),
		&aichat.Message{Role: aichat.RoleUser, Content: "o link é vozko.com"},
	)
	want := []string{"user: " + agentloop.UserRequest("monte o e-mail"), "assistant: Pronto.", "user: " + agentloop.AppNotice("o botão não tem link"), "assistant: Falta o link do botão.", "user: " + agentloop.UserRequest("o link é vozko.com")}
	if got := contents(h); strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("history = %q, want %q", got, want)
	}
}

func TestAResumedAnswerKeepsTheNoticeThatOpenedIt(t *testing.T) {
	th, ms := &fakeThreads{thread: testThread()}, &fakeMessages{}
	svc := newService(&scriptAI{texts: []string{"Continuo."}}, th, ms)
	if err := svc.runTurn(context.Background(), th.thread, turnStart{content: "o editor abriu", notice: true}, ownerCtx, (&capture{}).emit); err != nil {
		t.Fatal(err)
	}
	turn, err := aichat.DecodeTranscript(ms.last().Transcript)
	if err != nil {
		t.Fatal(err)
	}
	if len(turn) != 2 || turn[0].Content != agentloop.AppNotice("o editor abriu") || turn[1].Content != "Continuo." {
		t.Fatalf("transcript = %+v, want the notice nobody else stores, then the reply", turn)
	}
}
