package googlesheet

import (
	"errors"
	"testing"
	"time"

	"vozko/domain/conversation"
)

func timelineSheet(t *testing.T) *Sheet {
	t.Helper()
	s, err := NewSheet(NewSheetInput{
		WorkspaceID: "ws-1", AccountID: "acct-1", Title: "Linha do tempo", TimeZone: "America/Sao_Paulo",
		Columns: []ColumnInput{
			{Header: "Quando", Type: TypeDateTime, Source: SourceMessageSentAt},
			{Header: "Quem", Type: TypeText, Source: SourceMessageSender},
			{Header: "Sentido", Type: TypeText, Source: SourceMessageDirection},
			{Header: "Mensagem", Type: TypeLongText, Source: SourceMessageText},
			{Header: "Comprovante", Type: TypeURL, Source: SourceMessageMediaURL},
			{Header: "Cliente", Type: TypeText, Source: SourceLeadName},
			{Header: "Telefone do lead", Type: TypePhone, Source: SourceLeadPhone},
			{Header: "Conversa", Type: TypeURL, Source: SourceConversationLink},
			{Header: "Valor", Type: TypeMoney},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	s.ID = "sheet-1"
	return s
}

func receipt() ExportedMessage {
	return ExportedMessage{
		ID: "msg-2", At: time.Date(2026, 10, 26, 15, 4, 5, 0, time.UTC), Sender: "Ana", Inbound: true,
		Text: "segue o comprovante", MediaURL: "https://cdn.example.com/receipt.jpg",
	}
}

func TestEachMessageFillsTheMessageColumnsAndRepeatsTheCustomerColumns(t *testing.T) {
	s := timelineSheet(t)
	conversationValues := SystemValues{SourceLeadName: "Ana Souza", SourceLeadPhone: "+5511999990000", SourceConversationLink: "https://app.example.com/c/1"}

	cells, err := s.Accept(map[string]any{"valor": "120,50"}, s.WithMessage(conversationValues, receipt()), acceptNow)
	if err != nil {
		t.Fatal(err)
	}
	want := Cells{
		"quando": "2026-10-26T12:04:05-03:00", "quem": "Ana", "sentido": DirectionReceived, "mensagem": "segue o comprovante",
		"comprovante": "https://cdn.example.com/receipt.jpg", "cliente": "Ana Souza", "telefone_do_lead": "+5511999990000",
		"conversa": "https://app.example.com/c/1",
	}
	for key, value := range want {
		if cells[key] != value {
			t.Fatalf("%s = %v, want %v (all: %v)", key, cells[key], value, cells)
		}
	}
	if _, touched := conversationValues[SourceMessageText]; touched {
		t.Fatal("the conversation values are shared by every row and must not carry one message")
	}

	reply := ExportedMessage{ID: "msg-3", At: receipt().At.Add(time.Minute), Sender: "Assistente", Text: "recebido"}
	cells, err = s.Accept(nil, s.WithMessage(conversationValues, reply), acceptNow)
	if err != nil || cells["sentido"] != DirectionSent || cells["comprovante"] != nil || cells["cliente"] != "Ana Souza" {
		t.Fatalf("reply cells = %v, %v", cells, err)
	}
}

func TestMessageColumnsAreFilledPerMessageSoTheyAreNeverRequired(t *testing.T) {
	_, err := NewSheet(NewSheetInput{
		WorkspaceID: "ws-1", AccountID: "acct-1", Title: "Linha do tempo",
		Columns: []ColumnInput{
			{Header: "Mensagem", Type: TypeLongText, Source: SourceMessageText, Required: true},
			{Header: "Nota", Type: TypeText},
		},
	})
	if !errors.Is(err, ErrInvalidSheet) {
		t.Fatalf("err = %v: a row written outside an export has no message, so a required message column would refuse it", err)
	}
	for _, source := range []Source{SourceMessageSentAt, SourceMessageSender, SourceMessageDirection, SourceMessageText, SourceMessageMediaURL} {
		if !source.Valid() || !source.IsSystem() || !source.PerMessage() {
			t.Fatalf("%s must be a per-message system source", source)
		}
	}
	if SourceLeadName.PerMessage() || SourceInput.PerMessage() {
		t.Fatal("only message sources change from row to row")
	}
}

func TestOnlyASheetWithMessageColumnsTakesAnExport(t *testing.T) {
	if !timelineSheet(t).HasMessageColumns() {
		t.Fatal("the timeline sheet has message columns")
	}
	if typedSheet(t).HasMessageColumns() {
		t.Fatal("a sheet without message columns would write the same row once per message")
	}
	s := timelineSheet(t)
	for i := range s.Columns {
		if s.Columns[i].Source.PerMessage() {
			s.Columns[i].Archived = true
		}
	}
	if s.HasMessageColumns() {
		t.Fatal("archived message columns are not written")
	}
}

func TestOnlyWhatTheCustomerOrTheBusinessSaidIsExported(t *testing.T) {
	media := &conversation.AttachedMedia{URL: "https://cdn.example.com/a.jpg"}
	retracted := time.Now()
	at := time.Date(2026, 10, 26, 10, 0, 0, 0, time.UTC)
	cases := []struct {
		name    string
		message *conversation.Message
		ok      bool
		want    ExportedMessage
	}{
		{"a customer text", &conversation.Message{ID: "m1", MessageType: conversation.MessageTypeUserMessage, Text: "oi", SentBy: conversation.SentByContact("5511"), SenderName: "Ana", CreatedAt: at}, true,
			ExportedMessage{ID: "m1", At: at, Sender: "Ana", Inbound: true, Text: "oi"}},
		{"an image with no caption", &conversation.Message{ID: "m2", MessageType: conversation.MessageTypeMedia, MediaType: conversation.MediaTypeImage, Media: media, SentBy: conversation.SentByContact("5511"), CreatedAt: at}, true,
			ExportedMessage{ID: "m2", At: at, Sender: "Cliente", Inbound: true, MediaURL: media.URL}},
		{"the agent's reply", &conversation.Message{ID: "m3", MessageType: conversation.MessageTypeAIResponse, Text: "recebido", SentBy: conversation.SentByAI("agent-1"), SenderName: "Assistente", CreatedAt: at}, true,
			ExportedMessage{ID: "m3", At: at, Sender: "Assistente", Text: "recebido"}},
		{"a tool call", &conversation.Message{ID: "m4", MessageType: conversation.MessageTypeToolCall, Text: "{}", SentBy: conversation.SentByAI("agent-1"), CreatedAt: at}, false, ExportedMessage{}},
		{"a tool result", &conversation.Message{ID: "m5", MessageType: conversation.MessageTypeToolResult, Text: "{}", CreatedAt: at}, false, ExportedMessage{}},
		{"a system note", &conversation.Message{ID: "m6", MessageType: conversation.MessageTypeSystem, Text: "x", CreatedAt: at}, false, ExportedMessage{}},
		{"a retracted message", &conversation.Message{ID: "m7", MessageType: conversation.MessageTypeUserMessage, Text: "apaguei", Media: media, RetractedAt: &retracted, SentBy: conversation.SentByContact("5511"), CreatedAt: at}, false, ExportedMessage{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := ExportedMessageOf(tc.message)
			if ok != tc.ok || got != tc.want {
				t.Fatalf("ExportedMessageOf = %+v, %v; want %+v, %v", got, ok, tc.want, tc.ok)
			}
		})
	}
}

func TestExportPositionsOrderByTimeThenID(t *testing.T) {
	at := time.Date(2026, 10, 26, 10, 0, 0, 0, time.UTC)
	earlier, sameTimeLaterID, later := ExportPosition{At: at, MessageID: "a"}, ExportPosition{At: at, MessageID: "b"}, ExportPosition{At: at.Add(time.Microsecond), MessageID: "a"}
	if !earlier.Before(sameTimeLaterID) || !sameTimeLaterID.Before(later) || later.Before(earlier) || earlier.Before(earlier) {
		t.Fatal("positions must order by time, then by message id")
	}
	if !(ExportPosition{}).IsZero() || earlier.IsZero() {
		t.Fatal("only the empty position is zero")
	}
}

func TestMessageRowsKeepTheConversationOrderAndOneKeyPerMessage(t *testing.T) {
	s := timelineSheet(t)
	origin := Origin{Kind: OriginWorkflow, WorkflowID: "wf-1", RunID: "run-1", EntryID: "conv-1", EntryType: "whatsapp"}
	rows, err := NewMessageRows(s, []MessageRow{{MessageID: "m1", Cells: Cells{"quem": "x"}}, {MessageID: "m2", Cells: Cells{"quem": "y"}}}, origin, acceptNow)
	if err != nil || len(rows) != 2 {
		t.Fatalf("rows = %v, %v", rows, err)
	}
	if !rows[0].CreatedAt.Before(rows[1].CreatedAt) {
		t.Fatal("the ledger orders by creation time, so rows must be created in message order")
	}
	if rows[0].IdempotencyKey != KeyForMessage("whatsapp", "conv-1", "m1") || rows[1].IdempotencyKey == rows[0].IdempotencyKey {
		t.Fatalf("keys = %q, %q", rows[0].IdempotencyKey, rows[1].IdempotencyKey)
	}
	if KeyForMessage("whatsapp", "conv-1", "m1") != "msg:whatsapp:conv-1:m1" {
		t.Fatalf("message key = %q", KeyForMessage("whatsapp", "conv-1", "m1"))
	}
	if _, err := NewMessageRows(s, []MessageRow{{MessageID: "", Cells: Cells{}}}, origin, acceptNow); !errors.Is(err, ErrIdempotencyKeyRequired) {
		t.Fatalf("a row with no message id was accepted: %v", err)
	}
}
