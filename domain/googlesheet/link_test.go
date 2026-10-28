package googlesheet

import (
	"errors"
	"testing"
	"time"
)

const (
	timelineID = "8f8e5f7a-3c1d-4a77-9a55-0d6f0f3b2a11"
	anchorRow  = "2a4c9b0e-7d3f-4c1e-8b6a-1f2e3d4c5b6a"
)

func paymentsInput() NewSheetInput {
	return NewSheetInput{
		WorkspaceID: "ws-1", AccountID: "acct-1", Title: "Pagamentos",
		Columns: []ColumnInput{
			{Header: "Valor", Type: TypeMoney},
			{Header: "Baixa", Type: TypeChoice, Options: []string{"SIM", "NÃO"}, Colors: map[string]OptionColor{"SIM": ColorGreen, "NÃO": ColorRed}},
			{Header: "Negociação", Type: TypeLink, Source: SourceExportLink, LinkSheetID: timelineID},
		},
	}
}

func TestLinkColumnRules(t *testing.T) {
	s, err := NewSheet(paymentsInput())
	if err != nil {
		t.Fatalf("NewSheet: %v", err)
	}
	link, _ := s.Column("negociacao")
	if link.LinkLabel != DefaultLinkLabel || link.LinkSheetID != timelineID {
		t.Fatalf("link = %+v", link)
	}
	cases := []struct {
		name   string
		mutate func(*ColumnInput)
		code   string
	}{
		{"no target", func(c *ColumnInput) { c.LinkSheetID = "" }, CodeLinkTargetRequired},
		{"target not an id", func(c *ColumnInput) { c.LinkSheetID = "Negociações" }, CodeLinkTargetRequired},
		{"label too long", func(c *ColumnInput) { c.LinkLabel = string(make([]rune, MaxLinkLabelRunes+1)) }, CodeLinkLabelInvalid},
		{"label with a line break", func(c *ColumnInput) { c.LinkLabel = "Ver\nmensagens" }, CodeLinkLabelInvalid},
		{"wrong type", func(c *ColumnInput) { c.Type = TypeURL }, CodeSourceTypeMismatch},
		{"required link", func(c *ColumnInput) { c.Required = true }, CodeLinkRequired},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			in := paymentsInput()
			c.mutate(&in.Columns[2])
			_, err := NewSheet(in)
			if ErrorCode(err) != c.code {
				t.Fatalf("err = %v, want %s", err, c.code)
			}
		})
	}
	in := paymentsInput()
	in.Columns[0].LinkSheetID = timelineID
	if _, err := NewSheet(in); ErrorCode(err) != CodeLinkFieldsNotLink {
		t.Fatalf("link target on a money column = %v", err)
	}
}

func TestCheckLinkTarget(t *testing.T) {
	payments, _ := NewSheet(paymentsInput())
	payments.ID, payments.SpreadsheetID = "pay", "file-1"
	timeline := timelineSheet(t)
	timeline.ID, timeline.SpreadsheetID = timelineID, "file-1"
	link, _ := payments.Column("negociacao")
	if err := CheckLinkTarget(payments, link, timeline); err != nil {
		t.Fatalf("valid target refused: %v", err)
	}
	elsewhere := *timeline
	elsewhere.SpreadsheetID = "file-2"
	if err := CheckLinkTarget(payments, link, &elsewhere); !errors.Is(err, ErrLinkTargetInvalid) {
		t.Fatalf("target in another file = %v", err)
	}
	if err := CheckLinkTarget(payments, link, payments); !errors.Is(err, ErrLinkTargetInvalid) {
		t.Fatalf("self target = %v", err)
	}
	noMessages, _ := NewSheet(validInput())
	noMessages.ID, noMessages.SpreadsheetID = timelineID, "file-1"
	if err := CheckLinkTarget(payments, link, noMessages); !errors.Is(err, ErrLinkTargetInvalid) {
		t.Fatalf("target without message columns = %v", err)
	}
}

func TestAcceptStoresTheLinkFromTheExportAnchor(t *testing.T) {
	s, _ := NewSheet(paymentsInput())
	system := SystemValues{}.WithLink(timelineID, ExportAnchor{RowID: anchorRow, Period: "2026-11"})
	cells, err := s.Accept(map[string]any{"valor": "120", "baixa": "sim"}, system, acceptNow)
	if err != nil {
		t.Fatalf("Accept: %v", err)
	}
	ref, err := ParseLinkRef(cells["negociacao"].(string))
	if err != nil || ref != (LinkRef{SheetID: timelineID, Period: "2026-11", RowID: anchorRow}) {
		t.Fatalf("stored ref = %v, %v", cells["negociacao"], err)
	}
	if _, err := s.Accept(map[string]any{"valor": "120"}, SystemValues{}, acceptNow); !errors.Is(err, ErrLinkExportMissing) {
		t.Fatalf("missing anchor = %v, want ErrLinkExportMissing", err)
	}
	if _, err := s.Accept(map[string]any{"negociacao": "x"}, system, acceptNow); !errors.Is(err, ErrSystemColumn) {
		t.Fatalf("link typed by hand = %v", err)
	}
}

func TestLinkAnchorsSurviveTheMessageMerge(t *testing.T) {
	s := timelineSheet(t)
	system := SystemValues{SourceLeadName: "Ana"}.WithLink(timelineID, ExportAnchor{RowID: anchorRow})
	merged := s.WithMessage(system, receipt())
	if anchor, ok := merged.Link(timelineID); !ok || anchor.RowID != anchorRow {
		t.Fatalf("anchor lost: %+v %v", anchor, ok)
	}
}

func TestLinkRefParsingIsStrict(t *testing.T) {
	cases := []string{"", "a/b", timelineID + "/2026-11/not-a-uuid", "x/2026-11/" + anchorRow, timelineID + "/2026-11/" + anchorRow + "/x"}
	for _, text := range cases {
		if _, err := ParseLinkRef(text); err == nil {
			t.Fatalf("ParseLinkRef(%q) accepted", text)
		}
	}
	ref := LinkRef{SheetID: timelineID, RowID: anchorRow}
	back, err := ParseLinkRef(ref.String())
	if err != nil || back != ref {
		t.Fatalf("round trip = %+v %v", back, err)
	}
}

func TestLinkFormulaQuotesEverything(t *testing.T) {
	wants := map[Separator]Formula{
		SeparatorComma:     Formula(`=HYPERLINK("#gid=123&range=B"&MATCH("` + anchorRow + `",'Negociações d''Ana'!$A:$A,0),"Ver ""negociação""")`),
		SeparatorSemicolon: Formula(`=HYPERLINK("#gid=123&range=B"&MATCH("` + anchorRow + `";'Negociações d''Ana'!$A:$A;0);"Ver ""negociação""")`),
	}
	for sep, want := range wants {
		got, err := LinkFormula(TabRef{ID: 123, Title: "Negociações d'Ana"}, anchorRow, `Ver "negociação"`, sep)
		if err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Fatalf("formula with %q\n got %s\nwant %s", sep, got, want)
		}
	}
	refusals := []struct {
		tab   TabRef
		row   string
		label string
	}{
		{TabRef{ID: 1, Title: "T"}, `x")&IMPORTXML("http://evil`, "Ver"},
		{TabRef{ID: 1, Title: ""}, anchorRow, "Ver"},
		{TabRef{ID: -1, Title: "T"}, anchorRow, "Ver"},
		{TabRef{ID: 1, Title: "T"}, anchorRow, ""},
		{TabRef{ID: 1, Title: "T"}, anchorRow, "a\nb"},
	}
	for _, r := range refusals {
		if _, err := LinkFormula(r.tab, r.row, r.label, SeparatorSemicolon); err == nil {
			t.Fatalf("LinkFormula(%+v, %q, %q) accepted", r.tab, r.row, r.label)
		}
	}
}

func TestEncodeRowTypesCellsAndBuildsTheOnlyFormula(t *testing.T) {
	s, _ := NewSheet(paymentsInput())
	system := SystemValues{}.WithLink(timelineID, ExportAnchor{RowID: anchorRow, Period: "2026-11"})
	cells, err := s.Accept(map[string]any{"valor": "=1+1", "baixa": "NÃO"}, system, acceptNow)
	if err == nil {
		t.Fatalf("money accepted formula text: %v", cells)
	}
	cells, _ = s.Accept(map[string]any{"valor": "10", "baixa": "NÃO"}, system, acceptNow)
	tabs := LinkTabs{LinkTabKey(timelineID, "2026-11"): {ID: 55, Title: "Negociações · novembro 2026"}}
	for locale, sep := range map[string]Separator{"pt_BR": SeparatorSemicolon, "en_US": SeparatorComma} {
		row, err := s.EncodeRow("row-1", cells, Links{Tabs: tabs, Locale: locale})
		if err != nil {
			t.Fatalf("EncodeRow: %v", err)
		}
		if row[0] != "row-1" || row[1] != 10.0 || row[2] != "NÃO" {
			t.Fatalf("row = %#v", row)
		}
		formula, ok := row[3].(Formula)
		if !ok {
			t.Fatalf("link cell = %#v, want a Formula", row[3])
		}
		if want, _ := LinkFormula(TabRef{ID: 55, Title: "Negociações · novembro 2026"}, anchorRow, DefaultLinkLabel, sep); formula != want {
			t.Fatalf("formula in %s = %s", locale, formula)
		}
	}
	if _, err := s.EncodeRow("row-1", cells, Links{Locale: "pt_BR"}); !errors.Is(err, ErrLinkTargetPending) {
		t.Fatalf("missing tab = %v, want ErrLinkTargetPending", err)
	}
	for _, locale := range []string{"", "xx_YY"} {
		if _, err := s.EncodeRow("row-1", cells, Links{Tabs: tabs, Locale: locale}); !errors.Is(err, ErrLocaleUnsupported) {
			t.Fatalf("locale %q = %v, want ErrLocaleUnsupported", locale, err)
		}
	}
	if row, err := s.EncodeRow("row-1", Cells{"valor": 10.0}, Links{}); err != nil || row[3] != "" {
		t.Fatalf("a row without a link needs no locale: %#v %v", row, err)
	}
	refs := s.LinkRefs(cells)
	if len(refs) != 1 || refs[0].SheetID != timelineID || refs[0].Period != "2026-11" {
		t.Fatalf("LinkRefs = %+v", refs)
	}
}

func TestTextThatLooksLikeAFormulaNeverBecomesOne(t *testing.T) {
	s := typedSheet(t)
	cells, err := s.Accept(map[string]any{"nome": "=IMPORTXML(\"http://x\")"}, system, acceptNow)
	if err != nil {
		t.Fatal(err)
	}
	row, err := s.EncodeRow("row-1", cells, Links{})
	if err != nil {
		t.Fatal(err)
	}
	for _, cell := range row {
		if _, isFormula := cell.(Formula); isFormula {
			t.Fatalf("user text became a formula: %#v", row)
		}
	}
}

func TestDisplayCellsShowTheLinkLabel(t *testing.T) {
	s, _ := NewSheet(paymentsInput())
	system := SystemValues{}.WithLink(timelineID, ExportAnchor{RowID: anchorRow})
	cells, _ := s.Accept(map[string]any{"valor": "10"}, system, time.Now())
	shown := s.DisplayCells(cells)
	if shown["negociacao"] != DefaultLinkLabel || shown["valor"] != 10.0 {
		t.Fatalf("shown = %#v", shown)
	}
	if _, ok := cells["negociacao"].(string); !ok || cells["negociacao"] == DefaultLinkLabel {
		t.Fatal("DisplayCells changed the stored cells")
	}
}
