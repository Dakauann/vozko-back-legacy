package googlesheet

import (
	"strings"
	"testing"
)

func TestEveryPresetBuildsValidSheets(t *testing.T) {
	keys := map[string]bool{}
	for _, preset := range Presets() {
		if keys[preset.Key] {
			t.Fatalf("preset %s twice", preset.Key)
		}
		keys[preset.Key] = true
		refs := map[string]string{}
		for i, sheet := range preset.Sheets {
			refs[sheet.Ref] = "00000000-0000-4000-8000-00000000000" + string(rune('1'+i))
		}
		for _, sheet := range preset.Sheets {
			in := NewSheetInput{
				WorkspaceID: "ws", AccountID: "acct", Title: preset.Key + " " + sheet.Ref,
				TabPeriod: sheet.TabPeriod, Style: sheet.Style,
			}
			if sheet.TabPeriod == PeriodMonth {
				in.TabPattern = "Aba · {mes} {ano}"
			}
			for _, col := range sheet.Columns {
				input := ColumnInput{Header: col.Key, Type: col.Type, Source: col.Source, Required: col.Required, Options: col.Options}
				if len(col.Colors) > 0 {
					input.Colors = col.Colors
				}
				if col.LinkRef != "" {
					target, ok := refs[col.LinkRef]
					if !ok {
						t.Fatalf("%s/%s links to unknown ref %q", preset.Key, sheet.Ref, col.LinkRef)
					}
					input.LinkSheetID = target
				}
				in.Columns = append(in.Columns, input)
			}
			in.Unique = sheet.UniqueInput()
			if _, err := NewSheet(in); err != nil {
				t.Fatalf("preset %s sheet %s: %v", preset.Key, sheet.Ref, err)
			}
		}
	}
	for _, want := range []string{"cobranca", "timeline", "qualified_leads", "scheduling", "support_tickets", "deals", "form_answers"} {
		if !keys[want] {
			t.Fatalf("preset %s missing", want)
		}
	}
}

func TestCobrancaLinksPaymentsToTheTimeline(t *testing.T) {
	preset, ok := PresetByKey("cobranca")
	if !ok || len(preset.Sheets) != 2 {
		t.Fatalf("cobranca = %+v", preset)
	}
	timeline, payments := preset.Sheets[0], preset.Sheets[1]
	if timeline.TabPeriod != PeriodNone || payments.TabPeriod != PeriodMonth {
		t.Fatalf("periods = %s %s", timeline.TabPeriod, payments.TabPeriod)
	}
	var linked, colored bool
	for _, col := range payments.Columns {
		if col.Source == SourceExportLink && col.LinkRef == timeline.Ref {
			linked = true
		}
		if col.Type == TypeChoice && col.Colors["sim"] == ColorGreen && col.Colors["nao"] == ColorRed {
			colored = true
		}
	}
	if !linked || !colored {
		t.Fatalf("payments linked %v colored %v", linked, colored)
	}
	if _, ok := PresetByKey("nope"); ok {
		t.Fatal("unknown preset found")
	}
}

func TestPresetCopyKeysCoverEveryTextTheFrontShows(t *testing.T) {
	keys := PresetCopyKeys()
	seen := map[string]bool{}
	for _, key := range keys {
		if seen[key] {
			t.Fatalf("copy key %s twice", key)
		}
		seen[key] = true
		if strings.ContainsAny(key, " /") {
			t.Fatalf("copy key %q is not a catalog path", key)
		}
	}
	for _, want := range []string{
		"cobranca.name", "cobranca.description",
		"cobranca.sheets.negociacoes.title", "cobranca.sheets.pagamentos.tabPattern",
		"cobranca.sheets.pagamentos.columns.baixa.header", "cobranca.sheets.pagamentos.columns.baixa.options.sim",
		"cobranca.sheets.pagamentos.columns.negociacao.linkLabel",
	} {
		if !seen[want] {
			t.Fatalf("copy key %s missing", want)
		}
	}
}

func TestOnlyThePaymentsTabKeepsOneRowPerConversationByDefault(t *testing.T) {
	for _, preset := range Presets() {
		for _, sheet := range preset.Sheets {
			want := UniqueNone
			if preset.Key == "cobranca" && sheet.Ref == "pagamentos" {
				want = UniqueConversation
			}
			if got := sheet.Unique.Normalized().Mode; got != want {
				t.Fatalf("%s/%s unique = %s, want %s", preset.Key, sheet.Ref, got, want)
			}
		}
	}
}

func TestAPresetUniqueRuleNamesItsColumnsByPosition(t *testing.T) {
	sheet := PresetSheet{
		Columns: []PresetColumn{inputColumn("data", TypeDate), inputColumn("matricula", TypeText)},
		Unique:  UniqueRule{Mode: UniqueColumns, Columns: []string{"matricula", "ausente"}},
	}
	got := sheet.UniqueInput()
	if got.Mode != UniqueColumns || len(got.Columns) != 2 || got.Columns[0] != 1 || got.Columns[1] != -1 {
		t.Fatalf("unique input = %+v", got)
	}
}
