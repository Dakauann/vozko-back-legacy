package googlesheet

import (
	"errors"
	"strings"
	"testing"
)

func TestNewSheetRefusesEachDefinitionProblemWithItsOwnCode(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*NewSheetInput)
		code   string
		field  string
		column int
	}{
		{"no title", func(in *NewSheetInput) { in.Title = " " }, CodeTitleRequired, FieldTitle, -1},
		{"title too long", func(in *NewSheetInput) { in.Title = strings.Repeat("a", MaxTitleRunes+1) }, CodeTitleTooLong, FieldTitle, -1},
		{"no account", func(in *NewSheetInput) { in.AccountID = "" }, CodeAccountRequired, FieldAccount, -1},
		{"no workspace", func(in *NewSheetInput) { in.WorkspaceID = "" }, CodeWorkspaceRequired, FieldWorkspace, -1},
		{"no columns", func(in *NewSheetInput) { in.Columns = nil }, CodeNoColumns, FieldColumns, -1},
		{"duplicate header", func(in *NewSheetInput) { in.Columns[1].Header = "nome do cliente" }, CodeHeaderRepeated, FieldColumns, 1},
		{"blank header", func(in *NewSheetInput) { in.Columns[1].Header = "  " }, CodeHeaderRequired, FieldColumns, 1},
		{"header too long", func(in *NewSheetInput) { in.Columns[1].Header = strings.Repeat("h", MaxHeaderRunes+1) }, CodeHeaderTooLong, FieldColumns, 1},
		{"choice without options", func(in *NewSheetInput) { in.Columns[2].Options = nil }, CodeOptionsRequired, FieldColumns, 2},
		{"repeated option", func(in *NewSheetInput) { in.Columns[2].Options = []string{"Novo", "novo"} }, CodeOptionRepeated, FieldColumns, 2},
		{"blank option", func(in *NewSheetInput) { in.Columns[2].Options = []string{"Novo", " "} }, CodeOptionBlank, FieldColumns, 2},
		{"option too long", func(in *NewSheetInput) { in.Columns[2].Options = []string{strings.Repeat("o", MaxOptionRunes+1)} }, CodeOptionTooLong, FieldColumns, 2},
		{"options on a text", func(in *NewSheetInput) { in.Columns[0].Options = []string{"a"} }, CodeOptionsNotChoice, FieldColumns, 0},
		{"unknown type", func(in *NewSheetInput) { in.Columns[0].Type = "currency" }, CodeTypeUnknown, FieldColumns, 0},
		{"link type typed by hand", func(in *NewSheetInput) { in.Columns[0].Type = TypeLink }, CodeTypeUnknown, FieldColumns, 0},
		{"unknown source", func(in *NewSheetInput) { in.Columns[3].Source = "lead_cpf" }, CodeSourceUnknown, FieldColumns, 3},
		{"source of the wrong type", func(in *NewSheetInput) { in.Columns[4].Type = TypeNumber }, CodeSourceTypeMismatch, FieldColumns, 4},
		{"instruction too long", func(in *NewSheetInput) { in.Columns[0].Instruction = strings.Repeat("a", MaxInstructionRunes+1) }, CodeInstructionTooLong, FieldColumns, 0},
		{"per message required", func(in *NewSheetInput) {
			in.Columns = append(in.Columns, ColumnInput{Header: "Texto", Type: TypeLongText, Source: SourceMessageText, Required: true})
		}, CodePerMessageRequired, FieldColumns, 5},
		{"unknown time zone", func(in *NewSheetInput) { in.TimeZone = "Mars/Olympus" }, CodeTimeZoneUnknown, FieldTimeZone, -1},
		{"bad locale", func(in *NewSheetInput) { in.Locale = "portuguese" }, CodeLocaleInvalid, FieldLocale, -1},
		{"tab name with quote", func(in *NewSheetInput) { in.TabName = "Pedidos'" }, CodeTabNameForbidden, FieldTabName, -1},
		{"tab name too long", func(in *NewSheetInput) { in.TabName = strings.Repeat("t", MaxTabRunes+1) }, CodeTabNameTooLong, FieldTabName, -1},
		{"unknown style", func(in *NewSheetInput) { in.Style = "neon" }, CodeStyleUnknown, FieldStyle, -1},
		{"unknown period", func(in *NewSheetInput) { in.TabPeriod = "week" }, CodeTabPeriodUnknown, FieldTabPeriod, -1},
		{"pattern without period", func(in *NewSheetInput) { in.TabPattern = "Pedidos {mes} {ano}" }, CodeTabPatternNotPeriodic, FieldTabPattern, -1},
		{"colour on a text", func(in *NewSheetInput) { in.Columns[0].Colors = map[string]OptionColor{"a": ColorGreen} }, CodeColorsNotChoice, FieldColumns, 0},
		{"colour of no option", func(in *NewSheetInput) { in.Columns[2].Colors = map[string]OptionColor{"Cancelado": ColorRed} }, CodeColorNotOption, FieldColumns, 2},
		{"unknown colour", func(in *NewSheetInput) { in.Columns[2].Colors = map[string]OptionColor{"Novo": "pink"} }, CodeColorUnknown, FieldColumns, 2},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			in := validInput()
			c.mutate(&in)
			_, err := NewSheet(in)
			if !errors.Is(err, ErrInvalidSheet) {
				t.Fatalf("err = %v, want ErrInvalidSheet", err)
			}
			var def *DefinitionError
			if !errors.As(err, &def) {
				t.Fatalf("err = %T %v, want a DefinitionError", err, err)
			}
			if def.Code != c.code || ErrorCode(err) != c.code {
				t.Fatalf("code = %q (ErrorCode %q), want %q", def.Code, ErrorCode(err), c.code)
			}
			if def.Field != c.field || def.Column != c.column {
				t.Fatalf("field %q column %d, want %q %d", def.Field, def.Column, c.field, c.column)
			}
		})
	}
}

func TestDefinitionErrorsNameTheColumnAndTheLimit(t *testing.T) {
	in := validInput()
	in.Columns[1].Header = strings.Repeat("h", MaxHeaderRunes+1)
	_, err := NewSheet(in)
	var def *DefinitionError
	if !errors.As(err, &def) || def.Limit != MaxHeaderRunes {
		t.Fatalf("err = %+v, want the header limit", def)
	}
	in = validInput()
	in.Columns[2].Options = []string{"Novo", "novo"}
	_, err = NewSheet(in)
	if !errors.As(err, &def) || def.Header != "Status" || def.Option != "novo" {
		t.Fatalf("err = %+v, want header Status and option novo", def)
	}
}

func TestEveryDefinitionCodeIsListedForTranslation(t *testing.T) {
	listed := map[string]bool{}
	for _, code := range Codes() {
		listed[code] = true
	}
	for _, code := range DefinitionCodes() {
		if !listed[code] {
			t.Fatalf("%s is not in Codes()", code)
		}
	}
}

func TestATimelineMadeOnlyOfSystemColumnsIsAValidSheet(t *testing.T) {
	s, err := NewSheet(NewSheetInput{
		WorkspaceID: "ws-1", AccountID: "acct-1", Title: "Negociações",
		Columns: []ColumnInput{
			{Header: "Data", Type: TypeDateTime, Source: SourceMessageSentAt},
			{Header: "Quem", Type: TypeText, Source: SourceMessageSender},
			{Header: "Sentido", Type: TypeText, Source: SourceMessageDirection},
			{Header: "Texto", Type: TypeLongText, Source: SourceMessageText},
			{Header: "Mídia", Type: TypeURL, Source: SourceMessageMediaURL},
			{Header: "Conversa", Type: TypeURL, Source: SourceConversationLink},
			{Header: "Registrado", Type: TypeDateTime, Source: SourceWrittenAt},
			{Header: "Cliente", Type: TypeText, Source: SourceLeadName},
			{Header: "Telefone", Type: TypePhone, Source: SourceLeadPhone},
		},
	})
	if err != nil {
		t.Fatalf("a timeline of system columns was refused: %v", err)
	}
	if s.TakesInput() {
		t.Fatal("a sheet without input columns says it takes input")
	}
	if !s.HasMessageColumns() {
		t.Fatal("timeline lost its message columns")
	}
}

func TestArchivingEveryInputColumnIsAllowed(t *testing.T) {
	s, _ := NewSheet(validInput())
	archive := true
	err := s.EditColumns(ColumnsEdit{Update: []ColumnUpdate{
		{Key: "nome_do_cliente", Archived: &archive}, {Key: "valor", Archived: &archive}, {Key: "status", Archived: &archive},
	}})
	if err != nil {
		t.Fatalf("archiving inputs was refused: %v", err)
	}
	if s.TakesInput() {
		t.Fatal("TakesInput with every input archived")
	}
}

func TestEditColumnsRefusesWithTheColumnKey(t *testing.T) {
	s, _ := NewSheet(validInput())
	err := s.EditColumns(ColumnsEdit{Update: []ColumnUpdate{{Key: "status", Options: []string{"Novo"}}}})
	var def *DefinitionError
	if !errors.As(err, &def) || def.Code != CodeOptionRemoved || def.Key != "status" || def.Option != "Pago" {
		t.Fatalf("err = %+v", err)
	}
	err = s.EditColumns(ColumnsEdit{})
	if !errors.As(err, &def) || def.Code != CodeEditEmpty {
		t.Fatalf("empty edit = %v", err)
	}
	err = s.EditColumns(ColumnsEdit{Add: []ColumnInput{{Header: "Status", Type: TypeText}}})
	if !errors.As(err, &def) || def.Code != CodeHeaderRepeated || def.Column != 0 || def.Field != FieldAdd {
		t.Fatalf("repeated added header = %+v", def)
	}
}

func TestCheckRowInputRefusesASheetWithoutInputColumns(t *testing.T) {
	s, _ := NewSheet(validInput())
	if err := s.CheckTakesInput(); err != nil {
		t.Fatalf("CheckTakesInput = %v", err)
	}
	archive := true
	_ = s.EditColumns(ColumnsEdit{Update: []ColumnUpdate{
		{Key: "nome_do_cliente", Archived: &archive}, {Key: "valor", Archived: &archive}, {Key: "status", Archived: &archive},
	}})
	if err := s.CheckTakesInput(); !errors.Is(err, ErrNoInputColumns) || ErrorCode(err) != CodeNoInputColumns {
		t.Fatalf("CheckTakesInput = %v", err)
	}
}
