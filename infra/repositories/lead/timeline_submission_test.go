package lead

import (
	"strings"
	"testing"
)

func TestPageFormSubmissionsAreReadOnlyForViewersOfSubmissions(t *testing.T) {
	scan := fullScan()
	sql, args := timelineQuery(scan)
	if strings.Contains(sql, "site_submissions ts") {
		t.Fatal("submissions read for a viewer who may not read them")
	}
	scan.Submissions = true
	sql, args = timelineQuery(scan)
	if countPlaceholders(sql) != len(args) {
		t.Fatalf("%d placeholders for %d args", countPlaceholders(sql), len(args))
	}
	want := "FROM site_submissions ts WHERE ts.workspace_id = ? AND ts.lead_id = ?"
	if !strings.Contains(sql, want) || !strings.Contains(sql, "'projectId', ts.project_id") || strings.Contains(sql, "ts.fields") {
		t.Fatalf("submission branch missing or leaking visitor data: %s", sql)
	}
}

func TestASubmissionRowCarriesItsPage(t *testing.T) {
	row := timelineRow{Kind: "site_submission", ItemID: "site_submission:s-1", RefType: "site_submission", RefID: "s-1",
		Summary: []byte(`{"title":"contato","text":"/oferta","source":"meta","projectId":"p-1"}`)}
	item, err := row.item()
	if err != nil || item.Summary.ProjectID != "p-1" || item.Summary.Title != "contato" || item.Summary.Text != "/oferta" || item.Summary.Source != "meta" {
		t.Fatalf("item %+v %v", item, err)
	}
}
