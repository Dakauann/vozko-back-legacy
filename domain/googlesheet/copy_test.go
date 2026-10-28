package googlesheet

import (
	"errors"
	"reflect"
	"testing"
)

func TestAPreferredKeySurvivesARenamedHeader(t *testing.T) {
	in := validInput()
	in.Columns = []ColumnInput{
		{Header: "Valor pago", Type: TypeMoney, PreferredKey: "valor"},
		{Header: "Valor", Type: TypeMoney},
		{Header: "Total", Type: TypeMoney, PreferredKey: "valor"},
	}
	s, err := NewSheet(in)
	if err != nil {
		t.Fatalf("NewSheet: %v", err)
	}
	if got := s.keys(); !reflect.DeepEqual(got, []string{"valor", "valor_2", "valor_3"}) {
		t.Fatalf("keys = %v", got)
	}
}

func sourceSheet(t *testing.T) *Sheet {
	t.Helper()
	in := validInput()
	in.TabPeriod, in.TabPattern, in.Style, in.Description = PeriodMonth, "Pedidos · {mes} {ano}", StyleGreen, "Pedidos da loja"
	in.Columns = []ColumnInput{
		{Header: "Nome", Type: TypeText, Required: true, Instruction: "Nome completo"},
		{Header: "Antigo", Type: TypeText},
		{Header: "Status", Type: TypeChoice, Options: []string{"Novo", "Pago"}, Colors: map[string]OptionColor{"Pago": ColorGreen}},
		{Header: "Negociação", Type: TypeLink, Source: SourceExportLink, LinkSheetID: "6f1c3c5d-3766-458c-a6a5-000000000001", LinkLabel: "Ver"},
		{Header: "Telefone", Type: TypePhone, Source: SourceLeadPhone},
	}
	in.Unique = UniqueInput{Mode: UniqueColumns, Columns: []int{4}}
	s, err := NewSheet(in)
	if err != nil {
		t.Fatal(err)
	}
	s.ID = "source"
	s.Columns[0].Header = "Nome do cliente"
	s.Columns[1].Archived = true
	return s
}

func TestACopyKeepsTheDefinitionAndPointsLinksAtTheSourceTabs(t *testing.T) {
	s := sourceSheet(t)
	got := s.Copy()
	want := []ColumnInput{
		{Header: "Nome do cliente", Type: TypeText, Required: true, Instruction: "Nome completo", Source: SourceInput, PreferredKey: "nome"},
		{Header: "Status", Type: TypeChoice, Options: []string{"Novo", "Pago"}, Colors: map[string]OptionColor{"Pago": ColorGreen}, Source: SourceInput, PreferredKey: "status"},
		{Header: "Negociação", Type: TypeLink, Source: SourceExportLink, LinkLabel: "Ver", PreferredKey: "negociacao"},
		{Header: "Telefone", Type: TypePhone, Source: SourceLeadPhone, PreferredKey: "telefone"},
	}
	if !reflect.DeepEqual(got.Columns, want) {
		t.Fatalf("columns = %+v", got.Columns)
	}
	if !reflect.DeepEqual(got.LinkRefs, map[int]string{2: "6f1c3c5d-3766-458c-a6a5-000000000001"}) {
		t.Fatalf("link refs = %v", got.LinkRefs)
	}
	if got.Unique.Mode != UniqueColumns || !reflect.DeepEqual(got.Unique.Columns, []int{3}) || got.Unique.OnRepeat != RepeatSkip {
		t.Fatalf("unique = %+v", got.Unique)
	}
	got.Columns[1].Options[0] = "changed"
	if s.Columns[2].Options[0] != "Novo" {
		t.Fatal("the copy shares options with the source")
	}
}

func TestCopyOrderPutsEveryLinkTargetBeforeTheTabThatLinksToIt(t *testing.T) {
	timeline := &Sheet{ID: "t", Columns: []Column{{Key: "m", Source: SourceMessageText}}}
	payments := &Sheet{ID: "p", Columns: []Column{{Key: "l", Source: SourceExportLink, LinkSheetID: "t"}}}
	notes := &Sheet{ID: "n", Columns: []Column{{Key: "x", Source: SourceInput}}}
	got, err := CopyOrder([]*Sheet{payments, notes, timeline})
	if err != nil {
		t.Fatal(err)
	}
	if ids := []string{got[0].ID, got[1].ID, got[2].ID}; !reflect.DeepEqual(ids, []string{"t", "p", "n"}) {
		t.Fatalf("order = %v", ids)
	}
	if _, err := CopyOrder([]*Sheet{payments}); !errors.Is(err, ErrLinkTargetInvalid) {
		t.Fatalf("a link to a tab outside the copy = %v", err)
	}
	loop := &Sheet{ID: "t", Columns: []Column{{Key: "m", Source: SourceMessageText}, {Key: "back", Source: SourceExportLink, LinkSheetID: "p"}}}
	if _, err := CopyOrder([]*Sheet{payments, loop}); !errors.Is(err, ErrLinkTargetInvalid) {
		t.Fatalf("a link loop = %v", err)
	}
}
