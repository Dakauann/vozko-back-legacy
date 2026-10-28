package googlesheet

import (
	"errors"
	"testing"
)

const otherRow = "6b1d2c3e-4f5a-4b6c-8d7e-9f0a1b2c3d4e"

func TestFormulaSeparatorFollowsTheDecimalMarkOfTheLocale(t *testing.T) {
	cases := []struct {
		locale string
		want   Separator
	}{
		{"pt_BR", SeparatorSemicolon},
		{"pt_PT", SeparatorSemicolon},
		{"de_DE", SeparatorSemicolon},
		{"es_ES", SeparatorSemicolon},
		{"es_AR", SeparatorSemicolon},
		{"fr_FR", SeparatorSemicolon},
		{"it_IT", SeparatorSemicolon},
		{"nl_NL", SeparatorSemicolon},
		{"ru_RU", SeparatorSemicolon},
		{"pt", SeparatorSemicolon},
		{"en_US", SeparatorComma},
		{"en_GB", SeparatorComma},
		{"en_IN", SeparatorComma},
		{"es_MX", SeparatorComma},
		{"es_419", SeparatorComma},
		{"de_CH", SeparatorComma},
		{"it_CH", SeparatorComma},
		{"ja_JP", SeparatorComma},
		{"zh_CN", SeparatorComma},
		{"en", SeparatorComma},
	}
	for _, c := range cases {
		got, err := FormulaSeparator(c.locale)
		if err != nil || got != c.want {
			t.Errorf("FormulaSeparator(%q) = %q, %v, want %q", c.locale, got, err, c.want)
		}
	}
}

func TestFormulaSeparatorRefusesALocaleItDoesNotKnow(t *testing.T) {
	for _, locale := range []string{"", "xx_YY", "pt-BR", "PT_BR", "en_ZA", " pt_BR"} {
		if got, err := FormulaSeparator(locale); !errors.Is(err, ErrLocaleUnsupported) || got != "" {
			t.Errorf("FormulaSeparator(%q) = %q, %v, want ErrLocaleUnsupported", locale, got, err)
		}
	}
}

func TestTheLocaleTheFilesAreCreatedWithHasAKnownSeparator(t *testing.T) {
	if got, err := FormulaSeparator(DefaultLocale); err != nil || got != SeparatorSemicolon {
		t.Fatalf("FormulaSeparator(DefaultLocale) = %q, %v", got, err)
	}
}

func TestLinkFormulaUsesTheSeparatorOfTheFile(t *testing.T) {
	tab := TabRef{ID: 0, Title: "Negociações"}
	comma, err := LinkFormula(tab, anchorRow, "Ver negociação", SeparatorComma)
	if err != nil {
		t.Fatal(err)
	}
	if want := Formula(`=HYPERLINK("#gid=0&range=B"&MATCH("` + anchorRow + `",'Negociações'!$A:$A,0),"Ver negociação")`); comma != want {
		t.Fatalf("comma formula\n got %s\nwant %s", comma, want)
	}
	semicolon, err := LinkFormula(tab, anchorRow, "Ver negociação", SeparatorSemicolon)
	if err != nil {
		t.Fatal(err)
	}
	if want := Formula(`=HYPERLINK("#gid=0&range=B"&MATCH("` + anchorRow + `";'Negociações'!$A:$A;0);"Ver negociação")`); semicolon != want {
		t.Fatalf("semicolon formula\n got %s\nwant %s", semicolon, want)
	}
	for _, sep := range []Separator{"", ".", "|", ",;"} {
		if _, err := LinkFormula(tab, anchorRow, "Ver", sep); err == nil {
			t.Fatalf("LinkFormula accepted separator %q", sep)
		}
	}
}

func TestRelinkSwapsOnlyTheSeparatorOfALinkFormula(t *testing.T) {
	tab := TabRef{ID: 77, Title: "Negociações d'Ana; 2026, novembro"}
	label := `Ver "negociação", já; agora`
	comma, _ := LinkFormula(tab, anchorRow, label, SeparatorComma)
	semicolon, _ := LinkFormula(tab, anchorRow, label, SeparatorSemicolon)
	got, changed := comma.Relink(SeparatorSemicolon)
	if !changed || got != semicolon {
		t.Fatalf("Relink(;) = %s, %v, want %s", got, changed, semicolon)
	}
	back, changed := semicolon.Relink(SeparatorComma)
	if !changed || back != comma {
		t.Fatalf("Relink(,) = %s, %v, want %s", back, changed, comma)
	}
	if same, changed := semicolon.Relink(SeparatorSemicolon); changed || same != semicolon {
		t.Fatalf("a formula already in the separator changed: %s %v", same, changed)
	}
}

func TestRelinkLeavesEveryOtherFormulaAlone(t *testing.T) {
	formulas := []Formula{
		"",
		"=1+1",
		`=SUM(A1,A2)`,
		`=HYPERLINK("https://example.com","Ver")`,
		`=HYPERLINK("#gid=0&range=B"&MATCH("not-a-row",'T'!$A:$A,0),"Ver")`,
		`=HYPERLINK("#gid=0&range=B"&MATCH("` + anchorRow + `",'T'!$A:$A;0),"Ver")`,
		`=HYPERLINK("#gid=0&range=B"&MATCH("` + anchorRow + `",'T'!$A:$A,0),"Ver")&IMPORTXML("http://x","//a")`,
		`=HYPERLINK("#gid=0&range=B"&MATCH("` + anchorRow + `",'T'!$A:$A,0),"a` + "\n" + `b")`,
		`=HYPERLINK("#gid=0&range=B"&MATCH("` + anchorRow + `",'it's'!$A:$A,0),"Ver")`,
		`=HYPERLINK("#gid=0&range=B"&MATCH("` + anchorRow + `",'T'!$A:$A,0),"say "hi"")`,
	}
	for _, f := range formulas {
		for _, sep := range []Separator{SeparatorComma, SeparatorSemicolon} {
			if got, changed := f.Relink(sep); changed || got != f {
				t.Errorf("Relink(%q) of %s = %s, %v", sep, f, got, changed)
			}
		}
	}
}

func brokenLink(t *testing.T, row int, ref string) LinkCell {
	t.Helper()
	f, err := LinkFormula(TabRef{ID: 3, Title: "Negociações"}, anchorRow, "Ver negociação", SeparatorComma)
	if err != nil {
		t.Fatal(err)
	}
	return LinkCell{Row: row, Column: 3, RowRef: ref, Formula: f, Failed: true}
}

func TestLinkRepairsRewriteOnlyFailedLinkCellsOfVozkoRows(t *testing.T) {
	healthy, _ := LinkFormula(TabRef{ID: 3, Title: "Negociações"}, anchorRow, "Ver negociação", SeparatorSemicolon)
	user := brokenLink(t, 6, "linha do João")
	cells := []LinkCell{
		brokenLink(t, 2, otherRow),
		{Row: 3, Column: 3, RowRef: anchorRow, Formula: healthy},
		{Row: 4, Column: 3, RowRef: anchorRow, Formula: "=SUM(A1,A2)", Failed: true},
		func() LinkCell { c := brokenLink(t, 5, anchorRow); c.Failed = false; return c }(),
		user,
		brokenLink(t, 7, anchorRow),
	}
	fixes, more := LinkRepairs(cells, SeparatorSemicolon, 10)
	if more || len(fixes) != 2 {
		t.Fatalf("fixes = %+v more %v", fixes, more)
	}
	for i, row := range []int{2, 7} {
		if fixes[i].Row != row || fixes[i].Column != 3 || fixes[i].Formula != healthy {
			t.Fatalf("fix %d = %+v, want row %d with %s", i, fixes[i], row, healthy)
		}
	}
}

func TestLinkRepairsStopAtTheBudgetAndSayMoreRemain(t *testing.T) {
	cells := []LinkCell{brokenLink(t, 2, anchorRow), brokenLink(t, 3, otherRow), brokenLink(t, 4, anchorRow)}
	fixes, more := LinkRepairs(cells, SeparatorSemicolon, 2)
	if !more || len(fixes) != 2 || fixes[1].Row != 3 {
		t.Fatalf("fixes = %+v more %v", fixes, more)
	}
	if fixes, more := LinkRepairs(cells, SeparatorSemicolon, 3); more || len(fixes) != 3 {
		t.Fatalf("exact budget = %+v more %v", fixes, more)
	}
	if fixes, more := LinkRepairs(cells, SeparatorComma, 3); more || len(fixes) != 0 {
		t.Fatalf("cells already in the separator were rewritten: %+v %v", fixes, more)
	}
}

func TestLinkGridColumnsAreTheLiveLinkColumnsAfterTheRef(t *testing.T) {
	s, _ := NewSheet(paymentsInput())
	if got := s.LinkGridColumns(); len(got) != 1 || got[0] != 3 {
		t.Fatalf("LinkGridColumns = %v", got)
	}
	s.Columns[2].Archived = true
	if got := s.LinkGridColumns(); len(got) != 0 {
		t.Fatalf("archived link column still scanned: %v", got)
	}
}

func TestAnUnsupportedLocaleFailsTheRowWithItsOwnCode(t *testing.T) {
	refused := &ValueError{Column: "negociacao", Header: "Negociação", Reason: ErrLocaleUnsupported}
	if got := EncodeFailureCode(refused); got != CodeWriteLocaleUnsupported {
		t.Fatalf("EncodeFailureCode = %q", got)
	}
	if got := EncodeFailureCode(&ValueError{Reason: ErrInvalidValue}); got != CodeInvalidValue {
		t.Fatalf("an invalid value = %q", got)
	}
	found := false
	for _, code := range RowErrorCodes() {
		found = found || code == CodeWriteLocaleUnsupported
	}
	if !found {
		t.Fatal("the locale code is not a row error code the front translates")
	}
}

func TestColumnBodyRangeSkipsTheHeader(t *testing.T) {
	for index, want := range map[int]string{0: "A2:A", 3: "D2:D", 25: "Z2:Z", 26: "AA2:AA"} {
		if got := ColumnBodyRange(index); got != want {
			t.Errorf("ColumnBodyRange(%d) = %q, want %q", index, got, want)
		}
	}
}
