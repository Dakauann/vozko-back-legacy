package launchcheck

import "testing"

func hasBlock(r Report, code Code) bool {
	for _, f := range r.Findings {
		if f.Level == Block && f.Code == code {
			return true
		}
	}
	return false
}

func TestTheSpreadsheetOfACampaignMustFitAndBeHealthy(t *testing.T) {
	base := Snapshot{Pending: 10, AccessAllowed: true}
	if r := Check(base); hasBlock(r, CodeSheetVariableUnfit) || hasBlock(r, CodeSheetVariableAttention) {
		t.Fatalf("a campaign without a Planilha = %+v", r.Findings)
	}
	unfit := base
	unfit.Sheets = &SheetFacts{Unfit: true}
	if !hasBlock(Check(unfit), CodeSheetVariableUnfit) {
		t.Fatal("an unfit spreadsheet must block")
	}
	attention := base
	attention.Sheets = &SheetFacts{Attention: true}
	if !hasBlock(Check(attention), CodeSheetVariableAttention) {
		t.Fatal("a tab that needs attention must block")
	}
}
