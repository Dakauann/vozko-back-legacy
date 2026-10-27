package workflow

import "testing"

func TestAStageDestinationNoteNamesTheFunnelWithoutBlocking(t *testing.T) {
	issue := StageDestinationIssue("pago", "st-pago", "Altiva", "pagamento efetuado")
	if issue.Code != LintStageDestination || issue.Severity != SeverityInfo || issue.NodeID != "pago" || issue.Field != "stage_id" {
		t.Fatalf("issue = %+v", issue)
	}
	if issue.Values["pipeline"] != "Altiva" || issue.Values["stage"] != "pagamento efetuado" || issue.Values["stage_id"] != "st-pago" {
		t.Fatalf("values = %v", issue.Values)
	}
	if !(LintReport{Issues: []LintIssue{issue}}).IsGreen() {
		t.Fatal("a destination note must never block")
	}
}
