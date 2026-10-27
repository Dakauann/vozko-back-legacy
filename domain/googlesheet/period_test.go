package googlesheet

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func monthlyInput(pattern string) NewSheetInput {
	in := validInput()
	in.TabPeriod = PeriodMonth
	in.TabPattern = pattern
	return in
}

func TestPeriodOfUsesTheSheetTimeZone(t *testing.T) {
	s, err := NewSheet(monthlyInput("Pagamentos · {mes} {ano}"))
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		at   time.Time
		want string
	}{
		{time.Date(2026, 11, 1, 2, 59, 0, 0, time.UTC), "2026-10"},
		{time.Date(2026, 11, 1, 3, 0, 0, 0, time.UTC), "2026-11"},
		{time.Date(2027, 1, 1, 3, 0, 0, 0, time.UTC), "2027-01"},
	}
	for _, c := range cases {
		if got := s.PeriodOf(c.at); got != c.want {
			t.Fatalf("PeriodOf(%s) = %q, want %q", c.at, got, c.want)
		}
	}
	plain, _ := NewSheet(validInput())
	if got := plain.PeriodOf(time.Now()); got != "" {
		t.Fatalf("a sheet without period tabs has period %q", got)
	}
}

func TestTabTitleForRendersTheTokens(t *testing.T) {
	cases := []struct {
		pattern string
		period  string
		want    string
	}{
		{"Pagamentos · {mes} {ano}", "2026-11", "Pagamentos · novembro 2026"},
		{"{mm}-{ano} pagos", "2026-03", "03-2026 pagos"},
		{"Cobrança {mes} de {ano}", "2027-02", "Cobrança fevereiro de 2027"},
	}
	for _, c := range cases {
		s, err := NewSheet(monthlyInput(c.pattern))
		if err != nil {
			t.Fatalf("%q: %v", c.pattern, err)
		}
		got, err := s.TabTitleFor(c.period)
		if err != nil || got != c.want {
			t.Fatalf("TabTitleFor(%q) with %q = %q, %v; want %q", c.period, c.pattern, got, err, c.want)
		}
	}
}

func TestTabPatternRules(t *testing.T) {
	cases := map[string]string{
		"no year":           "Pagamentos {mes}",
		"no month":          "Pagamentos {ano}",
		"unknown token":     "Pagamentos {dia} {mes} {ano}",
		"open brace":        "Pagamentos {mes {ano}",
		"forbidden char":    "Pagamentos/{mes}/{ano}",
		"too long rendered": strings.Repeat("p", MaxTabRunes-8) + "{mes}{ano}",
	}
	for name, pattern := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := NewSheet(monthlyInput(pattern))
			var def *DefinitionError
			if !errors.As(err, &def) || def.Code != CodeTabPatternInvalid || def.Field != FieldTabPattern {
				t.Fatalf("pattern %q = %v, want %s", pattern, err, CodeTabPatternInvalid)
			}
		})
	}
}

func TestMonthlySheetWithoutPatternDerivesOneFromTheTitle(t *testing.T) {
	in := monthlyInput("")
	s, err := NewSheet(in)
	if err != nil {
		t.Fatal(err)
	}
	if s.TabPattern != "Pedidos · {mes} {ano}" {
		t.Fatalf("pattern = %q", s.TabPattern)
	}
}

func TestOpenPeriodPointsTheSheetAtTheCurrentPeriodTab(t *testing.T) {
	s, _ := NewSheet(monthlyInput("Pagamentos · {mes} {ano}"))
	if err := s.OpenPeriod(time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	if s.TabName != "Pagamentos · novembro 2026" || s.TabPeriodKey != "2026-11" {
		t.Fatalf("tab = %q period %q", s.TabName, s.TabPeriodKey)
	}
	plain, _ := NewSheet(validInput())
	if err := plain.OpenPeriod(time.Now()); err != nil || plain.TabName != "Pedidos" || plain.TabPeriodKey != "" {
		t.Fatalf("plain sheet changed: %q %q %v", plain.TabName, plain.TabPeriodKey, err)
	}
}

func TestNewRowFixesThePeriodWhenItIsEnqueued(t *testing.T) {
	s, _ := NewSheet(monthlyInput("Pagamentos · {mes} {ano}"))
	row, err := NewRow(s, Cells{"nome_do_cliente": "Ana"}, "k", Origin{Kind: OriginManual}, time.Date(2026, 11, 1, 2, 59, 0, 0, time.UTC))
	if err != nil || row.PeriodKey != "2026-10" {
		t.Fatalf("row period = %q, %v", row.PeriodKey, err)
	}
}

func TestPreviewTabTitle(t *testing.T) {
	title, period, err := PreviewTabTitle("Pagamentos · {mes} {ano}", "America/Sao_Paulo", time.Date(2026, 11, 1, 2, 0, 0, 0, time.UTC))
	if err != nil || title != "Pagamentos · outubro 2026" || period != "2026-10" {
		t.Fatalf("preview = %q %q %v", title, period, err)
	}
	if _, _, err := PreviewTabTitle("Pagamentos", "America/Sao_Paulo", time.Now()); ErrorCode(err) != CodeTabPatternInvalid {
		t.Fatalf("bad pattern preview = %v", err)
	}
}

func TestTabPatternCanChangeButThePeriodModeCannot(t *testing.T) {
	s, _ := NewSheet(monthlyInput("Pagamentos · {mes} {ano}"))
	pattern := "Pagos {mm}-{ano}"
	if err := s.ChangeLayout(nil, &pattern); err != nil || s.TabPattern != pattern || s.Version != 2 {
		t.Fatalf("ChangeLayout = %v, pattern %q version %d", err, s.TabPattern, s.Version)
	}
	plain, _ := NewSheet(validInput())
	if err := plain.ChangeLayout(nil, &pattern); ErrorCode(err) != CodeTabPatternNotPeriodic {
		t.Fatalf("pattern on a plain sheet = %v", err)
	}
	style := StyleBlue
	if err := plain.ChangeLayout(&style, nil); err != nil || plain.Style != StyleBlue {
		t.Fatalf("style change = %v, %q", err, plain.Style)
	}
	bad := StylePreset("neon")
	if err := plain.ChangeLayout(&bad, nil); ErrorCode(err) != CodeStyleUnknown {
		t.Fatalf("unknown style = %v", err)
	}
	if err := plain.ChangeLayout(nil, nil); ErrorCode(err) != CodeEditEmpty {
		t.Fatalf("empty layout change = %v", err)
	}
}

func TestWorkbookRoomCountsEveryTab(t *testing.T) {
	wb := Workbook{Tabs: []TabInfo{{ID: 0, Title: "Negociações", Rows: 400_000, Columns: 12}, {ID: 7, Title: "Pagamentos", Rows: 300_000, Columns: 15}}}
	if wb.Cells() != 400_000*12+300_000*15 {
		t.Fatalf("cells = %d", wb.Cells())
	}
	if wb.NearlyFull(0) {
		t.Fatal("9.3M cells is not 95% of 10M")
	}
	if !wb.NearlyFull(300_000) {
		t.Fatal("9.6M cells should be nearly full")
	}
	if tab, ok := wb.Titled("pagamentos"); !ok || tab.ID != 7 {
		t.Fatalf("Titled is case-insensitive like Google: %+v %v", tab, ok)
	}
	if _, ok := wb.Tab(9); ok {
		t.Fatal("found a tab that does not exist")
	}
	if id := wb.FreeTabID(7); id == 7 || id == 0 || id < 0 {
		t.Fatalf("FreeTabID = %d", id)
	}
}
