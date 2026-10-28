package googlesheet

import (
	"errors"
	"slices"
	"testing"
	"time"
)

func uniquePaymentsInput() NewSheetInput {
	in := validInput()
	in.Title, in.TabName = "Pagamentos", ""
	in.TabPeriod = PeriodMonth
	in.Columns = []ColumnInput{
		{Header: "Matrícula", Type: TypeText},
		{Header: "Valor", Type: TypeMoney, Required: true},
		{Header: "Forma", Type: TypeChoice, Options: []string{"Pix", "Boleto"}},
		{Header: "Cliente", Type: TypeText, Source: SourceLeadName},
		{Header: "Registrado em", Type: TypeDateTime, Source: SourceWrittenAt},
		{Header: "Mensagem", Type: TypeLongText, Source: SourceMessageText},
		{Header: "Pago", Type: TypeBoolean},
	}
	return in
}

func uniqueSheet(t *testing.T, unique UniqueInput) *Sheet {
	t.Helper()
	in := uniquePaymentsInput()
	in.Unique = unique
	s, err := NewSheet(in)
	if err != nil {
		t.Fatalf("NewSheet: %v", err)
	}
	return s
}

func definitionCode(err error) string {
	var def *DefinitionError
	if errors.As(err, &def) {
		return def.Code
	}
	return ""
}

func conversationOrigin(entryID string) Origin {
	return Origin{Kind: OriginAgent, AgentID: "agent-1", EntryID: entryID, EntryType: "whatsapp"}
}

func TestASheetWithoutAUniqueRuleKeepsWritingEveryRecord(t *testing.T) {
	s := uniqueSheet(t, UniqueInput{})
	if s.Unique.Mode != UniqueNone || s.Unique.OnRepeat != RepeatSkip || len(s.Unique.Columns) != 0 {
		t.Fatalf("unique = %+v", s.Unique)
	}
	key, err := s.UniqueKeyOf(Cells{"valor": 10.0}, Origin{Kind: OriginManual, ActorID: "u"}, "2026-10")
	if err != nil || !key.Empty() {
		t.Fatalf("key = %+v, %v", key, err)
	}
}

func TestCreateNamesUniqueColumnsByTheirPositionInTheRequest(t *testing.T) {
	s := uniqueSheet(t, UniqueInput{Mode: UniqueColumns, Columns: []int{0, 2}})
	if !slices.Equal(s.Unique.Columns, []string{"matricula", "forma"}) || s.Unique.OnRepeat != RepeatSkip {
		t.Fatalf("unique = %+v", s.Unique)
	}
}

func TestUniqueRuleDefinitionsAreRefusedWithACode(t *testing.T) {
	cases := []struct {
		name   string
		unique UniqueInput
		code   string
		column int
	}{
		{"unknown mode", UniqueInput{Mode: "sometimes"}, CodeUniqueModeUnknown, -1},
		{"unknown repeat action", UniqueInput{Mode: UniqueConversation, OnRepeat: "update"}, CodeUniqueRepeatUnknown, -1},
		{"columns mode without columns", UniqueInput{Mode: UniqueColumns}, CodeUniqueColumnsRequired, -1},
		{"columns outside columns mode", UniqueInput{Mode: UniqueConversation, Columns: []int{0}}, CodeUniqueColumnsUnexpected, -1},
		{"position past the last column", UniqueInput{Mode: UniqueColumns, Columns: []int{9}}, CodeUniqueColumnUnknown, 9},
		{"written at as key", UniqueInput{Mode: UniqueColumns, Columns: []int{4}}, CodeUniqueColumnNotAllowed, 4},
		{"per message column as key", UniqueInput{Mode: UniqueColumns, Columns: []int{5}}, CodeUniqueColumnNotAllowed, 5},
		{"same column twice", UniqueInput{Mode: UniqueColumns, Columns: []int{0, 0}}, CodeUniqueColumnRepeated, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			in := uniquePaymentsInput()
			in.Unique = c.unique
			_, err := NewSheet(in)
			var def *DefinitionError
			if !errors.As(err, &def) || def.Code != c.code || def.Field != FieldUnique || def.Column != c.column {
				t.Fatalf("err = %#v, want %s at %d", err, c.code, c.column)
			}
			if !errors.Is(err, ErrInvalidSheet) {
				t.Fatalf("a unique refusal must be a definition refusal: %v", err)
			}
		})
	}
}

func TestALinkColumnCannotBeAUniqueKey(t *testing.T) {
	in := uniquePaymentsInput()
	in.Columns = append(in.Columns, ColumnInput{Header: "Negociação", Type: TypeLink, Source: SourceExportLink, LinkSheetID: "8f8e5f7a-3c1d-4a77-9a55-0d6f0f3b2a11"})
	in.Unique = UniqueInput{Mode: UniqueColumns, Columns: []int{len(in.Columns) - 1}}
	if _, err := NewSheet(in); definitionCode(err) != CodeUniqueColumnNotAllowed {
		t.Fatalf("err = %v", err)
	}
}

func TestOnlyInputAndStableSystemSourcesCanBeUniqueKeys(t *testing.T) {
	allowed := UniqueSources()
	for _, source := range []Source{SourceWrittenAt, SourceExportLink, SourceMessageText, SourceMessageSentAt} {
		if slices.Contains(allowed, source) {
			t.Fatalf("%s must not be a unique key source", source)
		}
	}
	for _, source := range []Source{SourceInput, SourceLeadName, SourceLeadPhone, SourceLeadEmail, SourceConversationLink} {
		if !slices.Contains(allowed, source) {
			t.Fatalf("%s must be a unique key source", source)
		}
	}
}

func TestChangingTheUniqueRuleChecksTheSavedColumnsAndBumpsTheVersion(t *testing.T) {
	s := uniqueSheet(t, UniqueInput{})
	if err := s.ChangeUnique(UniqueRule{Mode: UniqueColumns, Columns: []string{"matricula"}}); err != nil {
		t.Fatalf("ChangeUnique: %v", err)
	}
	if s.Version != 2 || s.Unique.OnRepeat != RepeatSkip || !slices.Equal(s.Unique.Columns, []string{"matricula"}) {
		t.Fatalf("sheet = %+v", s)
	}
	if err := s.ChangeUnique(UniqueRule{Mode: UniqueColumns, Columns: []string{"nao_existe"}}); definitionCode(err) != CodeUniqueColumnUnknown {
		t.Fatalf("unknown key = %v", err)
	}
	if err := s.ChangeUnique(UniqueRule{Mode: UniqueColumns, Columns: []string{"registrado_em"}}); definitionCode(err) != CodeUniqueColumnNotAllowed {
		t.Fatalf("written at = %v", err)
	}
	if s.Version != 2 || s.Unique.Columns[0] != "matricula" {
		t.Fatalf("a refused change must leave the sheet as it was: %+v", s.Unique)
	}
}

func TestAnArchivedColumnCannotBeChosenNorArchivedWhileTheRuleUsesIt(t *testing.T) {
	s := uniqueSheet(t, UniqueInput{Mode: UniqueColumns, Columns: []int{0}})
	archived := true
	err := s.EditColumns(ColumnsEdit{Update: []ColumnUpdate{{Key: "matricula", Archived: &archived}}})
	var def *DefinitionError
	if !errors.As(err, &def) || def.Code != CodeUniqueColumnArchived || def.Field != FieldUpdate || def.Key != "matricula" {
		t.Fatalf("archive key column = %#v", err)
	}
	if s.Columns[0].Archived {
		t.Fatal("the refused archive must not apply")
	}
	if err := s.EditColumns(ColumnsEdit{Update: []ColumnUpdate{{Key: "forma", Archived: &archived}}}); err != nil {
		t.Fatalf("archive another column: %v", err)
	}
	if err := s.ChangeUnique(UniqueRule{Mode: UniqueColumns, Columns: []string{"forma"}}); definitionCode(err) != CodeUniqueColumnArchived {
		t.Fatalf("choose archived = %v", err)
	}
}

func TestConversationKeysScopeToTheConversationAndThePeriod(t *testing.T) {
	s := uniqueSheet(t, UniqueInput{Mode: UniqueConversation})
	first, err := s.UniqueKeyOf(Cells{"valor": 10.0}, conversationOrigin("conv-1"), "2026-10")
	if err != nil || first.Empty() || first.Period != "2026-10" || len(first.Hash) != 64 {
		t.Fatalf("key = %+v, %v", first, err)
	}
	other := Origin{Kind: OriginWorkflow, RunID: "run-9", EntryID: " conv-1 ", EntryType: "whatsapp"}
	again, _ := s.UniqueKeyOf(Cells{"valor": 99.0}, other, "2026-10")
	if again != first {
		t.Fatalf("another writer in the same conversation and month must collide: %+v vs %+v", again, first)
	}
	nextMonth, _ := s.UniqueKeyOf(Cells{"valor": 10.0}, conversationOrigin("conv-1"), "2026-11")
	otherChat, _ := s.UniqueKeyOf(Cells{"valor": 10.0}, conversationOrigin("conv-2"), "2026-10")
	if nextMonth.Hash != first.Hash || nextMonth.Period == first.Period || otherChat.Hash == first.Hash {
		t.Fatalf("next month = %+v, other chat = %+v", nextMonth, otherChat)
	}
}

func TestAConversationRuleRefusesAWriteWithoutAConversation(t *testing.T) {
	s := uniqueSheet(t, UniqueInput{Mode: UniqueConversation})
	for name, origin := range map[string]Origin{
		"manual add":       {Kind: OriginManual, ActorID: "user-1"},
		"elo":              {Kind: OriginCopilot, ActorID: "user-1"},
		"workflow on lead": {Kind: OriginWorkflow, EntryID: "lead-1", EntryType: "lead", LeadID: "lead-1"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := s.UniqueKeyOf(Cells{"valor": 1.0}, origin, "2026-10")
			if !errors.Is(err, ErrUniqueNeedsConversation) || ErrorCode(err) != CodeUniqueNeedsConversation {
				t.Fatalf("err = %v", err)
			}
		})
	}
}

func TestColumnKeysCompareValuesTheWayTheyAreStored(t *testing.T) {
	s := uniqueSheet(t, UniqueInput{Mode: UniqueColumns, Columns: []int{0, 2}})
	now := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	first, err := s.Accept(map[string]any{"matricula": "  ABC-123 ", "valor": "10", "forma": "pix"}, nil, now)
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.Accept(map[string]any{"matricula": "abc-123", "valor": 50, "forma": "PIX"}, nil, now)
	if err != nil {
		t.Fatal(err)
	}
	a, err := s.UniqueKeyOf(first, conversationOrigin("conv-1"), "2026-10")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := s.UniqueKeyOf(second, Origin{Kind: OriginManual, ActorID: "u"}, "2026-10")
	if a != b {
		t.Fatalf("the same matricula and option must collide whoever writes: %+v vs %+v", a, b)
	}
	other, _ := s.UniqueKeyOf(Cells{"matricula": "abc-124", "forma": "Pix"}, conversationOrigin("conv-1"), "2026-10")
	if other.Hash == a.Hash {
		t.Fatal("another matricula must not collide")
	}
	conversation := uniqueSheet(t, UniqueInput{Mode: UniqueConversation})
	byChat, _ := conversation.UniqueKeyOf(first, conversationOrigin("conv-1"), "2026-10")
	if byChat.Hash == a.Hash {
		t.Fatal("keys of different modes must never collide")
	}
}

func TestNumbersAndBooleansKeyByTheirStoredValue(t *testing.T) {
	s := uniqueSheet(t, UniqueInput{Mode: UniqueColumns, Columns: []int{1, 6}})
	now := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	first, _ := s.Accept(map[string]any{"valor": "R$ 1.234,50", "pago": "sim"}, nil, now)
	second, _ := s.Accept(map[string]any{"valor": 1234.5, "pago": true}, nil, now)
	a, errA := s.UniqueKeyOf(first, Origin{Kind: OriginManual}, "")
	b, errB := s.UniqueKeyOf(second, Origin{Kind: OriginManual}, "")
	if errA != nil || errB != nil || a != b {
		t.Fatalf("a = %+v %v, b = %+v %v", a, errA, b, errB)
	}
}

func TestAKeyColumnWithoutAValueRefusesTheWrite(t *testing.T) {
	s := uniqueSheet(t, UniqueInput{Mode: UniqueColumns, Columns: []int{0}})
	_, err := s.UniqueKeyOf(Cells{"valor": 10.0}, conversationOrigin("conv-1"), "2026-10")
	var valueErr *ValueError
	if !errors.As(err, &valueErr) || valueErr.Column != "matricula" || valueErr.Header != "Matrícula" || ErrorCode(err) != CodeUniqueValueMissing {
		t.Fatalf("err = %#v", err)
	}
}

func TestARepeatNamesTheExistingRowAndTheRuleThatMatched(t *testing.T) {
	cases := []struct {
		name   string
		period TabPeriod
		unique UniqueInput
		code   string
		header []string
	}{
		{"conversation in a monthly tab", PeriodMonth, UniqueInput{Mode: UniqueConversation}, CodeAlreadyRecordedConversationMonth, nil},
		{"conversation in a single tab", PeriodNone, UniqueInput{Mode: UniqueConversation}, CodeAlreadyRecordedConversation, nil},
		{"columns in a monthly tab", PeriodMonth, UniqueInput{Mode: UniqueColumns, Columns: []int{0, 2}}, CodeAlreadyRecordedColumnsMonth, []string{"Matrícula", "Forma"}},
		{"columns in a single tab", PeriodNone, UniqueInput{Mode: UniqueColumns, Columns: []int{0}}, CodeAlreadyRecordedColumns, []string{"Matrícula"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			in := uniquePaymentsInput()
			in.TabPeriod, in.Unique = c.period, c.unique
			if c.period == PeriodNone {
				in.TabName = "Pagamentos"
			}
			s, err := NewSheet(in)
			if err != nil {
				t.Fatal(err)
			}
			repeat := s.Repeat("row-1")
			if repeat.RowID != "row-1" || repeat.Code != c.code || !slices.Equal(repeat.Headers, c.header) {
				t.Fatalf("repeat = %+v", repeat)
			}
			if !errors.Is(repeat, ErrAlreadyRecorded) || ErrorCode(repeat) != c.code {
				t.Fatalf("code = %s", ErrorCode(repeat))
			}
		})
	}
}

func TestEveryUniqueCodeIsListedForTranslation(t *testing.T) {
	codes := Codes()
	for _, code := range []string{
		CodeUniqueModeUnknown, CodeUniqueColumnsRequired, CodeUniqueColumnsUnexpected, CodeUniqueColumnUnknown, CodeUniqueColumnArchived,
		CodeUniqueColumnNotAllowed, CodeUniqueColumnRepeated, CodeUniqueRepeatUnknown, CodeUniqueNeedsConversation, CodeUniqueValueMissing,
		CodeAlreadyRecordedConversation, CodeAlreadyRecordedConversationMonth, CodeAlreadyRecordedColumns, CodeAlreadyRecordedColumnsMonth,
	} {
		if !slices.Contains(codes, code) {
			t.Fatalf("%s is not listed", code)
		}
	}
}

func TestAStoredRuleWithoutAModeReadsAsNone(t *testing.T) {
	if got := (UniqueRule{}).Normalized(); got.Mode != UniqueNone || got.OnRepeat != RepeatSkip {
		t.Fatalf("normalized = %+v", got)
	}
}

func TestLedgerRowsAreClaimedWithTheSameKeyANewWriteWouldGet(t *testing.T) {
	s := uniqueSheet(t, UniqueInput{Mode: UniqueConversation})
	chat := &Row{ID: "row-1", Cells: Cells{"valor": 10.0}, PeriodKey: "2026-10", Origin: conversationOrigin("conv-1")}
	manual := &Row{ID: "row-2", Cells: Cells{"valor": 20.0}, PeriodKey: "2026-10", Origin: Origin{Kind: OriginManual, ActorID: "u"}}
	claims := s.ClaimsOf([]*Row{chat, manual})
	if len(claims) != 1 || claims[0].Row != chat {
		t.Fatalf("claims = %+v", claims)
	}
	fresh, _ := s.UniqueKeyOf(Cells{"valor": 30.0}, conversationOrigin("conv-1"), "2026-10")
	if claims[0].Key != fresh {
		t.Fatalf("claim = %+v, a new write = %+v", claims[0].Key, fresh)
	}
	if none := uniqueSheet(t, UniqueInput{}).ClaimsOf([]*Row{chat}); len(none) != 0 {
		t.Fatalf("a sheet without a rule claims nothing: %+v", none)
	}
}

func TestTwoRulesAreTheSameWhenTheyKeyTheSameWay(t *testing.T) {
	stored := UniqueRule{Mode: UniqueColumns, Columns: []string{"matricula"}, OnRepeat: RepeatSkip}
	if !stored.Same(UniqueRule{Mode: UniqueColumns, Columns: []string{" matricula "}}) || !(UniqueRule{}).Same(UniqueRule{Mode: UniqueNone, OnRepeat: RepeatSkip}) {
		t.Fatal("equal rules differ")
	}
	if stored.Same(UniqueRule{Mode: UniqueColumns, Columns: []string{"matricula", "forma"}}) || stored.Same(UniqueRule{Mode: UniqueConversation}) {
		t.Fatal("different rules are the same")
	}
}

func TestARepeatExplainsItselfInPortuguese(t *testing.T) {
	month := &RepeatError{Code: CodeAlreadyRecordedConversationMonth}
	columns := &RepeatError{Code: CodeAlreadyRecordedColumns, Headers: []string{"Matrícula", "Forma"}}
	if month.Message() != "Já existe uma linha para esta conversa neste mês." || columns.Message() != "Já existe uma linha com o mesmo valor em Matrícula, Forma nesta aba." {
		t.Fatalf("messages = %q, %q", month.Message(), columns.Message())
	}
}
